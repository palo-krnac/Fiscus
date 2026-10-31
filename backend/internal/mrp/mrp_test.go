package mrp

import (
	"errors"
	"os"
	"testing"
)

func TestParse(t *testing.T) {
	zipped, err := os.ReadFile("testdata/export.zip")
	if err != nil {
		t.Fatal(err)
	}
	single, _ := os.ReadFile("testdata/FAKVY.XML")
	tests := []struct {
		name                       string
		data                       []byte
		file                       string
		invoices, items, addresses int
		unknown                    int
	}{
		{"ZIP", zipped, "export.zip", 5, 3, 2, 0},
		{"samotné FAKVY.XML", single, "FAKVY.XML", 5, 0, 0, 0},
		{"tabuľka podľa polí", single, "faktury.xml", 5, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(tc.data, tc.file, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Invoices) != tc.invoices || len(d.Items) != tc.items || len(d.Addresses) != tc.addresses || len(d.Unknown) != tc.unknown {
				t.Errorf("%d %d %d %v", len(d.Invoices), len(d.Items), len(d.Addresses), d.Unknown)
			}
			if f := d.Invoices[0]; f.Get("cislo") != "2026100001" || f.Get("poznamka") != "Montáž rozvádzača" || f.Get("dph2") != "32.41" {
				t.Errorf("faktúra: %v", f)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"POHODA", `<?xml version="1.0"?><dataPack/>`},
		{"prázdny", ""},
	} {
		if _, err := Parse([]byte(tc.in), "a.xml", 1<<20); !errors.Is(err, ErrNotMRP) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	if _, err := Parse([]byte("<document><datasets>"), "a.xml", 1<<20); err == nil {
		t.Error("nedokončené XML")
	}
}
