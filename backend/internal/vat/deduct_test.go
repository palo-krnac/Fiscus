package vat

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestDeductPart(t *testing.T) {
	for _, c := range []struct {
		tax  string
		pct  int
		want string
	}{
		{"23.00", 0, "23"}, {"23.00", 100, "23"}, {"23.00", 50, "11.5"}, {"11.97", 50, "5.99"}, {"100", 37, "37"},
	} {
		if got := DeductPart(money.MustParse(c.tax), c.pct); !got.Equal(money.MustParse(c.want)) {
			t.Errorf("%s × %d %% = %s, chcem %s", c.tax, c.pct, got, c.want)
		}
	}
}
