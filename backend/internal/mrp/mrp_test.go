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
		name                                                   string
		data                                                   []byte
		file                                                   string
		invoices, items, payments, purchases, addresses, banks int
	}{
		{"ZIP", zipped, "export.zip", 6, 4, 2, 2, 3, 1},
		{"samotné FAKVY.XML", single, "FAKVY.XML", 6, 0, 0, 0, 0, 0},
		{"tabuľka podľa polí", single, "faktury.xml", 6, 0, 0, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(tc.data, tc.file, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			got := []int{len(d.Invoices), len(d.Items), len(d.Payments), len(d.Purchases), len(d.Addresses), len(d.Banks), len(d.Unknown)}
			want := []int{tc.invoices, tc.items, tc.payments, tc.purchases, tc.addresses, tc.banks, 0}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("počty %v, chcem %v", got, want)
				}
			}
			if f := d.Invoices[0]; f.Get("cislo") != "2026100001" || f.Get("poznamka") != "Montáž rozvádzača" || f.Get("dph2") != "32.41" {
				t.Errorf("faktúra: %v", f)
			}
		})
	}
}

func TestKind(t *testing.T) {
	for _, tc := range []struct {
		file string
		row  Row
		want string
	}{
		{"FP_ADRES.XML", nil, "addresses"},
		{"FP_ADRESBAN.XML", nil, "banks"},
		{"FAKPRUHR.XML", nil, "purchasePayments"},
		{"FAKVYTOTAL.XML", nil, "skip"},
		{"a.xml", Row{"idfak": "1", "cislo": "x", "datdanpov": ""}, "purchases"},
		{"a.xml", Row{"idfak": "1", "ciastka": "1", "doklad": "x"}, "payments"},
		{"a.xml", Row{"idradr": "1", "iban": "x"}, "banks"},
		{"a.xml", Row{"x": "1"}, ""},
	} {
		var rows []Row
		if tc.row != nil {
			rows = []Row{tc.row}
		}
		if got := kind(tc.file, rows); got != tc.want {
			t.Errorf("%s %v: %q", tc.file, tc.row, got)
		}
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
