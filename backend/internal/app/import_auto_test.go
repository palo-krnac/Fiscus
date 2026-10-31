package app

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func zipFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, b := range files {
		w, _ := zw.Create(n)
		w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportExport(t *testing.T) {
	read := func(p string) []byte {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	pohodaXML := read("../pohoda/testdata/export.xml")
	moneyXML := read("../moneys3/testdata/export.xml")
	omegaTXT := read("../omega/testdata/export.txt")
	mrpZIP := read("../mrp/testdata/export.zip")
	tests := []struct {
		name     string
		data     []byte
		format   string
		invoices int
	}{
		{"POHODA XML", pohodaXML, fmtPohoda, 2},
		{"POHODA v ZIP", zipFiles(t, map[string][]byte{"faktury.xml": pohodaXML}), fmtPohoda, 2},
		{"Money S3 v ZIP", zipFiles(t, map[string][]byte{"readme.txt": []byte("x"), "money.xml": moneyXML}), fmtMoney, 3},
		{"Omega TXT", omegaTXT, fmtOmega, 3},
		{"Omega v ZIP", zipFiles(t, map[string][]byte{"omega.txt": omegaTXT}), fmtOmega, 3},
		{"MRP ZIP", mrpZIP, fmtMRP, 3},
		{"nič", []byte("Názov;IČO\n"), "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if f := sniffFormat(tc.data); f != tc.format {
				t.Fatalf("formát %q, chcem %q", f, tc.format)
			}
			a, c := newApp(t, edition.Desktop)
			res, err := a.ImportExport(ctx, c.ID, "SuperFaktúra", tc.data, MigrateOptions{}, true)
			if tc.format == "" {
				if !fieldErr(err, "file") {
					t.Fatalf("chcem chybu súboru: %v", err)
				}
				return
			}
			if err != nil || res.Source != "SuperFaktúra" || res.Invoices.New != tc.invoices || res.Contacts.New == 0 {
				t.Fatalf("%+v %v", res, err)
			}
		})
	}
}
