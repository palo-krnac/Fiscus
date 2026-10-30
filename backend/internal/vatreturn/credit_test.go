package vatreturn

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
)

func TestPurchaseCreditNotes(t *testing.T) {
	p, _ := ParsePeriod("2026-10")
	buy := pur(1, "P1", "2026-10-05", "SK2120455106", 23, "1000", "230")
	credit := pur(2, "P2", "2026-10-20", "SK2120455106", 23, "-200", "-46")
	credit.CorrectsNumber = "FV 2026/15"
	noNumber := pur(3, "P3", "2026-10-21", "SK2120455106", 23, "-10", "-2.30")
	noDeduction := pur(4, "P4", "2026-10-22", "SK2120455106", 23, "-50", "-11.50")
	noDeduction.NoDeduction = true
	foreign := pur(5, "P5", "2026-10-23", "", 23, "-100", "-23")
	foreign.Supplier.Country, foreign.ReverseCharge, foreign.CorrectsNumber = "CZ", true, "F1"
	foreign.SupplyKind = invoice.SupplyServices
	foreign.Supplier.ICDPH = "cz12345678"
	euGoods := pur(6, "P6", "2026-10-24", "", 23, "-100", "-23")
	euGoods.Supplier.Country, euGoods.ReverseCharge, euGoods.CorrectsNumber, euGoods.SupplyKind = "DE", true, "R7", invoice.SupplyGoods

	ret := Compute(Input{Period: p, Purchases: []purchase.Purchase{buy, credit, noNumber, noDeduction, foreign, euGoods}})
	// odpočet z faktúry ostáva, dobropis zvyšuje daň na úhradu cez r. 28;
	// dobropis k službe zo zahraničia zníži samozdanenie (r. 09b, 10b) aj odpočet (r. 19)
	// dobropis k tovaru z DE bez nájdenej pôvodnej faktúry: oprava v tomto období (r. 07–08)
	if ret.Rows["r19"].String() != "184.00" || ret.Rows["r28"].String() != "46.00" ||
		ret.Rows["r09b"].String() != "-100.00" || ret.Rows["r10b"].String() != "-23.00" ||
		ret.Rows["r07"].String() != "-100.00" || ret.Rows["r08"].String() != "-23.00" {
		t.Fatalf("riadky: %+v", ret.Rows)
	}
	if want := "138.00"; ret.Deduction.Sub(ret.Rows["r28"]).String() != want {
		t.Fatalf("odpočet po oprave: %s - %s", ret.Deduction, ret.Rows["r28"])
	}
	if len(ret.KV.C2) != 3 {
		t.Fatalf("C.2: %+v", ret.KV.C2)
	}
	if f := ret.KV.C2[1]; f.Partner != "CZ12345678" || f.Original != "F1" || f.BaseDiff.String() != "-100.00" || f.DeductedDiff.String() != "-23.00" {
		t.Fatalf("C.2 služba zo zahraničia: %+v", f)
	}
	c := ret.KV.C2[0]
	if c.Partner != "SK2120455106" || c.Number != "DP2" || c.Original != "FV2026/15" || c.BaseDiff.String() != "-200.00" ||
		c.VATDiff.String() != "-46.00" || c.DeductedDiff.String() != "-46.00" || c.Rate != 23 {
		t.Fatalf("riadok C.2: %+v", c)
	}
	var msgs []string
	for _, w := range ret.Warnings {
		msgs = append(msgs, w.Message)
	}
	if len(msgs) != 2 || !strings.Contains(msgs[0], "čísla pôvodnej faktúry") || !strings.Contains(msgs[1], "R7 sa vo Fiškuse nenašla") {
		t.Fatalf("upozornenia: %q", msgs)
	}
	kv := string(KVXML(ret, Taxpayer{Name: "Firma", DIC: "2020372640", ICDPH: "SK2020372640", City: "Zvolen"}))
	if !strings.Contains(kv, `<C2 Dod="SK2120455106" FO="DP2" FP="FV2026/15" ZR="-200.00" DR="-46.00" S="23" OR="-46.00"/>`) {
		t.Fatalf("XML:\n%s", kv)
	}
	if !strings.Contains(string(DPHXML(ret, Taxpayer{Name: "Firma", DIC: "2020372640", City: "Zvolen"}, d("2026-11-10"))), "<r28>46.00</r28>") {
		t.Fatal("r28 v XML priznania")
	}
}

