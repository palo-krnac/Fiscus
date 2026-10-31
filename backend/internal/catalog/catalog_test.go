package catalog

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestPrices(t *testing.T) {
	cases := []struct {
		price string
		mode  vat.PriceMode
		rate  vat.Rate
		net   string
		gross string
	}{
		{"100", vat.PriceNet, 23, "100.00", "123.00"},
		{"9.99", vat.PriceNet, 23, "9.99", "12.29"},
		{"123", vat.PriceGross, 23, "100.00", "123.00"},
		{"10", vat.PriceGross, 19, "8.40", "10.00"},
		{"0.45", vat.PriceNet, 5, "0.45", "0.47"},
		{"50", vat.PriceNet, 0, "50.00", "50.00"},
		{"-20", vat.PriceNet, 23, "-20.00", "-24.60"}, // zľava
	}
	for _, c := range cases {
		p := Item{Price: m.MustParse(c.price), PriceMode: c.mode, Rate: c.rate}.Prices()
		if p.Net.String() != c.net || p.Gross.String() != c.gross {
			t.Errorf("%s %s %d %%: %s / %s, chcel som %s / %s", c.price, c.mode, c.rate, p.Net, p.Gross, c.net, c.gross)
		}
	}
}

func TestNormalizeValidate(t *testing.T) {
	it := Item{Name: "  Montáž  ", Code: " M1 ", Rate: 23}
	it.Normalize()
	if it.Name != "Montáž" || it.Code != "M1" || it.Unit != "ks" || it.PriceMode != vat.PriceNet {
		t.Fatalf("normalizácia: %+v", it)
	}
	if err := it.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		edit  func(*Item)
		field string
	}{
		{func(i *Item) { i.Name = "" }, "name"},
		{func(i *Item) { i.Code = "012345678901234567890123456789X" }, "code"},
		{func(i *Item) { i.Unit = "veľmidlhájednotka" }, "unit"},
		{func(i *Item) { i.PriceMode = "x" }, "priceMode"},
		{func(i *Item) { i.Rate = 7 }, "vatRate"},
	}
	for _, c := range cases {
		x := it
		c.edit(&x)
		var ve *invoice.ValidationError
		if err := x.Validate(); !errors.As(err, &ve) || ve.Fields[0].Field != c.field {
			t.Errorf("%s: %v", c.field, err)
		}
	}
}

func TestValidEAN(t *testing.T) {
	tests := map[string]bool{
		"8586000000006": false, "5901234123457": true, "96385074": true, "036000291452": true, "15400141288763": true,
		"5901234123458": false, "590123412345": false, "59012341234a7": false, "": false,
	}
	for in, want := range tests {
		if got := ValidEAN(in); got != want {
			t.Errorf("ValidEAN(%q) = %v", in, got)
		}
	}
}

func TestItemCard(t *testing.T) {
	base := func() Item {
		return Item{Name: "Kábel CYKY 3x2,5", Unit: "m", PriceMode: "net", Rate: 23}
	}
	tests := []struct {
		name  string
		edit  func(*Item)
		field string
	}{
		{"platná karta", func(it *Item) {
			it.EAN, it.Category, it.Brand = "5901234123457", "Káble", "NKT"
			it.Params = []Param{{Name: "Prierez", Value: "2,5", Unit: "mm²"}, {Name: " ", Value: ""}}
		}, ""},
		{"zlý EAN", func(it *Item) { it.EAN = "5901234123458" }, "ean"},
		{"parameter bez mena", func(it *Item) { it.Params = []Param{{Value: "5"}} }, "params[0].name"},
		{"parameter dvakrát", func(it *Item) { it.Params = []Param{{Name: "Farba", Value: "a"}, {Name: "farba", Value: "b"}} }, "params[1].name"},
		{"záporná hmotnosť", func(it *Item) { it.Weight = m.FromInt(-1) }, "weight"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := base()
			tt.edit(&it)
			it.Normalize()
			err := it.Validate()
			if tt.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				if len(it.Params) != 1 || it.EAN != "5901234123457" {
					t.Errorf("karta %+v", it)
				}
				return
			}
			ve, ok := err.(*invoice.ValidationError)
			found := false
			for _, f := range func() []invoice.FieldError {
				if ok {
					return ve.Fields
				}
				return nil
			}() {
				found = found || f.Field == tt.field
			}
			if !found {
				t.Errorf("chcem chybu %s, mám %v", tt.field, err)
			}
		})
	}
}
