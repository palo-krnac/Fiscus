package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func validPurchase() purchase.Purchase {
	return purchase.Purchase{
		SupplierNumber: "FV-2026/0815",
		Supplier:       invoice.Party{Name: "Slovnaft, a.s.", ICO: "31322832", ICDPH: "SK2020372640"},
		IssueDate:      invoice.MustDate("2026-10-05"),
		ReceivedDate:   invoice.MustDate("2026-10-07"),
		DueDate:        invoice.MustDate("2026-10-19"),
		SupplierIBAN:   "sk3112000000198742637541",
		Rates:          []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("81.30")}},
	}
}

func TestSavePurchase(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	p, warns, err := a.SavePurchase(ctx, c.ID, validPurchase())
	if err != nil {
		t.Fatal(err)
	}
	if p.Number != "PF20260001" || p.VariableSymbol != "20260815" || p.SupplierIBAN != "SK31 1200 0000 1987 4263 7541" || len(warns) != 0 {
		t.Fatalf("%s VS %s IBAN %s %v", p.Number, p.VariableSymbol, p.SupplierIBAN, warns)
	}
	if p.Summary.VAT.String() != "18.70" || p.Summary.Total.String() != "100.00" {
		t.Fatalf("%+v", p.Summary)
	}
	// dvojité zaevidovanie
	if _, _, err := a.SavePurchase(ctx, c.ID, validPurchase()); err == nil {
		t.Fatal("duplicitná faktúra")
	}
	// úprava nemení interné číslo ani stav, aj keď ich klient pošle
	p.Number, p.Status, p.Note = "PODVRH", purchase.StatusPaid, "Tankovanie služobné auto"
	p, _, err = a.SavePurchase(ctx, c.ID, p)
	if err != nil || p.Number != "PF20260001" || p.Status != purchase.StatusBooked {
		t.Fatalf("úprava: %v %s %s", err, p.Number, p.Status)
	}
	// úhrada → uzamknutá
	if _, err := a.MarkPurchasePaid(ctx, c.ID, p.ID, invoice.Date{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.SavePurchase(ctx, c.ID, p); !errors.Is(err, purchase.ErrPaid) {
		t.Fatalf("uhradená sa nedá meniť: %v", err)
	}
	if err := a.DeletePurchase(ctx, c.ID, p.ID); !errors.Is(err, purchase.ErrPaid) {
		t.Fatal("uhradená sa nedá zmazať")
	}
	if _, err := a.UnpayPurchase(ctx, c.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.DeletePurchase(ctx, c.ID, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPurchaseNumberFormatAndValidationGap(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	bad := validPurchase()
	bad.Supplier.Name = ""
	if _, _, err := a.SavePurchase(ctx, c.ID, bad); err == nil {
		t.Fatal("neplatná")
	}
	c.NumberFormats = map[string]string{PurchaseFormatKey: "N{RR}-{NNN}"}
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	p, _, err := a.SavePurchase(ctx, c.ID, validPurchase())
	if err != nil || p.Number != "N26-001" {
		t.Fatalf("neplatný doklad nemá spotrebovať číslo: %s %v", p.Number, err)
	}
}

func TestPreviewPurchase(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	p := validPurchase()
	p.Rates[0].VAT = m.MustParse("20.00")
	p.Supplier.ICDPH = ""
	out := a.PreviewPurchase(ctx, p)
	if len(out.Warnings) != 1 || len(out.Errors) != 1 || out.Errors[0].Field != "supplier.icdph" {
		t.Fatalf("%+v %+v", out.Warnings, out.Errors)
	}
	if out.Summary.Total.String() != "101.30" {
		t.Fatal(out.Summary.Total)
	}
}
