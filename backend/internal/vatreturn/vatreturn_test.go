package vatreturn

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

var d = invoice.MustDate
var m = money.MustParse

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		in, start, end, label string
		ok                    bool
	}{
		{"2026-10", "2026-10-01", "2026-10-31", "október 2026", true},
		{"2026-02", "2026-02-01", "2026-02-28", "február 2026", true},
		{"2026-q4", "2026-10-01", "2026-12-31", "4. štvrťrok 2026", true},
		{"2025-Q3", "2025-07-01", "2025-09-30", "3. štvrťrok 2025", true},
		{"2026-13", "", "", "", false},
		{"2026-Q5", "", "", "", false},
		{"október", "", "", "", false},
	}
	for _, tt := range tests {
		p, err := ParsePeriod(tt.in)
		if (err == nil) != tt.ok {
			t.Errorf("%s: chyba %v", tt.in, err)
			continue
		}
		if !tt.ok {
			continue
		}
		if p.Start().String() != tt.start || p.End().String() != tt.end || p.Label() != tt.label {
			t.Errorf("%s: %s–%s %q", tt.in, p.Start(), p.End(), p.Label())
		}
	}
	old, _ := ParsePeriod("2025-06")
	cur, _ := ParsePeriod("2025-07")
	if old.Supported() || !cur.Supported() {
		t.Error("podpora tlačiva DPHv25 od 7/2025")
	}
}

// inv vytvorí vystavenú faktúru s rekapituláciou podľa sadzieb (sadzba → základ).
func inv(id int64, num string, delivery string, icdph string, rates map[vat.Rate]string) invoice.Invoice {
	i := invoice.Invoice{ID: id, Number: num, Kind: invoice.KindInvoice, Status: invoice.StatusIssued,
		IssueDate: d(delivery), DeliveryDate: d(delivery), Customer: invoice.Party{Name: "Odberateľ " + num, ICDPH: icdph, Country: "SK"}}
	for _, r := range []vat.Rate{23, 19, 5, 0} {
		if b, ok := rates[r]; ok {
			base := m(b)
			tax := base.MulRatio(money.FromInt(int64(r)), money.FromInt(100)).RoundCents()
			i.Summary.Rates = append(i.Summary.Rates, vat.RateSummary{Rate: r, Base: base, VAT: tax, Total: base.Add(tax)})
			i.Summary.Base = i.Summary.Base.Add(base)
			i.Summary.VAT = i.Summary.VAT.Add(tax)
		}
	}
	i.Summary.Total = i.Summary.Base.Add(i.Summary.VAT)
	return i
}

func pur(id int64, num, received, icdph string, rate vat.Rate, base, tax string) purchase.Purchase {
	p := purchase.Purchase{ID: id, Number: num, SupplierNumber: "D " + num, Status: purchase.StatusBooked,
		Supplier:  invoice.Party{Name: "Dodávateľ " + num, ICDPH: icdph, Country: "SK"},
		IssueDate: d(received), DeliveryDate: d(received), ReceivedDate: d(received),
		Rates: []purchase.RateRow{{Rate: rate, Base: m(base), VAT: m(tax)}}}
	p.Normalize()
	return p
}

