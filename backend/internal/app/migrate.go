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
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Prenos dát z iných programov (POHODA, Money S3, …): každý formát sa
// prevedie na spoločný „migration“ a ten sa prenesie rovnako — najprv
// náhľad (čo pribudne, čo už je, čo sa preskočí a prečo), potom zápis.
// Vydané faktúry si ponechajú pôvodné číslo a sú vystavené (uhradené podľa
// údajov zo zdroja alebo voľby „uhradené so splatnosťou do“); prijaté
// dostanú poradové číslo Fiškusa a číslo dodávateľa zo zdroja.

// MaxMigrateBytes je najväčší súbor na prenos.
const MaxMigrateBytes = 50 << 20

// MaxPohodaBytes je najväčší XML export z POHODY.
const MaxPohodaBytes = MaxMigrateBytes

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

// MigrateOptions sú voľby prenosu.
type MigrateOptions struct {
	// PaidUntil: vydané aj prijaté faktúry so splatnosťou do tohto dňa
	// budú uhradené (zdroj úhrady neexportuje, staré roky sú vyrovnané).
	PaidUntil invoice.Date
}

type migContact struct {
	Party invoice.Party
	Phone string
	Note  string
}

// migInvoice je vydaná faktúra alebo dobropis zo zdroja.
type migInvoice struct {
	Number   string
	Credit   bool
	Original string // číslo opravovanej faktúry (dobropis)
	Reason   string
	Inv      invoice.Invoice // hlavička a riadky z položiek
	Summary  []invoice.Line  // riadky z rekapitulácie DPH (náhrada položiek)
	Total    money.Dec       // suma zo zdroja (0 = neznáma)
	Paid     money.Dec       // uhradené podľa zdroja
	Err      error
}

type migPurchase struct {
	Ref  string // na upozornenia
	P    purchase.Purchase
	Paid bool
	Err  error
}

type migration struct {
	Source    string
	Contacts  []migContact
	Items     []catalog.Item
	Invoices  []migInvoice
	Purchases []migPurchase
	// Skips sú doklady, ktoré sa neprenášajú (zálohy…), s dôvodom.
	Skips    []migSkip
	Other    map[string]int
	Warnings []string // upozornenia z prevodu (neznámy štát…)
}

type migSkip struct {
	Purchase bool
	Text     string
}

func (a *App) requireMigrate(ctx context.Context, companyID int64) error {
	for _, p := range []auth.Perm{auth.PermContactsWrite, auth.PermInvoicesWrite, auth.PermPurchasesWrite} {
		if err := a.Require(ctx, companyID, p); err != nil {
			return err
		}
	}
	return nil
}

func absDec(d money.Dec) money.Dec {
	if d.Sign() < 0 {
		return d.Neg()
	}
	return d
}

var migTolerance = money.MustParse("0.05")

