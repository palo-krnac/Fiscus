// Package demo naplní úložisko ukážkovými dátami (vymyslené firmy).
package demo

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
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
		ct, err := a.SaveContact(ctx, c.ID, store.Contact{Name: cu.name, City: cu.city, ICO: cu.ico, ICDPH: cu.icdph, Country: "SK", DueDays: due})
		if err != nil {
			return c, err
		}
		contactIDs[i] = ct.ID
	}

	today := a.Today()
	for i := 39; i >= 0; i-- { // od najstaršej, aby čísla rástli s dátumom
		ci := (i * 5) % len(customers)
		cu := customers[ci]
		issue := today.AddDays(-i*4 - 1)
		inv := invoice.Invoice{
			Customer:     invoice.Party{Name: cu.name, City: cu.city, Country: "SK", ICO: cu.ico, ICDPH: cu.icdph},
			ContactID:    contactIDs[ci],
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

	// prijaté faktúry
	suppliers := []struct {
		name, ico, icdph, what string
		base                   string
		rate                   vat.Rate
		day                    int
	}{
		{"Čerpacia stanica Hron s.r.o.", ico("4855101"), icdph(2120455101), "Pohonné hmoty", "81.30", vat.Rate23, 3},
		{"TeleNet Slovensko s.r.o.", ico("4855202"), icdph(2120455202), "Mobilné služby", "40.65", vat.Rate23, 8},
		{"Elektro veľkoobchod BB s.r.o.", ico("4855303"), icdph(2120455303), "Elektroinštalačný materiál", "612.40", vat.Rate23, 12},
		{"Prenájom priestorov Ján Malý", ico("4855404"), "", "Nájom kancelárie (neplatiteľ DPH)", "350.00", vat.Rate0, 20},
		{"Energie Stred s.r.o.", ico("4855505"), icdph(2120455505), "Elektrina", "54.10", vat.Rate23, 33},
		{"Kníhkupectvo pri fontáne s.r.o.", ico("4855606"), icdph(2120455606), "Odborná literatúra", "38.00", vat.Rate5, 45},
	}
	for i, sp := range suppliers {
		received := today.AddDays(-sp.day)
		pu := purchase.Purchase{
			SupplierNumber: fmt.Sprintf("%d%05d", received.Year(), 1200+i*37),
			Supplier:       invoice.Party{Name: sp.name, ICO: sp.ico, ICDPH: sp.icdph, Country: "SK"},
			IssueDate:      received.AddDays(-2),
			ReceivedDate:   received,
			DueDate:        received.AddDays(12),
			Description:    sp.what,
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
	return c, nil
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
