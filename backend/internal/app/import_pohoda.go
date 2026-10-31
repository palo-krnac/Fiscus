package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/pohoda"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Prenos dát z programu POHODA: adresár, cenník (zásoby), vydané faktúry
// a dobropisy, prijaté faktúry. Najprv náhľad (čo pribudne, čo už je,
// čo sa preskočí a prečo), potom import. Vydané faktúry si ponechajú
// číslo z POHODY a sú vystavené (uhradené podľa likvidácie); prijaté
// dostanú poradové číslo Fiškusa a číslo dodávateľa z POHODY.

// MaxPohodaBytes je najväčší XML export.
const MaxPohodaBytes = 50 << 20

// ImportCount sú počty jednej agendy.
type ImportCount struct {
	New      int `json:"new"`
	Existing int `json:"existing"` // už vo Fiškuse (preskočí sa)
	Skipped  int `json:"skipped"`  // nedá sa preniesť (dôvod v upozorneniach)
}

// ImportReport je náhľad alebo výsledok prenosu.
type ImportReport struct {
	Source    string         `json:"source"`
	Contacts  ImportCount    `json:"contacts"`
	Items     ImportCount    `json:"items"`
	Invoices  ImportCount    `json:"invoices"`
	Purchases ImportCount    `json:"purchases"`
	Other     map[string]int `json:"other"` // agendy, ktoré sa neprenášajú
	Warnings  []string       `json:"warnings"`
	Done      bool           `json:"done"`
}

func (r *ImportReport) warn(format string, args ...any) {
	if len(r.Warnings) < 200 {
		r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
	}
}

// pohodaRate prevedie sadzbu POHODY na sadzbu DPH: percento z dokladu,
// inak podľa druhu sadzby a dátumu (od 2025 základná 23 %, znížené 19 % a 5 %).
func pohodaRate(kind, percent string, d invoice.Date) vat.Rate {
	if p, err := money.Parse(strings.TrimSpace(percent)); err == nil && strings.TrimSpace(percent) != "" {
		switch p.RoundTo(0).String() {
		case "23.00", "23":
			return vat.Rate23
		case "20.00", "20":
			return vat.Rate20
		case "19.00", "19":
			return vat.Rate19
		case "10.00", "10":
			return vat.Rate10
		case "5.00", "5":
			return vat.Rate5
		case "0.00", "0":
			return vat.Rate0
		}
	}
	since2025 := d.IsZero() || !d.Before(invoice.MustDate("2025-01-01"))
	switch strings.TrimPrefix(strings.TrimSpace(kind), "history") {
	case "high", "High":
		if since2025 {
			return vat.Rate23
		}
		return vat.Rate20
	case "low", "Low":
		if since2025 {
			return vat.Rate19
		}
		return vat.Rate10
	case "third", "Third":
		return vat.Rate5
	case "none", "":
		return vat.Rate0
	}
	return vat.Rate0
}

func pdec(s string) money.Dec {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if s == "" {
		return money.Zero
	}
	d, err := money.Parse(strings.Replace(s, ",", ".", 1))
	if err != nil {
		return money.Zero
	}
	return d
}

func pdate(s string) invoice.Date {
	d, err := invoice.ParseDate(strings.TrimSpace(s))
	if err != nil {
		return invoice.Date{}
	}
	return d
}

func pohodaParty(a pohoda.Address) invoice.Party {
	country := strings.ToUpper(strings.TrimSpace(a.Country.IDs))
	if country == "" {
		country = "SK"
	}
	return invoice.Party{Name: a.DisplayName(), Street: strings.TrimSpace(a.Street), City: strings.TrimSpace(a.City),
		Zip: strings.TrimSpace(a.Zip), Country: country, ICO: strings.ReplaceAll(strings.TrimSpace(a.ICO), " ", ""),
		DIC: strings.TrimSpace(a.DIC), ICDPH: strings.ReplaceAll(strings.TrimSpace(a.ICDPH), " ", ""), Email: strings.TrimSpace(a.Email)}
}

