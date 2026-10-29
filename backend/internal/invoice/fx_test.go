package invoice

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
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
	if p.NeedsReceipt() {
		t.Fatal("záloha v cudzej mene potrebuje doklad k platbe")
	}
}
