package app

import (
	"os"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestImportOmega(t *testing.T) {
	data, err := os.ReadFile("../omega/testdata/export.txt")
	if err != nil {
		t.Fatal(err)
	}
	a, c := newApp(t, edition.Desktop)
	prev, err := a.ImportOmega(ctx, c.ID, data, MigrateOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := ImportReport{Contacts: ImportCount{New: 2}, Items: ImportCount{New: 1}, Invoices: ImportCount{New: 3, Skipped: 2}, Purchases: ImportCount{New: 2}}
	if prev.Contacts != want.Contacts || prev.Items != want.Items || prev.Invoices != want.Invoices || prev.Purchases != want.Purchases ||
		prev.Other["cenové ponuky"] != 1 || prev.Other["bankové výpisy"] != 1 {
		t.Fatalf("náhľad: %+v", prev)
	}
	if w := strings.Join(prev.Warnings, "\n"); !strings.Contains(w, "Preddavková faktúra ZF2026001") || !strings.Contains(w, "mene USD") {
		t.Errorf("upozornenia: %s", w)
	}
	res, err := a.ImportOmega(ctx, c.ID, data, MigrateOptions{}, true)
	if err != nil || res.Invoices.New != 3 || res.Purchases.New != 2 {
		t.Fatalf("prenos: %+v %v", res, err)
	}
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
	by := map[string]invoice.Invoice{}
	for _, inv := range invs {
		by[inv.Number] = inv
	}
	tests := []struct {
		number, total string
		lines         int
		status        invoice.Status
		kind          invoice.Kind
	}{
		{"2026001", "173.31", 2, invoice.StatusIssued, invoice.KindInvoice},
		{"OF2026010", "123.00", 1, invoice.StatusPaid, invoice.KindInvoice},
		{"OF2026011", "-24.60", 1, invoice.StatusIssued, invoice.KindCreditNote},
	}
	for _, tc := range tests {
		inv := by[tc.number]
		if inv.Summary.Total.String() != tc.total || len(inv.Lines) != tc.lines || inv.Status != tc.status || inv.Kind != tc.kind {
			t.Errorf("%s: %s %d %s %s", tc.number, inv.Summary.Total, len(inv.Lines), inv.Status, inv.Kind)
		}
	}
	if of := by["OF2026010"]; of.PaidDate.String() != "2026-04-20" || of.Customer.City != "Nitra" || of.Customer.ICDPH != "SK2020123457" || of.VariableSymbol != "2026010" {
		t.Errorf("OF: %s %+v %s", of.PaidDate, of.Customer, of.VariableSymbol)
	}
	if fa := by["2026001"]; fa.Lines[0].Unit != "h" || fa.Subject != "Montáž rozvádzača" || fa.Customer.Zip != "94901" {
		t.Errorf("fakturácia: %+v %q", fa.Lines[0], fa.Subject)
	}
	if db := by["OF2026011"]; db.Correction == nil || db.Correction.OriginalNumber != "OF2026010" {
		t.Errorf("dobropis: %+v", db.Correction)
	}
	purs, _ := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	byP := map[string]purchase.Purchase{}
	for _, p := range purs {
		byP[p.SupplierNumber] = p
	}
	if p := byP["DF2026001"]; p.Summary.Total.String() != "134.90" || p.IssueDate.String() != "2026-03-05" || p.ReceivedDate.String() != "2026-03-07" {
		t.Errorf("došlá (fakturácia): %+v", p)
	}
	if p := byP["FA-889"]; p.Summary.Total.String() != "61.50" || p.ReceivedDate.String() != "2026-04-03" || p.Supplier.ICDPH != "SK2021757441" {
		t.Errorf("došlá (EUD): %+v", p)
	}
	if _, err := a.ImportOmega(ctx, c.ID, []byte("<MoneyData/>"), MigrateOptions{}, false); !fieldErr(err, "file") {
		t.Errorf("iný súbor: %v", err)
	}
}