func scenario() Input {
	p, _ := ParsePeriod("2026-10")
	credit := inv(5, "DB20260001", "2026-10-20", "SK2020372640", map[vat.Rate]string{23: "-100"})
	credit.Kind = invoice.KindCreditNote
	credit.Correction = &invoice.Correction{OriginalNumber: "FA20260001"}
	draft := inv(6, "", "2026-10-05", "", map[vat.Rate]string{23: "999"})
	draft.Status = invoice.StatusDraft
	cancelled := inv(7, "FA20260007", "2026-10-05", "", map[vat.Rate]string{23: "999"})
	cancelled.Status = invoice.StatusCancelled
	eu := inv(8, "FA20260008", "2026-10-06", "CZ12345678", map[vat.Rate]string{0: "500"})
	eu.Customer.Country = "CZ"
	eu.ReverseCharge = true
	lateIssued := inv(9, "FA20260009", "2026-10-31", "", map[vat.Rate]string{19: "10"})
	lateIssued.IssueDate = d("2026-11-03") // dodané v októbri, vystavené v novembri → október

	noDed := pur(4, "PF4", "2026-10-10", "SK2020000001", 23, "100", "23")
	noDed.NoDeduction = true
	rc := pur(5, "PF5", "2026-10-11", "SK2020000002", 23, "1000", "230")
	rc.ReverseCharge = true
	rc.Normalize()
	late := pur(6, "PF6", "2026-11-02", "SK2020000003", 23, "100", "23") // doručená v novembri
	early := pur(7, "PF7", "2026-10-02", "SK2020000004", 23, "50", "11.50")
	early.DeliveryDate = d("2026-09-28") // dodaná v septembri, doručená v októbri → október
	noID := pur(8, "PF8", "2026-10-12", "", 23, "10", "2.30")

	return Input{
		Period: p,
		Invoices: []invoice.Invoice{
			inv(1, "FA20260001", "2026-10-01", "SK2020372640", map[vat.Rate]string{23: "1000", 19: "200"}),
			inv(2, "FA20260002", "2026-10-15", "", map[vat.Rate]string{23: "100", 5: "40", 0: "30"}),
			inv(3, "FA20260003", "2026-09-30", "SK2020372640", map[vat.Rate]string{23: "777"}), // september
			credit, draft, cancelled, eu, lateIssued,
		},
		Purchases: []purchase.Purchase{
			pur(1, "PF1", "2026-10-03", "SK2120455106", 23, "81.30", "18.70"),
			pur(2, "PF2", "2026-10-04", "SK2120455106", 19, "100", "19"),
			pur(3, "PF3", "2026-10-05", "SK2120455106", 5, "20", "1"),
			noDed, rc, late, early, noID,
		},
	}
}

func TestCompute(t *testing.T) {
	r := Compute(scenario())
	want := map[string]string{
		"r01": "210", "r02": "39.9", // 200 + 10 (faktúra vystavená v novembri, dodaná v októbri)
		"r01a": "40", "r02a": "2",
		"r03": "1100", "r04": "253",
		"r09b": "1000", "r10b": "230", // prenesenie daňovej povinnosti
		"r13": "30",
		"r17": "524.9", // 39.90 + 2 + 253 + 230
		"r18": "19", "r18a": "1",
		"r19": "260.2", // 18.70 + 230 + 11.50
		"r20": "19", "r20a": "1", "r21": "30.2",
		"r24": "-100", "r25": "-23",
		"r32": "221.7", // 524.90 − 19 − 1 − 260.20 − 23
		"r35": "221.7",
	}
	for k, v := range want {
		if got := r.Rows[k]; !got.Equal(m(v)) {
			t.Errorf("%s = %s, chcem %s", k, got, v)
		}
	}
	for k := range r.Rows {
		if _, ok := want[k]; !ok {
			t.Errorf("nečakaný riadok %s = %s", k, r.Rows[k])
		}
	}

	// KV
	if len(r.KV.A1) != 2 || r.KV.A1[0].Partner != "SK2020372640" || r.KV.A1[0].Number != "FA20260001" {
		t.Errorf("A.1: %+v", r.KV.A1)
	}
	if len(r.KV.C1) != 1 || r.KV.C1[0].Original != "FA20260001" || !r.KV.C1[0].VATDiff.Equal(m("-23")) {
		t.Errorf("C.1: %+v", r.KV.C1)
	}
	d2 := r.KV.D2
	if d2.Count != 2 || !d2.Base.Equal(m("100")) || !d2.VAT.Equal(m("23")) || !d2.BaseReduced.Equal(m("50")) || !d2.VATReduced.Equal(m("3.9")) {
		t.Errorf("D.2: %+v", d2)
	}
	if len(r.KV.B2) != 4 || r.KV.B2[0].Number != "DPF1" {
		t.Errorf("B.2: %+v", r.KV.B2)
	}
	if len(r.KV.B1) != 1 || !r.KV.B1[0].Deducted.Equal(m("230")) {
		t.Errorf("B.1: %+v", r.KV.B1)
	}
	// upozornenia: EÚ odberateľ, dodávateľ bez IČ DPH
	var msgs []string
	for _, w := range r.Warnings {
		msgs = append(msgs, w.Doc+": "+w.Message)
	}
	all := strings.Join(msgs, "\n")
	if len(r.Warnings) != 2 || !strings.Contains(all, "FA20260008") || !strings.Contains(all, "PF8") {
		t.Errorf("upozornenia:\n%s", all)
	}
	if !r.Deduction.Equal(m("280.2")) {
		t.Errorf("odpočet spolu %s", r.Deduction)
	}
	if r.Invoices != 4 || r.Purchase != 5 {
		t.Errorf("zahrnuté %d faktúr, %d prijatých", r.Invoices, r.Purchase)
	}
}

