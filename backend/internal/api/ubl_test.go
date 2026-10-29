package api

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

func TestInvoiceUBL(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	var d invoice.Invoice
	e.do("POST", base, `{"customer":{"name":"Odberateľ s.r.o.","ico":"31322832","dic":"2020372640","city":"Bratislava","country":"SK"},
		"lines":[{"description":"Programovanie","quantity":"10","unit":"hod","unitPrice":"45.50","vatRate":23}]}`, 201, &d)
	url := fmt.Sprintf("%s/%d/ubl", base, d.ID)

	// koncept a firma bez DIČ a adresy: 422 s tým, čo chýba
	var body struct {
		Fields []struct{ Field string }
	}
	e.do("GET", url, nil, 422, &body)
	got := map[string]bool{}
	for _, f := range body.Fields {
		got[f.Field] = true
	}
	for _, f := range []string{"status", "seller.dic", "seller.address", "seller.iban"} {
		if !got[f] {
			t.Errorf("chýba %s v %v", f, body.Fields)
		}
	}

	co, _ := e.app.Store.GetCompany(context.Background(), e.cid)
	co.DIC, co.Street, co.City, co.Zip, co.IBAN = "2120000003", "Námestie SNP 1", "Banská Bystrica", "974 01", "SK3112000000198742637541"
	if err := e.app.Store.UpdateCompany(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, d.ID), nil, 200, &d)
	res, err := e.client.Get(e.srv.URL + url)
	if err != nil {
		t.Fatal(err)
	}
	xmlBody, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/xml" ||
		!strings.Contains(res.Header.Get("Content-Disposition"), d.Number+".xml") ||
		!strings.Contains(string(xmlBody), `<cbc:EndpointID schemeID="0245">2020372640</cbc:EndpointID>`) {
		t.Fatalf("%d %v\n%s", res.StatusCode, res.Header, xmlBody)
	}

	// tá istá e-faktúra načítaná ako prijatá (koncept, nič sa neuloží)
	var draft app.PurchaseDraft
	e.do("POST", fmt.Sprintf("/api/companies/%d/purchases/import-ubl?name=f.xml", e.cid), string(xmlBody), 200, &draft)
	if draft.Purchase.SupplierNumber != d.Number || draft.Purchase.Summary.Total.String() != "559.65" || draft.Purchase.ID != 0 {
		t.Fatalf("%+v", draft.Purchase)
	}
	e.do("POST", fmt.Sprintf("/api/companies/%d/purchases/import-ubl", e.cid), "<html/>", 422, nil)
}
