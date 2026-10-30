// Package demo naplní úložisko ukážkovými dátami (vymyslené firmy).
package demo

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/asset"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// ico doplní k 7 číslicam platnú kontrolnú číslicu.
func ico(seven string) string {
	sum := 0
	for i := 0; i < 7; i++ {
		sum += int(seven[i]-'0') * (8 - i)
	}
	return fmt.Sprintf("%s%d", seven, (11-sum%11)%10)
}

// icdph vráti najbližšie platné slovenské IČ DPH (10 číslic deliteľných 11) od n.
func icdph(n int) string {
	for n%11 != 0 {
		n++
	}
	return fmt.Sprintf("SK%d", n)
}

type customer struct{ name, city, ico, icdph string }

// dic: platiteľ má DIČ v IČ DPH, ostatným sa zostaví z IČO (ukážkové údaje).
func (c customer) dic() string {
	if c.icdph != "" {
		return strings.TrimPrefix(c.icdph, "SK")
	}
	return "10" + c.ico
}

var customers = []customer{
	{"Elektro Hron s.r.o.", "Banská Bystrica", ico("4612345"), icdph(2120461234)},
	{"Stavmont Zvolen, a.s.", "Zvolen", ico("3687412"), icdph(2120368741)},
	{"Agro Liptov družstvo", "Liptovský Mikuláš", ico("0061234"), ""},
	{"TechnoPack Slovakia s.r.o.", "Žilina", ico("5098765"), icdph(2120509876)},
	{"Mlyn Brezno s.r.o.", "Brezno", ico("4477120"), ""},
	{"Ing. Peter Kováč – PK Automation", "Banská Bystrica", ico("5233341"), ""},
	{"Automotive Parts Nitra s.r.o.", "Nitra", ico("4790011"), ""},
}

type item struct {
	desc, unit, qty, price string
	rate                   vat.Rate
}

var items = []item{
	{"Programovanie PLC Siemens S7-1500", "hod", "16", "48", vat.Rate23},
	{"Servisný výjazd", "km", "124", "0.45", vat.Rate23},
	{"Úprava vizualizácie HMI", "hod", "6", "42", vat.Rate23},
	{"Rozvádzač RM1 – montáž a zapojenie", "ks", "1", "1850", vat.Rate23},
	{"Kábel CYKY-J 5x2,5", "m", "85", "2.19", vat.Rate23},
	{"Odborná literatúra – normy STN", "ks", "2", "34.90", vat.Rate5},
	{"Revízia elektrickej inštalácie", "ks", "1", "290", vat.Rate23},
	{"Frekvenčný menič 4 kW", "ks", "2", "412.50", vat.Rate23},
}

// subjects sú predmety ukážkových faktúr; prázdny = faktúra bez predmetu.
var subjects = []string{
	"Modernizácia riadenia baliacej linky",
	"Servisný zásah – porucha frekvenčného meniča",
	"",
	"Rozšírenie rozvádzača RM1 v hale B",
	"Revízia elektroinštalácie skladu",
	"Úprava vizualizácie dispečingu",
	"",
	"Oprava osvetlenia výrobnej haly",
}