// migrate prenesie dáta (apply=false: iba náhľad).
func (a *App) migrate(ctx context.Context, companyID int64, m migration, opt MigrateOptions, apply bool) (ImportReport, error) {
	rep := ImportReport{Source: m.Source, Warnings: []string{}, Other: m.Other, Done: apply}
	if rep.Other == nil {
		rep.Other = map[string]int{}
	}
	for _, w := range m.Warnings {
		rep.warn("%s", w)
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
	for _, c := range m.Contacts {
		p := c.Party
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
				ICO: p.ICO, DIC: p.DIC, ICDPH: p.ICDPH, Email: p.Email, Phone: c.Phone, Note: c.Note}); err != nil {
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
	for _, it := range m.Items {
		if it.Name == "" {
			rep.Items.Skipped++
			rep.warn("Cenník: položka %s bez názvu sa preskočila", it.Code)
			continue
		}
		if it.Code != "" && codes[strings.ToUpper(it.Code)] {
			rep.Items.Existing++
			continue
		}
		if it.Code != "" {
			codes[strings.ToUpper(it.Code)] = true
		}
		if len([]rune(it.Name)) > 200 {
			it.Name = string([]rune(it.Name)[:200])
		}
		rep.Items.New++
		if apply {
			if _, err := a.SaveItem(ctx, companyID, it); err != nil {
				rep.Items.New--
				rep.Items.Skipped++
				rep.warn("Cenník %s: %v", it.Name, err)
			}
		}
	}

	// vydané faktúry podľa čísla; dobropisy až po faktúrach
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
	list := append([]migInvoice(nil), m.Invoices...)
	sort.SliceStable(list, func(i, j int) bool { return !list[i].Credit && list[j].Credit })
	for _, mi := range list {
		num := strings.TrimSpace(mi.Number)
		if num == "" {
			rep.Invoices.Skipped++
			rep.warn("Vydaná faktúra bez čísla sa preskočila")
			continue
		}
		if _, ok := numbers[num]; ok {
			rep.Invoices.Existing++
			continue
		}
		inv, err := a.migInvoiceBuild(ctx, companyID, mi, numbers, &rep)
		if err != nil {
			rep.Invoices.Skipped++
			rep.warn("Faktúra %s: %v", num, err)
			continue
		}
		rep.Invoices.New++
		numbers[num] = -1
		if !apply {
			continue
		}
		if err := a.migInvoiceSave(ctx, companyID, &inv, mi, opt, m.Source); err != nil {
			rep.Invoices.New--
			rep.Invoices.Skipped++
			rep.warn("Faktúra %s: %v", num, err)
			continue
		}
		numbers[num] = inv.ID
	}

	// prijaté faktúry podľa dodávateľa a jeho čísla
	purs, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
	if err != nil {
		return rep, err
	}
	supplierDocs := map[string]bool{}
	for _, p := range purs {
		supplierDocs[p.Supplier.ICO+"|"+p.SupplierNumber] = true
	}
	for _, mp := range m.Purchases {
		if mp.Err != nil {
			rep.Purchases.Skipped++
			rep.warn("Prijatá faktúra %s: %v", mp.Ref, mp.Err)
			continue
		}
		p := mp.P
		k := p.Supplier.ICO + "|" + p.SupplierNumber
		if supplierDocs[k] {
			rep.Purchases.Existing++
			continue
		}
		supplierDocs[k] = true
		paid := mp.Paid || (!opt.PaidUntil.IsZero() && !p.DueDate.IsZero() && !p.DueDate.After(opt.PaidUntil))
		rep.Purchases.New++
		if apply {
			saved, _, err := a.SavePurchase(ctx, companyID, p)
			if err != nil {
				rep.Purchases.New--
				rep.Purchases.Skipped++
				rep.warn("Prijatá faktúra %s od %s: %v", p.SupplierNumber, p.Supplier.Name, err)
			} else if paid {
				if _, err := a.MarkPurchasePaid(ctx, companyID, saved.ID, a.paidOn(p.DueDate)); err != nil {
					rep.warn("Prijatá faktúra %s od %s: prenesená, ale neuhradená: %v", p.SupplierNumber, p.Supplier.Name, err)
				}
			}
		}
	}

	for _, s := range m.Skips {
		if s.Purchase {
			rep.Purchases.Skipped++
		} else {
			rep.Invoices.Skipped++
		}
		rep.warn("%s", s.Text)
	}
	if apply {
		a.record(ctx, companyID, audit.CatSettings, "import", "migrate", 0, fmt.Sprintf(
			"Prenos z %s: adresár %d, cenník %d, vydané faktúry %d, prijaté faktúry %d",
			m.Source, rep.Contacts.New, rep.Items.New, rep.Invoices.New, rep.Purchases.New), nil, nil)
	}
	return rep, nil
}

// paidOn je dátum úhrady prenesenej faktúry: splatnosť, najneskôr dnes.
func (a *App) paidOn(due invoice.Date) invoice.Date {
	if due.IsZero() || due.After(a.Today()) {
		return a.Today()
	}
	return due
}

// migInvoiceBuild zostaví faktúru: z položiek, a ak nesedia so sumou zo
// zdroja (zľavy, zaokrúhlenie), z rekapitulácie DPH.
func (a *App) migInvoiceBuild(ctx context.Context, companyID int64, mi migInvoice, numbers map[string]int64, rep *ImportReport) (invoice.Invoice, error) {
	if mi.Err != nil {
		return invoice.Invoice{}, mi.Err
	}
	inv := mi.Inv
	inv.Currency = "EUR"
	if inv.Kind == "" {
		inv.Kind = invoice.KindInvoice
	}
	if inv.Customer.Name == "" {
		return inv, fmt.Errorf("chýba odberateľ")
	}
	if inv.IssueDate.IsZero() {
		return inv, fmt.Errorf("chýba dátum vystavenia")
	}
	if inv.DeliveryDate.IsZero() {
		inv.DeliveryDate = inv.IssueDate
	}
	if inv.DueDate.IsZero() {
		inv.DueDate = inv.IssueDate
	}
	if mi.Credit {
		inv.Kind = invoice.KindCreditNote
		orig := strings.TrimSpace(mi.Original)
		id, ok := numbers[orig]
		if orig == "" || !ok {
			return inv, fmt.Errorf("dobropis k faktúre %q, ktorá nie je vo Fiškuse ani v súbore — preneste aj pôvodnú faktúru", orig)
		}
		inv.Correction = &invoice.Correction{OriginalID: id, OriginalNumber: orig, Reason: firstNonBlank(mi.Reason, "Oprava faktúry")}
	}
	if err := a.applyCompany(ctx, companyID, &inv); err != nil {
		return inv, err
	}
	calc := func(lines []invoice.Line, mode vat.PriceMode) (invoice.Invoice, error) {
		x := inv
		x.Lines = append([]invoice.Line(nil), lines...)
		x.PriceMode = mode
		if err := x.Recalculate(); err != nil {
			return x, err
		}
		if mi.Credit && x.Summary.Total.Sign() > 0 { // dobropis je záporný
			for i := range x.Lines {
				x.Lines[i].Quantity = x.Lines[i].Quantity.Neg()
			}
			if err := x.Recalculate(); err != nil {
				return x, err
			}
		}
		return x, nil
	}
	fits := func(x invoice.Invoice) bool {
		return mi.Total.IsZero() || absDec(absDec(x.Summary.Total).Sub(absDec(mi.Total))).Cmp(migTolerance) <= 0
	}
	if len(inv.Lines) > 0 {
		x, err := calc(inv.Lines, inv.PriceMode)
		if err == nil && fits(x) {
			return x, nil
		}
		if len(mi.Summary) == 0 {
			if err != nil {
				return x, err
			}
			x.Note = strings.TrimSpace(x.Note + "\nPrenos: suma v pôvodnom programe " + mi.Total.Display() + " €")
			rep.warn("Faktúra %s: suma %s € sa líši od pôvodnej %s € — skontrolujte ju", mi.Number, x.Summary.Total.Display(), mi.Total.Display())
			return x, nil
		}
		rep.warn("Faktúra %s: položky nesedia so sumou (zľava, zaokrúhlenie) — prenesená podľa rekapitulácie DPH", mi.Number)
	}
	if len(mi.Summary) == 0 {
		return inv, fmt.Errorf("faktúra nemá položky ani sumu")
	}
	return calc(mi.Summary, vat.PriceNet)
}

// migInvoiceSave uloží faktúru s pôvodným číslom ako vystavenú, prípadne uhradenú.
func (a *App) migInvoiceSave(ctx context.Context, companyID int64, inv *invoice.Invoice, mi migInvoice, opt MigrateOptions, source string) error {
	inv.Number = strings.TrimSpace(mi.Number)
	inv.Status = invoice.StatusIssued
	total := inv.Summary.Total
	paid := !total.IsZero() && mi.Paid.Sign() == total.Sign() && absDec(mi.Paid).Cmp(absDec(total)) >= 0
	if paid || (!opt.PaidUntil.IsZero() && !inv.DueDate.After(opt.PaidUntil)) {
		if err := inv.AddPayment(invoice.Payment{Date: a.paidOn(inv.DueDate), Amount: inv.Remaining(), Source: "import"}); err != nil {
			return err
		}
	}
	if err := a.Store.SaveInvoice(ctx, companyID, inv); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "import", "invoice", inv.ID, "Prenesená faktúra "+inv.Number+" z "+source, nil, nil)
	return nil
}

// rateFromPercent prevedie percento DPH zo zdroja na sadzbu.
func rateFromPercent(s string) (vat.Rate, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return vat.Rate0, false
	}
	p, err := money.Parse(strings.Replace(s, ",", ".", 1))
	if err != nil {
		return vat.Rate0, false
	}
	switch p.RoundTo(0).String() {
	case "23.00", "23":
		return vat.Rate23, true
	case "20.00", "20":
		return vat.Rate20, true
	case "19.00", "19":
		return vat.Rate19, true
	case "10.00", "10":
		return vat.Rate10, true
	case "5.00", "5":
		return vat.Rate5, true
	case "0.00", "0":
		return vat.Rate0, true
	}
	return vat.Rate0, false
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
	s = strings.TrimSpace(s)
	if len(s) > 10 {
		s = s[:10] // 2026-03-02T00:00:00
	}
	d, err := invoice.ParseDate(s)
	if err != nil {
		return invoice.Date{}
	}
	return d
}

func firstNonBlank(ss ...string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// migSubject skráti text dokladu na predmet faktúry.
func migSubject(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > invoice.SubjectMax {
		s = string(r[:invoice.SubjectMax])
	}
	return s
}
