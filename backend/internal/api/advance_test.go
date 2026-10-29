package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestAdvanceAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	type doc struct {
		ID      int64
		Kind    string
		Number  string
		Status  string
		Advance *struct {
			ReceiptID     int64  `json:"receiptId"`
			ReceiptNumber string `json:"receiptNumber"`
			Deductions    []struct{ Number string }
		}
		Summary struct{ Total string }
	}
	var p doc
	e.do("POST", base, `{"kind":"proforma","customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-10-01","deliveryDate":"2026-10-01",`+
		`"dueDate":"2026-10-08","lines":[{"description":"Záloha na rozvádzač","quantity":"1","unit":"ks","unitPrice":"1000","vatRate":23}]}`, 201, &p)
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, p.ID), nil, 200, &p)
	e.do("POST", fmt.Sprintf("%s/%d/settle", base, p.ID), nil, 422, nil) // ešte neuhradená
	e.do("POST", fmt.Sprintf("%s/%d/pay", base, p.ID), nil, 200, &p)
	if p.Kind != "proforma" || p.Advance == nil || p.Advance.ReceiptNumber != "ZD20260001" {
		t.Fatalf("úhrada zálohy: %+v", p)
	}
	var f doc
	e.do("POST", fmt.Sprintf("%s/%d/settle", base, p.ID), nil, 200, &f)
	if f.Kind != "invoice" || f.Status != "draft" || f.Summary.Total != "0.00" || f.Advance.Deductions[0].Number != "ZD20260001" {
		t.Fatalf("vyúčtovanie: %+v", f)
	}
	e.do("POST", base, `{"kind":"advance_receipt","customer":{"name":"X"}}`, 422, nil)
}