func TestExcessDeduction(t *testing.T) {
	p, _ := ParsePeriod("2026-10")
	tests := []struct {
		name, out, in, carried string
		want                   map[string]string
	}{
		{"vlastná povinnosť", "100", "40", "0", map[string]string{"r32": "60", "r35": "60"}},
		{"nadmerný odpočet", "10", "50", "0", map[string]string{"r33": "40"}},
		{"započítanie z minula", "100", "40", "25", map[string]string{"r32": "60", "r34": "25", "r35": "35"}},
		{"započíta sa najviac povinnosť", "100", "40", "80", map[string]string{"r32": "60", "r34": "60"}},
	}
	for _, tt := range tests {
		in := Input{Period: p, ExcessCarried: m(tt.carried),
			Invoices:  []invoice.Invoice{inv(1, "F1", "2026-10-01", "", map[vat.Rate]string{23: "1"})},
			Purchases: []purchase.Purchase{pur(1, "P1", "2026-10-01", "SK2120455106", 23, "1", "1")}}
		in.Invoices[0].Summary.Rates[0].VAT = m(tt.out)
		in.Purchases[0].Rates[0].VAT = m(tt.in)
		in.Purchases[0].Normalize()
		r := Compute(in)
		for _, k := range []string{"r32", "r33", "r34", "r35"} {
			w, ok := tt.want[k]
			if !ok {
				w = "0"
			}
			if !r.Rows[k].Equal(m(w)) {
				t.Errorf("%s: %s = %s, chcem %s", tt.name, k, r.Rows[k], w)
			}
		}
	}
}

