package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/mrp"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/tabimport"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Prenos z MRP-K/S podľa dokumentácie importu faktúr (MRP-K/S 5.53):
// vydané faktúry FAKVY s položkami FAKVYPOL a úhradami FAKVYUHR, prijaté
// FAKPR, FAKPRPOL a FAKPRUHR, adresy FV_ADRES/FP_ADRES a ich bankové účty
// *_ADRESBAN, najlepšie spolu v ZIP. Faktúra uvádza partnera iba IČO
// (pri drobných odberateľoch vlastné číslo, napr. X0001) — meno, adresu
// a IČ DPH doplní adresár.

// ImportMRP prenesie dáta z XML tabuliek MRP (apply=false: iba náhľad).
func (a *App) ImportMRP(ctx context.Context, companyID int64, data []byte, opt MigrateOptions, apply bool) (ImportReport, error) {
	if err := a.requireMigrate(ctx, companyID); err != nil {
		return ImportReport{}, err
	}
	empty := ImportReport{Source: "MRP", Warnings: []string{}, Other: map[string]int{}}
	d, err := mrp.Parse(data, "", MaxMigrateBytes)
	if err != nil {
		return empty, mfaFieldErr("file", err.Error())
	}
	if len(d.Invoices)+len(d.Purchases)+len(d.Addresses) == 0 {
		return empty, mfaFieldErr("file", "V súbore nie sú faktúry (FAKVY.XML, FAKPR.XML) ani adresy (FV_ADRES.XML) z MRP-K/S")
	}
	return a.migrate(ctx, companyID, mrpMigration(d), opt, apply)
}

// mrpICO: skutočné IČO má 6 až 8 číslic; vlastné čísla drobných
// odberateľov (X0001) sa do kontaktu nezapisujú.
func mrpICO(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if len(s) < 6 || len(s) > 8 || strings.Trim(s, "0123456789") != "" {
		return ""
	}
	return s
}

func mrpParty(r mrp.Row) tabimport.Party {
	return tabimport.Party{Name: firstNonBlank(r.Get("firma"), r.Get("meno")), ICO: mrpICO(r.Get("ico")),
		Street: r.Get("ulica"), City: r.Get("mesto"), Zip: strings.ReplaceAll(r.Get("psc"), " ", ""), Country: firstNonBlank(r.Get("kodstat"), r.Get("stat")),
		Email: firstNonBlank(r.Get("fakemail"), r.Get("email")), Phone: r.Get("telefon"), Note: r.Get("poznamka")}
}

// mrpDoc je faktúra vydaná alebo prijatá s položkami a úhradami.
type mrpDoc struct {
	head     mrp.Row
	items    []mrp.Row
	payments []mrp.Row
}

func mrpGroup(heads, items, pays []mrp.Row) []mrpDoc {
	byID := map[string][]mrp.Row{}
	for _, it := range items {
		byID[it.Get("idfak")] = append(byID[it.Get("idfak")], it)
	}
	payID := map[string][]mrp.Row{}
	payNum := map[string][]mrp.Row{}
	for _, p := range pays {
		if n := p.Get("cislofak"); n != "" {
			payNum[n] = append(payNum[n], p)
		} else {
			payID[p.Get("idfak")] = append(payID[p.Get("idfak")], p)
		}
	}
	out := make([]mrpDoc, 0, len(heads))
	for _, h := range heads {
		ps := append(payID[h.Get("idfak")], payNum[h.Get("cislo")]...)
		out = append(out, mrpDoc{head: h, items: byID[h.Get("idfak")], payments: ps})
	}
	return out
}

// mrpPaid: súčet úhrad v domácej mene a dátum poslednej.
func mrpPaid(pays []mrp.Row) (money.Dec, invoice.Date) {
	sum, last := money.Zero, invoice.Date{}
	for _, p := range pays {
		sum = sum.Add(odec(p.Get("ciastka")))
		if d := odate(p.Get("datum")); d.After(last) {
			last = d
		}
	}
	return sum, last
}

// mrpKind: druh faktúry a typ dokladu podľa dokumentácie. skip je dôvod,
// prečo sa doklad neprenáša.
func mrpKind(h mrp.Row) (credit bool, skip string) {
	switch strings.ToUpper(h.Get("druh")) {
	case "X":
		return false, "predfaktúra (zálohy zadajte vo Fiškuse)"
	case "P":
		return false, "penalizačná faktúra (zadajte ju vo Fiškuse)"
	}
	switch strings.ToUpper(h.Get("typ_dokl")) {
	case "", "F":
		return h.Get("dobropis_pro") != "", ""
	case "D":
		return true, ""
	case "V":
		return false, "vrubopis (zadajte ho vo Fiškuse ako opravnú faktúru)"
	default:
		return false, "doklad typu " + h.Get("typ_dokl")
	}
}

