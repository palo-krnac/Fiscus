package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/moneys3"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Prenos dát z Money S3 (XML export): adresár firiem, zásoby do cenníka,
// vydané faktúry a dobropisy, prijaté faktúry. Money S3 v exporte faktúr
// neuvádza, či sú uhradené — na to je voľba „uhradené so splatnosťou do“.

// ImportMoney prenesie dáta z XML exportu Money S3 (apply=false: iba náhľad).
func (a *App) ImportMoney(ctx context.Context, companyID int64, data []byte, opt MigrateOptions, apply bool) (ImportReport, error) {
	if err := a.requireMigrate(ctx, companyID); err != nil {
		return ImportReport{}, err
	}
	d, err := moneys3.ParseArchive(data, MaxMigrateBytes)
	if err != nil {
		return ImportReport{Source: "Money S3", Warnings: []string{}, Other: map[string]int{}}, mfaFieldErr("file", err.Error())
	}
	if len(d.Contacts)+len(d.Stocks)+len(d.Invoices) == 0 {
		return ImportReport{Source: "Money S3", Warnings: []string{}, Other: d.Other},
			mfaFieldErr("file", "V súbore nie je adresár, zásoby ani faktúry — v Money S3 cez XML prenosy (Money → Import/export) vyexportujte adresár, zásoby a faktúry")
	}
	return a.migrate(ctx, companyID, moneyMigration(d), opt, apply)
}

// moneyParty prevedie partnera Money S3 (fakturačný názov a adresa).
func moneyParty(p moneys3.Partner) (invoice.Party, string) {
	name, ad := p.Display()
	dic, icdph := p.TaxIDs()
	warn := ""
	country := party.CountryCode(firstNonBlank(ad.CountryC, ad.Country))
	if country == "" && len(icdph) > 2 {
		country = icdph[:2]
	}
	if country == "" {
		if s := strings.TrimSpace(ad.Country); s != "" {
			warn = fmt.Sprintf("%s: neznámy štát %q — doplňte ho v adresári", name, s)
		}
		country = "SK"
	}
	return invoice.Party{Name: name, Street: strings.TrimSpace(ad.Street), City: strings.TrimSpace(ad.City),
		Zip: strings.ReplaceAll(strings.TrimSpace(ad.Zip), " ", ""), Country: country, ICO: strings.ReplaceAll(strings.TrimSpace(p.ICO), " ", ""),
		DIC: dic, ICDPH: icdph, Email: strings.TrimSpace(p.Email)}, warn
}

// moneyRate: percento z dokladu, inak predvolená sadzba.
func moneyRate(percent string, def vat.Rate) vat.Rate {
	if r, ok := rateFromPercent(percent); ok {
		return r
	}
	return def
}

// moneyLevels sú sadzby zníženej a základnej hladiny DPH v čase plnenia
// (Money ich na doklade uvádza v SazbaDPH1 a SazbaDPH2).
func moneyLevels(inv moneys3.Invoice, d invoice.Date) (low, high vat.Rate) {
	low, high = vat.Rate19, vat.Rate23
	if !d.IsZero() && d.Before(invoice.MustDate("2025-01-01")) {
		low, high = vat.Rate10, vat.Rate20
	}
	return moneyRate(inv.RateLow, low), moneyRate(inv.RateHigh, high)
}

// moneyTotal: Celkem, inak súčet rekapitulácie.
func moneyTotal(inv moneys3.Invoice) money.Dec {
	if t := pdec(inv.Total); !t.IsZero() {
		return t
	}
	s := inv.Summary
	t := money.Zero
	for _, v := range []string{s.Base0, s.BaseLow, s.VATLow, s.BaseHigh, s.VATHigh} {
		t = t.Add(pdec(v))
	}
	for _, o := range s.Others {
		t = t.Add(pdec(o.Base)).Add(pdec(o.VAT))
	}
	return t
}