// Seed vytvorí firmu a 40 faktúr v rôznych stavoch.
func Seed(ctx context.Context, a *app.App) (store.Company, error) {
	c, err := a.CreateCompany(ctx, store.Company{
		Name: "Demo elektroservis", Street: "Námestie SNP 1", City: "Banská Bystrica", Zip: "974 01",
		ICO: ico("5512345"), DIC: "2120000003", ICDPH: "SK2120000003", IBAN: "SK31 1200 0000 1987 4263 7541", VATPayer: true,
		TaxOffice: "Daňový úrad Banská Bystrica", VATPeriod: "month",
	})
	if err != nil {
		return c, err
	}
	// adresár
	contactIDs := make([]int64, len(customers))
	for i, cu := range customers {
		due := 0
		if i%3 == 0 {
			due = 30
		}
		ct, err := a.SaveContact(ctx, c.ID, store.Contact{Name: cu.name, City: cu.city, ICO: cu.ico, DIC: cu.dic(), ICDPH: cu.icdph, Country: "SK", DueDays: due})
		if err != nil {
			return c, err
		}
		contactIDs[i] = ct.ID
	}

	// cenník z tých istých položiek, ktoré sú na ukážkových faktúrach
	for i, it := range items {
		if _, err := a.SaveItem(ctx, c.ID, catalog.Item{Code: fmt.Sprintf("P%02d", i+1), Name: it.desc, Unit: it.unit,
			Price: m.MustParse(it.price), Rate: it.rate}); err != nil {
			return c, fmt.Errorf("demo cenník: %w", err)
		}
	}

	today := a.Today()
	for i := 39; i >= 0; i-- { // od najstaršej, aby čísla rástli s dátumom
		ci := (i * 5) % len(customers)
		cu := customers[ci]
		issue := today.AddDays(-i*4 - 1)
		inv := invoice.Invoice{
			Customer:     invoice.Party{Name: cu.name, City: cu.city, Country: "SK", ICO: cu.ico, DIC: cu.dic(), ICDPH: cu.icdph},
			ContactID:    contactIDs[ci],
			Subject:      subjects[i%len(subjects)],
			IssueDate:    issue,
			DeliveryDate: issue,
			DueDate:      issue.AddDays(14),
		}
		for j := 0; j <= i%3; j++ {
			it := items[(i+j*3)%len(items)]
			inv.Lines = append(inv.Lines, invoice.Line{Description: it.desc, Unit: it.unit,
				Quantity: m.MustParse(it.qty), UnitPrice: m.MustParse(it.price), Rate: it.rate})
		}
		d, err := a.SaveDraft(ctx, c.ID, inv)
		if err != nil {
			return c, err
		}
		if i < 2 {
			continue // posledné dve ostanú ako koncepty
		}
		if _, err := a.Issue(ctx, c.ID, d.ID); err != nil {
			return c, err
		}
		switch {
		case i == 9:
			_, err = a.Cancel(ctx, c.ID, d.ID)
		case i%4 != 3 && i > 5: // niektoré staršie ostanú neuhradené → po splatnosti
			_, err = a.MarkPaid(ctx, c.ID, d.ID, issue.AddDays(7+i%6))
		}
		if err != nil {
			return c, err
		}
	}
	// zahraniční odberatelia (vymyslené firmy): tovar do Nemecka, služba do Rakúska
	foreign := []struct {
		name, city, country, icdph, desc string
		kind                             invoice.SupplyKind
		qty, price                       string
		day                              int
	}{
		{"Brenner Steuerungstechnik GmbH", "Regensburg", "DE", "DE299871234", "Rozvádzač RM-12 podľa výkresu", invoice.SupplyGoods, "2", "1450.00", 6},
		{"Alpenwerk Automation GmbH", "Linz", "AT", "ATU71234567", "Programovanie PLC – oživenie linky", invoice.SupplyServices, "16", "55.00", 9},
	}
	for _, f := range foreign {
		day := today.AddDays(-f.day)
		d, err := a.SaveDraft(ctx, c.ID, invoice.Invoice{
			Customer:  invoice.Party{Name: f.name, City: f.city, Country: f.country, ICDPH: f.icdph},
			IssueDate: day, DeliveryDate: day, DueDate: day.AddDays(30),
			ReverseCharge: true, SupplyKind: f.kind,
			Lines: []invoice.Line{{Description: f.desc, Unit: "ks", Quantity: m.MustParse(f.qty), UnitPrice: m.MustParse(f.price), Rate: vat.Rate23}},
		})
		if err != nil {
			return c, fmt.Errorf("demo zahraničná faktúra: %w", err)
		}
		if _, err := a.Issue(ctx, c.ID, d.ID); err != nil {
			return c, err
		}
	}

	// prijaté faktúry (druh výdavku pre peňažný denník podľa predmetu)
	suppliers := []struct {
		name, ico, icdph, what string
		base, iban             string
		rate                   vat.Rate
		day                    int
	}{
		{"Čerpacia stanica Hron s.r.o.", ico("4855101"), icdph(2120455101), "Pohonné hmoty", "81.30", "", vat.Rate23, 3},
		{"TeleNet Slovensko s.r.o.", ico("4855202"), icdph(2120455202), "Mobilné služby", "40.65", "SK66 0900 0000 0050 1234 5678", vat.Rate23, 8},
		{"Elektro veľkoobchod BB s.r.o.", ico("4855303"), icdph(2120455303), "Elektroinštalačný materiál", "612.40", "SK29 1100 0000 0029 3048 1726", vat.Rate23, 12},
		{"Prenájom priestorov Ján Malý", ico("4855404"), "", "Nájom kancelárie (neplatiteľ DPH)", "350.00", "SK21 0200 0000 0044 1029 8375", vat.Rate0, 20},
		{"Energie Stred s.r.o.", ico("4855505"), icdph(2120455505), "Elektrina", "54.10", "SK35 7500 0000 0010 2938 4756", vat.Rate23, 33},
		{"Kníhkupectvo pri fontáne s.r.o.", ico("4855606"), icdph(2120455606), "Odborná literatúra", "38.00", "SK76 5600 0000 0033 8475 6102", vat.Rate5, 45},
	}
	for i, sp := range suppliers {
		received := today.AddDays(-sp.day)
		pu := purchase.Purchase{
			SupplierNumber: fmt.Sprintf("%d%05d", received.Year(), 1200+i*37),
			Supplier:       invoice.Party{Name: sp.name, ICO: sp.ico, ICDPH: sp.icdph, Country: "SK"},
			SupplierIBAN:   sp.iban,
			IssueDate:      received.AddDays(-2),
			ReceivedDate:   received,
			DueDate:        received.AddDays(12),
			Description:    sp.what,
			Expense:        demoExpense(sp.what),
			Rates:          []purchase.RateRow{{Rate: sp.rate, Base: m.MustParse(sp.base)}},
		}
		saved, _, err := a.SavePurchase(ctx, c.ID, pu)
		if err != nil {
			return c, fmt.Errorf("demo nákup %s: %w", sp.name, err)
		}
		if sp.day > 15 {
			if _, err := a.MarkPurchasePaid(ctx, c.ID, saved.ID, received.AddDays(5)); err != nil {
				return c, err
			}
		}
	}
	for _, d := range []cash.Document{
		{Kind: cash.In, Date: today.AddDays(-25), Amount: m.MustParse("500.00"), Partner: "Majiteľ", Purpose: "Vklad hotovosti do pokladne"},
		{Kind: cash.Out, Date: today.AddDays(-18), Amount: m.MustParse("23.40"), Partner: "Papiernictvo Centrum", Purpose: "Kancelárske potreby"},
		{Kind: cash.In, Date: today.AddDays(-12), Amount: m.MustParse("85.00"), Partner: "Ján Novák", Purpose: "Drobná oprava – hotovosť"},
		{Kind: cash.Out, Date: today.AddDays(-4), Amount: m.MustParse("12.90"), Partner: "Slovenská pošta", Purpose: "Poštovné"},
	} {
		if _, err := a.CreateCash(ctx, c.ID, d); err != nil {
			return c, fmt.Errorf("demo pokladňa: %w", err)
		}
	}
	if err := seedBank(ctx, a, c); err != nil {
		return c, fmt.Errorf("demo výpis: %w", err)
	}
	if err := seedAssets(ctx, a, c); err != nil {
		return c, fmt.Errorf("demo majetok: %w", err)
	}
	if err := seedQuotes(ctx, a, c, contactIDs); err != nil {
		return c, fmt.Errorf("demo ponuky: %w", err)
	}
	if err := seedAdvances(ctx, a, c, contactIDs); err != nil {
		return c, fmt.Errorf("demo zálohy: %w", err)
	}
	if err := seedDomesticRC(ctx, a, c); err != nil {
		return c, fmt.Errorf("demo prenesenie: %w", err)
	}
	if err := seedForeign(ctx, a, c); err != nil {
		return c, fmt.Errorf("demo cudzia mena: %w", err)
	}
	if err := seedRecurring(ctx, a, c, contactIDs); err != nil {
		return c, fmt.Errorf("demo pravidelná fakturácia: %w", err)
	}
	// upomienka najväčšiemu dlžníkovi (ostatní čakajú na prvú upomienku)
	if groups, err := a.Overdue(ctx, c.ID); err == nil && len(groups) > 0 {
		ids := []int64{}
		for _, inv := range groups[0].Invoices {
			ids = append(ids, inv.ID)
		}
		if _, err := a.CreateReminder(ctx, c.ID, app.ReminderRequest{InvoiceIDs: ids,
			Options: reminder.Options{Date: a.Today().AddDays(-3), PayBy: a.Today().AddDays(4), ChargeFee: true}}); err != nil {
			return c, fmt.Errorf("demo upomienka: %w", err)
		}
	}
	return c, nil
}