// ImportPohoda prenesie dáta z XML exportu POHODY (apply=false: iba náhľad).
func (a *App) ImportPohoda(ctx context.Context, companyID int64, data []byte, apply bool) (ImportReport, error) {
	for _, p := range []auth.Perm{auth.PermContactsWrite, auth.PermInvoicesWrite, auth.PermPurchasesWrite} {
		if err := a.Require(ctx, companyID, p); err != nil {
			return ImportReport{}, err
		}
	}
	rep := ImportReport{Source: "POHODA", Warnings: []string{}, Other: map[string]int{}, Done: apply}
	d, err := pohoda.Parse(data)
	if err != nil {
		return rep, mfaFieldErr("file", err.Error())
	}
	rep.Other = d.Other
	if len(d.Contacts)+len(d.Stocks)+len(d.Invoices) == 0 {
		return rep, mfaFieldErr("file", "V súbore nie je adresár, zásoby ani faktúry — v POHODE zvoľte Súbor → Dátová komunikácia → XML export")
	}

	// adresár: rovnaké IČO, bez IČO rovnaký názov
	contacts, err := a.Store.ListContacts(ctx, companyID, "")
	if err != nil {
		return rep, err
	}
	known := map[string]bool{}
	key := func(ico, name string) string {
		if ico = strings.ReplaceAll(strings.TrimSpace(ico), " ", ""); ico != "" {
			return "ico:" + ico
		}
		return "name:" + store.SearchKey(name)
	}
	for _, c := range contacts {
		known[key(c.ICO, c.Name)] = true
	}
	for _, c := range d.Contacts {
		ad := c.Address()
		p := pohodaParty(ad)
		if p.Name == "" {
			rep.Contacts.Skipped++
			rep.warn("Adresár: záznam bez názvu sa preskočil")
			continue
		}
		k := key(p.ICO, p.Name)
		if known[k] {
			rep.Contacts.Existing++
			continue
		}
		known[k] = true
		rep.Contacts.New++
		if apply {
			if _, err := a.SaveContact(ctx, companyID, store.Contact{Name: p.Name, Street: p.Street, City: p.City, Zip: p.Zip, Country: p.Country,
				ICO: p.ICO, DIC: p.DIC, ICDPH: p.ICDPH, Email: p.Email, Phone: strings.TrimSpace(ad.Phone), Note: strings.TrimSpace(c.Header.Note)}); err != nil {
				rep.Contacts.New--
				rep.Contacts.Skipped++
				rep.warn("Adresár %s: %v", p.Name, err)
			}
		}
	}

	// cenník: rovnaký kód
	items, err := a.Store.ListItems(ctx, companyID)
	if err != nil {
		return rep, err
	}
	codes := map[string]bool{}
	for _, it := range items {
		if it.Code != "" {
			codes[strings.ToUpper(it.Code)] = true
		}
	}
	today := a.Today()
	for _, s := range d.Stocks {
		h := s.Header
		name, code := strings.TrimSpace(h.Name), strings.TrimSpace(h.Code)
		if name == "" {
			rep.Items.Skipped++
			rep.warn("Zásoby: položka %s bez názvu sa preskočila", code)
			continue
		}
		if code != "" && codes[strings.ToUpper(code)] {
			rep.Items.Existing++
			continue
		}
		codes[strings.ToUpper(code)] = code != ""
		rep.Items.New++
		if !apply {
			continue
		}
		it := catalog.Item{Code: code, Name: name, Unit: firstNonBlank(h.Unit, "ks"), Price: pdec(h.SellingPrice.Value).RoundCents(),
			PriceMode: vat.PriceNet, Rate: pohodaRate(h.SellingRate, "", today), Note: strings.TrimSpace(h.Note)}
		if h.SellingPrice.PayVAT == "true" {
			it.PriceMode = vat.PriceGross
		}
		if len([]rune(it.Name)) > 200 {
			it.Name = string([]rune(it.Name)[:200])
		}
		if _, err := a.SaveItem(ctx, companyID, it); err != nil {
			rep.Items.New--
			rep.Items.Skipped++
			rep.warn("Zásoby %s: %v", name, err)
		}
	}

	// faktúry: vydané podľa čísla, prijaté podľa dodávateľa a jeho čísla
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
	if err != nil {
		return rep, err
	}
	numbers := map[string]int64{}
	for _, inv := range invs {
		if inv.Number != "" {
			numbers[inv.Number] = inv.ID
		}
	}
	purs, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
	if err != nil {
		return rep, err
	}
	supplierDocs := map[string]bool{}
	for _, p := range purs {
		supplierDocs[p.Supplier.ICO+"|"+p.SupplierNumber] = true
	}
	// dobropisy až po faktúrach, aby sa našla opravovaná faktúra
	list := append([]pohoda.Invoice(nil), d.Invoices...)
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Header.Type != "issuedCreditNotice" && list[j].Header.Type == "issuedCreditNotice"
	})
	for _, pi := range list {
		h := pi.Header
		num := strings.TrimSpace(h.Number.Requested)
		switch h.Type {
		case "issuedInvoice", "issuedCreditNotice":
			if num == "" {
				rep.Invoices.Skipped++
				rep.warn("Vydaná faktúra bez čísla sa preskočila")
				continue
			}
			if _, ok := numbers[num]; ok {
				rep.Invoices.Existing++
				continue
			}
			inv, err := a.pohodaInvoice(pi, numbers)
			if err != nil {
				rep.Invoices.Skipped++
				rep.warn("Faktúra %s: %v", num, err)
				continue
			}
			rep.Invoices.New++
			numbers[num] = -1
			if apply {
				if err := a.saveImportedInvoice(ctx, companyID, &inv, pi); err != nil {
					rep.Invoices.New--
					rep.Invoices.Skipped++
					rep.warn("Faktúra %s: %v", num, err)
					continue
				}
				numbers[num] = inv.ID
			}
		case "receivedInvoice", "receivedCreditNotice":
			p, err := pohodaPurchase(pi)
			if err != nil {
				rep.Purchases.Skipped++
				rep.warn("Prijatá faktúra %s: %v", firstNonBlank(h.OriginalDocument, num), err)
				continue
			}
			k := p.Supplier.ICO + "|" + p.SupplierNumber
			if supplierDocs[k] {
				rep.Purchases.Existing++
				continue
			}
			supplierDocs[k] = true
			rep.Purchases.New++
			if apply {
				if _, _, err := a.SavePurchase(ctx, companyID, p); err != nil {
					rep.Purchases.New--
					rep.Purchases.Skipped++
					rep.warn("Prijatá faktúra %s od %s: %v", p.SupplierNumber, p.Supplier.Name, err)
				}
			}
		default:
			label := map[string]string{"issuedProformaInvoice": "zálohová faktúra", "issuedAdvanceInvoice": "zálohová faktúra",
				"receivedProformaInvoice": "prijatá zálohová faktúra", "receivedAdvanceInvoice": "prijatá zálohová faktúra",
				"receivable": "ostatná pohľadávka", "commitment": "ostatný záväzok"}[h.Type]
			if label == "" {
				label = h.Type
			}
			if strings.HasPrefix(h.Type, "issued") || h.Type == "receivable" || h.Type == "penalty" {
				rep.Invoices.Skipped++
			} else {
				rep.Purchases.Skipped++
			}
			rep.warn("%s %s sa neprenáša (%s)", strings.ToUpper(label[:1])+label[1:], num, "zálohy a ostatné doklady zadajte vo Fiškuse")
		}
	}
	if apply {
		a.record(ctx, companyID, audit.CatSettings, "import", "pohoda", 0, fmt.Sprintf(
			"Prenos z POHODY: adresár %d, cenník %d, vydané faktúry %d, prijaté faktúry %d",
			rep.Contacts.New, rep.Items.New, rep.Invoices.New, rep.Purchases.New), nil, nil)
	}
	return rep, nil
}

