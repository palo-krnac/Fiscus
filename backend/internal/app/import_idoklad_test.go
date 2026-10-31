package app

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestImportIDoklad(t *testing.T) {
	raw, err := os.ReadFile("../pohoda/testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	// iDoklad neposiela adresár: vzorku bez adresára a zásob zabalíme do ZIP
	noBook := bytes.NewBuffer(nil)
	zw := zip.NewWriter(noBook)
	w, _ := zw.Create("iDoklad2PohodaExportIssuedInvoice.xml")
	w.Write(raw)
	zw.Close()

	tests := []struct {
		name     string
		data     []byte
		contacts ImportCount
		invoices int
		err      bool
	}{
		{"ZIP", noBook.Bytes(), ImportCount{New: 3}, 2, false}, // 2 z adresára + dodávateľ z prijatej faktúry
		{"Money S3 nie", []byte("<MoneyData/>"), ImportCount{}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, c := newApp(t, edition.Desktop)
			res, err := a.ImportIDoklad(ctx, c.ID, tc.data, MigrateOptions{}, true)
			if tc.err {
				if !fieldErr(err, "file") {
					t.Fatalf("chcem chybu súboru: %v", err)
				}
				return
			}
			if err != nil || res.Source != "iDoklad" || res.Invoices.New != tc.invoices || res.Contacts != tc.contacts {
				t.Fatalf("%+v %v", res, err)
			}
		})
	}
}