// seedAdvances pripraví zálohovú faktúru uhradenú (s dokladom k prijatej
// platbe a konceptom vyúčtovania) a jednu čakajúcu na úhradu.
func seedAdvances(ctx context.Context, a *app.App, c store.Company, contactIDs []int64) error {
	today := a.Today()
	mk := func(ci int, desc, price string, day int) (invoice.Invoice, error) {
		cu := customers[ci]
		issue := today.AddDays(-day)
		d, err := a.SaveDraft(ctx, c.ID, invoice.Invoice{Kind: invoice.KindProforma,
			Customer:  invoice.Party{Name: cu.name, City: cu.city, Country: "SK", ICO: cu.ico, DIC: cu.dic(), ICDPH: cu.icdph},
			ContactID: contactIDs[ci], IssueDate: issue, DeliveryDate: issue, DueDate: issue.AddDays(7),
			Lines: []invoice.Line{{Description: desc, Unit: "ks", Quantity: m.MustParse("1"), UnitPrice: m.MustParse(price), Rate: vat.Rate23}}})
		if err != nil {
			return d, err
		}
		return a.Issue(ctx, c.ID, d.ID)
	}
	p, err := mk(5, "Záloha 50 % – rozvádzač RM2 podľa ponuky", "2400", 12)
	if err != nil {
		return err
	}
	if p, err = a.MarkPaid(ctx, c.ID, p.ID, today.AddDays(-6)); err != nil {
		return err
	}
	if _, err := a.SettleAdvance(ctx, c.ID, p.ID); err != nil {
		return err
	}
	_, err = mk(6, "Záloha na materiál – kabeláž a prístroje", "850", 2)
	return err
}

