package invoice

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestForeignCurrency(t *testing.T) {
	tests := []struct {
		name     string
		currency string
		rate     string
		local    bool
		field    string
	}{
		{"euro", "EUR", "25", false, ""},
		{"koruny s kurzom", "CZK", "25.123", true, ""},
		{"koruny bez kurzu", "CZK", "0", false, "fxRate"},
		{"neznáma mena", "XYZ", "2", true, "currency"},
	}
	for _, tt := range tests {
		inv := sample()
		inv.Currency, inv.FXRate, inv.FXManual = tt.currency, money.MustParse(tt.rate), true
		if err := inv.Recalculate(); err != nil {
			t.Fatal(err)
		}
		if (inv.Local != nil) != tt.local {
			t.Errorf("%s: Local = %+v", tt.name, inv.Local)
		}
		if tt.currency == "EUR" && (!inv.FXRate.IsZero() || inv.FXManual) {
			t.Errorf("%s: kurz ostal pri eurách", tt.name)
		}
		err := inv.Validate()
		var ve *ValidationError
		switch {
		case tt.field == "" && err != nil:
			t.Errorf("%s: %v", tt.name, err)
		case tt.field != "" && (!errors.As(err, &ve) || !hasField(ve, tt.field)):
			t.Errorf("%s: %v", tt.name, err)
		}
		s, ok := inv.TaxSummary()
		if ok != (tt.currency == "EUR" || tt.local) || (tt.currency == "EUR" && s.Total != inv.Summary.Total) {
			t.Errorf("%s: TaxSummary %v %+v", tt.name, ok, s)
		}
	}
}

func hasField(ve *ValidationError, f string) bool {
	for _, x := range ve.Fields {
		if x.Field == f {
			return true
		}
	}
	return false
}

func TestToEUR(t *testing.T) {
	inv := sample()
	inv.Currency, inv.FXRate = "CZK", money.MustParse("25.123")
	_ = inv.Recalculate()
	l := inv.Local
	// každá sadzba sa prepočíta zvlášť; spolu = súčet zaokrúhlených
	for i, r := range inv.Summary.Rates {
		if l.Rates[i].Base != r.Base.DivCents(inv.FXRate) || l.Rates[i].VAT != r.VAT.DivCents(inv.FXRate) ||
			l.Rates[i].Total != l.Rates[i].Base.Add(l.Rates[i].VAT) {
			t.Fatalf("sadzba %d: %+v", r.Rate, l.Rates[i])
		}
	}
	if l.Total != l.Base.Add(l.VAT) {
		t.Fatalf("spolu: %+v", l)
	}
}

func TestForeignCreditAndAdvance(t *testing.T) {
	inv := sample()
	inv.Currency, inv.FXRate, inv.FXDate = "USD", money.MustParse("1.0823"), MustDate("2026-10-23")
	if err := inv.Issue("FA1"); err != nil {
		t.Fatal(err)
	}
	cn, err := NewCreditNote(inv, MustDate("2026-11-10"))
	if err != nil {
		t.Fatal(err)
	}
	if cn.FXRate != inv.FXRate || cn.FXDate != inv.FXDate || !cn.FXManual || cn.Currency != "USD" {
		t.Fatalf("dobropis: %+v %s %v", cn.FXRate, cn.FXDate, cn.FXManual)
	}
	p := sample()
	p.Kind, p.Currency = KindProforma, "USD"
	if !p.NeedsReceipt() {
		t.Fatal("tuzemská záloha v cudzej mene má doklad k platbe (kurz ku dňu platby)")
	}
}

func TestForeignAdvanceSettlement(t *testing.T) {
	// záloha 1 230 CZK (1 000 + 230 DPH) uhradená pri kurze 25,000 → doklad
	// k platbe 40,00 + 9,20 €; vyúčtovanie 2 460 CZK pri kurze 24,600 →
	// celá dodávka 81,30 + 18,70 €, mínus záloha v eurách podľa dokladu
	p := Invoice{ID: 3, Kind: KindProforma, Number: "ZF1", Status: StatusPaid, PaidDate: MustDate("2026-10-05"), Currency: "CZK",
		Customer: Party{Name: "Odberateľ", Country: "SK"}, PriceMode: vat.PriceNet,
		Lines: []Line{{Description: "Práca", Quantity: money.MustParse("1"), UnitPrice: money.MustParse("1000"), Rate: 23}}}
	_ = p.Recalculate()
	r, err := NewAdvanceReceipt(p, p.Summary.Total, p.PaidDate, MustDate("2026-10-06"))
	if err != nil {
		t.Fatal(err)
	}
	r.ID, r.Number, r.FXRate, r.FXDate = 9, "ZD1", money.MustParse("25"), MustDate("2026-10-04")
	_ = r.Recalculate()
	if r.Currency != "CZK" || !r.Local.Base.Equal(money.MustParse("40")) || !r.Local.VAT.Equal(money.MustParse("9.2")) {
		t.Fatalf("doklad k platbe: %+v", r.Local)
	}
	f, err := NewFinalInvoice(p, []Invoice{r}, MustDate("2026-10-20"), 14)
	if err != nil {
		t.Fatal(err)
	}
	f.Lines[0].Quantity = money.MustParse("2")
	f.FXRate = money.MustParse("24.6")
	_ = f.Recalculate()
	// v mene: 2 460 − 1 230 = 1 230 CZK na úhradu
	if !f.Summary.Total.Equal(money.MustParse("1230")) {
		t.Fatalf("zostatok v CZK: %+v", f.Summary)
	}
	// v eurách: 2000/24,6 = 81,30; 460/24,6 = 18,70 → mínus 40 a 9,20
	if f.Local == nil || !f.Local.Base.Equal(money.MustParse("41.3")) || !f.Local.VAT.Equal(money.MustParse("9.5")) {
		t.Fatalf("zostatok v eurách: %+v", f.Local)
	}
}
