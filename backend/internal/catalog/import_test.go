package catalog

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestParseNumber(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1 234,50", "1234.50"}, {"1234.5", "1234.50"}, {"23 %", "23.00"}, {"12,50 €", "12.50"},
		{"1.234,56", "1234.56"}, {"0", "0.00"}, {"1 000", "1000.00"},
	}
	for _, tt := range tests {
		got, err := parseNumber(tt.in)
		if err != nil || got.String() != money.MustParse(tt.want).String() {
			t.Errorf("%q → %s %v", tt.in, got, err)
		}
	}
	if _, err := parseNumber("dvanásť"); err == nil {
		t.Error("text ako číslo")
	}
}

func TestParseItems(t *testing.T) {
	rows := [][]string{
		{"Kód", "Názov", "MJ", "Cena bez DPH", "DPH (%)", "Cena s DPH", "Zadaná cena", "Archivovaná", "Poznámka"},
		{"PLC1", "Programovanie PLC", "hod", "48", "23", "59,04", "bez DPH", "", "interné"},
		{"KAB", "Kábel CYKY", "m", "1,78", "23", "2,19", "s DPH", "áno", ""},
		{"", "Kniha", "", "19.90", "5 %", "", "", "", ""},
		{"X", "", "ks", "10", "", "", "", "", ""},
		{"Y", "Zlá cena", "ks", "desať", "23", "", "", "", ""},
		{"Z", "Zlá sadzba", "ks", "10", "21,5", "", "", "", ""},
		{"W", "Záporná", "ks", "-1", "", "", "", "", ""},
	}
	got, err := ParseItems(rows, vat.Rate23)
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		code, name, unit, price string
		mode                    vat.PriceMode
		rate                    vat.Rate
		archived                bool
		errField                string
	}
	wants := []want{
		{"PLC1", "Programovanie PLC", "hod", "48", vat.PriceNet, 23, false, ""},
		{"KAB", "Kábel CYKY", "m", "2.19", vat.PriceGross, 23, true, ""},
		{"", "Kniha", "ks", "19.90", vat.PriceNet, 5, false, ""},
		{"X", "", "ks", "10", vat.PriceNet, 23, false, "name"},
		{"Y", "Zlá cena", "ks", "0", vat.PriceNet, 23, false, "price"},
		{"Z", "Zlá sadzba", "ks", "10", vat.PriceNet, 23, false, "vatRate"},
		{"W", "Záporná", "ks", "0", vat.PriceNet, 23, false, "price"},
	}
	for i, w := range wants {
		r := got[i]
		it := r.Item
		if r.Line != i+2 || it.Code != w.code || it.Name != w.name || it.Unit != w.unit || it.Price.String() != money.MustParse(w.price).String() ||
			it.PriceMode != w.mode || it.Rate != w.rate || it.Archived != w.archived {
			t.Errorf("riadok %d: %+v", r.Line, it)
		}
		if (w.errField == "") != (len(r.Errors) == 0) || (w.errField != "" && r.Errors[0].Field != w.errField) {
			t.Errorf("riadok %d: chyby %+v", r.Line, r.Errors)
		}
	}
	if got[0].Item.Note != "interné" {
		t.Error("poznámka")
	}
}

func TestParseItemsHeaders(t *testing.T) {
	tests := []struct {
		name string
		rows [][]string
		err  string
	}{
		{"prázdny", nil, "prázdny"},
		{"bez názvu", [][]string{{"Kód", "Cena"}, {"1", "2"}}, "Názov"},
		{"bez ceny", [][]string{{"Názov", "MJ"}, {"a", "ks"}}, "Cena"},
		{"iba hlavička", [][]string{{"nazov", "cena"}}, "žiadne položky"},
		{"iné názvy stĺpcov", [][]string{{"POPIS", "Jednotka", "Cena s DPH"}, {"Servis", "hod", "60"}}, ""},
	}
	for _, tt := range tests {
		got, err := ParseItems(tt.rows, vat.Rate23)
		if tt.err == "" {
			if err != nil || got[0].Item.PriceMode != vat.PriceGross || got[0].Item.Unit != "hod" {
				t.Errorf("%s: %+v %v", tt.name, got, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tt.err) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

func TestMatch(t *testing.T) {
	existing := []Item{{ID: 1, Code: "PLC1", Name: "Starý názov"}, {ID: 2, Name: "Kniha"}, {ID: 3, Code: "K9", Name: "Kábel"}}
	rows := []ImportRow{
		{Line: 2, Action: "create", Item: Item{Code: "plc1", Name: "Programovanie"}},
		{Line: 3, Action: "create", Item: Item{Name: "kniha"}},
		{Line: 4, Action: "create", Item: Item{Name: "Kábel"}}, // existujúci má kód → nová položka
		{Line: 5, Action: "create", Item: Item{Code: "NOVA", Name: "Nová"}},
		{Line: 6, Action: "create", Item: Item{Code: "nova", Name: "Duplicitná"}},
		{Line: 7, Action: "create", Item: Item{Code: "C"}, Errors: []invoice.FieldError{{Field: "name"}}},
	}
	Match(rows, existing)
	want := []struct {
		action string
		id     int64
	}{{"update", 1}, {"update", 2}, {"create", 0}, {"create", 0}, {"skip", 0}, {"skip", 0}}
	for i, w := range want {
		if rows[i].Action != w.action || rows[i].Item.ID != w.id {
			t.Errorf("riadok %d: %s %d %+v", rows[i].Line, rows[i].Action, rows[i].Item.ID, rows[i].Errors)
		}
	}
	if !strings.Contains(rows[4].Errors[0].Message, "riadku 5") {
		t.Errorf("duplicita: %+v", rows[4].Errors)
	}
}