// seedForeign pripraví kurzy ECB za posledných 120 dní (demo beží bez
// internetu), faktúru v korunách českému odberateľovi a prijatú faktúru v dolároch.
func seedForeign(ctx context.Context, a *app.App, c store.Company) error {
	today := a.Today()
	var rates []fx.Rate
	for i := 120; i >= 1; i-- {
		d := today.AddDays(-i)
		if wd := time.Date(d.Year(), time.Month(d.Month()), d.Day(), 0, 0, 0, 0, time.UTC).Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		// mierne kolísanie okolo 25,1 Kč a 1,08 $
		czk := m.MustParse(fmt.Sprintf("25.%03d", 80+(i*37)%90))
		usd := m.MustParse(fmt.Sprintf("1.%04d", 750+(i*53)%160))
		rates = append(rates,
			fx.Rate{Currency: "CZK", Date: d.String(), Rate: czk, Source: fx.SourceECB},
			fx.Rate{Currency: "USD", Date: d.String(), Rate: usd, Source: fx.SourceECB})
	}
	if err := a.Store.SaveRates(ctx, rates); err != nil {
		return err
	}
	issue := today.AddDays(-4)
	d, err := a.SaveDraft(ctx, c.ID, invoice.Invoice{
		Customer: invoice.Party{Name: "Bohemia Control s.r.o.", Street: "Purkyňova 99", City: "Brno", Zip: "612 00", Country: "CZ", ICDPH: "CZ27654321"},
		Currency: "CZK", SupplyKind: invoice.SupplyServices, ReverseCharge: true,
		IssueDate: issue, DeliveryDate: issue, DueDate: issue.AddDays(14),
		Lines: []invoice.Line{{Description: "Programovanie riadiaceho systému linky – október", Unit: "hod", Quantity: m.MustParse("40"),
			UnitPrice: m.MustParse("1200"), Rate: vat.Rate23}},
	})
	if err != nil {
		return err
	}
	if _, err = a.Issue(ctx, c.ID, d.ID); err != nil {
		return err
	}
	// prijatá faktúra v dolároch: cloudové služby od írskeho dodávateľa (samozdanenie)
	got := today.AddDays(-6)
	_, _, err = a.SavePurchase(ctx, c.ID, purchase.Purchase{
		SupplierNumber: "INV-2026-10-4471", Supplier: invoice.Party{Name: "Northwind Cloud Ltd.", City: "Dublin", Country: "IE", ICDPH: "IE9876543W"},
		Currency: "USD", SupplyKind: invoice.SupplyServices, ReverseCharge: true,
		IssueDate: got.AddDays(-1), DeliveryDate: got.AddDays(-1), ReceivedDate: got, DueDate: got.AddDays(20),
		Description: "Cloudový server a zálohovanie – október",
		Rates:       []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("149.00")}},
	})
	return err
}

