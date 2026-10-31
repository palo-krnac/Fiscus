package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/pohoda"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Prenos dát z programu POHODA (XML export): adresár, zásoby do cenníka,
// vydané faktúry a dobropisy, prijaté faktúry. Prenos robí migrate.

// pohodaRate prevedie sadzbu POHODY na sadzbu DPH: percento z dokladu,
// inak podľa druhu sadzby a dátumu (od 2025 základná 23 %, znížené 19 % a 5 %).
func pohodaRate(kind, percent string, d invoice.Date) vat.Rate {
	if r, ok := rateFromPercent(percent); ok {
		return r
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
func (a *App) ImportPohoda(ctx context.Context, companyID int64, data []byte, opt MigrateOptions, apply bool) (ImportReport, error) {
	if err := a.requireMigrate(ctx, companyID); err != nil {
		return ImportReport{}, err
	}
	d, err := pohoda.Parse(data)
	if err != nil {
		return ImportReport{Source: "POHODA", Warnings: []string{}, Other: map[string]int{}}, mfaFieldErr("file", err.Error())
	}
	if len(d.Contacts)+len(d.Stocks)+len(d.Invoices) == 0 {
		return ImportReport{Source: "POHODA", Warnings: []string{}, Other: d.Other},
			mfaFieldErr("file", "V súbore nie je adresár, zásoby ani faktúry — v POHODE zvoľte Súbor → Dátová komunikácia → XML export")
	}
	return a.migrate(ctx, companyID, pohodaMigration(d, a.Today()), opt, apply)
}

// pohodaMigration prevedie export POHODY na spoločný prenos.
func pohodaMigration(d pohoda.Data, today invoice.Date) migration {
	m := migration{Source: "POHODA", Other: d.Other}
	for _, c := range d.Contacts {
		ad := c.Address()
		m.Contacts = append(m.Contacts, migContact{Party: pohodaParty(ad), Phone: strings.TrimSpace(ad.Phone), Note: strings.TrimSpace(c.Header.Note)})
	}
	for _, st := range d.Stocks {
		h := st.Header
		it := catalog.Item{Code: strings.TrimSpace(h.Code), Name: strings.TrimSpace(h.Name), Unit: firstNonBlank(h.Unit, "ks"),
			Price: pdec(h.SellingPrice.Value).RoundCents(), PriceMode: vat.PriceNet, Rate: pohodaRate(h.SellingRate, "", today), Note: strings.TrimSpace(h.Note)}
		if h.SellingPrice.PayVAT == "true" {
			it.PriceMode = vat.PriceGross
		}
		m.Items = append(m.Items, it)
	}
	for _, pi := range d.Invoices {
		h := pi.Header
		num := strings.TrimSpace(h.Number.Requested)
		switch h.Type {
		case "issuedInvoice", "issuedCreditNotice":
			m.Invoices = append(m.Invoices, pohodaInvoice(pi))
		case "receivedInvoice", "receivedCreditNotice":
			p, err := pohodaPurchase(pi)
			m.Purchases = append(m.Purchases, migPurchase{Ref: firstNonBlank(h.OriginalDocument, num), P: p, Err: err,
				Paid: err == nil && pdec(h.Liquidation.AmountHome).Sign() > 0 && pdec(h.Liquidation.AmountHome).Cmp(pohodaTotal(pi)) >= 0})
		default:
			label := map[string]string{"issuedProformaInvoice": "zálohová faktúra", "issuedAdvanceInvoice": "zálohová faktúra",
				"receivedProformaInvoice": "prijatá zálohová faktúra", "receivedAdvanceInvoice": "prijatá zálohová faktúra",
				"receivable": "ostatná pohľadávka", "commitment": "ostatný záväzok"}[h.Type]
			if label == "" {
				label = h.Type
			}
			issued := strings.HasPrefix(h.Type, "issued") || h.Type == "receivable" || h.Type == "penalty"
			m.Skips = append(m.Skips, migSkip{Purchase: !issued,
				Text: fmt.Sprintf("%s %s sa neprenáša (zálohy a ostatné doklady zadajte vo Fiškuse)", strings.ToUpper(label[:1])+label[1:], num)})
		}
	}
	return m
}

// pohodaInvoice zostaví vydanú faktúru alebo dobropis.
func pohodaInvoice(pi pohoda.Invoice) migInvoice {
	h := pi.Header
	mi := migInvoice{Number: strings.TrimSpace(h.Number.Requested), Credit: h.Type == "issuedCreditNotice",
		Original: strings.TrimSpace(h.OriginalDocument), Reason: h.Text, Total: pohodaTotal(pi), Paid: pdec(h.Liquidation.AmountHome)}
	if pi.Foreign() {
		mi.Err = fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", pi.Summary.Foreign.Currency.IDs)
		return mi
	}
	delivery := pdate(firstNonBlank(h.DateTax, h.DateDelivery, h.Date))
	mi.Inv = invoice.Invoice{Customer: pohodaParty(h.Partner.Address), IssueDate: pdate(h.Date), DeliveryDate: delivery,
		DueDate: pdate(firstNonBlank(h.DateDue, h.Date)), VariableSymbol: strings.TrimSpace(h.SymVar), PriceMode: vat.PriceNet, Note: strings.TrimSpace(h.Note), Subject: migSubject(h.Text)}
	for _, it := range pi.Items {
		if it.PayVAT == "true" {
			mi.Inv.PriceMode = vat.PriceGross
		}
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
		mi.Inv.Lines = append(mi.Inv.Lines, invoice.Line{Description: firstNonBlank(text, "Položka"), Quantity: q, Unit: firstNonBlank(it.Unit, "ks"),
			UnitPrice: pdec(it.Home.UnitPrice), Rate: pohodaRate(it.RateVAT, it.PercentVAT, delivery)})
	}
	s := pi.Summary.Home
	for _, r := range []struct{ base, kind string }{{s.PriceNone, "none"}, {s.PriceLow, "low"}, {s.PriceHigh, "high"}, {s.Price3, "third"}} {
		if b := pdec(r.base); !b.IsZero() {
			mi.Summary = append(mi.Summary, invoice.Line{Description: firstNonBlank(h.Text, "Plnenie podľa faktúry"), Quantity: money.FromInt(1),
				Unit: "ks", UnitPrice: b, Rate: pohodaRate(r.kind, "", delivery)})
		}
	}
	return mi
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
	return p, nil
}
