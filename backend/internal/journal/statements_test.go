package journal

import (
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func rowMap(rows []Row) map[string]Row {
	out := map[string]Row{}
	for _, r := range rows {
		out[r.Code] = r
	}
	return out
}

func TestIncomeStatement(t *testing.T) {
	cur := Totals{Columns: map[Column]m.Dec{
		IncGoods: m.MustParse("2900.40"), IncServices: m.MustParse("880.50"), IncOther: m.MustParse("0.49"),
		ExpStock: m.MustParse("81.30"), ExpInsur: m.MustParse("216.12"), ExpOther: m.MustParse("-0.50"), IncVAT: m.MustParse("999"),
	}}
	prev := Totals{Columns: map[Column]m.Dec{IncServices: m.MustParse("100")}}
	r := rowMap(IncomeStatement(cur, prev))
	want := map[string]string{"01": "2900.00", "02": "881.00", "03": "0.00", "04": "3781.00", "05": "81.00", "08": "216.00",
		"10": "-1.00", "11": "296.00", "12": "3485.00"}
	for code, w := range want {
		if r[code].Current.String() != w {
			t.Errorf("r. %s: %s, chcem %s", code, r[code].Current, w)
		}
	}
	if r["02"].Previous.String() != "100.00" || r["12"].Previous.String() != "100.00" || !r["04"].Sum || len(r) != 12 {
		t.Errorf("predchádzajúce obdobie / súčty: %+v", r)
	}
}

func TestAssetStatement(t *testing.T) {
	cur := AssetValues{
		Inputs: AssetInputs{TangibleDM: m.MustParse("4200"), Material: m.MustParse("150.60"), Goods: m.MustParse("49.50"),
			OtherLiab: m.MustParse("300"), Loans: m.MustParse("1000"), OtherRecv: m.MustParse("120")},
		Facts: AssetFacts{Receivables: m.MustParse("37309.99"), Payables: m.MustParse("940.53"), Cash: m.MustParse("548.70"),
			Bank: m.MustParse("16834.72"), Transit: m.MustParse("-200")},
	}
	r := rowMap(AssetStatement(cur, AssetValues{}))
	want := map[string]string{"02": "4200.00", "04": "201.00", "05": "151.00", "06": "50.00", "08": "37430.00", "09": "17384.00",
		"10": "549.00", "11": "16835.00", "13": "-200.00", "15": "59015.00", "17": "1241.00", "18": "1000.00", "20": "2241.00", "21": "56774.00"}
	for code, w := range want {
		if r[code].Current.String() != w {
			t.Errorf("r. %s: %s, chcem %s", code, r[code].Current, w)
		}
	}
	if r["21"].Previous.String() != "0.00" || r["02"].Input != "r02" || r["10"].Input != "" || len(r) != 21 {
		t.Errorf("%+v", r)
	}
}