// seedRecurring pripraví pravidelné faktúry: mesačný paušál (dnes sa vystaví),
// štvrťročný prenájom (koncepty na kontrolu) a pozastavenú ročnú licenciu.
func seedRecurring(ctx context.Context, a *app.App, c store.Company, contactIDs []int64) error {
	today := a.Today()
	line := func(desc, unit, price string) []invoice.Line {
		return []invoice.Line{{Description: desc, Unit: unit, Quantity: m.MustParse("1"), UnitPrice: m.MustParse(price), Rate: vat.Rate23}}
	}
	party := func(ci int) invoice.Party {
		cu := customers[ci]
		return invoice.Party{Name: cu.name, City: cu.city, Country: "SK", ICO: cu.ico, DIC: cu.dic(), ICDPH: cu.icdph}
	}
	plan := []recurring.Template{
		{Name: "Servisný paušál – údržba PLC", Active: true, Interval: recurring.Monthly, NextDate: today, AutoIssue: true,
			Customer: party(0), ContactID: contactIDs[0], Subject: "Servis PLC – {obdobie}", Note: "Paušál za servis a vzdialenú podporu za {obdobie}.",
			Lines: line("Servisný paušál {mesiac} {rok}", "mes", "180")},
		{Name: "Prenájom meracej techniky", Active: true, Interval: recurring.Quarterly, NextDate: today.AddDays(20),
			Customer: party(3), ContactID: contactIDs[3], DueDays: 30,
			Lines: line("Prenájom analyzátora siete – {obdobie}", "štvrťrok", "390")},
		// bez automatického vystavenia: koncept čaká na kontrolu (počet hodín sa mení)
		{Name: "Technická podpora – hodiny", Active: true, Interval: recurring.Monthly, NextDate: today.AddDays(-2),
			Customer: party(1), ContactID: contactIDs[1], Note: "Hodiny podľa výkazu za {obdobie} — upravte pred vystavením.",
			Lines: line("Technická podpora {mesiac} {rok}", "hod", "45")},
		{Name: "Licencia vizualizácie", Interval: recurring.Yearly, NextDate: today.AddDays(75), AutoIssue: true,
			Customer: party(6), ContactID: contactIDs[6], EndDate: today.AddDays(800),
			Lines: line("Ročná licencia a aktualizácie HMI {obdobie}", "rok", "640")},
	}
	for _, t := range plan {
		if _, err := a.SaveRecurring(ctx, c.ID, t); err != nil {
			return err
		}
	}
	_, err := a.RunRecurring(ctx)
	return err
}

// seedQuotes pripraví cenové ponuky vo všetkých stavoch.
func seedQuotes(ctx context.Context, a *app.App, c store.Company, contactIDs []int64) error {
	today := a.Today()
	type action func(context.Context, int64, int64) (quote.Quote, error)
	plan := []struct {
		ci, day, valid int
		lines          []int
		subject, note  string
		then           []action
	}{
		{2, 40, 30, []int{0, 3}, "Nový rozvádzač a program linky", "", []action{a.IssueQuote, a.AcceptQuote}}, // vyfakturuje sa nižšie
		{4, 35, 14, []int{1}, "Servisné výjazdy", "", []action{a.IssueQuote, a.RejectQuote}},
		{1, 25, 14, []int{2, 5}, "Úprava HMI podľa normy", "", []action{a.IssueQuote}}, // po platnosti
		{6, 9, 30, []int{0, 1, 4}, "Automatizácia čerpacej stanice", "Cena platí pri objednávke do 30 dní. Termín realizácie do 3 týždňov od objednávky.", []action{a.IssueQuote}},
		{3, 5, 30, []int{3}, "", "", []action{a.IssueQuote, a.AcceptQuote}},
		{0, 0, 30, []int{2, 4}, "Kabeláž novej linky", "", nil}, // koncept
	}
	var first int64
	for _, p := range plan {
		cu := customers[p.ci]
		day := today.AddDays(-p.day)
		q := quote.Quote{
			Customer:  invoice.Party{Name: cu.name, City: cu.city, Country: "SK", ICO: cu.ico, DIC: cu.dic(), ICDPH: cu.icdph},
			ContactID: contactIDs[p.ci], IssueDate: day, ValidUntil: day.AddDays(p.valid), Note: p.note, Subject: p.subject,
		}
		if p.note != "" {
			q.Intro = "Na základe obhliadky na mieste vám ponúkame:"
		}
		for _, li := range p.lines {
			it := items[li%len(items)]
			q.Lines = append(q.Lines, invoice.Line{Description: it.desc, Unit: it.unit,
				Quantity: m.MustParse(it.qty), UnitPrice: m.MustParse(it.price), Rate: it.rate})
		}
		saved, err := a.SaveQuote(ctx, c.ID, q)
		if err != nil {
			return err
		}
		for _, step := range p.then {
			if _, err := step(ctx, c.ID, saved.ID); err != nil {
				return err
			}
		}
		if first == 0 {
			first = saved.ID
		}
	}
	_, err := a.InvoiceQuote(ctx, c.ID, first)
	return err
}

