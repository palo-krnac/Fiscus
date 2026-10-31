package pohoda

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	data, err := os.ReadFile("testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Contacts) != 2 || len(d.Stocks) != 1 || len(d.Invoices) != 4 || d.Other["order"] != 1 {
		t.Fatalf("počty: %d %d %d %v", len(d.Contacts), len(d.Stocks), len(d.Invoices), d.Other)
	}
	a := d.Contacts[0].Address()
	if a.DisplayName() != "Kováč a syn s.r.o." || a.City != "Žilina" || a.ICDPH != "SK2020123457" || a.Email != "info@kovac.sk" || a.Country.IDs != "SK" {
		t.Errorf("adresa (Windows-1250): %+v", a)
	}
	if d.Contacts[1].Address().DisplayName() != "Ján Šťastný" {
		t.Errorf("osoba: %+v", d.Contacts[1].Address())
	}
	s := d.Stocks[0].Header
	if s.Code != "REL24" || s.Name != "Relé 24 V" || s.SellingPrice.Value != "12.5" || s.SellingPrice.PayVAT != "false" || s.SellingRate != "high" {
		t.Errorf("zásoba: %+v", s)
	}
	inv := d.Invoices[0]
	if inv.Header.Type != "issuedInvoice" || inv.Header.Number.Requested != "260100015" || len(inv.Items) != 2 ||
		inv.Items[0].Home.UnitPrice != "40" || inv.Header.Liquidation.AmountHome != "173.31" || inv.Summary.Home.PriceHigh != "140.9" {
		t.Errorf("faktúra: %+v", inv)
	}
	if d.Invoices[2].Summary.Home.Price3VAT != "0.5" || d.Invoices[1].Header.OriginalDocument != "260100015" {
		t.Errorf("prijatá / dobropis: %+v %+v", d.Invoices[2].Summary, d.Invoices[1].Header)
	}
	if _, err := Parse([]byte(`<?xml version="1.0"?><root/>`)); err != ErrNotPohoda {
		t.Errorf("iný súbor: %v", err)
	}
	if _, err := Parse([]byte(`nie je xml`)); err == nil {
		t.Error("neplatné XML")
	}
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseArchive(t *testing.T) {
	raw, err := os.ReadFile("testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	single, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		data     []byte
		max      int64
		invoices int
		err      string
	}{
		{"jeden XML", raw, 1 << 20, len(single.Invoices), ""},
		{"ZIP z iDokladu", zipOf(t, map[string][]byte{"iDoklad2PohodaExportIssuedInvoice.xml": raw, "iDoklad2PohodaExportReceivedInvoice.XML": raw, "info.txt": []byte("x")}),
			1 << 20, 2 * len(single.Invoices), ""},
		{"ZIP bez XML", zipOf(t, map[string][]byte{"a.txt": []byte("x")}), 1 << 20, 0, "nie je žiadny súbor XML"},
		{"príliš veľký", zipOf(t, map[string][]byte{"a.xml": raw}), 100, 0, "väčší ako"},
		{"iný XML v ZIP", zipOf(t, map[string][]byte{"a.xml": []byte("<MoneyData/>")}), 1 << 20, 0, "a.xml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := ParseArchive(tc.data, tc.max)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Errorf("chyba %v, chcem %q", err, tc.err)
				}
				return
			}
			if err != nil || len(d.Invoices) != tc.invoices {
				t.Errorf("%d faktúr, %v", len(d.Invoices), err)
			}
		})
	}
}