func TestDPHXML(t *testing.T) {
	r := Compute(scenario())
	tp := Taxpayer{Name: "Demo & syn s.r.o.", Street: "Námestie SNP 12/A", City: "Banská Bystrica", Zip: "974 01", DIC: "2120000003", ICDPH: "SK2120000003"}
	out := DPHXML(r, tp, d("2026-11-20"))

	var doc struct {
		Hlavicka struct {
			IC struct {
				Kod   string `xml:"kodStatu"`
				Cislo string `xml:"cislo"`
			} `xml:"identifikacneCislo"`
			Obd struct {
				Mesiac string `xml:"mesiac"`
				Rok    string `xml:"rok"`
			} `xml:"zdanObd"`
			Meno  string `xml:"meno>riadok"`
			Ulica string `xml:"adresa>ulica"`
			Cislo string `xml:"adresa>cislo"`
			Psc   string `xml:"adresa>psc"`
			Datum string `xml:"datumVyhlasenia"`
		} `xml:"hlavicka"`
		Telo struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"telo"`
	}
	if err := xml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("XML nie je platné: %v\n%s", err, out)
	}
	h := doc.Hlavicka
	if h.IC.Kod != "SK" || h.IC.Cislo != "2120000003" || h.Meno != tp.Name || h.Ulica != "Námestie SNP" ||
		h.Cislo != "12/A" || h.Psc != "97401" || h.Datum != "20.11.2026" {
		t.Errorf("hlavička: %+v", h)
	}
	// poradie prvkov tela podľa schémy a hodnoty
	dec := xml.NewDecoder(bytes.NewReader(doc.Telo.Inner))
	var names []string
	vals := map[string]string{}
	var cur string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch v := tok.(type) {
		case xml.StartElement:
			cur = v.Name.Local
			names = append(names, cur)
		case xml.CharData:
			vals[cur] = string(v)
		}
	}
	if len(names) != len(Rows)+1 || names[0] != "r01" || names[len(names)-1] != "r37" {
		t.Fatalf("prvky tela: %v", names)
	}
	for i, n := range names {
		if n == "splneniePodmienok" && names[i-1] != "r32" {
			t.Error("splneniePodmienok musí nasledovať po r32")
		}
	}
	if vals["r04"] != "253.00" || vals["r24"] != "-100.00" || vals["r05"] != "" {
		t.Errorf("hodnoty: r04=%q r24=%q r05=%q", vals["r04"], vals["r24"], vals["r05"])
	}
}

func TestKVXML(t *testing.T) {
	r := Compute(scenario())
	tp := Taxpayer{Name: "Demo", Street: "Hlavná 1", City: "Zvolen", Zip: "960 01", ICDPH: "SK2120000003"}
	out := KVXML(r, tp)
	type line struct {
		Odb, Dod, F, Den, Z, D, S, O string `xml:",attr"`
		FO, FP, ZR, DR               string `xml:",attr"`
		ZZn, DZn                     string `xml:",attr"`
	}
	var doc struct {
		XMLName xml.Name
		Ic      string `xml:"Identifikacia>IcDphPlatitela"`
		Druh    string `xml:"Identifikacia>Druh"`
		Rok     string `xml:"Identifikacia>Obdobie>Rok"`
		Mesiac  string `xml:"Identifikacia>Obdobie>Mesiac"`
		A1      []line `xml:"Transakcie>A1"`
		B1      []line `xml:"Transakcie>B1"`
		B2      []line `xml:"Transakcie>B2"`
		C1      []line `xml:"Transakcie>C1"`
		D2      []line `xml:"Transakcie>D2"`
	}
	if err := xml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("XML nie je platné: %v\n%s", err, out)
	}
	if doc.XMLName.Space != KVNamespace || doc.XMLName.Local != "KVDPH_2025" || doc.Ic != "SK2120000003" || doc.Druh != "R" || doc.Rok != "2026" || doc.Mesiac != "10" {
		t.Errorf("identifikácia: %+v", doc)
	}
	if len(doc.A1) != 2 || doc.A1[0] != (line{Odb: "SK2020372640", F: "FA20260001", Den: "2026-10-01", Z: "1000.00", D: "230.00", S: "23"}) {
		t.Errorf("A1: %+v", doc.A1)
	}
	if len(doc.C1) != 1 || doc.C1[0].FP != "FA20260001" || doc.C1[0].ZR != "-100.00" {
		t.Errorf("C1: %+v", doc.C1)
	}
	if len(doc.D2) != 1 || doc.D2[0].ZZn != "50.00" || doc.D2[0].DZn != "3.90" {
		t.Errorf("D2: %+v", doc.D2)
	}
	if len(doc.B2) != 4 || doc.B2[0].O != "18.70" || len(doc.B1) != 1 || doc.B1[0].Dod != "SK2020000002" {
		t.Errorf("B: %+v %+v", doc.B1, doc.B2)
	}
	// bez D.2, ak nie sú faktúry pre neplatiteľov
	r.KV.D2 = KVSum{}
	if bytes.Contains(KVXML(r, tp), []byte("<D2")) {
		t.Error("prázdny D2 sa nemá uvádzať")
	}
}

func TestForeignCurrencyInvoice(t *testing.T) {
	p, _ := ParsePeriod("2026-10")
	czk := inv(1, "FA1", "2026-10-10", "SK2020372640", map[vat.Rate]string{23: "2512.30"})
	czk.Currency, czk.FXRate = "CZK", m("25.123")
	local := invoice.ToEUR(czk.Summary, czk.FXRate)
	czk.Local = &local
	noRate := inv(2, "FA2", "2026-10-11", "", map[vat.Rate]string{23: "100"})
	noRate.Currency = "USD"
	ret := Compute(Input{Period: p, Invoices: []invoice.Invoice{czk, noRate}})
	if ret.Rows["r03"].String() != "100.00" || ret.Rows["r04"].String() != "23.00" {
		t.Fatalf("riadky v eurách: r03=%s r04=%s", ret.Rows["r03"], ret.Rows["r04"])
	}
	if len(ret.KV.A1) != 1 || ret.KV.A1[0].Base.String() != "100.00" {
		t.Fatalf("KV A.1: %+v", ret.KV.A1)
	}
	if len(ret.Warnings) != 1 || !strings.Contains(ret.Warnings[0].Message, "nemá kurz") {
		t.Fatalf("upozornenia: %+v", ret.Warnings)
	}
}

func TestForeignCurrencyPurchase(t *testing.T) {
	p, _ := ParsePeriod("2026-10")
	svc := pur(1, "PF1", "2026-10-10", "CZ12345678", 23, "2500", "0")
	svc.Supplier.Country, svc.SupplyKind, svc.ReverseCharge = "CZ", invoice.SupplyServices, true
	svc.Currency, svc.FXRate = "CZK", m("25")
	svc.Normalize()
	noRate := pur(2, "PF2", "2026-10-11", "SK2120455106", 23, "100", "0")
	noRate.Currency = "USD"
	noRate.Normalize()
	ret := Compute(Input{Period: p, Purchases: []purchase.Purchase{svc, noRate}})
	for k, want := range map[string]string{"r09b": "100", "r10b": "23", "r19": "23"} {
		if !ret.Rows[k].Equal(m(want)) {
			t.Errorf("%s = %s, chcem %s (v eurách)", k, ret.Rows[k], want)
		}
	}
	if len(ret.Warnings) != 1 || !strings.Contains(ret.Warnings[0].Message, "nemá kurz") {
		t.Fatalf("upozornenia: %+v", ret.Warnings)
	}
}

func TestDomesticReverseA2(t *testing.T) {
	p, _ := ParsePeriod("2026-10")
	line := func(desc, q, unit, price string, k invoice.DomesticRC, code string) invoice.Line {
		return invoice.Line{Description: desc, Quantity: m(q), Unit: unit, UnitPrice: m(price), Rate: 23, Reverse: k, Commodity: code}
	}
	build := func(id int64, num string, lines ...invoice.Line) invoice.Invoice {
		i := invoice.Invoice{ID: id, Number: num, Kind: invoice.KindInvoice, Status: invoice.StatusIssued,
			IssueDate: d("2026-10-12"), DeliveryDate: d("2026-10-12"),
			Customer: invoice.Party{Name: "Stavby s.r.o.", ICDPH: "sk2020372640", Country: "SK"}, Lines: lines}
		if err := i.Recalculate(); err != nil {
			t.Fatal(err)
		}
		return i
	}
	steel := build(1, "FA 1",
		line("Projekt", "1", "ks", "200", "", ""),
		line("Montáž rozvádzača v budove", "1", "ks", "1500", invoice.RCConstruction, ""),
		line("Elektroinštalácia", "1", "ks", "500", invoice.RCConstruction, ""),
		line("Nosníky", "2", "t", "2400", invoice.RCMetals, "7216 33"),
		line("Profily", "500", "kg", "2", invoice.RCMetals, "7216 10"),
		line("Plech", "1", "t", "900", invoice.RCMetals, "7208"),
		line("Šrot", "3", "t", "300", invoice.RCScrap, ""),
	)
	credit := build(2, "DB1", line("Montáž", "-1", "ks", "100", invoice.RCConstruction, ""))
	credit.Kind = invoice.KindCreditNote
	ret := Compute(Input{Period: p, Invoices: []invoice.Invoice{steel, credit}})

	// bežná DPH len z projektu; položky s prenesením nejdú do r. 01–04 ani do r. 13
	if ret.Rows["r03"].String() != "200.00" || ret.Rows["r04"].String() != "46.00" || !ret.Rows["r13"].IsZero() {
		t.Fatalf("riadky: r03=%s r04=%s r13=%s", ret.Rows["r03"], ret.Rows["r04"], ret.Rows["r13"])
	}
	if len(ret.KV.A1) != 1 || ret.KV.A1[0].Base.String() != "200.00" {
		t.Fatalf("A.1: %+v", ret.KV.A1)
	}
	type row struct{ code, goods, base, qty, unit string }
	var got []row
	for _, a := range ret.KV.A2 {
		q := ""
		if a.Quantity != nil {
			q = a.Quantity.String()
		}
		if a.Partner != "SK2020372640" || a.Number != "FA1" {
			t.Errorf("hlavička A.2: %+v", a)
		}
		got = append(got, row{a.Code, a.Goods, a.Base.String(), q, a.Unit})
	}
	want := []row{
		{"", "", "2000.00", "", ""},          // stavebné práce spolu, bez množstva
		{"7216", "", "5800.00", "2.00", "t"}, // rovnaký kód: sčíta sa (kg iná jednotka → upozornenie)
		{"7208", "", "900.00", "1.00", "t"},
	}
	if len(got) != len(want) {
		t.Fatalf("A.2: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("A.2[%d] = %+v, chcem %+v", i, got[i], want[i])
		}
	}
	msgs := ""
	for _, w := range ret.Warnings {
		msgs += w.Message + "\n"
	}
	if !strings.Contains(msgs, "rôzne merné jednotky") || !strings.Contains(msgs, "Dobropis k dodaniu s prenesením") {
		t.Fatalf("upozornenia: %s", msgs)
	}

	out := KVXML(ret, Taxpayer{Name: "Demo", City: "Zvolen", ICDPH: "SK2120000003"})
	for _, s := range []string{
		`<A2 Odb="SK2020372640" F="FA1" Den="2026-10-12" Z="2000.00"/>`,
		`<A2 Odb="SK2020372640" F="FA1" Den="2026-10-12" Z="5800.00" TK="7216" Mn="2.00" MJ="t"/>`,
	} {
		if !strings.Contains(string(out), s) {
			t.Errorf("XML bez %s:\n%s", s, out)
		}
	}
	if strings.Index(string(out), "<A2") < strings.Index(string(out), "<A1") {
		t.Error("A2 má byť za A1")
	}
}
