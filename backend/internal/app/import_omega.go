package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/omega"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/tabimport"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Prenos z KROS Omega (textový export): partneri, skladové karty,
// fakturácia (odberateľské faktúry, došlé faktúry) a doklady EUD
// (odoslané a došlé faktúry s úhradami). Adresu partnera na doklade EUD
// doplní číselník partnerov z toho istého súboru.

// ImportOmega prenesie dáta z textového exportu Omegy (apply=false: iba náhľad).
func (a *App) ImportOmega(ctx context.Context, companyID int64, data []byte, opt MigrateOptions, apply bool) (ImportReport, error) {
	if err := a.requireMigrate(ctx, companyID); err != nil {
		return ImportReport{}, err
	}
	empty := ImportReport{Source: "KROS Omega", Warnings: []string{}, Other: map[string]int{}}
	f, err := omega.Parse(data)
	if err != nil {
		return empty, mfaFieldErr("file", err.Error())
	}
	m := omegaMigration(f)
	if len(m.Contacts)+len(m.Items)+len(m.Invoices)+len(m.Purchases)+len(m.Skips) == 0 {
		empty.Other = m.Other
		return empty, mfaFieldErr("file", "V súbore nie sú partneri, skladové karty ani faktúry — v Omege zvoľte Firma → Export → Export do textového súboru")
	}
	return a.migrate(ctx, companyID, m, opt, apply)
}

func odec(s string) money.Dec {
	if strings.TrimSpace(s) == "" {
		return money.Zero
	}
	d, err := tabimport.Number(s)
	if err != nil {
		return money.Zero
	}
	return d
}

func odate(s string) invoice.Date {
	d, _ := tabimport.Date(s)
	return d
}

// omegaICDPH spojí kód štátu a IČ DPH (Omega ich má v dvoch stĺpcoch).
func omegaICDPH(code, id string) string {
	id = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(id), " ", ""))
	code = strings.ToUpper(strings.TrimSpace(code))
	if id == "" {
		return ""
	}
	if len(id) > 2 && id[0] >= 'A' && id[0] <= 'Z' && id[1] >= 'A' && id[1] <= 'Z' {
		return id
	}
	return code + id
}

func omegaParty(name, ico, dic, icdph, street, zip, city, country string) (invoice.Party, string) {
	return tableParty(tabimport.Party{Name: strings.TrimSpace(name), ICO: strings.ReplaceAll(strings.TrimSpace(ico), " ", ""),
		DIC: strings.ReplaceAll(strings.TrimSpace(dic), " ", ""), ICDPH: icdph, Street: strings.TrimSpace(street),
		Zip: strings.ReplaceAll(strings.TrimSpace(zip), " ", ""), City: strings.TrimSpace(city), Country: strings.TrimSpace(country)})
}

// omegaLevels: sadzby nižšej a vyššej hladiny z dokladu, inak podľa dátumu.
func omegaLevels(low, high string, d invoice.Date) (vat.Rate, vat.Rate) {
	l, h := vat.Rate19, vat.Rate23
	if !d.IsZero() && d.Before(invoice.MustDate("2025-01-01")) {
		l, h = vat.Rate10, vat.Rate20
	}
	return moneyRate(low, l), moneyRate(high, h)
}

// omegaAmounts sú riadky rekapitulácie DPH (základ 0 a neobsahuje DPH spolu).
func omegaAmounts(base0, baseFree, baseLow, vatLow, baseHigh, vatHigh string, low, high vat.Rate) []purchase.RateRow {
	var out []purchase.RateRow
	if b := odec(base0).Add(odec(baseFree)); !b.IsZero() {
		out = append(out, purchase.RateRow{Rate: vat.Rate0, Base: b})
	}
	if b := odec(baseLow); !b.IsZero() {
		out = append(out, purchase.RateRow{Rate: low, Base: b, VAT: odec(vatLow)})
	}
	if b := odec(baseHigh); !b.IsZero() {
		out = append(out, purchase.RateRow{Rate: high, Base: b, VAT: odec(vatHigh)})
	}
	return out
}

