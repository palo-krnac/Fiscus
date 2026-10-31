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

// Prenos z MRP-K/S: vydané faktúry (FAKVY.XML) s položkami (FAKVYPOL.XML)
// a adresár (FV_ADRES.XML), najlepšie spolu v ZIP. Tabuľka faktúr uvádza
// odberateľa iba IČO — meno a adresu doplní adresár.

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
	if len(d.Invoices)+len(d.Addresses) == 0 {
		return empty, mfaFieldErr("file", "V súbore nie sú faktúry (FAKVY.XML) ani adresy (FV_ADRES.XML) z MRP-K/S")
	}
	return a.migrate(ctx, companyID, mrpMigration(d), opt, apply)
}

func mrpParty(r mrp.Row) tabimport.Party {
	return tabimport.Party{Name: firstNonBlank(r.Get("firma"), r.Get("meno")), ICO: strings.ReplaceAll(r.Get("ico"), " ", ""),
		Street: r.Get("ulica"), City: r.Get("mesto"), Zip: strings.ReplaceAll(r.Get("psc"), " ", ""), Country: r.Get("stat"),
		Email: r.Get("email"), Phone: r.Get("telefon"), Note: r.Get("poznamka")}
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
	byICO := map[string]invoice.Party{}
	for _, r := range d.Addresses {
		tp := mrpParty(r)
		tp.DIC, tp.ICDPH = mrpTaxIDs(r.Get("dic"), r.Get("icdph"), r.Get("vatnumber"))
		p, warn := tableParty(tp)
		if warn != "" {
			m.Warnings = append(m.Warnings, "Adresár: "+warn)
		}
		m.Contacts = append(m.Contacts, migContact{Party: p, Phone: tp.Phone, Note: tp.Note})
		if p.ICO != "" {
			byICO[p.ICO] = p
		}
	}
	items := map[string][]mrp.Row{}
	for _, it := range d.Items {
		items[it.Get("idfak")] = append(items[it.Get("idfak")], it)
	}
	for _, r := range d.Invoices {
		num := r.Get("cislo")
		druh := strings.ToUpper(r.Get("druh"))
		credit := druh == "D" || r.Get("dobropis_pro") != ""
		switch {
		case druh == "" || druh == "F" || credit:
		case druh == "Z" || druh == "P" || druh == "L":
			m.Skips = append(m.Skips, migSkip{Agenda: agInvoices, Text: fmt.Sprintf("Zálohová faktúra %s sa neprenáša (zálohy zadajte vo Fiškuse)", num)})
			continue
		default:
			m.Skips = append(m.Skips, migSkip{Agenda: agInvoices, Text: fmt.Sprintf("Doklad %s (druh %s) sa neprenáša", num, druh)})
			continue
		}
		issue := odate(r.Get("datvystave"))
		delivery := firstDate(odate(r.Get("datzdanpln")), odate(r.Get("datdodani")), issue)
		rows := mrpRates(r, delivery)
		total := omegaTotal("", rows)
		mi := migInvoice{Number: num, Ref: "faktúra " + r.Get("idfak"), Credit: credit, Original: r.Get("dobropis_pro"),
			Reason: r.Get("poznamka"), Total: total}
		ico := strings.ReplaceAll(r.Get("ico"), " ", "")
		cust, ok := byICO[ico]
		switch {
		case foreignCurrency(r.Get("mena")):
			mi.Err = fmt.Errorf("faktúra v mene %s sa zatiaľ neprenáša — zadajte ju ručne", r.Get("mena"))
		case ico == "":
			mi.Err = errors.New("faktúra bez IČO odberateľa — MRP v tabuľke faktúr meno neuvádza, zadajte ju ručne")
		case !ok:
			mi.Err = fmt.Errorf("odberateľ s IČO %s nie je v adresári — nahrajte aj FV_ADRES.XML", ico)
		}
		if mi.Err == nil {
			dic, icdph := mrpTaxIDs(r.Get("vatnumber"))
			if icdph != "" {
				cust.ICDPH = icdph
				if cust.DIC == "" {
					cust.DIC = dic
				}
			}
			mi.Inv = invoice.Invoice{Customer: cust, IssueDate: issue, DeliveryDate: delivery, DueDate: odate(r.Get("datsplatno")),
				VariableSymbol: r.Get("varsymb"), PriceMode: vat.PriceNet, Note: r.Get("poznamka")}
			mi.Inv.Lines, mi.Inv.PriceMode = mrpLines(items[r.Get("idfak")], r.Get("cenysdph") == "T")
			mi.Summary = summaryLines(rows, "")
		}
		m.Invoices = append(m.Invoices, mi)
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
		text := it.Get("text")
		q := odec(it.Get("pocetmj"))
		price := odec(it.Get("cenamj"))
		if text == "" && price.IsZero() {
			continue
		}
		if q.IsZero() {
			if price.IsZero() {
				continue // textový riadok
			}
			q = money.FromInt(1)
		}
		rate, ok := rateFromPercent(it.Get("sadzbadph"))
		if !ok {
			return nil, vat.PriceNet
		}
		if disc := odec(it.Get("zlava")); !disc.IsZero() {
			price = price.MulRatio(money.FromInt(100).Sub(disc), money.FromInt(100))
		}
		out = append(out, invoice.Line{Description: firstNonBlank(text, "Položka"), Quantity: q, Unit: firstNonBlank(it.Get("mj"), "ks"),
			UnitPrice: price, Rate: rate})
	}
	return out, mode
}
