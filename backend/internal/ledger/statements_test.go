package ledger

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func turn(pairs ...string) []Turnover {
	var out []Turnover
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, Turnover{Account: pairs[i], Closing: m(pairs[i+1])})
	}
	return out
}

func rowsBy(rs []StatementRow) map[string]StatementRow {
	out := map[string]StatementRow{}
	for _, r := range rs {
		out[r.Code] = r
	}
	return out
}

func TestStatements(t *testing.T) {
	// vklad 5000 (221/491), predaj služieb 1000 + 230 DPH, nákup materiálu 400 + 92,
	// auto 12000 s oprávkami 3000 (odpisy), neuhradená DPH, 343 preplatok minulý mesiac
	cur := turn(
		"221001", "5838.00", // banka
		"211", "100.40", // pokladnica
		"022", "12000", "082", "-3000", // auto a oprávky
		"311", "1230", // odberateľ
		"321", "-492", // dodávateľ
		"343", "-138", // DPH na úhradu
		"491", "-14000", "461", "-500",
		"602", "-1000.40", "501", "400", "551", "3000",
		"568", "0.40",
	)
	st := BuildStatements(cur, nil)
	a, l, i := rowsBy(st.Assets), rowsBy(st.Liabilities), rowsBy(st.Income)
	cases := []struct {
		name string
		got  money.Dec
		want string
	}{
		{"DHM brutto", a["06"].Brutto, "12000.00"},
		{"DHM korekcia", a["06"].Korekcia, "3000.00"},
		{"DHM netto", a["06"].Netto, "9000.00"},
		{"neobežný", a["02"].Netto, "9000.00"},
		{"pohľadávky", a["18"].Netto, "1230.00"},
		{"peniaze (zaokrúhlené)", a["22"].Netto, "5938.00"},
		{"spolu majetok", a["01"].Netto, "16168.00"},
		{"tržby", i["03"].Netto, "1000.00"},
		{"materiál", i["10"].Netto, "400.00"},
		{"odpisy", i["14"].Netto, "3000.00"},
		{"VH hosp. činnosť", i["18"].Netto, "-2400.00"},
		{"pridaná hodnota", i["19"].Netto, "600.00"},
		{"VH fin.", i["34"].Netto, "0.00"},
		{"VH po zdanení", i["38"].Netto, "-2400.00"},
		{"VH v súvahe", l["33"].Netto, "-2400.00"},
		{"základné imanie FO", l["27"].Netto, "14000.00"},
		{"dodávatelia", l["39"].Netto, "492.00"},
		{"daňové záväzky", l["41"].Netto, "138.00"},
		{"dlhodobé úvery", l["37"].Netto, "500.00"},
		{"spolu pasíva", l["24"].Netto, "12730.00"},
	}
	for _, c := range cases {
		if c.got.String() != c.want {
			t.Errorf("%s: %s, chcem %s", c.name, c.got, c.want)
		}
	}
	if len(st.Unmapped) != 0 {
		t.Errorf("nepriradené: %+v", st.Unmapped)
	}
	if st.Difference.String() != "3438.00" {
		t.Errorf("rozdiel: %s", st.Difference) // vstup nie je vyrovnaný — kontrola ho ukáže
	}
	// 343 s pohľadávkou ide do aktív, neznámy účet do nepriradených
	st = BuildStatements(turn("343", "50", "395", "0", "702", "10", "999", "5"), turn("311", "100"))
	if a := rowsBy(st.Assets); a["19"].Netto.String() != "50.00" || a["18"].Previous.String() != "100.00" || len(st.Unmapped) != 0 {
		t.Errorf("A/P a minulé obdobie: %+v %+v", a["19"], st.Unmapped)
	}
}

func TestOpeningFromClosing(t *testing.T) {
	ls := OpeningFromClosing(turn("221", "1500", "311", "200", "491", "-1000", "602", "-1000", "501", "300", "431", "0"))
	want := "221 MD 1500.00, 311 MD 200.00, 431 D 700.00, 491 D 1000.00"
	got := ""
	for i, l := range ls {
		if i > 0 {
			got += ", "
		}
		if !l.Debit.IsZero() {
			got += l.Account + " MD " + l.Debit.String()
		} else {
			got += l.Account + " D " + l.Credit.String()
		}
	}
	if got != want || !Balanced(ls) {
		t.Errorf("%s", got)
	}
}