func omegaTotal(total string, rows []purchase.RateRow) money.Dec {
	if t := odec(total); !t.IsZero() {
		return t
	}
	t := money.Zero
	for _, r := range rows {
		t = t.Add(r.Base).Add(r.VAT)
	}
	return t
}

func summaryLines(rows []purchase.RateRow, text string) []invoice.Line {
	var out []invoice.Line
	for _, r := range rows {
		out = append(out, invoice.Line{Description: firstNonBlank(text, "Plnenie podľa faktúry"), Quantity: money.FromInt(1), Unit: "ks",
			UnitPrice: r.Base, Rate: r.Rate})
	}
	return out
}

func foreignCurrency(c string) bool {
	c = strings.ToUpper(strings.TrimSpace(c))
	return c != "" && c != "EUR" && c != "€"
}

// omegaKinds: typy dokladov fakturácie (T01, stĺpec 18) a EUD (T00, stĺpec 2),
// ktoré sa neprenášajú, s názvom agendy.
var omegaOtherT01 = map[string]string{"2": "dodacie listy", "3": "odoslané objednávky", "5": "zákazkové listy", "6": "likvidačné listy",
	"7": "reklamácie", "8": "upomienky", "11": "došlé objednávky", "12": "cenové ponuky", "13": "predajky"}
var omegaOtherT00 = map[string]string{"160": "pokladničné doklady", "170": "bankové výpisy", "180": "interné doklady", "190": "ostatné doklady"}

