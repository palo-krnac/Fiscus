package app

import (
	"os"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestImportPohoda(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	data, err := os.ReadFile("../pohoda/testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	prev, err := a.ImportPohoda(ctx, c.ID, data, MigrateOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := ImportReport{Contacts: ImportCount{New: 2}, Items: ImportCount{New: 1}, Invoices: ImportCount{New: 2, Skipped: 1}, Purchases: ImportCount{New: 1}}
	if prev.Contacts != want.Contacts || prev.Items != want.Items || prev.Invoices != want.Invoices || prev.Purchases != want.Purchases || prev.Done || prev.Other["order"] != 1 {
		t.Fatalf("náhľad: %+v", prev)
	}
	if list, _ := a.Store.ListContacts(ctx, c.ID, ""); len(list) != 0 {
		t.Fatal("náhľad nič neukladá")
	}
	res, err := a.ImportPohoda(ctx, c.ID, data, MigrateOptions{}, true)
	if err != nil || !res.Done || res.Invoices.New != 2 || res.Purchases.New != 1 || res.Contacts.New != 2 {
		t.Fatalf("import: %+v %v", res, err)
	}
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
	by := map[string]invoice.Invoice{}
	for _, inv := range invs {
		by[inv.Number] = inv
	}
	fa := by["260100015"]
	if fa.Status != invoice.StatusPaid || fa.Summary.Total.String() != "173.31" || fa.DeliveryDate.String() != "2026-03-01" ||
		fa.Lines[0].Rate != vat.Rate23 || fa.Customer.ICDPH != "SK2020123457" {
		t.Errorf("faktúra: %+v", fa)
	}
	db := by["260200001"]
	if db.Kind != invoice.KindCreditNote || db.Correction == nil || db.Correction.OriginalID != fa.ID || db.Summary.Total.String() != "-14.98" || db.Status != invoice.StatusIssued {
		t.Errorf("dobropis: %+v %+v", db.Summary, db.Correction)
	}
	purs, _ := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	if len(purs) != 1 || purs[0].SupplierNumber != "2026001234" || purs[0].Summary.Total.String() != "133.50" || purs[0].ReceivedDate.String() != "2026-03-07" ||
		purs[0].Status != purchase.StatusBooked {
		t.Errorf("prijatá: %+v", purs)
	}
	// opakovaný import nič nezdvojí
	again, err := a.ImportPohoda(ctx, c.ID, data, MigrateOptions{}, true)
	if err != nil || again.Contacts.Existing != 2 || again.Items.Existing != 1 || again.Invoices.Existing != 2 || again.Purchases.Existing != 1 ||
		again.Contacts.New+again.Items.New+again.Invoices.New+again.Purchases.New != 0 {
		t.Fatalf("opakovaný import: %+v", again)
	}
	if _, err := a.ImportPohoda(ctx, c.ID, []byte("<x/>"), MigrateOptions{}, false); !fieldErr(err, "file") {
		t.Errorf("iný súbor: %v", err)
	}
	// nová faktúra vo Fiškuse nedostane číslo prenesenej faktúry
	clash := fa
	clash.ID, clash.Number = 0, "FA20260001"
	if err := a.Store.SaveInvoice(ctx, c.ID, &clash); err != nil {
		t.Fatal(err)
	}
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	if inv, err := a.Issue(ctx, c.ID, d.ID); err != nil || inv.Number != "FA20260002" {
		t.Errorf("číslo: %s %v", inv.Number, err)
	}
}
