package invoice

import (
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// stavba: montáž (bežná DPH) + stavebné práce a oceľ s prenesením
func domestic() Invoice {
	inv := sample()
	inv.Lines = []Line{
		{Description: "Projekt", Quantity: m.MustParse("1"), Unit: "ks", UnitPrice: m.MustParse("200"), Rate: vat.Rate23},
		{Description: "Elektroinštalácia budovy", Quantity: m.MustParse("1"), Unit: "ks", UnitPrice: m.MustParse("1500"), Rate: vat.Rate23, Reverse: RCConstruction},
		{Description: "Oceľové nosníky", Quantity: m.MustParse("2.5"), Unit: "t", UnitPrice: m.MustParse("2400"), Rate: vat.Rate23, Reverse: RCMetals, Commodity: "7216 33 10"},
	}
	return inv
}

func TestDomesticReverseSummary(t *testing.T) {
	inv := domestic()
	if err := inv.Recalculate(); err != nil {
		t.Fatal(err)
	}
	// 200 + 23 % DPH; 1500 + 6000 bez DPH (daň platí odberateľ)
	if inv.Summary.Base.String() != "7700.00" || inv.Summary.VAT.String() != "46.00" || inv.Summary.Total.String() != "7746.00" {
		t.Fatalf("súhrn %+v", inv.Summary)
	}
	if n := len(inv.Summary.Rates); n != 2 || !inv.Summary.Rates[1].Reverse || inv.Summary.Rates[1].Base.String() != "7500.00" {
		t.Fatalf("rekapitulácia %+v", inv.Summary.Rates)
	}
	if !inv.DomesticReverse() || !inv.AnyReverse() {
		t.Fatal("prenesenie")
	}
}

func TestDomesticReverseValidate(t *testing.T) {
	cases := []struct {
		name  string
		mod   func(*Invoice)
		field string
	}{
		{"platná", func(*Invoice) {}, ""},
		{"zahraničný odberateľ", func(i *Invoice) {
			i.Customer.Country, i.Customer.ICDPH = "CZ", "CZ12345678"
			i.SupplyKind = SupplyServices
		}, "customer.country"},
		{"odberateľ bez IČ DPH", func(i *Invoice) { i.Customer.ICDPH = "" }, "customer.icdph"},
		{"neplatiteľ", func(i *Invoice) { i.NonVATPayer = true }, "lines"},
		{"prenesenie celej faktúry v tuzemsku", func(i *Invoice) { i.ReverseCharge = true }, "reverseCharge"},
		{"neznámy druh", func(i *Invoice) { i.Lines[1].Reverse = "x" }, "lines[1].reverse"},
		{"oceľ bez kódu", func(i *Invoice) { i.Lines[2].Commodity = "" }, "lines[2].commodity"},
		{"oceľ s kódom plodiny", func(i *Invoice) { i.Lines[2].Commodity = "1001" }, "lines[2].commodity"},
		{"oceľ 7308", func(i *Invoice) { i.Lines[2].Commodity = "7308.90" }, ""},
		{"plodiny s kódom 1201", func(i *Invoice) { i.Lines[2].Reverse, i.Lines[2].Commodity = RCCrops, "1201" }, ""},
		{"jednotka mimo KV", func(i *Invoice) { i.Lines[2].Unit = "bal" }, "lines[2].unit"},
		{"m² sa uvedie ako m", func(i *Invoice) { i.Lines[2].Unit = "m²" }, ""},
		{"oceľ pod 5 000 €", func(i *Invoice) { i.Lines[2].Quantity = m.MustParse("2") }, "lines[2].reverse"},
		{"stavebné práce bez hranice", func(i *Invoice) { i.Lines[1].UnitPrice = m.MustParse("10") }, ""},
		{"dobropis bez hranice", func(i *Invoice) { i.Kind = KindCreditNote; i.Lines[2].Quantity = m.MustParse("-1") }, ""},
		{"hranica v eurách pri cudzej mene", func(i *Invoice) { i.Currency, i.FXRate = "CZK", m.MustParse("25") }, "lines[2].reverse"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv := domestic()
			c.mod(&inv)
			got := map[string]bool{}
			for _, e := range inv.validateDomesticRC() {
				got[e.Field] = true
			}
			if c.field == "" && len(got) > 0 || c.field != "" && !got[c.field] {
				t.Fatalf("chyby %v, chcem %q", got, c.field)
			}
		})
	}
}

func TestKVHelpers(t *testing.T) {
	for in, want := range map[string]string{"7216 33 10": "7216", "10": "10", "": "", "1001.99": "1001"} {
		if got := CommodityCode(in); got != want {
			t.Errorf("CommodityCode(%q) = %q", in, got)
		}
	}
	for in, want := range map[string]string{"t": "t", "KG": "kg", "m3": "m", "bm": "m", "ks.": "ks", "hod": ""} {
		if got := KVUnit(in); got != want {
			t.Errorf("KVUnit(%q) = %q", in, got)
		}
	}
	if !RCChips.Goods() || RCConstruction.Goods() || RCScrap.Reported() || !RCConstruction.Reported() || RCPhones.KVKind() != "MT" {
		t.Fatal("vlastnosti druhov")
	}
}
