package party

import "testing"

func TestValidICO(t *testing.T) {
	cases := map[string]bool{
		"31322832":   true, // Slovnaft, a.s.
		"31 322 832": true,
		"31322833":   false, // zlá kontrolná číslica
		"3132283":    false,
		"3132283A":   false,
		"":           false,
	}
	for in, want := range cases {
		if got := ValidICO(in); got != want {
			t.Errorf("ValidICO(%q) = %v, chcem %v", in, got, want)
		}
	}
}

func TestValidICDPH(t *testing.T) {
	cases := map[string]bool{
		"SK2020372640":  true, // Slovnaft, a.s.
		"sk 2020372640": true,
		"SK2020372641":  false, // nie je deliteľné 11
		"CZ2020372640":  false,
		"SK0020372640":  false,
		"SK20203726":    false,
	}
	for in, want := range cases {
		if got := ValidICDPH(in); got != want {
			t.Errorf("ValidICDPH(%q) = %v, chcem %v", in, got, want)
		}
	}
}

func TestValidDIC(t *testing.T) {
	if !ValidDIC("2020372640") || ValidDIC("202037264") || ValidDIC("0020372640") {
		t.Fatal("ValidDIC")
	}
}