func omegaMigration(f omega.File) migration {
	m := migration{Source: "KROS Omega", Other: map[string]int{}}
	// partneri: aj na doplnenie adresy na dokladoch EUD
	partners := map[string]invoice.Party{}
	for _, d := range f.Docs("T04") {
		h := d.Head
		p, warn := omegaParty(h.Col(omega.PName), h.Col(omega.PICO), h.Col(omega.PDIC), omegaICDPH(h.Col(omega.PVATCode), h.Col(omega.PICDPH)),
			h.Col(omega.PStreet), h.Col(omega.PZip), h.Col(omega.PCity), h.Col(omega.PCountry))
		p.Email = h.Col(omega.PEmail)
		if warn != "" {
			m.Warnings = append(m.Warnings, "Partneri: "+warn)
		}
		m.Contacts = append(m.Contacts, migContact{Party: p, Note: h.Col(omega.PNote)})
		if p.ICO != "" {
			partners["ico:"+p.ICO] = p
		}
		partners["name:"+strings.ToLower(p.Name)] = p
	}
	lookup := func(p invoice.Party) invoice.Party {
		q, ok := partners["ico:"+p.ICO]
		if !ok || p.ICO == "" {
			q, ok = partners["name:"+strings.ToLower(p.Name)]
		}
		if !ok {
			return p
		}
		if p.Street == "" && p.City == "" {
			p.Street, p.City, p.Zip, p.Country = q.Street, q.City, q.Zip, q.Country
		}
		if p.ICDPH == "" {
			p.ICDPH = q.ICDPH
		}
		if p.DIC == "" {
			p.DIC = q.DIC
		}
		if p.Email == "" {
			p.Email = q.Email
		}
		return p
	}

	// skladové karty → cenník
	for _, d := range f.Docs("T03") {
		for _, it := range d.Items {
			c := catalog.Item{Code: it.Col(omega.SNumber), Name: it.Col(omega.SName), Unit: firstNonBlank(it.Col(omega.SUnit), "ks"),
				Price: odec(it.Col(omega.SPrice)).RoundCents(), PriceMode: vat.PriceNet, Note: it.Col(omega.SNote)}
			low, high := omegaLevels("", "", invoice.Date{})
			c.Rate = map[string]vat.Rate{"0": vat.Rate0, "N": low, "V": high}[strings.ToUpper(it.Col(omega.SRate))]
			if c.Rate == 0 && strings.ToUpper(it.Col(omega.SRate)) != "0" {
				c.Rate = high
			}
			if c.Price.IsZero() && !odec(it.Col(omega.SPriceVAT)).IsZero() {
				c.Price, c.PriceMode = odec(it.Col(omega.SPriceVAT)).RoundCents(), vat.PriceGross
			}
			m.Items = append(m.Items, c)
		}
	}

	// fakturácia (T01)
	for _, d := range f.Docs("T01") {
		h := d.Head
		num := h.Col(omega.FNumber)
		kind := h.Col(omega.FKind)
		switch kind {
		case "", "0", "4", "14":
		case "1":
			m.Skips = append(m.Skips, migSkip{Agenda: agInvoices, Text: fmt.Sprintf("Preddavková faktúra %s sa neprenáša (zálohy zadajte vo Fiškuse)", num)})
			continue
		case "10":
			m.Skips = append(m.Skips, migSkip{Agenda: agInvoices, Text: fmt.Sprintf("Storno faktúra %s sa neprenáša", num)})
			continue
		case "9":
			m.Skips = append(m.Skips, migSkip{Agenda: agInvoices, Text: fmt.Sprintf("Penalizačná faktúra %s sa neprenáša — zadajte ju vo Fiškuse", num)})
			continue
		default:
			label := omegaOtherT01[kind]
			if label == "" {
				label = "doklady typu " + kind
			}
			m.Other[label]++
			continue
		}
		issue := odate(h.Col(omega.FIssued))
		if kind == "14" && !odate(h.Col(omega.FIssuedDF)).IsZero() {
			issue = odate(h.Col(omega.FIssuedDF))
		}
		delivery := odate(firstNonBlank(h.Col(omega.FTaxDate), h.Col(omega.FIssued)))
		low, high := omegaLevels(h.Col(omega.FRateLow), h.Col(omega.FRateHigh), delivery)
		rows := omegaAmounts(h.Col(omega.FBase0), h.Col(omega.FBaseFree), h.Col(omega.FBaseLow), h.Col(omega.FVATLow), h.Col(omega.FBaseHigh), h.Col(omega.FVATHigh), low, high)
		p, warn := omegaParty(h.Col(omega.FPartner), h.Col(omega.FICO), h.Col(omega.FDIC), omegaICDPH(h.Col(omega.FVATCode), h.Col(omega.FICDPH)),
			h.Col(omega.FStreet), h.Col(omega.FZip), h.Col(omega.FCity), h.Col(omega.FCountry))
		p = lookup(p)
		if warn != "" {
			m.Warnings = append(m.Warnings, "Doklad "+num+": "+warn)
		}
		var err error
		if foreignCurrency(h.Col(omega.FCurrency)) {
			err = fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", h.Col(omega.FCurrency))
		}
		subject := firstNonBlank(h.Col(omega.FSubject), h.Col(omega.FNote))
		if kind == "14" {
			mp := migPurchase{Ref: num, Err: err}
			if err == nil {
				mp.P = purchase.Purchase{SupplierNumber: num, VariableSymbol: h.Col(omega.FVarSymbol), Supplier: p, IssueDate: issue,
					DeliveryDate: delivery, ReceivedDate: firstDate(odate(h.Col(omega.FIssued)), issue), DueDate: firstDate(odate(h.Col(omega.FDue)), issue),
					Description: firstNonBlank(subject, "Prenos z KROS Omega"), Note: h.Col(omega.FNote), Expense: purchase.ExpenseServices, Rates: rows}
				if issue.IsZero() {
					mp.Err = errors.New("chýba dátum")
				} else if len(rows) == 0 {
					mp.Err = errors.New("faktúra nemá sumu")
				}
			}
			m.Purchases = append(m.Purchases, mp)
			continue
		}
		mi := migInvoice{Number: num, Ref: fmt.Sprintf("riadok %d", d.Line), Credit: kind == "4", Reason: subject,
			Total: omegaTotal(h.Col(omega.FTotal), rows), Err: err}
		if mi.Credit && err == nil {
			mi.Err = errors.New("dobropis z fakturácie Omegy neobsahuje číslo opravovanej faktúry — zadajte ho vo Fiškuse")
		}
		mi.Inv = invoice.Invoice{Customer: p, IssueDate: issue, DeliveryDate: delivery, DueDate: odate(h.Col(omega.FDue)),
			VariableSymbol: h.Col(omega.FVarSymbol), PriceMode: vat.PriceNet, Note: h.Col(omega.FNote), Subject: migSubject(subject)}
		mi.Inv.Lines, mi.Inv.PriceMode = omegaLines(d.Items, low, high)
		mi.Summary = summaryLines(rows, subject)
		m.Invoices = append(m.Invoices, mi)
	}

	// doklady EUD (T00): odoslané a došlé faktúry
	for _, d := range f.Docs("T00") {
		h := d.Head
		kind := h.Col(omega.EKind)
		num := firstNonBlank(h.Col(omega.EInternal), h.Col(omega.EExternal))
		switch kind {
		case "100", "130":
		case "110", "140":
			m.Skips = append(m.Skips, migSkip{Agenda: map[bool]string{true: agInvoices, false: agPurchases}[kind == "110"],
				Text: fmt.Sprintf("Preddavková faktúra %s sa neprenáša (zálohy zadajte vo Fiškuse)", num)})
			continue
		case "120", "150":
			m.Skips = append(m.Skips, migSkip{Agenda: map[bool]string{true: agInvoices, false: agPurchases}[kind == "120"],
				Text: fmt.Sprintf("Doklad %s (ostatné %s) sa neprenáša", num, map[bool]string{true: "pohľadávky", false: "záväzky"}[kind == "120"])})
			continue
		default:
			label := omegaOtherT00[kind]
			if label == "" {
				label = "doklady EUD typu " + kind
			}
			m.Other[label]++
			continue
		}
		issue := odate(h.Col(omega.EIssued))
		delivery := firstDate(odate(h.Col(omega.ETaxDate)), issue)
		low, high := omegaLevels(h.Col(omega.ERateLow), h.Col(omega.ERateHigh), delivery)
		rows := omegaAmounts(h.Col(omega.EBase0), h.Col(omega.EBaseFree), h.Col(omega.EBaseLow), h.Col(omega.EVATLow), h.Col(omega.EBaseHigh), h.Col(omega.EVATHigh), low, high)
		p, warn := omegaParty(h.Col(omega.EPartner), h.Col(omega.EICO), h.Col(omega.EDIC), omegaICDPH(h.Col(omega.EVATCode), h.Col(omega.EICDPH)), "", "", "", "")
		p = lookup(p)
		if warn != "" {
			m.Warnings = append(m.Warnings, "Doklad "+num+": "+warn)
		}
		total := omegaTotal(h.Col(omega.ETotal), rows)
		// úhrady (R02 typ 1)
		paid, paidOn := money.Zero, invoice.Date{}
		for _, it := range d.Items {
			if it.Col(omega.EItemType) != "1" {
				continue
			}
			paid = paid.Add(odec(it.Col(omega.EPayAmount)))
			if dt := odate(it.Col(omega.EPayDate)); dt.After(paidOn) {
				paidOn = dt
			}
		}
		full := !total.IsZero() && absDec(paid).Cmp(absDec(total)) >= 0
		var err error
		if foreignCurrency(h.Col(omega.ECurrency)) {
			err = fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", h.Col(omega.ECurrency))
		}
		text := firstNonBlank(h.Col(omega.EText), h.Col(omega.ENote))
		if kind == "130" {
			mp := migPurchase{Ref: firstNonBlank(h.Col(omega.EExternal), num), Err: err, Paid: full}
			if full {
				mp.PaidDate = paidOn
			}
			if err == nil {
				mp.P = purchase.Purchase{SupplierNumber: firstNonBlank(h.Col(omega.EExternal), num), Supplier: p, IssueDate: issue,
					DeliveryDate: delivery, ReceivedDate: firstDate(odate(h.Col(omega.EReceived)), issue), DueDate: firstDate(odate(h.Col(omega.EDue)), issue),
					Description: firstNonBlank(text, "Prenos z KROS Omega"), Note: h.Col(omega.ENote), Expense: purchase.ExpenseServices, Rates: rows}
				if issue.IsZero() {
					mp.Err = errors.New("chýba dátum")
				} else if len(rows) == 0 {
					mp.Err = errors.New("faktúra nemá sumu")
				}
			}
			m.Purchases = append(m.Purchases, mp)
			continue
		}
		orig := firstToken(h.Col(omega.EOriginal))
		mi := migInvoice{Number: num, Ref: fmt.Sprintf("riadok %d", d.Line), Credit: total.Sign() < 0, Original: orig, Reason: text,
			Total: total, PaidAll: full, Err: err}
		if full {
			mi.PaidDate = paidOn
		}
		vs := h.Col(omega.EExternal)
		if strings.Trim(vs, "0123456789") != "" {
			vs = ""
		}
		mi.Inv = invoice.Invoice{Customer: p, IssueDate: issue, DeliveryDate: delivery, DueDate: odate(h.Col(omega.EDue)), VariableSymbol: vs,
			PriceMode: vat.PriceNet, Note: h.Col(omega.ENote), Subject: migSubject(text)}
		mi.Summary = summaryLines(rows, text)
		m.Invoices = append(m.Invoices, mi)
	}
	return m
}