// seedAssets zaradí ukážkový majetok: dodávku, meracie prístroje (zrýchlene),
// softvér a vyradený starý počítač.
func seedAssets(ctx context.Context, a *app.App, c store.Company) error {
	y := a.Today().Year()
	date := func(dy int, md string) invoice.Date { return invoice.MustDate(fmt.Sprintf("%d-%s", y+dy, md)) }
	for _, x := range []asset.Asset{
		{Name: "Dodávka Ford Transit", Group: 1, Acquired: date(-2, "04-12"), Cost: m.MustParse("28490"), Share: 90, Note: "ZA-123BC, 10 % súkromne"},
		{Name: "Revízny merací prístroj", Group: 2, Method: asset.Accelerated, Acquired: date(-1, "09-03"), Cost: m.MustParse("3850")},
		{Name: "Projekčný softvér", Kind: asset.Intangible, Years: 3, Acquired: date(0, "02-01"), Cost: m.MustParse("2990")},
		{Name: "Počítač s monitorom", Group: 1, Acquired: date(-3, "11-20"), Cost: m.MustParse("1890"),
			Disposed: date(0, "03-31"), DisposalKind: asset.DisposalLiquidation},
	} {
		if _, err := a.SaveAsset(ctx, c.ID, x); err != nil {
			return err
		}
	}
	return nil
}

// seedBank naimportuje ukážkový výpis: dve presné úhrady (spárujú sa samé),
// čiastočnú úhradu, platbu dodávateľovi, poplatok a neznámu platbu.
func seedBank(ctx context.Context, a *app.App, c store.Company) error {
	invs, err := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{Status: invoice.StatusIssued})
	if err != nil {
		return err
	}
	purs, err := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	if err != nil {
		return err
	}
	today := a.Today()
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.02"><BkToCstmrStmt><Stmt>`)
	fmt.Fprintf(&b, `<Acct><Id><IBAN>%s</IBAN></Id></Acct>`, strings.ReplaceAll(c.IBAN, " ", ""))
	n := 0
	entry := func(dir, amount, e2e, name, msg string, day int) {
		n++
		party := "Dbtr"
		if dir == "DBIT" {
			party = "Cdtr"
		}
		fmt.Fprintf(&b, `<Ntry><Amt Ccy="EUR">%s</Amt><CdtDbtInd>%s</CdtDbtInd><Sts>BOOK</Sts><BookgDt><Dt>%s</Dt></BookgDt>`+
			`<AcctSvcrRef>DEMO-%03d</AcctSvcrRef><NtryDtls><TxDtls><Refs><EndToEndId>%s</EndToEndId></Refs>`+
			`<RltdPties><%s><Nm>%s</Nm></%s></RltdPties><RmtInf><Ustrd>%s</Ustrd></RmtInf></TxDtls></NtryDtls></Ntry>`,
			amount, dir, today.AddDays(-day).String(), n, e2e, party, xmlText(name), party, xmlText(msg))
	}
	k := 0
	for _, inv := range invs {
		if inv.Status != invoice.StatusIssued || inv.Kind != invoice.KindInvoice {
			continue
		}
		switch k {
		case 0, 1:
			entry("CRDT", inv.Summary.Total.String(), "/VS"+inv.VariableSymbol+"/SS/KS0308", inv.Customer.Name, "Uhrada faktury "+inv.Number, 2+k)
		case 2:
			half := inv.Summary.Total.MulRatio(m.FromInt(1), m.FromInt(2)).RoundCents()
			entry("CRDT", half.String(), "/VS"+inv.VariableSymbol, inv.Customer.Name, "Ciastocna uhrada", 4)
		}
		k++
	}
	for _, p := range purs {
		if p.Status == purchase.StatusBooked {
			entry("DBIT", p.Summary.Total.String(), "/VS"+p.VariableSymbol, p.Supplier.Name, p.Description, 1)
			break
		}
	}
	entry("DBIT", "4.90", "NOTPROVIDED", "", "Poplatok za vedenie uctu", 0)
	entry("CRDT", "150.00", "NOTPROVIDED", "Neznámy platca", "zaloha", 3)
	b.WriteString(`</Stmt></BkToCstmrStmt></Document>`)
	_, err = a.ImportStatement(ctx, c.ID, "demo-vypis.xml", []byte(b.String()))
	return err
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Ukážkoví používatelia pre serverový režim (heslo všetkých: DemoPassword).
const (
	DemoAdmin    = "spravca@demo.sk"
	DemoPassword = "demo-fiskus"
)

