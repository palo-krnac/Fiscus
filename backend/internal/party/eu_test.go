package party

import "testing"

func TestEU(t *testing.T) {
	tests := []struct {
		country     string
		eu, foreign bool
	}{
		{"SK", true, false}, {"", false, false}, {"cz", true, true}, {"DE", true, true}, {"CH", false, true}, {"GB", false, true},
	}
	for _, tt := range tests {
		if IsEU(tt.country) != tt.eu || Foreign(tt.country) != tt.foreign {
			t.Errorf("%q: eu=%v foreign=%v", tt.country, IsEU(tt.country), Foreign(tt.country))
		}
	}
	for _, tt := range []struct{ icdph, country, want string }{
		{"DE123456789", "DE", "DE"}, {"cz 12345678", "", "CZ"}, {"", "GR", "EL"}, {"EL123456789", "GR", "EL"}, {"", "AT", "AT"},
	} {
		if got := VATCountry(tt.icdph, tt.country); got != tt.want {
			t.Errorf("%s/%s → %s", tt.icdph, tt.country, got)
		}
	}
}

func TestCountryName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"SK", "Slovensko"}, {"de", "Nemecko"}, {" cz ", "Česko"}, {"XX", "XX"}, {"", ""},
	}
	for _, tt := range tests {
		if got := CountryName(tt.in); got != tt.want {
			t.Errorf("CountryName(%q) = %q, chcem %q", tt.in, got, tt.want)
		}
	}
}
