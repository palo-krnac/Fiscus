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
