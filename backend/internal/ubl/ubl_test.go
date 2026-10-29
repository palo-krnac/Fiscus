package ubl

import (
	"encoding/xml"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

var seller = Seller{Name: "Demo elektroservis", Street: "Námestie SNP 1", City: "Banská Bystrica", Zip: "974 01",
	ICO: "55123457", DIC: "2120000003", ICDPH: "SK2120000003", IBAN: "SK31 1200 0000 1987 4263 7541", Email: "faktury@demo.sk", VATPayer: true}

func issued(lines ...invoice.Line) invoice.Invoice {
	inv := invoice.Invoice{Kind: invoice.KindInvoice, Status: invoice.StatusIssued, Number: "FA20260041", VariableSymbol: "20260041",
		Customer:  invoice.Party{Name: "Stavmont Zvolen, a.s.", Street: "Hlavná 5", City: "Zvolen", Zip: "960 01", Country: "SK", ICO: "36874124", DIC: "2020368745", ICDPH: "SK2020368745"},
		IssueDate: invoice.MustDate("2026-10-20"), DeliveryDate: invoice.MustDate("2026-10-20"), DueDate: invoice.MustDate("2026-11-19"),
		PaymentMethod: invoice.PayTransfer, Lines: lines}
	if err := inv.Recalculate(); err != nil {
		panic(err)
	}
	return inv
}

func line(desc, q, unit, price string, rate vat.Rate) invoice.Line {
	return invoice.Line{Description: desc, Quantity: m.MustParse(q), Unit: unit, UnitPrice: m.MustParse(price), Rate: rate}
}

// doc je kontrolná štruktúra vytvoreného XML (pravidlá EN 16931 o súčtoch).
type doc struct {
	XMLName  xml.Name
	Customer string   `xml:"CustomizationID"`
	Type     string   `xml:"InvoiceTypeCode"`
	CType    string   `xml:"CreditNoteTypeCode"`
	Notes    []string `xml:"Note"`
	Endpoint []struct {
		V string `xml:",chardata"`
		S string `xml:"schemeID,attr"`
	} `xml:"AccountingSupplierParty>Party>EndpointID"`
	BuyerEndpoint struct {
		V string `xml:",chardata"`
		S string `xml:"schemeID,attr"`
	} `xml:"AccountingCustomerParty>Party>EndpointID"`
	TaxTotals []struct {
		Amount string `xml:"TaxAmount"`
		Subs   []struct {
			Base, Tax, Cat, Pct, Reason string
		} `xml:"-"`
		Raw []struct {
			Base string `xml:"TaxableAmount"`
			Tax  string `xml:"TaxAmount"`
			Cat  string `xml:"TaxCategory>ID"`
			Pct  string `xml:"TaxCategory>Percent"`
		} `xml:"TaxSubtotal"`
	} `xml:"TaxTotal"`
	LineExt string `xml:"LegalMonetaryTotal>LineExtensionAmount"`
	Incl    string `xml:"LegalMonetaryTotal>TaxInclusiveAmount"`
	Payable string `xml:"LegalMonetaryTotal>PayableAmount"`
	Lines   []struct {
		Qty   string `xml:"InvoicedQuantity"`
		CQty  string `xml:"CreditedQuantity"`
		Ext   string `xml:"LineExtensionAmount"`
		Cat   string `xml:"Item>ClassifiedTaxCategory>ID"`
		Pct   string `xml:"Item>ClassifiedTaxCategory>Percent"`
		HS    string `xml:"Item>CommodityClassification>ItemClassificationCode"`
		Price string `xml:"Price>PriceAmount"`
		BaseQ string `xml:"Price>BaseQuantity"`
	} `xml:"InvoiceLine"`
	CLines []struct {
		Qty string `xml:"CreditedQuantity"`
		Ext string `xml:"LineExtensionAmount"`
		Cat string `xml:"Item>ClassifiedTaxCategory>ID"`
	} `xml:"CreditNoteLine"`
	BillingRef string `xml:"BillingReference>InvoiceDocumentReference>ID"`
	IBAN       string `xml:"PaymentMeans>PayeeFinancialAccount>ID"`
	PaymentID  string `xml:"PaymentMeans>PaymentID"`
}

func parse(t *testing.T, b []byte) doc {
	t.Helper()
	var d doc
	if err := xml.Unmarshal(b, &d); err != nil {
		t.Fatalf("neplatné XML: %v\n%s", err, b)
	}
	return d
}

// checkSums overí BR-CO-10 (súčet riadkov), BR-S-08/BR-AE-08 (základ kategórie
// = súčet jej riadkov) a BR-CO-15 (spolu = základ + DPH).
func checkSums(t *testing.T, d doc) {
	t.Helper()
	sum := m.Zero
	perCat := map[string]m.Dec{}
	for _, l := range d.Lines {
		v := m.MustParse(l.Ext)
		sum = sum.Add(v)
		perCat[l.Cat+"/"+l.Pct] = perCat[l.Cat+"/"+l.Pct].Add(v)
		// PEPPOL-EN16931-R120: suma riadku = množstvo × cena / základné množstvo
		q, price := m.MustParse(l.Qty), m.MustParse(l.Price)
		bq := m.FromInt(1)
		if l.BaseQ != "" {
			bq = m.MustParse(l.BaseQ)
		}
		if got := q.MulRatio(price, bq).RoundCents(); !got.Equal(v) {
			t.Errorf("riadok %s × %s / %s = %s, v XML %s", l.Qty, l.Price, bq, got, l.Ext)
		}
	}
	if !sum.Equal(m.MustParse(d.LineExt)) {
		t.Errorf("súčet riadkov %s ≠ %s", sum, d.LineExt)
	}
	vatSum := m.Zero
	for _, st := range d.TaxTotals[0].Raw {
		if !perCat[st.Cat+"/"+st.Pct].Equal(m.MustParse(st.Base)) {
			t.Errorf("kategória %s %s: riadky %s ≠ %s", st.Cat, st.Pct, perCat[st.Cat+"/"+st.Pct], st.Base)
		}
		vatSum = vatSum.Add(m.MustParse(st.Tax))
	}
	if !vatSum.Equal(m.MustParse(d.TaxTotals[0].Amount)) || !m.MustParse(d.LineExt).Add(vatSum).Equal(m.MustParse(d.Incl)) {
		t.Errorf("DPH %s, spolu %s", vatSum, d.Incl)
	}
}

func TestWriteInvoice(t *testing.T) {
	gross := issued(line("Kábel", "3", "m", "1.99", vat.Rate23), line("Svorka", "7", "ks", "0.33", vat.Rate23), line("Kniha", "1", "ks", "19.99", vat.Rate5))
	gross.PriceMode = vat.PriceGross
	_ = gross.Recalculate()
	rc := issued(line("Projekt", "16", "hod", "45", vat.Rate23),
		invoice.Line{Description: "Montáž", Quantity: m.MustParse("1"), Unit: "ks", UnitPrice: m.MustParse("3200"), Rate: vat.Rate23, Reverse: invoice.RCConstruction},
		invoice.Line{Description: "Oceľ", Quantity: m.MustParse("2.4"), Unit: "t", UnitPrice: m.MustParse("2300"), Rate: vat.Rate23, Reverse: invoice.RCMetals, Commodity: "7308 90"})
	czk := issued(line("Programovanie", "40", "hod", "1200", vat.Rate23))
	texts := issued(line("Servis", "1", "ks", "100", vat.Rate23))
	texts.Subject, texts.Intro, texts.Note = "Servis október", "Podľa zmluvy č. 7", "Ďakujeme"
	czk.Customer = invoice.Party{Name: "Bohemia Control s.r.o.", City: "Brno", Country: "CZ", ICDPH: "CZ27654321"}
	czk.ReverseCharge, czk.SupplyKind, czk.Currency, czk.FXRate = true, invoice.SupplyServices, "CZK", m.MustParse("25")
	_ = czk.Recalculate()

	tests := []struct {
		name    string
		inv     invoice.Invoice
		cats    string
		payable string
		check   func(t *testing.T, d doc)
	}{
		{"bežná faktúra", issued(line("Programovanie PLC", "8", "hod", "45", vat.Rate23), line("Kábel", "2.5", "m", "1.99", vat.Rate23), line("Kniha", "1", "ks", "20", vat.Rate5)),
			"S/23.00 S/5.00", "469.93", func(t *testing.T, d doc) {
				if d.Endpoint[0].S != "0245" || d.Endpoint[0].V != "2120000003" || d.BuyerEndpoint.V != "2020368745" {
					t.Errorf("Peppol adresy: %+v %+v", d.Endpoint, d.BuyerEndpoint)
				}
				if d.Type != "380" || d.IBAN != "SK3112000000198742637541" || d.PaymentID != "20260041" || d.Customer != CustomizationID {
					t.Errorf("hlavička: %+v", d)
				}
			}},
		{"ceny s DPH: riadky bez DPH sedia na základ", gross, "S/23.00 S/5.00", "", nil},
		{"tuzemské prenesenie pri položkách", rc, "S/23.00 AE/0.00", "9605.60", func(t *testing.T, d doc) {
			if d.Lines[2].HS != "7308" || len(d.Notes) == 0 || !strings.Contains(strings.Join(d.Notes, " "), "§ 69 ods. 12 písm. g), j)") {
				t.Errorf("KN kód / text: %+v %v", d.Lines[2], d.Notes)
			}
		}},
		{"predmet a texty v jednej poznámke (PEPPOL-EN16931-R002)", texts, "S/23.00", "123.00", func(t *testing.T, d doc) {
			if len(d.Notes) != 1 || d.Notes[0] != "Servis október\nPodľa zmluvy č. 7\nĎakujeme" {
				t.Errorf("poznámka: %q", d.Notes)
			}
		}},
		{"služba do EÚ v CZK", czk, "AE/0.00", "48000.00", func(t *testing.T, d doc) {
			if len(d.TaxTotals) != 2 || d.BuyerEndpoint.S != "9929" {
				t.Errorf("DPH v eurách (BR-53) / schéma CZ: %d %+v", len(d.TaxTotals), d.BuyerEndpoint)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Write(tt.inv, seller)
			if err != nil {
				t.Fatal(err)
			}
			d := parse(t, b)
			checkSums(t, d)
			var cats []string
			for _, st := range d.TaxTotals[0].Raw {
				cats = append(cats, st.Cat+"/"+st.Pct)
			}
			if strings.Join(cats, " ") != tt.cats {
				t.Errorf("kategórie %v, chcem %s", cats, tt.cats)
			}
			if tt.payable != "" && d.Payable != tt.payable {
				t.Errorf("na úhradu %s, chcem %s", d.Payable, tt.payable)
			}
			if tt.check != nil {
				tt.check(t, d)
			}
			if regexp.MustCompile(`\d\.\d{3,}</cbc:(Taxable|Tax|LineExtension|Payable)Amount`).Match(b) {
				t.Error("sumy majú mať najviac 2 desatinné miesta")
			}
		})
	}
}

func TestWriteCreditNote(t *testing.T) {
	cn := issued(line("Vrátenie – kábel", "-2", "m", "10", vat.Rate23))
	cn.Kind, cn.Number = invoice.KindCreditNote, "DB20260001"
	cn.Correction = &invoice.Correction{OriginalNumber: "FA20260041", Reason: "Vrátený tovar"}
	b, err := Write(cn, seller)
	if err != nil {
		t.Fatal(err)
	}
	d := parse(t, b)
	if d.XMLName.Local != "CreditNote" || d.CType != "381" || d.BillingRef != "FA20260041" || d.Payable != "24.60" ||
		len(d.CLines) != 1 || d.CLines[0].Qty != "2.00" || d.CLines[0].Ext != "20.00" {
		t.Fatalf("dobropis má kladné sumy: %+v\n%s", d, b)
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name  string
		mod   func(*invoice.Invoice, *Seller)
		field string
	}{
		{"úplná", func(*invoice.Invoice, *Seller) {}, ""},
		{"koncept", func(i *invoice.Invoice, _ *Seller) { i.Status = invoice.StatusDraft }, "status"},
		{"zálohová faktúra", func(i *invoice.Invoice, _ *Seller) { i.Kind = invoice.KindProforma }, "kind"},
		{"firma bez DIČ", func(_ *invoice.Invoice, s *Seller) { s.DIC = "" }, "seller.dic"},
		{"odberateľ bez DIČ", func(i *invoice.Invoice, _ *Seller) { i.Customer.DIC = "" }, "customer.dic"},
		{"bez IBAN pri prevode", func(_ *invoice.Invoice, s *Seller) { s.IBAN = "" }, "seller.iban"},
		{"v hotovosti bez IBAN", func(i *invoice.Invoice, s *Seller) { s.IBAN = ""; i.PaymentMethod = invoice.PayCash }, ""},
		{"platiteľ bez IČ DPH", func(_ *invoice.Invoice, s *Seller) { s.ICDPH = "" }, "seller.icdph"},
		{"zahraničný mimo EÚ", func(i *invoice.Invoice, _ *Seller) { i.Customer.Country, i.Customer.ICDPH = "US", "" }, "customer.icdph"},
	}
	for _, tt := range tests {
		inv, s := issued(line("A", "1", "ks", "1", vat.Rate23)), seller
		tt.mod(&inv, &s)
		got := map[string]bool{}
		for _, p := range Check(inv, s) {
			got[p.Field] = true
		}
		if (tt.field == "" && len(got) > 0) || (tt.field != "" && !got[tt.field]) {
			t.Errorf("%s: %v", tt.name, got)
		}
		if tt.field != "" {
			var ie *IncompleteError
			if _, err := Write(inv, s); !errors.As(err, &ie) {
				t.Errorf("%s: Write %v", tt.name, err)
			}
		}
	}
}

func TestUnitCode(t *testing.T) {
	for in, want := range map[string]string{"ks": "H87", "hod": "HUR", "m²": "MTK", "KG": "KGM", "bal.": "XPK", "čokoľvek": "C62"} {
		if got := UnitCode(in); got != want {
			t.Errorf("%s → %s", in, got)
		}
	}
}

// Prijatá e-faktúra: čo vytvorí Fiškus, to Fiškus aj prečíta ako nákup.
func TestReadRoundTrip(t *testing.T) {
	rc := issued(line("Projekt", "16", "hod", "45", vat.Rate23), line("Kniha", "1", "ks", "20", vat.Rate5),
		invoice.Line{Description: "Montáž", Quantity: m.MustParse("1"), Unit: "ks", UnitPrice: m.MustParse("3200"), Rate: vat.Rate23, Reverse: invoice.RCConstruction})
	b, err := Write(rc, seller)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(b, invoice.MustDate("2026-10-22"))
	if err != nil {
		t.Fatal(err)
	}
	p := got.Purchase
	if p.Supplier.Name != "Demo elektroservis" || p.Supplier.ICO != "55123457" || p.Supplier.DIC != "2120000003" || p.Supplier.ICDPH != "SK2120000003" ||
		p.SupplierNumber != "FA20260041" || p.VariableSymbol != "20260041" || p.SupplierIBAN != "SK3112000000198742637541" ||
		p.IssueDate.String() != "2026-10-20" || p.DueDate.String() != "2026-11-19" || p.ReceivedDate.String() != "2026-10-22" || p.ReverseCharge {
		t.Fatalf("%+v", p)
	}
	// bežná DPH 23 %: 720 a 5 %: 20; prenesenie (3 200) sa eviduje zvlášť — upozornenie
	if len(p.Rates) != 2 || p.Rates[0].Base.String() != "720.00" || p.Rates[0].VAT.String() != "165.60" || p.Rates[1].Base.String() != "20.00" ||
		!strings.Contains(strings.Join(got.Warnings, " "), "základ 3") {
		t.Fatalf("rekapitulácia %+v %v", p.Rates, got.Warnings)
	}
	// iba prenesenie → celý doklad s prenesením
	only := issued(invoice.Line{Description: "Montáž", Quantity: m.MustParse("1"), Unit: "ks", UnitPrice: m.MustParse("3200"), Rate: vat.Rate23, Reverse: invoice.RCConstruction})
	b, _ = Write(only, seller)
	got, _ = Read(b, invoice.MustDate("2026-10-22"))
	if !got.Purchase.ReverseCharge || len(got.Purchase.Rates) != 1 || got.Purchase.Rates[0].Rate != vat.Rate23 || got.Purchase.Rates[0].Base.String() != "3200.00" {
		t.Fatalf("prenesenie: %+v", got.Purchase)
	}
	if !strings.HasPrefix(p.Description, "Projekt a ďalšie") {
		t.Error(p.Description)
	}
}

func TestReadErrors(t *testing.T) {
	for _, s := range []string{"", "<html></html>", "nie xml", `<Order xmlns="x"/>`} {
		if _, err := Read([]byte(s), invoice.MustDate("2026-10-22")); !errors.Is(err, ErrNotUBL) {
			t.Errorf("%q: %v", s, err)
		}
	}
}