// moneyRates sú riadky rekapitulácie DPH: sadzba, základ, daň.
func moneyRates(inv moneys3.Invoice, d invoice.Date) []purchase.RateRow {
	low, high := moneyLevels(inv, d)
	s := inv.Summary
	rows := []purchase.RateRow{
		{Rate: vat.Rate0, Base: pdec(s.Base0)},
		{Rate: low, Base: pdec(s.BaseLow), VAT: pdec(s.VATLow)},
		{Rate: high, Base: pdec(s.BaseHigh), VAT: pdec(s.VATHigh)},
	}
	for _, o := range s.Others {
		rows = append(rows, purchase.RateRow{Rate: moneyRate(o.Rate, vat.Rate5), Base: pdec(o.Base), VAT: pdec(o.VAT)})
	}
	out := rows[:0]
	for _, r := range rows {
		if !r.Base.IsZero() {
			out = append(out, r)
		}
	}
	return out
}

// moneyMigration prevedie export Money S3 na spoločný prenos.
func moneyMigration(d moneys3.Data) migration {
	m := migration{Source: "Money S3", Other: d.Other}
	for _, c := range d.Contacts {
		p, warn := moneyParty(c)
		if warn != "" {
			m.Warnings = append(m.Warnings, "Adresár "+warn)
		}
		m.Contacts = append(m.Contacts, migContact{Party: p, Phone: c.PhoneNumber(), Note: strings.TrimSpace(c.Note)})
	}
	for _, st := range d.Stocks {
		c := st.Card
		it := catalog.Item{Code: c.Code(), Name: strings.TrimSpace(c.Name), Unit: firstNonBlank(c.Unit, "ks"),
			Price: pdec(st.Price.Price1.Price).RoundCents(), PriceMode: vat.PriceNet, Rate: moneyRate(c.RateOut, vat.Rate23), Note: strings.TrimSpace(c.Note)}
		if strings.TrimSpace(st.Price.Price1.Gross) == "1" {
			it.PriceMode = vat.PriceGross
		}
		m.Items = append(m.Items, it)
	}
	for _, inv := range d.Invoices {
		num := strings.TrimSpace(inv.Number)
		if k := strings.ToUpper(strings.TrimSpace(inv.Kind)); k != "" && k != "N" {
			label := map[string]string{"L": "zálohová faktúra", "Z": "zálohová faktúra", "D": "daňový doklad k platbe"}[k]
			if label == "" {
				label = "doklad druhu " + k
			}
			if inv.Received {
				label = "prijatá " + label
			}
			m.Skips = append(m.Skips, migSkip{Agenda: map[bool]string{true: agPurchases, false: agInvoices}[inv.Received],
				Text: fmt.Sprintf("%s %s sa neprenáša (zálohy a ostatné doklady zadajte vo Fiškuse)", strings.ToUpper(label[:1])+label[1:], num)})
			continue
		}
		if inv.Received {
			p, err := moneyPurchase(inv)
			m.Purchases = append(m.Purchases, migPurchase{Ref: firstNonBlank(inv.SupplierNumber, inv.VarSymbol, num), P: p, Err: err})
			continue
		}
		mi, warn := moneyInvoice(inv)
		if warn != "" {
			m.Warnings = append(m.Warnings, "Faktúra "+num+" — "+warn)
		}
		m.Invoices = append(m.Invoices, mi)
	}
	return m
}

func moneyCurrencyErr(inv moneys3.Invoice) error {
	if h := inv.HomeCurrency(); h != "EUR" {
		return fmt.Errorf("účtovníctvo v Money je v mene %s — Fiškus vedie doklady v eurách", h)
	}
	if c := inv.Currency(); c != "" {
		return fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", c)
	}
	return nil
}