func firstNonBlank(ss ...string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// pohodaInvoice zostaví vydanú faktúru alebo dobropis.
func (a *App) pohodaInvoice(pi pohoda.Invoice, numbers map[string]int64) (invoice.Invoice, error) {
	h := pi.Header
	if pi.Foreign() {
		return invoice.Invoice{}, fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", pi.Summary.Foreign.Currency.IDs)
	}
	issue := pdate(h.Date)
	if issue.IsZero() {
		return invoice.Invoice{}, fmt.Errorf("chýba dátum vystavenia")
	}
	delivery := pdate(firstNonBlank(h.DateTax, h.DateDelivery, h.Date))
	inv := invoice.Invoice{Kind: invoice.KindInvoice, Customer: pohodaParty(h.Partner.Address), IssueDate: issue, DeliveryDate: delivery,
		DueDate: pdate(firstNonBlank(h.DateDue, h.Date)), VariableSymbol: strings.TrimSpace(h.SymVar), PriceMode: vat.PriceNet,
		Currency: "EUR", Note: strings.TrimSpace(h.Note)}
	if inv.Customer.Name == "" {
		return inv, fmt.Errorf("chýba odberateľ")
	}
	credit := h.Type == "issuedCreditNotice"
	if credit {
		inv.Kind = invoice.KindCreditNote
		orig := strings.TrimSpace(h.OriginalDocument)
		id, ok := numbers[orig]
		if orig == "" || !ok {
			return inv, fmt.Errorf("dobropis k faktúre %q, ktorá nie je vo Fiškuse ani v súbore — preneste aj pôvodnú faktúru", orig)
		}
		inv.Correction = &invoice.Correction{OriginalID: id, OriginalNumber: orig, Reason: firstNonBlank(h.Text, "Oprava faktúry")}
	}
	gross := false
	for _, it := range pi.Items {
		if it.PayVAT == "true" {
			gross = true
		}
	}
	if gross {
		inv.PriceMode = vat.PriceGross
	}
	for _, it := range pi.Items {
		q := pdec(it.Quantity)
		text := firstNonBlank(it.Text, it.Note)
		if text == "" && q.IsZero() {
			continue
		}
		if q.IsZero() {
			q = money.FromInt(1)
		}
		if credit && q.Sign() > 0 && pdec(it.Home.Price).Sign() < 0 {
			q = q.Neg()
		}
		inv.Lines = append(inv.Lines, invoice.Line{Description: firstNonBlank(text, "Položka"), Quantity: q, Unit: firstNonBlank(it.Unit, "ks"),
			UnitPrice: pdec(it.Home.UnitPrice), Rate: pohodaRate(it.RateVAT, it.PercentVAT, delivery)})
	}
	if len(inv.Lines) == 0 {
		// faktúra bez položiek: riadky podľa rekapitulácie
		s := pi.Summary.Home
		for _, r := range []struct {
			base string
			kind string
		}{{s.PriceNone, "none"}, {s.PriceLow, "low"}, {s.PriceHigh, "high"}, {s.Price3, "third"}} {
			if b := pdec(r.base); !b.IsZero() {
				inv.Lines = append(inv.Lines, invoice.Line{Description: firstNonBlank(h.Text, "Plnenie podľa faktúry"), Quantity: money.FromInt(1),
					Unit: "ks", UnitPrice: b, Rate: pohodaRate(r.kind, "", delivery)})
			}
		}
		inv.PriceMode = vat.PriceNet
	}
	if len(inv.Lines) == 0 {
		return inv, fmt.Errorf("faktúra nemá položky ani sumu")
	}
	if err := inv.Recalculate(); err != nil {
		return inv, err
	}
	return inv, nil
}

// pohodaTotal je suma faktúry podľa rekapitulácie POHODY.
func pohodaTotal(pi pohoda.Invoice) money.Dec {
	s := pi.Summary.Home
	t := money.Zero
	for _, v := range []string{s.PriceNone, s.PriceLow, s.PriceLowVAT, s.PriceHigh, s.PriceHighVAT, s.Price3, s.Price3VAT} {
		t = t.Add(pdec(v))
	}
	return t
}

// saveImportedInvoice uloží vydanú faktúru s číslom z POHODY ako
// vystavenú; uhradenú podľa likvidácie (dátum úhrady = splatnosť).
func (a *App) saveImportedInvoice(ctx context.Context, companyID int64, inv *invoice.Invoice, pi pohoda.Invoice) error {
	if err := a.applyCompany(ctx, companyID, inv); err != nil {
		return err
	}
	if err := inv.Recalculate(); err != nil {
		return err
	}
	if diff := inv.Summary.Total.Sub(pohodaTotal(pi)); pohodaTotal(pi).Sign() != 0 && (diff.Cmp(money.MustParse("0.05")) > 0 || diff.Neg().Cmp(money.MustParse("0.05")) > 0) {
		inv.Note = strings.TrimSpace(inv.Note + "\nPrenos z POHODY: suma v POHODE " + pohodaTotal(pi).Display() + " €")
	}
	inv.Number = strings.TrimSpace(pi.Header.Number.Requested)
	inv.Status = invoice.StatusIssued
	if paid := pdec(pi.Header.Liquidation.AmountHome); inv.Summary.Total.Sign() > 0 && paid.Cmp(inv.Summary.Total) >= 0 {
		when := inv.DueDate
		if when.After(a.Today()) {
			when = a.Today()
		}
		inv.Payments = []invoice.Payment{{Date: when, Amount: inv.Summary.Total, Source: "import"}}
		inv.Status, inv.PaidDate = invoice.StatusPaid, when
	}
	if err := a.Store.SaveInvoice(ctx, companyID, inv); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "import", "invoice", inv.ID, "Prenesená faktúra "+inv.Number+" z POHODY", nil, nil)
	return nil
}

