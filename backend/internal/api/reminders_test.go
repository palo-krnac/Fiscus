package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestRemindersAPI(t *testing.T) {
	e := setup(t, edition.Desktop) // dnes 29. 10. 2026
	inv := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	var a, b struct{ ID int64 }
	e.do("POST", inv, `{"customer":{"name":"Dlžník s.r.o.","ico":"31322832","country":"SK"},"issueDate":"2026-09-01","deliveryDate":"2026-09-01",`+
		`"dueDate":"2026-09-30","lines":[{"description":"Práca","quantity":"10","unit":"hod","unitPrice":"40","vatRate":23}]}`, 201, &a)
	e.do("POST", fmt.Sprintf("%s/%d/issue", inv, a.ID), nil, 200, nil)
	e.do("POST", inv, `{"customer":{"name":"Iný","country":"SK"},"issueDate":"2026-09-01","deliveryDate":"2026-09-01",`+
		`"dueDate":"2026-12-30","lines":[{"description":"Práca","quantity":"1","unit":"hod","unitPrice":"40","vatRate":23}]}`, 201, &b)
	e.do("POST", fmt.Sprintf("%s/%d/issue", inv, b.ID), nil, 200, nil)

	var ov struct {
		Groups []struct {
			Total    string
			Invoices []struct{ ID int64 }
		}
		ECBUpdated string `json:"ecbUpdated"`
		FlatFee    string `json:"flatFee"`
	}
	e.do("GET", fmt.Sprintf("/api/companies/%d/overdue", e.cid), nil, 200, &ov)
	if len(ov.Groups) != 1 || ov.Groups[0].Total != "492.00" || ov.ECBUpdated == "" || ov.FlatFee != "40.00" {
		t.Fatalf("po splatnosti: %+v", ov)
	}
	base := fmt.Sprintf("/api/companies/%d/reminders", e.cid)
	body := fmt.Sprintf(`{"invoiceIds":[%d],"chargeFee":true,"note":"Prosíme o úhradu."}`, a.ID)
	var pv struct {
		Number        string
		InterestTotal string `json:"interestTotal"`
		Total         string
	}
	e.do("POST", base+"/preview", body, 200, &pv)
	// 492 € × 11,65 % × 29 / 365 = 4,55 € + 40 € paušál
	if pv.Number != "" || pv.InterestTotal != "4.55" || pv.Total != "536.55" {
		t.Fatalf("náhľad: %+v", pv)
	}
	var r struct {
		ID     int64
		Number string
		Level  int
	}
	e.do("POST", base, body, 201, &r)
	if r.Number != "UP20260001" || r.Level != 1 {
		t.Fatalf("upomienka: %+v", r)
	}
	e.do("POST", base, fmt.Sprintf(`{"invoiceIds":[%d]}`, b.ID), 422, nil)
	e.do("POST", base, `{"invoiceIds":[]}`, 422, nil)
	e.do("POST", base, fmt.Sprintf(`{"invoiceIds":[%d],"interestMode":"custom","customRate":"0"}`, a.ID), 422, nil)
	var list []struct{ Number string }
	e.do("GET", base+"?year=2026", nil, 200, &list)
	if len(list) != 1 {
		t.Fatalf("zoznam: %+v", list)
	}
	e.do("GET", fmt.Sprintf("%s/%d", base, r.ID), nil, 200, nil)
	e.do("DELETE", fmt.Sprintf("%s/%d", base, r.ID), nil, 204, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, r.ID), nil, 404, nil)
}