var demoUsers = []app.NewMember{
	{Email: "uctovnicka@demo.sk", Name: "Jana Účtovníčka", Role: auth.RoleAccountant},
	{Email: "fakturant@demo.sk", Name: "Fero Fakturant", Role: auth.RoleInvoicer},
	{Email: "klient@demo.sk", Name: "Klient Kancelárie", Role: auth.RoleClient},
}

// SeedUsers vytvorí správcu inštalácie a ukážkových používateľov s rolami vo firme.
func SeedUsers(ctx context.Context, a *app.App, companyID int64) error {
	if _, err := a.EnsureAdmin(ctx, DemoAdmin, "Správca Demo", DemoPassword); err != nil {
		return err
	}
	for _, u := range demoUsers {
		u.Password = DemoPassword
		if _, err := a.AddMember(ctx, companyID, u); err != nil {
			return err
		}
	}
	return nil
}

// seedDomesticRC vystaví stavebnej firme faktúru s tuzemským prenesením
// daňovej povinnosti pri položkách (stavebné práce, oceľová konštrukcia).
func seedDomesticRC(ctx context.Context, a *app.App, c store.Company) error {
	day := a.Today().AddDays(-9)
	cu := customers[1]
	d, err := a.SaveDraft(ctx, c.ID, invoice.Invoice{
		Customer:  invoice.Party{Name: cu.name, City: cu.city, Country: "SK", ICO: cu.ico, DIC: cu.dic(), ICDPH: cu.icdph},
		IssueDate: day, DeliveryDate: day, DueDate: day.AddDays(30),
		Lines: []invoice.Line{
			{Description: "Programovanie riadiaceho systému vetrania", Unit: "hod", Quantity: m.MustParse("16"), UnitPrice: m.MustParse("45"), Rate: vat.Rate23},
			{Description: "Montáž rozvádzača a elektroinštalácia v hale", Unit: "ks", Quantity: m.MustParse("1"), UnitPrice: m.MustParse("3200"), Rate: vat.Rate23, Reverse: invoice.RCConstruction},
			{Description: "Oceľová nosná konštrukcia káblových trás", Unit: "t", Quantity: m.MustParse("2.4"), UnitPrice: m.MustParse("2300"), Rate: vat.Rate23, Reverse: invoice.RCMetals, Commodity: "7308"},
		},
	})
	if err != nil {
		return err
	}
	_, err = a.Issue(ctx, c.ID, d.ID)
	return err
}

// demoExpense priradí ukážkovým nákupom druh výdavku pre peňažný denník.
func demoExpense(what string) purchase.ExpenseKind {
	switch {
	case strings.Contains(what, "materiál"), strings.Contains(what, "Pohonné"):
		return purchase.ExpenseStock
	case strings.Contains(what, "literatúra"):
		return purchase.ExpenseOther
	}
	return purchase.ExpenseServices
}
