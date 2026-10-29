package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestEInvoiceAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	co, _ := e.app.Store.GetCompany(context.Background(), e.cid)
	co.DIC, co.Street, co.City, co.IBAN = "2020372640", "Hlavná 1", "Zvolen", "SK3112000000198742637541"
	_ = e.app.Store.UpdateCompany(context.Background(), co)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	var d invoice.Invoice
	e.do("POST", base+"/invoices", `{"customer":{"name":"Odberateľ s.r.o.","dic":"2120000003","city":"Bratislava","country":"SK"},
		"lines":[{"description":"Práca","quantity":"1","unit":"hod","unitPrice":"100","vatRate":23}]}`, 201, &d)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, d.ID), nil, 200, &d)

	e.do("POST", fmt.Sprintf("%s/invoices/%d/einvoice", base, d.ID), nil, 409, nil) // nenastavené
	var bad struct{ Fields []struct{ Field string } }
	e.do("PUT", base+"/einvoice-settings", `{"mode":"folder"}`, 422, &bad)
	if len(bad.Fields) != 2 {
		t.Fatalf("chyby nastavenia %+v", bad)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in")
	e.do("PUT", base+"/einvoice-settings", map[string]string{"mode": "folder", "outDir": filepath.Join(dir, "out"), "inDir": in}, 200, nil)

	var sent store.EInvoice
	e.do("POST", fmt.Sprintf("%s/invoices/%d/einvoice", base, d.ID), nil, 200, &sent)
	if sent.PeppolID != "0245:2120000003" {
		t.Fatalf("%+v", sent)
	}
	var list []store.EInvoice
	e.do("GET", fmt.Sprintf("%s/einvoices?direction=out&docId=%d", base, d.ID), nil, 200, &list)
	if len(list) != 1 {
		t.Fatalf("zoznam %+v", list)
	}

	// vlastnú odoslanú e-faktúru vložíme do schránky ako prijatú
	data, _ := os.ReadFile(filepath.Join(dir, "out", sent.MessageID))
	_ = os.MkdirAll(in, 0o755)
	_ = os.WriteFile(filepath.Join(in, "prijata.xml"), data, 0o644)
	var box []app.InboxItem
	e.do("GET", base+"/einvoice-inbox", nil, 200, &box)
	if len(box) != 1 || box[0].Number != d.Number {
		t.Fatalf("schránka %+v", box)
	}
	var draft app.PurchaseDraft
	e.do("POST", base+"/einvoice-inbox/prijata.xml/take", nil, 200, &draft)
	if draft.Purchase.SupplierNumber != d.Number {
		t.Fatalf("%+v", draft.Purchase)
	}
	e.do("POST", base+"/einvoice-inbox/prijata.xml/take", nil, 404, nil) // už prevzatá
	e.do("GET", base+"/einvoices?direction=in", nil, 200, &list)
	if len(list) != 1 {
		t.Fatal(list)
	}
	e.do("POST", fmt.Sprintf("%s/einvoices/%d/reopen", base, list[0].ID), nil, 200, &draft)
}