// TestA2CreditNote: dobropis k dodaniu s prenesením ide do C.1 bez dane
// a sadzby; pri tovare s kódom a rozdielom množstva so znamienkom mínus.
func TestA2CreditNote(t *testing.T) {
	p, _ := ParsePeriod("2026-10")
	cn := invoice.Invoice{ID: 9, Kind: invoice.KindCreditNote, Number: "DB 9", Status: invoice.StatusIssued,
		IssueDate: d("2026-10-20"), DeliveryDate: d("2026-10-20"),
		Customer:   invoice.Party{Name: "Stavby s.r.o.", ICDPH: "sk2020372640", Country: "SK"},
		Correction: &invoice.Correction{OriginalNumber: "FA 1"},
		Lines: []invoice.Line{
			{Description: "Mobilné telefóny – vrátené", Quantity: m("-100"), Unit: "ks", UnitPrice: m("80"), Rate: 23, Reverse: invoice.RCPhones},
			{Description: "Nosníky", Quantity: m("-0.5"), Unit: "t", UnitPrice: m("2400"), Rate: 23, Reverse: invoice.RCMetals, Commodity: "7216 33"},
			{Description: "Montáž – zľava", Quantity: m("1"), Unit: "ks", UnitPrice: m("-300"), Rate: 23, Reverse: invoice.RCConstruction},
		}}
	if err := cn.Recalculate(); err != nil {
		t.Fatal(err)
	}
	ret := Compute(Input{Period: p, Invoices: []invoice.Invoice{cn}})
	type row struct{ base, code, goods, qty, unit string }
	var got []row
	for _, c := range ret.KV.C1 {
		q := ""
		if c.Quantity != nil {
			q = c.Quantity.String()
		}
		if !c.A2 || c.Partner != "SK2020372640" || c.Number != "DB9" || c.Original != "FA1" {
			t.Errorf("hlavička C.1: %+v", c)
		}
		got = append(got, row{c.BaseDiff.String(), c.Code, c.Goods, q, c.Unit})
	}
	want := []row{
		{"-8000.00", "", "MT", "-100.00", "ks"},
		{"-1200.00", "7216", "", "-0.50", "t"},
		{"-300.00", "", "", "", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("C.1: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("C.1[%d] = %+v, chcem %+v", i, got[i], want[i])
		}
	}
	if len(ret.Rows) != 0 && (!ret.Rows["r24"].IsZero() || !ret.Rows["r03"].IsZero()) {
		t.Errorf("dobropis s prenesením nejde do riadkov dane: %+v", ret.Rows)
	}
	if len(ret.Warnings) != 0 {
		t.Errorf("upozornenia: %+v", ret.Warnings)
	}
	kv := string(KVXML(ret, Taxpayer{Name: "Firma", DIC: "2020372640", ICDPH: "SK2020372640", City: "Zvolen"}))
	for _, s := range []string{
		`<C1 Odb="SK2020372640" FO="DB9" FP="FA1" ZR="-8000.00" TD="MT" Mn="-100.00" MJ="ks"/>`,
		`<C1 Odb="SK2020372640" FO="DB9" FP="FA1" ZR="-1200.00" TK="7216" Mn="-0.50" MJ="t"/>`,
		`<C1 Odb="SK2020372640" FO="DB9" FP="FA1" ZR="-300.00"/>`,
	} {
		if !strings.Contains(kv, s) {
			t.Errorf("XML bez %s:\n%s", s, kv)
		}
	}
}

// TestAcquisitionCreditPeriod: dobropis k nadobudnutiu tovaru z EÚ prijatý
// v novembri opravuje október (r. 07–08, 19 — dodatočné priznanie); C.2 je
// v novembri.
func TestAcquisitionCreditPeriod(t *testing.T) {
	buy := pur(1, "P1", "2026-10-05", "DE123456789", 23, "1000", "230")
	buy.Supplier.Country, buy.ReverseCharge, buy.SupplyKind, buy.SupplierNumber = "DE", true, invoice.SupplyGoods, "RE-77"
	buy.Normalize()
	cn := pur(2, "P2", "2026-11-04", "DE123456789", 23, "-200", "-46")
	cn.Supplier.Country, cn.ReverseCharge, cn.SupplyKind, cn.CorrectsNumber = "DE", true, invoice.SupplyGoods, "RE 77"
	cn.Normalize()
	all := []purchase.Purchase{buy, cn}

	oct, _ := ParsePeriod("2026-10")
	ret := Compute(Input{Period: oct, Purchases: all})
	if ret.Rows["r07"].String() != "800.00" || ret.Rows["r08"].String() != "184.00" || ret.Rows["r19"].String() != "184.00" {
		t.Fatalf("október (opravený): %+v", ret.Rows)
	}
	if len(ret.KV.C2) != 0 || len(ret.KV.B1) != 1 || len(ret.Warnings) != 1 || !strings.Contains(ret.Warnings[0].Message, "dodatočné") {
		t.Fatalf("október KV/upozornenia: %+v %+v", ret.KV.C2, ret.Warnings)
	}

	nov, _ := ParsePeriod("2026-11")
	ret = Compute(Input{Period: nov, Purchases: all})
	if !ret.Rows["r07"].IsZero() || !ret.Rows["r08"].IsZero() || !ret.Rows["r19"].IsZero() {
		t.Fatalf("november bez opravy riadkov: %+v", ret.Rows)
	}
	if len(ret.KV.C2) != 1 || ret.KV.C2[0].BaseDiff.String() != "-200.00" || ret.KV.C2[0].Original != "RE77" {
		t.Fatalf("november C.2: %+v", ret.KV.C2)
	}
	if len(ret.Warnings) != 1 || !strings.Contains(ret.Warnings[0].Message, "október 2026") {
		t.Fatalf("november upozornenie: %+v", ret.Warnings)
	}
}
