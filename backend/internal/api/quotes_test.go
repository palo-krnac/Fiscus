package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestQuotesAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/quotes", e.cid)
	type row struct {
		ID            int64
		Number        string
		Status        string
		Expired       bool
		InvoiceID     int64  `json:"invoiceId"`
		InvoiceNumber string `json:"invoiceNumber"`
		Summary       struct{ Total string }
		Errors        []struct{ Field string }
	}
	var pv row
	e.do("POST", base+"/preview", `{"customer":{"name":""},"lines":[{"description":"Práca","quantity":"2","unit":"hod","unitPrice":"50","vatRate":23}]}`, 200, &pv)
	if pv.Summary.Total != "123.00" || len(pv.Errors) == 0 {
		t.Fatalf("náhľad: %+v", pv)
	}
	var q row
	e.do("POST", base, `{"customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-10-01","validUntil":"2026-10-15",`+
		`"lines":[{"description":"Práca","quantity":"2","unit":"hod","unitPrice":"50","vatRate":23}]}`, 201, &q)
	e.do("PUT", fmt.Sprintf("%s/%d", base, q.ID), `{"customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-10-01",`+
		`"validUntil":"2026-10-15","lines":[{"description":"Práca","quantity":"3","unit":"hod","unitPrice":"50","vatRate":23}]}`, 200, &q)
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, q.ID), nil, 200, &q)
	if q.Number != "CP20260001" || q.Summary.Total != "184.50" || !q.Expired {
		t.Fatalf("vystavená: %+v", q) // platnosť do 15. 10., dnes je 29. 10.
	}
	e.do("PUT", fmt.Sprintf("%s/%d", base, q.ID), `{"customer":{"name":"x"}}`, 409, nil)
	e.do("POST", fmt.Sprintf("%s/%d/accept", base, q.ID), nil, 200, &q)
	var inv struct {
		ID     int64
		Status string
	}
	e.do("POST", fmt.Sprintf("%s/%d/invoice", base, q.ID), nil, 201, &inv)
	e.do("POST", fmt.Sprintf("%s/%d/invoice", base, q.ID), nil, 409, nil)
	e.do("POST", fmt.Sprintf("/api/companies/%d/invoices/%d/issue", e.cid, inv.ID), nil, 200, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, q.ID), nil, 200, &q)
	if q.Status != "invoiced" || q.InvoiceID != inv.ID || q.InvoiceNumber != "FA20260001" || q.Expired {
		t.Fatalf("po fakturácii: %+v", q)
	}
	var cp row
	e.do("POST", fmt.Sprintf("%s/%d/copy", base, q.ID), nil, 200, &cp)
	var list []row
	e.do("GET", base+"?year=2026", nil, 200, &list)
	if len(list) != 2 || list[0].ID != cp.ID || list[0].Status != "draft" {
		t.Fatalf("zoznam: %+v", list)
	}
	e.do("GET", base+"?year=x", nil, 400, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, 999), nil, 404, nil)
	e.do("POST", fmt.Sprintf("%s/%d/reject", base, cp.ID), nil, 409, nil)
}
