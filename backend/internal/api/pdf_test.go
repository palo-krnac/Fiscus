package api

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestPDFAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	inv := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	var a struct{ ID int64 }
	e.do("POST", inv, `{"customer":{"name":"Dlžník s.r.o.","ico":"31322832","country":"SK"},"issueDate":"2026-09-01","deliveryDate":"2026-09-01",`+
		`"dueDate":"2026-09-30","lines":[{"description":"Práca","quantity":"10","unit":"hod","unitPrice":"40","vatRate":23}]}`, 201, &a)
	e.do("POST", fmt.Sprintf("%s/%d/issue", inv, a.ID), nil, 200, nil)
	var q struct{ ID int64 }
	e.do("POST", fmt.Sprintf("/api/companies/%d/quotes", e.cid), `{"customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-10-01",`+
		`"validUntil":"2026-10-15","lines":[{"description":"Práca","quantity":"2","unit":"hod","unitPrice":"50","vatRate":23}]}`, 201, &q)
	var r struct{ ID int64 }
	e.do("POST", fmt.Sprintf("/api/companies/%d/reminders", e.cid), fmt.Sprintf(`{"invoiceIds":[%d]}`, a.ID), 201, &r)

	tests := []struct {
		name, path, file string
		inline           bool
		status           int
	}{
		{"faktúra", fmt.Sprintf("%s/%d/pdf", inv, a.ID), "Faktura-", false, 200},
		{"faktúra v prehliadači", fmt.Sprintf("%s/%d/pdf?inline=1", inv, a.ID), "Faktura-", true, 200},
		{"ponuka", fmt.Sprintf("/api/companies/%d/quotes/%d/pdf", e.cid, q.ID), "Ponuka-koncept.pdf", false, 200},
		{"upomienka", fmt.Sprintf("/api/companies/%d/reminders/%d/pdf", e.cid, r.ID), "Upomienka-UP20260001.pdf", false, 200},
		{"neexistuje", fmt.Sprintf("%s/999/pdf", inv), "", false, 404},
		{"cudzia firma", fmt.Sprintf("/api/companies/%d/invoices/%d/pdf", e.cid+100, a.ID), "", false, 403},
	}
	for _, tt := range tests {
		res, err := e.client.Get(e.srv.URL + tt.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != tt.status && !(tt.status == 403 && res.StatusCode == 404) {
			t.Errorf("%s: stav %d", tt.name, res.StatusCode)
			continue
		}
		if tt.status != 200 {
			continue
		}
		cd := res.Header.Get("Content-Disposition")
		if res.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(body, []byte("%PDF-")) ||
			!strings.Contains(cd, tt.file) || strings.HasPrefix(cd, "inline") != tt.inline {
			t.Errorf("%s: %s, %s, %d B", tt.name, res.Header.Get("Content-Type"), cd, len(body))
		}
	}
	// na serveri bez prihlásenia nič
	srv := setup(t, edition.Server).anonymous()
	srv.do("GET", fmt.Sprintf("/api/companies/%d/invoices/1/pdf", srv.cid), nil, 401, nil)
}