// pohodaPurchase zostaví prijatú faktúru zo sumárnych riadkov POHODY.
func pohodaPurchase(pi pohoda.Invoice) (purchase.Purchase, error) {
	h := pi.Header
	if pi.Foreign() {
		return purchase.Purchase{}, fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", pi.Summary.Foreign.Currency.IDs)
	}
	issue := pdate(h.Date)
	if issue.IsZero() {
		return purchase.Purchase{}, fmt.Errorf("chýba dátum")
	}
	supplierNo := firstNonBlank(h.OriginalDocument, h.Number.Requested)
	p := purchase.Purchase{SupplierNumber: supplierNo, VariableSymbol: strings.TrimSpace(h.SymVar), Supplier: pohodaParty(h.Partner.Address),
		IssueDate: issue, DeliveryDate: pdate(firstNonBlank(h.DateDelivery, h.DateTax, h.Date)), ReceivedDate: pdate(firstNonBlank(h.DateTax, h.DateAccounting, h.Date)),
		DueDate: pdate(firstNonBlank(h.DateDue, h.Date)), Description: firstNonBlank(h.Text, "Prenos z POHODY"), Note: strings.TrimSpace(h.Note),
		Expense: purchase.ExpenseServices}
	if p.Supplier.Name == "" {
		return p, fmt.Errorf("chýba dodávateľ")
	}
	s := pi.Summary.Home
	for _, r := range []struct{ base, tax, kind string }{{s.PriceNone, "", "none"}, {s.PriceLow, s.PriceLowVAT, "low"}, {s.PriceHigh, s.PriceHighVAT, "high"}, {s.Price3, s.Price3VAT, "third"}} {
		b := pdec(r.base)
		if b.IsZero() {
			continue
		}
		p.Rates = append(p.Rates, purchase.RateRow{Rate: pohodaRate(r.kind, "", p.DeliveryDate), Base: b, VAT: pdec(r.tax)})
	}
	if len(p.Rates) == 0 {
		return p, fmt.Errorf("faktúra nemá sumu")
	}
	if paid := pdec(h.Liquidation.AmountHome); paid.Sign() > 0 {
		total := pohodaTotal(pi)
		if paid.Cmp(total) >= 0 {
			p.Status, p.PaidDate = purchase.StatusPaid, p.DueDate
		}
	}
	return p, nil
}