// mrpTaxIDs rozdelí DIČ a IČ DPH (IČ DPH začína kódom štátu).
func mrpTaxIDs(vals ...string) (dic, icdph string) {
	for _, v := range vals {
		v = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v), " ", ""))
		switch {
		case v == "":
		case len(v) > 2 && v[0] >= 'A' && v[0] <= 'Z' && v[1] >= 'A' && v[1] <= 'Z':
			if icdph == "" {
				icdph = v
			}
		default:
			if dic == "" {
				dic = v
			}
		}
	}
	if dic == "" && strings.HasPrefix(icdph, "SK") {
		dic = icdph[2:]
	}
	return dic, icdph
}

// mrpRates: základy a DPH faktúry; sadzba z pomeru dane a základu, inak
// podľa hladiny (1 znížená, 2 základná, 3 druhá znížená) a dátumu.
func mrpRates(r mrp.Row, d invoice.Date) []purchase.RateRow {
	low, high := omegaLevels("", "", d)
	var out []purchase.RateRow
	if b := odec(r.Get("zakl0")).Add(odec(r.Get("mimodph"))); !b.IsZero() {
		out = append(out, purchase.RateRow{Rate: vat.Rate0, Base: b})
	}
	for _, lv := range []struct {
		base, tax string
		def       vat.Rate
	}{{"zakl1", "dph1", low}, {"zakl2", "dph2", high}, {"zakl3", "dph3", vat.Rate5}} {
		b := odec(r.Get(lv.base))
		if b.IsZero() {
			continue
		}
		t := odec(r.Get(lv.tax))
		rate := lv.def
		if g, ok := tabimport.GuessRate(b, t); ok && g != vat.Rate0 {
			rate = g
		}
		out = append(out, purchase.RateRow{Rate: rate, Base: b, VAT: t})
	}
	return out
}

