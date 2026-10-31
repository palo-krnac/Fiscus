package moneys3

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestParse(t *testing.T) {
	raw, err := os.ReadFile("testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Contacts) != 3 || len(d.Stocks) != 1 || len(d.Invoices) != 6 {
		t.Fatalf("počty: %d %d %d", len(d.Contacts), len(d.Stocks), len(d.Invoices))
	}
	if d.Other["ObjPrij"] != 1 || len(d.Other) != 1 || d.YearStart != "2026-01-01" {
		t.Errorf("ostatné: %v %s", d.Other, d.YearStart)
	}
	k := d.Contacts[0]
	if dic, icdph := k.TaxIDs(); dic != "2020123457" || icdph != "SK2020123457" || k.PhoneNumber() != "+421 905 123 456" {
		t.Errorf("kontakt: %s %s %s", dic, icdph, k.PhoneNumber())
	}
	if name, a := d.Contacts[1].Display(); name != "Ján Šťastný" || a.City != "Žilina" {
		t.Errorf("osoba: %s %+v", name, a)
	}
	if s := d.Stocks[0]; s.Card.Code() != "REL-24" || s.Price.Price1.Price != "12.18" {
		t.Errorf("zásoba: %+v", s)
	}
	f := d.Invoices[0]
	if f.Number != "2026001" || len(f.Items) != 2 || f.Items[1].UnitName() != "ks" || f.Items[0].UnitName() != "h" || f.Summary.VATHigh != "32.41" || f.Received {
		t.Errorf("faktúra: %+v", f)
	}
	if c := d.Invoices[1]; !c.Credit() || c.Original != "2026001" {
		t.Errorf("dobropis: %+v", c)
	}
	if u := d.Invoices[3]; u.Currency() != "USD" || d.Invoices[0].Currency() != "" {
		t.Errorf("mena: %q", u.Currency())
	}
	if p := d.Invoices[5]; !p.Received || p.SupplierNumber != "2026001234" || p.Summary.BaseLow != "10" {
		t.Errorf("prijatá: %+v", p)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"prázdny", ""},
		{"POHODA", `<?xml version="1.0"?><dataPack/>`},
		{"HTML", `<html><body/></html>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.in)); !errors.Is(err, ErrNotMoney) {
				t.Errorf("chcem ErrNotMoney, mám %v", err)
			}
		})
	}
	if _, err := Parse([]byte(`<MoneyData><SeznamFirem><Firma>`)); err == nil {
		t.Error("nedokončené XML má byť chyba")
	}
}

func TestParseArchive(t *testing.T) {
	raw, err := os.ReadFile("testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"faktury.xml", "adresar.XML"} {
		w, _ := zw.Create(n)
		w.Write(raw)
	}
	w, _ := zw.Create("info.txt")
	w.Write([]byte("x"))
	zw.Close()
	d, err := ParseArchive(buf.Bytes(), 1<<20)
	if err != nil || len(d.Invoices) != 12 || d.Other["ObjPrij"] != 2 {
		t.Errorf("%d %v %v", len(d.Invoices), d.Other, err)
	}
	if _, err := ParseArchive(buf.Bytes(), 100); err == nil {
		t.Error("príliš veľký archív")
	}
}
