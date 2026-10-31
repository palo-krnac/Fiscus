package party

import "testing"

func TestCountryCode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"SK", "SK"}, {"cz", "CZ"}, {"Slovenská republika", "SK"}, {"Česká republika", "CZ"}, {"Rakúsko", "AT"},
		{"Rakousko", "AT"}, {"Germany", "DE"}, {"Spojené kráľovstvo", "GB"}, {"Thailand", ""}, {"", ""},
	} {
		if got := CountryCode(tc.in); got != tc.want {
			t.Errorf("%q: %q, chcem %q", tc.in, got, tc.want)
		}
	}
}