// moneyInvoice zostaví vydanú faktúru alebo dobropis.
func moneyInvoice(inv moneys3.Invoice) (migInvoice, string) {
	mi := migInvoice{Number: strings.TrimSpace(inv.Number), Credit: inv.Credit(), Original: strings.TrimSpace(inv.Original),
		Reason: inv.Text, Total: moneyTotal(inv)}
	if mi.Err = moneyCurrencyErr(inv); mi.Err != nil {
		return mi, ""
	}
	cust, warn := moneyParty(inv.Partner)
	delivery := pdate(firstNonBlank(inv.TaxDate, inv.Issued))
	mi.Inv = invoice.Invoice{Customer: cust, IssueDate: pdate(inv.Issued), DeliveryDate: delivery, DueDate: pdate(inv.Due),
		VariableSymbol: strings.TrimSpace(inv.VarSymbol), PriceMode: vat.PriceNet, Note: strings.TrimSpace(inv.Note), Subject: migSubject(inv.Text)}

	// položky iba vtedy, keď sa dajú preniesť presne (jeden druh ceny, známa sadzba)
	usable := pdec(inv.Discount).IsZero()
	modes := map[string]bool{}
	var lines []invoice.Line
	for _, it := range inv.Items {
		q := pdec(it.Quantity)
		text := firstNonBlank(it.Text, it.Note)
		if text == "" && q.IsZero() {
			continue
		}
		if q.IsZero() {
			q = money.FromInt(1)
		}
		pt := strings.TrimSpace(it.PriceType)
		if pt == "" {
			pt = "0"
		}
		modes[pt] = true
		rate, ok := rateFromPercent(it.Rate)
		if !ok || (pt != "0" && pt != "1") {
			usable = false
		}
		price := pdec(it.Price)
		if disc := pdec(it.Discount); !disc.IsZero() {
			price = price.MulRatio(money.FromInt(100).Sub(disc), money.FromInt(100))
		}
		lines = append(lines, invoice.Line{Description: firstNonBlank(text, "Položka"), Quantity: q, Unit: firstNonBlank(it.UnitName(), "ks"),
			UnitPrice: price, Rate: rate})
	}
	if usable && len(modes) <= 1 {
		mi.Inv.Lines = lines
		if modes["1"] {
			mi.Inv.PriceMode = vat.PriceGross
		}
	}
	for _, r := range moneyRates(inv, delivery) {
		mi.Summary = append(mi.Summary, invoice.Line{Description: firstNonBlank(inv.Text, "Plnenie podľa faktúry"), Quantity: money.FromInt(1),
			Unit: "ks", UnitPrice: r.Base, Rate: r.Rate})
	}
	return mi, warn
}

// moneyPurchase zostaví prijatú faktúru z rekapitulácie DPH.
func moneyPurchase(inv moneys3.Invoice) (purchase.Purchase, error) {
	if err := moneyCurrencyErr(inv); err != nil {
		return purchase.Purchase{}, err
	}
	issue := pdate(inv.Issued)
	if issue.IsZero() {
		return purchase.Purchase{}, fmt.Errorf("chýba dátum")
	}
	sup, _ := moneyParty(inv.Partner)
	delivery := pdate(firstNonBlank(inv.TaxDate, inv.Issued))
	p := purchase.Purchase{SupplierNumber: firstNonBlank(inv.SupplierNumber, inv.VarSymbol, inv.Number), VariableSymbol: strings.TrimSpace(inv.VarSymbol),
		Supplier: sup, IssueDate: issue, DeliveryDate: delivery, ReceivedDate: pdate(firstNonBlank(inv.Accounting, inv.TaxDate, inv.Issued)),
		DueDate: pdate(firstNonBlank(inv.Due, inv.Issued)), Description: firstNonBlank(inv.Text, "Prenos z Money S3"), Note: strings.TrimSpace(inv.Note),
		Expense: purchase.ExpenseServices, Rates: moneyRates(inv, delivery)}
	if p.Supplier.Name == "" {
		return p, fmt.Errorf("chýba dodávateľ")
	}
	if len(p.Rates) == 0 {
		return p, fmt.Errorf("faktúra nemá sumu")
	}
	return p, nil
}
