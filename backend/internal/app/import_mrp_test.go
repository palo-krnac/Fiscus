package app

import (
	"os"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
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
	if res.Contacts.New != 3 || res.Invoices.New != 3 || res.Invoices.Skipped != 3 || res.Purchases.New != 2 || len(res.Other) != 0 {
		t.Fatalf("%+v", res)
	}
	w := strings.Join(res.Warnings, "\n")
	for _, want := range []string{"mene USD", "predfaktúra", "IČO 99999999"} {
		if !strings.Contains(w, want) {
			t.Errorf("chýba upozornenie %q:\n%s", want, w)
		}
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
		status        invoice.Status
	}{
		{"2026100001", "173.31", 2, invoice.KindInvoice, invoice.StatusPaid}, // zľava SLEVAMJ, textový riadok vynechaný, úhrady FAKVYUHR
		{"2026100002", "-14.98", 1, invoice.KindCreditNote, invoice.StatusIssued},
		{"2026100003", "134.90", 2, invoice.KindInvoice, invoice.StatusIssued}, // zľava za doklad → rekapitulácia
	}
	for _, tc := range tests {
		inv := by[tc.number]
		if inv.Summary.Total.String() != tc.total || len(inv.Lines) != tc.lines || inv.Kind != tc.kind || inv.Status != tc.status {
			t.Errorf("%s: %s %d %s %s", tc.number, inv.Summary.Total, len(inv.Lines), inv.Kind, inv.Status)
		}
	}
	fa := by["2026100001"]
	if fa.PaidDate.String() != "2026-03-12" || fa.Customer.ICDPH != "SK2020123457" || fa.Customer.City != "Nitra" || fa.Lines[0].UnitPrice.String() != "40.00" {
		t.Errorf("faktúra: %s %+v %+v", fa.PaidDate, fa.Customer, fa.Lines[0])
	}
	if db := by["2026100002"]; db.Correction == nil || db.Correction.OriginalNumber != "2026100001" {
		t.Errorf("dobropis: %+v", db.Correction)
	}
	if f3 := by["2026100003"]; f3.Customer.ICO != "" || f3.Customer.Name != "Ján Šťastný" || f3.Lines[0].Rate != vat.Rate19 {
		t.Errorf("drobný odberateľ: %+v %+v", f3.Customer, f3.Lines)
	}
	purs, _ := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	byP := map[string]purchase.Purchase{}
	for _, p := range purs {
		byP[p.SupplierNumber] = p
	}
	if p := byP["2026001234"]; p.Status != purchase.StatusPaid || p.PaidDate.String() != "2026-03-18" || p.Summary.Total.String() != "134.90" ||
		p.ReceivedDate.String() != "2026-03-07" || p.Supplier.ICDPH != "SK2021757441" {
		t.Errorf("prijatá: %+v", p)
	}
	if p := byP["FA-889"]; p.Status == purchase.StatusPaid || p.Summary.Total.String() != "61.50" {
		t.Errorf("prijatá neuhradená: %+v", p)
	}
	list, _ := a.Store.ListContacts(ctx, c.ID, "")
	for _, k := range list {
		if k.Name == "Kováč a syn s.r.o." && (k.IBAN == "" || k.Email != "fakturacia@kovac.sk" || !strings.Contains(k.Note, "Peter Kováč")) {
			t.Errorf("kontakt: %+v", k)
		}
	}
	if _, err := a.ImportMRP(ctx, c.ID, []byte(`<?xml version="1.0"?><dataPack/>`), MigrateOptions{}, false); !fieldErr(err, "file") {
		t.Errorf("POHODA: %v", err)
	}
}
