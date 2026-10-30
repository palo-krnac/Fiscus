package chart

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestDefaultTemplates(t *testing.T) {
	c := Default()
	seen := map[string]bool{}
	for _, tp := range DefaultTemplates {
		x := tp
		if err := c.ValidateTemplate(&x); err != nil {
			t.Errorf("%s: %v", tp.Code, err)
		}
		if seen[tp.Code] || !tp.Active {
			t.Errorf("%s: duplicitný alebo neaktívny", tp.Code)
		}
		seen[tp.Code] = true
	}
}

func TestTemplateApply(t *testing.T) {
	m := money.MustParse
	var fp Template
	for _, tp := range DefaultTemplates {
		if tp.Code == "FP-SLUZBY" {
			fp = tp
		}
	}
	tests := []struct {
		name string
		a    Amounts
		want []Entry
	}{
		{"bežná faktúra s 50 % odpočtom", Amounts{Total: m("123"), Base: m("100"), VAT: m("11.50"), VATNon: m("11.50")}, []Entry{
			{Account: "518", Debit: m("111.50")}, {Account: "343", Debit: m("11.50")}, {Account: "321", Credit: m("123")}}},
		{"prenesenie", Amounts{Total: m("100"), Base: m("100"), Self: m("23")}, []Entry{
			{Account: "518", Debit: m("100")}, {Account: "321", Credit: m("100")}, {Account: "343", Debit: m("23")}, {Account: "343", Credit: m("23")}}},
		{"neplatiteľ", Amounts{Total: m("50"), Base: m("50")}, []Entry{
			{Account: "518", Debit: m("50")}, {Account: "321", Credit: m("50")}}},
	}
	for _, tt := range tests {
		got := fp.Apply(tt.a)
		if len(got) != len(tt.want) {
			t.Fatalf("%s: %+v", tt.name, got)
		}
		for i := range got {
			if got[i].Account != tt.want[i].Account || !got[i].Debit.Equal(tt.want[i].Debit) || !got[i].Credit.Equal(tt.want[i].Credit) {
				t.Errorf("%s[%d] = %+v, chcem %+v", tt.name, i, got[i], tt.want[i])
			}
		}
	}
}

func TestValidateTemplate(t *testing.T) {
	c := Default()
	c.Accounts = append(c.Accounts, Account{Number: "221001", Name: "Tatra", Type: Asset, Active: true})
	_ = c.Delete("232") // skrytý
	tests := []struct {
		name  string
		tp    Template
		field string
	}{
		{"platná", Template{Code: "b-tatra", Name: "Úhrada cez Tatru", Doc: DocBank, Lines: []TemplateLine{{Debit, "321", PartTotal}, {Credit, "221.001", PartTotal}}}, ""},
		{"nevyrovnaná", Template{Code: "X", Name: "x", Doc: DocPurchase, Lines: []TemplateLine{{Debit, "518", PartBase}, {Credit, "321", PartTotal}}}, "lines"},
		{"samozdanenie iba na MD", Template{Code: "X", Name: "x", Doc: DocPurchase, Lines: []TemplateLine{{Debit, "518", PartBase}, {Debit, "343", PartVAT}, {Debit, "518", PartVATNon}, {Credit, "321", PartTotal}, {Debit, "343", PartSelf}}}, "lines"},
		{"neznámy účet", Template{Code: "X", Name: "x", Doc: DocBank, Lines: []TemplateLine{{Debit, "999", PartTotal}, {Credit, "221", PartTotal}}}, "lines[0].account"},
		{"skrytý účet", Template{Code: "X", Name: "x", Doc: DocBank, Lines: []TemplateLine{{Debit, "232", PartTotal}, {Credit, "221", PartTotal}}}, "lines[0].account"},
		{"jeden riadok", Template{Code: "X", Name: "x", Doc: DocBank, Lines: []TemplateLine{{Debit, "221", PartTotal}}}, "lines"},
		{"bez druhu", Template{Code: "X", Name: "x", Lines: []TemplateLine{{Debit, "321", PartTotal}, {Credit, "221", PartTotal}}}, "doc"},
		{"zlá strana", Template{Code: "X", Name: "x", Doc: DocBank, Lines: []TemplateLine{{"X", "321", PartTotal}, {Credit, "221", PartTotal}}}, "lines[0].side"},
	}
	for _, tt := range tests {
		x := tt.tp
		if f := field(c.ValidateTemplate(&x)); f != tt.field {
			t.Errorf("%s: pole %q, chcem %q", tt.name, f, tt.field)
		}
	}
	x := tests[0].tp
	_ = c.ValidateTemplate(&x)
	if x.Code != "B-TATRA" || x.Lines[1].Account != "221001" {
		t.Errorf("normalizácia: %+v", x)
	}
	var list []Template
	if n := SeedTemplates(&list, []string{"B-UROKY"}); n != len(DefaultTemplates)-1 || list[0].Doc != DocSale {
		t.Errorf("doplnenie: %d, prvá %+v", n, list[0])
	}
}
