package invoice

import (
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestFormatNumberWith(t *testing.T) {
	cases := []struct {
		f        string
		y, mo, n int
		want, vs string
	}{
		{"FA{RRRR}{NNNN}", 2026, 10, 7, "FA20260007", "20260007"},
		{"{RRRR}/{NNN}", 2026, 1, 42, "2026/042", "2026042"},
		{"DB-{RR}-{NNNNN}", 2026, 3, 12, "DB-26-00012", "2600012"},
		{"{RR}{MM}{NNNN}", 2026, 3, 5, "26030005", "26030005"},
		{"FA{RRRR}{NN}", 2026, 1, 123, "FA2026123", "2026123"},         // pretečenie šírky nevadí
		{"{RRRR}{NNNNNNNN}", 2026, 1, 1, "202600000001", "2600000001"}, // VS max 10 číslic
	}
	for _, c := range cases {
		got := FormatNumberWith(c.f, c.y, c.mo, c.n)
		if got != c.want || VariableSymbol(got) != c.vs {
			t.Errorf("%s → %s (VS %s), chcem %s (VS %s)", c.f, got, VariableSymbol(got), c.want, c.vs)
		}
	}
}

func TestValidateFormat(t *testing.T) {
	ok := []string{"FA{RRRR}{NNNN}", "{RRRR}/{NNN}", "DB-{RR}-{NNNNN}", "{RR}{MM}{NN}"}
	bad := []string{"", "FA{NNNN}", "FA{RRRR}", "FA{RRRR}{NNNN}{NN}", "FA{RRRR}{XXXX}", "FA č.{RRRR}{NNNN}", "FA {RRRR}{NNNN}", "{RRRR}{NNNNNNNNN}"}
	for _, f := range ok {
		if err := ValidateFormat(f); err != nil {
			t.Errorf("%q: %v", f, err)
		}
	}
	for _, f := range bad {
		if ValidateFormat(f) == nil {
			t.Errorf("%q mal byť odmietnutý", f)
		}
	}
	for k, f := range DefaultFormats {
		if err := ValidateFormat(f); err != nil {
			t.Errorf("predvolený formát %s: %v", k, err)
		}
	}
}

func TestVATRateByDeliveryDate(t *testing.T) {
	inv := sample()
	inv.IssueDate, inv.DeliveryDate, inv.DueDate = MustDate("2025-01-10"), MustDate("2024-12-20"), MustDate("2025-01-24")
	inv.Lines = []Line{{Description: "Práca v decembri 2024", Quantity: m.MustParse("1"), UnitPrice: m.MustParse("100"), Rate: vat.Rate20}}
	if err := inv.Validate(); err != nil {
		t.Fatalf("20 %% pri dodaní v 2024 je správne: %v", err)
	}
	inv.Lines[0].Rate = vat.Rate23
	if !fields(inv.Validate())["lines[0].vatRate"] {
		t.Fatal("23 % pri dodaní v 2024 je chyba")
	}
	// pri prenesení daňovej povinnosti sa sadzba nekontroluje (DPH je 0)
	inv.ReverseCharge = true
	if fields(inv.Validate())["lines[0].vatRate"] {
		t.Fatal("pri prenesení sa sadzba nekontroluje")
	}
}
