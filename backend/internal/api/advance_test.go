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

	// druhá záloha toho istého odberateľa → do vyúčtovania
	var p2 doc
	e.do("POST", base, `{"kind":"proforma","customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-10-02","deliveryDate":"2026-10-02",`+
		`"dueDate":"2026-10-09","lines":[{"description":"Záloha na montáž","quantity":"1","unit":"ks","unitPrice":"200","vatRate":23}]}`, 201, &p2)
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, p2.ID), nil, 200, &p2)
	var cands []struct{ ID int64 }
	e.do("GET", fmt.Sprintf("%s/%d/advances", base, f.ID), nil, 200, &cands)
	if len(cands) != 0 {
		t.Fatalf("neuhradená záloha medzi návrhmi: %+v", cands)
	}
	e.do("POST", fmt.Sprintf("%s/%d/pay", base, p2.ID), nil, 200, &p2)
	e.do("GET", fmt.Sprintf("%s/%d/advances", base, f.ID), nil, 200, &cands)
	if len(cands) != 1 || cands[0].ID != p2.ID {
		t.Fatalf("návrhy: %+v", cands)
	}
	e.do("POST", fmt.Sprintf("%s/%d/advances/%d", base, f.ID, p2.ID), nil, 200, &f)
	if len(f.Advance.Deductions) != 2 || f.Summary.Total != "0.00" {
		t.Fatalf("dve zálohy: %+v", f)
	}
	e.do("POST", fmt.Sprintf("%s/%d/advances/%d", base, f.ID, p2.ID), nil, 422, nil)  // už je vo vyúčtovaní
	e.do("DELETE", fmt.Sprintf("%s/%d/advances/%d", base, f.ID, p.ID), nil, 422, nil) // pôvodná záloha
	e.do("DELETE", fmt.Sprintf("%s/%d/advances/%d", base, f.ID, p2.ID), nil, 200, &f)
	if len(f.Advance.Deductions) != 1 {
		t.Fatalf("po odobraní: %+v", f)
	}
}

func TestAdvanceWithin15API(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	type doc struct {
		ID          int64
		AdvanceMode string                    `json:"advanceMode"`
		Uncovered   []struct{ Amount string } `json:"uncoveredPayments"`
		Advance     *struct {
			ReceiptNumber string `json:"receiptNumber"`
			Prepaid       string
		}
	}
	var p doc
	e.do("POST", base, `{"kind":"proforma","within15":true,"customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-10-01","deliveryDate":"2026-10-01",`+
		`"dueDate":"2026-10-08","lines":[{"description":"Montáž","quantity":"1","unit":"ks","unitPrice":"100","vatRate":23}]}`, 201, &p)
	if p.AdvanceMode != "within15" {
		t.Fatalf("režim: %+v", p)
	}
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, p.ID), nil, 200, &p)
	e.do("POST", fmt.Sprintf("%s/%d/pay", base, p.ID), nil, 200, &p)
	if p.Advance != nil && p.Advance.ReceiptNumber != "" || len(p.Uncovered) != 1 || p.Uncovered[0].Amount != "123.00" {
		t.Fatalf("úhrada bez dokladu: %+v", p)
	}
	var f doc
	e.do("POST", fmt.Sprintf("%s/%d/settle", base, p.ID), nil, 200, &f)
	if f.Advance.Prepaid != "123.00" {
		t.Fatalf("vyúčtovanie: %+v", f.Advance)
	}
	id := p.ID
	p = doc{}
	e.do("POST", fmt.Sprintf("%s/%d/receipts", base, id), nil, 200, &p)
	if p.Advance.ReceiptNumber != "ZD20260001" || len(p.Uncovered) != 0 {
		t.Fatalf("dodatočný doklad: %+v", p)
	}
	e.do("POST", fmt.Sprintf("%s/%d/receipts", base, p.ID), nil, 422, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, f.ID), nil, 200, &f)
	if f.Advance.Prepaid != "0.00" && f.Advance.Prepaid != "0" {
		t.Fatalf("prepočítané vyúčtovanie: %+v", f.Advance)
	}
}
