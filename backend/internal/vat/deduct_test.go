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

func TestCoefPercent(t *testing.T) {
	m := money.MustParse
	tests := []struct {
		withRight, exempt string
		want              int
	}{
		{"180000", "20000", 90},
		{"3411", "6589", 35}, // 0,3411 → 0,35
		{"9001", "999", 91},  // 0,9001 → 0,91
		{"100", "0", 100},    // bez oslobodených plnení
		{"0", "0", 100},
		{"0", "500", 0},
		{"1", "99999", 1}, // 0,00001 → 0,01
		{"9999", "1", 100},
	}
	for _, tt := range tests {
		if got := CoefPercent(m(tt.withRight), m(tt.exempt)); got != tt.want {
			t.Errorf("CoefPercent(%s, %s) = %d, chcem %d", tt.withRight, tt.exempt, got, tt.want)
		}
	}
}

func TestDeductShare(t *testing.T) {
	m := money.MustParse
	tests := []struct {
		tax           string
		percent, coef int
		want          string
	}{
		{"400", 0, 100, "400.00"},
		{"400", 0, 90, "360.00"},
		{"400", 50, 90, "180.00"}, // osobné auto 50 % a koeficient
		{"400", 0, 0, "0.00"},
		{"33.33", 50, 35, "5.83"}, // 16,67 × 0,35 = 5,8345
	}
	for _, tt := range tests {
		if got := DeductShare(m(tt.tax), tt.percent, tt.coef).String(); got != tt.want {
			t.Errorf("DeductShare(%s, %d, %d) = %s, chcem %s", tt.tax, tt.percent, tt.coef, got, tt.want)
		}
	}
}
