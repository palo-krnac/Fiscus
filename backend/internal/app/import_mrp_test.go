package app

import (
	"os"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestImportMRP(t *testing.T) {
	data, err := os.ReadFile("../mrp/testdata/export.zip")
	if err != nil {
		t.Fatal(err)
	}
	a, c := newApp(t, edition.Desktop)
	res, err := a.ImportMRP(ctx, c.ID, data, MigrateOptions{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Contacts.New != 2 || res.Invoices.New != 3 || res.Invoices.Skipped != 2 || len(res.Other) != 0 {
		t.Fatalf("%+v", res)
	}
	if w := strings.Join(res.Warnings, "\n"); !strings.Contains(w, "mene USD") || !strings.Contains(w, "bez IČO") {
		t.Errorf("upozornenia: %s", w)
	}
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
	by := map[string]invoice.Invoice{}
	for _, inv := range invs {
		by[inv.Number] = inv
	}
	tests := []struct {
		number, total string
		lines         int
		kind          invoice.Kind
	}{
		{"2026100001", "173.31", 2, invoice.KindInvoice},
		{"2026100002", "-14.98", 1, invoice.KindCreditNote},
		{"2026100003", "134.90", 2, invoice.KindInvoice},
	}
	for _, tc := range tests {
		inv := by[tc.number]
		if inv.Summary.Total.String() != tc.total || len(inv.Lines) != tc.lines || inv.Kind != tc.kind {
			t.Errorf("%s: %s %d %s", tc.number, inv.Summary.Total, len(inv.Lines), inv.Kind)
		}
	}
	fa := by["2026100001"]
	if fa.Customer.Name != "Kováč a syn s.r.o." || fa.Customer.City != "Nitra" || fa.Customer.ICDPH != "SK2020123457" || fa.Lines[0].Unit != "h" {
		t.Errorf("odberateľ: %+v %+v", fa.Customer, fa.Lines[0])
	}
	if l := by["2026100003"].Lines; l[0].Rate != vat.Rate19 || l[1].Rate != vat.Rate23 {
		t.Errorf("sadzby z rekapitulácie: %+v", l)
	}
	if db := by["2026100002"]; db.Correction == nil || db.Correction.OriginalNumber != "2026100001" {
		t.Errorf("dobropis: %+v", db.Correction)
	}
	if _, err := a.ImportMRP(ctx, c.ID, []byte(`<?xml version="1.0"?><dataPack/>`), MigrateOptions{}, false); !fieldErr(err, "file") {
		t.Errorf("POHODA: %v", err)
	}
}
