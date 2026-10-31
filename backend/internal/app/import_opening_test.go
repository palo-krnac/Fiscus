package app

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestImportOpening(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	tests := []struct {
		name, csv string
		lines     string // účet:MD:D
		balanced  bool
		warn      string
		err       bool
	}{
		{"predvaha z iného programu", "Účet;Názov;MD;D\n211000;Pokladňa;1000;\n221.999;Banka X;500;\n411;Základné imanie;;1500\n;Spolu;1500;1500",
			"211:1000.00:0.00|221999:500.00:0.00|411:0.00:1500.00", true, "221.999", false},
		{"nevyrovnané", "Účet;Zostatok\n311;100\n", "311:100.00:0.00", false, "nerovnajú", false},
		{"chybná suma", "Účet;MD;D\n311;x;\n", "", true, "chybná suma", false},
		{"nie tabuľka", "\x00\x01", "", false, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := a.ImportOpening(ctx, c.ID, []byte(tc.csv))
			if tc.err {
				if !fieldErr(err, "file") {
					t.Fatalf("chcem chybu súboru: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, l := range res.Lines {
				got = append(got, l.Account+":"+l.Debit.String()+":"+l.Credit.String())
			}
			if strings.Join(got, "|") != tc.lines || res.Balanced != tc.balanced || !strings.Contains(strings.Join(res.Warnings, "\n"), tc.warn) {
				t.Errorf("%s %v %v", strings.Join(got, "|"), res.Balanced, res.Warnings)
			}
		})
	}
	// pokladňa dostane názov z rozvrhu
	res, _ := a.ImportOpening(ctx, c.ID, []byte("Účet;MD;D\n211000;1000;\n411;;1000\n"))
	if res.Lines[0].Name == "" || res.Lines[0].Name == "Pokladňa X" {
		t.Errorf("názov: %+v", res.Lines[0])
	}
	// návrh sa dá uložiť ako počiatočné stavy
	if err := a.SetLedgerOpening(ctx, c.ID, 2026, res.Lines); err != nil {
		t.Errorf("uloženie: %v", err)
	}
}
