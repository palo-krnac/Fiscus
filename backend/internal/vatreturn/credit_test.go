package vatreturn

import (
	"strings"
	"testing"

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

	ret := Compute(Input{Period: p, Purchases: []purchase.Purchase{buy, credit, noNumber, noDeduction, foreign}})
	// odpočet z faktúry ostáva, dobropis zvyšuje daň na úhradu cez r. 28
	if ret.Rows["r19"].String() != "230.00" || ret.Rows["r28"].String() != "46.00" {
		t.Fatalf("riadky: %+v", ret.Rows)
	}
	if want := "184.00"; ret.Deduction.Sub(ret.Rows["r28"]).String() != want {
		t.Fatalf("odpočet po oprave: %s - %s", ret.Deduction, ret.Rows["r28"])
	}
	if len(ret.KV.C2) != 1 {
		t.Fatalf("C.2: %+v", ret.KV.C2)
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
	if len(msgs) != 2 || !strings.Contains(msgs[0], "čísla pôvodnej faktúry") || !strings.Contains(msgs[1], "zahraničného") {
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