func mrpMigration(d mrp.Data) migration {
	m := migration{Source: "MRP", Other: map[string]int{}}
	for _, u := range d.Unknown {
		m.Other["iné tabuľky ("+u+")"]++
	}
	ibans := map[string]string{}
	for _, b := range d.Banks {
		if v := strings.ReplaceAll(b.Get("iban"), " ", ""); v != "" && ibans[b.Get("idradr")] == "" {
			ibans[b.Get("idradr")] = v
		}
	}
	byICO := map[string]invoice.Party{}
	for _, r := range d.Addresses {
		tp := mrpParty(r)
		tp.DIC, tp.ICDPH = mrpTaxIDs(r.Get("ic_dph"), r.Get("dic"), r.Get("vatnumber"))
		p, warn := tableParty(tp)
		if warn != "" {
			m.Warnings = append(m.Warnings, "Adresár: "+warn)
		}
		note := contactNote(p.Name, r.Get("meno"), tp.Note)
		m.Contacts = append(m.Contacts, migContact{Party: p, Phone: tp.Phone, IBAN: ibans[r.Get("idradr")], Note: note})
		if k := strings.ReplaceAll(r.Get("ico"), " ", ""); k != "" {
			byICO[k] = p
		}
	}
	partner := func(h mrp.Row) (invoice.Party, error) {
		ico := strings.ReplaceAll(h.Get("ico"), " ", "")
		p, ok := byICO[ico]
		switch {
		case ico == "":
			return p, errors.New("chýba IČO partnera")
		case !ok:
			return p, fmt.Errorf("partner s IČO %s nie je v adresári — nahrajte aj FV_ADRES.XML / FP_ADRES.XML", ico)
		}
		if dic, icdph := mrpTaxIDs(h.Get("vatnumber")); icdph != "" && p.ICDPH == "" {
			p.ICDPH = icdph
			if p.DIC == "" {
				p.DIC = dic
			}
		}
		return p, nil
	}

	for _, doc := range mrpGroup(d.Invoices, d.Items, d.Payments) {
		r := doc.head
		num := r.Get("cislo")
		credit, skip := mrpKind(r)
		if skip != "" {
			m.Skips = append(m.Skips, migSkip{Agenda: agInvoices, Text: fmt.Sprintf("Faktúra %s sa neprenáša: %s", num, skip)})
			continue
		}
		issue := odate(r.Get("datvystave"))
		delivery := firstDate(odate(r.Get("datzdanpln")), odate(r.Get("datdodani")), issue)
		rows := mrpRates(r, delivery)
		paid, paidOn := mrpPaid(doc.payments)
		mi := migInvoice{Number: num, Ref: "faktúra " + r.Get("idfak"), Credit: credit, Original: firstNonBlank(r.Get("origcis2"), r.Get("dobropis_pro")),
			Reason: r.Get("poznamka"), Total: omegaTotal("", rows), Paid: paid, PaidDate: paidOn}
		cust, err := partner(r)
		switch {
		case foreignCurrency(r.Get("mena")):
			mi.Err = fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", r.Get("mena"))
		case err != nil:
			mi.Err = err
		}
		if mi.Err == nil {
			mi.Inv = invoice.Invoice{Customer: cust, IssueDate: issue, DeliveryDate: delivery, DueDate: odate(r.Get("datsplatno")),
				VariableSymbol: r.Get("varsymb"), PriceMode: vat.PriceNet, Note: r.Get("poznamka")}
			if odec(r.Get("zlava")).IsZero() { // zľava za celý doklad → rekapitulácia
				mi.Inv.Lines, mi.Inv.PriceMode = mrpLines(doc.items, r.Get("cenysdph") == "T")
			}
			mi.Summary = summaryLines(rows, "")
		}
		m.Invoices = append(m.Invoices, mi)
	}

	for _, doc := range mrpGroup(d.Purchases, d.PurchaseItems, d.PurchasePayments) {
		r := doc.head
		num := r.Get("cislo")
		ref := firstNonBlank(r.Get("origcisdok"), num)
		_, skip := mrpKind(r)
		if skip != "" {
			m.Skips = append(m.Skips, migSkip{Agenda: agPurchases, Text: fmt.Sprintf("Prijatá faktúra %s sa neprenáša: %s", ref, skip)})
			continue
		}
		issue := odate(r.Get("datvystave"))
		delivery := firstDate(odate(r.Get("datdodani")), odate(r.Get("datdanpov")), issue)
		rows := mrpRates(r, delivery)
		total := omegaTotal("", rows)
		paid, paidOn := mrpPaid(doc.payments)
		mp := migPurchase{Ref: ref, Paid: !total.IsZero() && absDec(paid).Cmp(absDec(total)) >= 0}
		if mp.Paid {
			mp.PaidDate = paidOn
		}
		sup, err := partner(r)
		switch {
		case foreignCurrency(r.Get("mena")):
			mp.Err = fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", r.Get("mena"))
		case err != nil:
			mp.Err = err
		case issue.IsZero():
			mp.Err = errors.New("chýba dátum vystavenia")
		case len(rows) == 0:
			mp.Err = errors.New("faktúra nemá sumu")
		}
		if mp.Err == nil {
			mp.P = purchase.Purchase{SupplierNumber: ref, VariableSymbol: r.Get("varsymb"), Supplier: sup, IssueDate: issue, DeliveryDate: delivery,
				ReceivedDate: firstDate(odate(r.Get("datzdanpln")), issue), DueDate: firstDate(odate(r.Get("datsplatno")), issue),
				Description: firstNonBlank(r.Get("poznamka"), "Prenos z MRP"), Expense: purchase.ExpenseServices, Rates: rows,
				CorrectsNumber: r.Get("origcis2")}
		}
		m.Purchases = append(m.Purchases, mp)
	}
	return m
}

// mrpLines zostaví riadky z položiek faktúry (sadzba v percentách).
func mrpLines(rows []mrp.Row, gross bool) ([]invoice.Line, vat.PriceMode) {
	mode := vat.PriceNet
	if gross {
		mode = vat.PriceGross
	}
	var out []invoice.Line
	for _, it := range rows {
		if it.Get("typ_radku") == "2" {
			continue // textový riadok
		}
		if it.Get("typ_sum") == "2" {
			return nil, vat.PriceNet // odpočet zálohy → rekapitulácia
		}
		text := it.Get("text")
		q := odec(it.Get("pocetmj"))
		price := odec(it.Get("cenamj")).Sub(odec(it.Get("slevamj")))
		if text == "" && price.IsZero() {
			continue
		}
		if q.IsZero() {
			if price.IsZero() {
				continue
			}
			q = money.FromInt(1)
		}
		rate, ok := rateFromPercent(it.Get("sadzbadph"))
		if !ok {
			return nil, vat.PriceNet
		}
		out = append(out, invoice.Line{Description: firstNonBlank(text, "Položka"), Quantity: q, Unit: firstNonBlank(it.Get("mj"), "ks"),
			UnitPrice: price, Rate: rate})
	}
	return out, mode
}
