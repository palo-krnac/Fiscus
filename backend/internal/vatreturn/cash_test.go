package vatreturn

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func cashDoc(id int64, kind cash.Kind, date, icdph string, rows map[vat.Rate]string) cash.Document {
	doc := cash.Document{ID: id, Number: "D" + string(rune('0'+id)), Kind: kind, Date: d(date), Partner: "Predajca " + icdph, Purpose: "x",
		Tax: &cash.Tax{SupplierICDPH: icdph}}
	for _, r := range []vat.Rate{23, 19, 5, 0} {
		if g, ok := rows[r]; ok {
			doc.Tax.Rows = append(doc.Tax.Rows, cash.VATRow{Rate: r, Gross: m(g)})
			doc.Amount = doc.Amount.Add(m(g))
		}
	}
	doc.NormalizeTax()
	return doc
}

func TestCashDocuments(t *testing.T) {
	p, _ := ParsePeriod("2026-10")
	fuel := cashDoc(1, cash.Out, "2026-10-05", "SK2020372640", map[vat.Rate]string{23: "61.50"})
	shop := cashDoc(2, cash.Out, "2026-10-08", "SK2020372640", map[vat.Rate]string{23: "12.30", 5: "10.50"})
	other := cashDoc(3, cash.Out, "2026-10-09", "SK2120000003", map[vat.Rate]string{19: "11.90"})
	lunch := cashDoc(4, cash.Out, "2026-10-09", "", map[vat.Rate]string{23: "24.60"})
	lunch.Tax.NoDeduction = true
	noID := cashDoc(5, cash.Out, "2026-10-10", "", map[vat.Rate]string{23: "6.15"})
	sales := cashDoc(6, cash.In, "2026-10-31", "", map[vat.Rate]string{23: "123.00", 5: "21.00", 0: "5.00"})
	cancelled := cashDoc(7, cash.In, "2026-10-31", "", map[vat.Rate]string{23: "1000"})
	cancelled.Cancelled = true
	november := cashDoc(8, cash.Out, "2026-11-01", "SK2020372640", map[vat.Rate]string{23: "123"})
	linked := cashDoc(9, cash.Out, "2026-10-02", "SK2020372640", map[vat.Rate]string{23: "123"})
	linked.LinkKind = "purchase"
	plain := cash.Document{ID: 10, Kind: cash.In, Date: d("2026-10-03"), Amount: m("500"), Purpose: "Vklad"}

	ret := Compute(Input{Period: p, CashDocs: []cash.Document{fuel, shop, other, lunch, noID, sales, cancelled, november, linked, plain}})
	rows := map[string]string{
		"r19": "13.80", "r21": "13.80", // 11.50 + 2.30 (23 %)
		"r18": "1.90", "r20": "1.90", // 19 %
		"r18a": "0.50", "r20a": "0.50", // 5 %
		"r03": "100.00", "r04": "23.00", "r01a": "20.00", "r02a": "1.00", "r13": "5.00",
	}
	for k, want := range rows {
		if got := ret.Rows[k].String(); got != want {
			t.Errorf("%s = %s, chcem %s", k, got, want)
		}
	}
	if ret.Cash != 4 {
		t.Errorf("zahrnuté doklady: %d", ret.Cash)
	}
	b := ret.KV.B31
	if b == nil || b.Count != 3 || b.Base.String() != "80.00" || b.VAT.String() != "16.20" || b.Deducted.String() != "16.20" || len(ret.KV.B32) != 0 {
		t.Fatalf("B.3.1: %+v %+v", b, ret.KV.B32)
	}
	if d1 := ret.KV.D1; d1.Count != 1 || d1.Base.String() != "100.00" || d1.VAT.String() != "23.00" || d1.BaseReduced.String() != "20.00" || d1.VATReduced.String() != "1.00" {
		t.Fatalf("D.1: %+v", d1)
	}
	var warns []string
	for _, w := range ret.Warnings {
		warns = append(warns, w.Message)
	}
	if len(warns) != 2 || !strings.Contains(warns[0], "bez IČ DPH") || !strings.Contains(warns[1], "B.3.1") {
		t.Fatalf("upozornenia: %q", warns)
	}
	kv := string(KVXML(ret, Taxpayer{Name: "Firma", DIC: "2020372640", ICDPH: "SK2020372640", City: "Zvolen"}))
	if !strings.Contains(kv, `<D1 Z="100.00" D="23.00" ZZn="20.00" DZn="1.00"/>`) {
		t.Fatalf("D1 v XML:\n%s", kv)
	}

	// od 3 000 € odpočtu po dodávateľoch (B.3.2)
	big := cashDoc(11, cash.Out, "2026-10-20", "SK2120000003", map[vat.Rate]string{23: "18450"})
	ret = Compute(Input{Period: p, CashDocs: []cash.Document{fuel, shop, other, big}})
	if ret.KV.B31 != nil || len(ret.KV.B32) != 2 {
		t.Fatalf("B.3.2: %+v %+v", ret.KV.B31, ret.KV.B32)
	}
	if s := ret.KV.B32[0]; s.Partner != "SK2020372640" || s.Count != 2 || s.Deducted.String() != "14.30" {
		t.Fatalf("dodávateľ 1: %+v", s)
	}
	if s := ret.KV.B32[1]; s.Partner != "SK2120000003" || s.Count != 2 || s.Deducted.String() != "3451.90" {
		t.Fatalf("dodávateľ 2: %+v", s)
	}
}
