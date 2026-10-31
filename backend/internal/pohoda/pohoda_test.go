package pohoda

import (
	"os"
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
