package tabimport

import (
	"strings"
	"testing"
)

func TestParseBalances(t *testing.T) {
	tests := []struct {
		name, csv string
		want      string // účet:MD:D|…
		err       string
	}{
		{"MD a D", "Účet;Názov účtu;MD;D\n211;Pokladnica;1 250,50;\n221.001;Banka;3000;\n411;Základné imanie;;4250,50\n;Spolu;4250,50;4250,50",
			"211:1250.50:0.00|221.001:3000.00:0.00|411:0.00:4250.50", ""},
		{"jeden stĺpec so znamienkom", "Účet;Zostatok\n311;500\n321;-500\n343;0", "311:500.00:0.00|321:0.00:500.00", ""},
		{"čistý zostatok z MD aj D", "Účet;MD;Dal\n311;800;300\n", "311:500.00:0.00", ""},
		{"súčtový riadok sa vynechá", "Účet;MD;D\nTrieda 2;100;\n211;100;\n", "211:100.00:0.00", ""},
		{"riadok bez účtu", "Účet;Názov;MD;D\n;Úprava;5;\n", ":5.00:0.00!chýba číslo účtu", ""},
		{"chybná suma", "Účet;MD;D\n211;sto;\n", "211:0.00:0.00!chybná suma", ""},
		{"bez účtu", "Názov;MD;D\nx;1;\n", "", "chýba stĺpec „Účet“"},
		{"bez súm", "Účet;Názov\n211;x\n", "", "MD"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bs, err := ParseBalances(split(tc.csv))
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("chyba %v, chcem %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, b := range bs {
				s := b.Account + ":" + b.Debit.String() + ":" + b.Credit.String()
				if b.Err != "" {
					s += "!" + b.Err
				}
				got = append(got, s)
			}
			if g := strings.Join(got, "|"); !strings.HasPrefix(g, tc.want) {
				t.Errorf("%s\nchcem %s", g, tc.want)
			}
		})
	}
}
