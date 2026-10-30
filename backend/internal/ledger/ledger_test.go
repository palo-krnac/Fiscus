package ledger

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/chart"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func m(s string) money.Dec { return money.MustParse(s) }

func TestCheckLines(t *testing.T) {
	c := chart.Default()
	_ = c.Delete("232")
	cases := []struct {
		name  string
		lines []Line
		err   error // nil = v poriadku; ErrUnbalanced/ErrNoLines alebo len chyba
		ok    bool
	}{
		{"vyrovnané", []Line{{Account: "518", Debit: m("100")}, {Account: "321", Credit: m("100")}}, nil, true},
		{"analytika s bodkou", []Line{{Account: "211", Debit: m("5")}, {Account: "221.", Credit: m("5")}}, nil, true},
		{"jeden riadok", []Line{{Account: "518", Debit: m("100")}}, ErrNoLines, false},
		{"nevyrovnané", []Line{{Account: "518", Debit: m("100")}, {Account: "321", Credit: m("90")}}, ErrUnbalanced, false},
		{"neznámy účet", []Line{{Account: "999", Debit: m("1")}, {Account: "321", Credit: m("1")}}, nil, false},
		{"skrytý účet", []Line{{Account: "232", Debit: m("1")}, {Account: "321", Credit: m("1")}}, nil, false},
		{"obe strany", []Line{{Account: "518", Debit: m("1"), Credit: m("1")}, {Account: "321", Credit: m("0")}}, nil, false},
		{"zlomok centu", []Line{{Account: "518", Debit: m("1.005")}, {Account: "321", Credit: m("1.005")}}, nil, false},
	}
	for _, tc := range cases {
		err := CheckLines(tc.lines, c)
		if (err == nil) != tc.ok || (tc.err != nil && !errors.Is(err, tc.err)) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestMoneySide(t *testing.T) {
	tpl := map[string]chart.Template{}
	for _, x := range chart.DefaultTemplates {
		tpl[x.Code] = x
	}
	cases := map[string]int{"B-ODBERATEL": 1, "B-DODAVATEL": -1, "B-UROKY": 1, "B-VKLAD": -1, "P-TRZBA": 1, "P-NAKUP": -1, "FV-SLUZBY": 0}
	for code, want := range cases {
		if got := MoneySide(tpl[code]); got != want {
			t.Errorf("%s: %d, chcem %d", code, got, want)
		}
	}
}

func TestBook(t *testing.T) {
	c := chart.Default()
	_, _ = c.Save(chart.Account{Number: "221001", Name: "Tatra", Active: true}, "")
	ps := []Posting{
		{Key: "sale:1", Lines: []Line{{Account: "311", Debit: m("123")}, {Account: "602", Credit: m("100")}, {Account: "343", Credit: m("23")}}},
		{Key: "bank:1", Lines: []Line{{Account: "221001", Debit: m("123")}, {Account: "311", Credit: m("123")}}},
		{Key: "bank:2", Problem: "Vyberte predkontáciu"},
		{Key: "bad", Lines: []Line{{Account: "518", Debit: m("1")}}}, // nevyrovnaný sa nezapočíta
	}
	opening := map[string]money.Dec{"221": m("1000"), "411": m("-1000")}
	accs, syn, tot := Book(ps, opening, c)
	got := map[string]Turnover{}
	for _, a := range accs {
		got[a.Account] = a
	}
	if len(accs) != 6 || got["311"].Closing.String() != "0.00" || got["602"].Closing.String() != "-100.00" || got["221001"].Name != "Tatra" {
		t.Fatalf("účty: %+v", accs)
	}
	var s221 Turnover
	for _, s := range syn {
		if s.Account == "221" {
			s221 = s
		}
	}
	if s221.Opening.String() != "1000.00" || s221.Debit.String() != "123.00" || s221.Closing.String() != "1123.00" || len(syn) != 5 {
		t.Fatalf("syntetické: %+v", syn)
	}
	if !tot.Balanced || tot.Debit.String() != "246.00" || tot.OpeningDebit.String() != "1000.00" || tot.ClosingCredit.String() != "1123.00" {
		t.Fatalf("súčty: %+v", tot)
	}
	_, _, tot = Book(ps, map[string]money.Dec{"221": m("5")}, c)
	if tot.Balanced {
		t.Errorf("nevyrovnané počiatočné stavy")
	}
}