func firstDate(ds ...invoice.Date) invoice.Date {
	for _, d := range ds {
		if !d.IsZero() {
			return d
		}
	}
	return invoice.Date{}
}

// omegaLines zostaví riadky faktúry z položiek; pri zmiešaných cenách
// (s DPH aj bez) alebo neznámej sadzbe vráti nič (použije sa rekapitulácia).
func omegaLines(items []omega.Rec, low, high vat.Rate) ([]invoice.Line, vat.PriceMode) {
	var lines []invoice.Line
	modes := map[bool]bool{}
	for _, it := range items {
		name := it.Col(omega.FItemName)
		q := odec(it.Col(omega.FItemQty))
		net := odec(it.Col(omega.FItemPrice))
		if name == "" && q.IsZero() {
			continue
		}
		if q.IsZero() && net.IsZero() {
			continue // textový riadok
		}
		if q.IsZero() {
			q = money.FromInt(1)
		}
		rate, ok := map[string]vat.Rate{"0": vat.Rate0, "N": low, "V": high}[strings.ToUpper(it.Col(omega.FItemRate))]
		if !ok {
			return nil, vat.PriceNet
		}
		gross := it.Col(omega.FItemGross) == "-1" && it.Col(omega.FItemPriceV) != ""
		modes[gross] = true
		price := net
		if gross {
			price = odec(it.Col(omega.FItemPriceV))
		}
		if disc := odec(it.Col(omega.FItemDisc)); !disc.IsZero() {
			price = price.MulRatio(money.FromInt(100).Sub(disc), money.FromInt(100))
		}
		lines = append(lines, invoice.Line{Description: firstNonBlank(name, "Položka"), Quantity: q, Unit: firstNonBlank(it.Col(omega.FItemUnit), "ks"),
			UnitPrice: price, Rate: rate})
	}
	if len(modes) > 1 {
		return nil, vat.PriceNet
	}
	if modes[true] {
		return lines, vat.PriceGross
	}
	return lines, vat.PriceNet
}

// firstToken vráti prvé číslo zo zoznamu „FV1, FV2“.
func firstToken(s string) string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' })
	if len(f) == 0 {
		return ""
	}
	return f[0]
}
