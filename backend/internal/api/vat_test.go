package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestVATAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	var draft struct{ ID int64 }
	e.do("POST", base+"/invoices", `{
	  "customer": {"name": "Odberateľ s.r.o.", "ico": "31322832", "icdph": "SK2020372640"},
	  "issueDate": "2026-10-10", "deliveryDate": "2026-10-10",
	  "lines": [{"description": "Programovanie", "quantity": "10", "unit": "hod", "unitPrice": "45.50", "vatRate": 23}]
	}`, 201, &draft)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, draft.ID), nil, 200, nil)

	var ret struct {
		Label string            `json:"label"`
		Rows  map[string]string `json:"rows"`
		KV    struct {
			A1 []struct {
				Number string `json:"number"`
				Base   string `json:"base"`
			} `json:"a1"`
		} `json:"kv"`
	}
	e.do("GET", base+"/vat?period=2026-10", nil, 200, &ret)
	if ret.Label != "október 2026" || ret.Rows["r03"] != "455.00" || ret.Rows["r04"] != "104.65" || len(ret.KV.A1) != 1 {
		t.Fatalf("priznanie: %+v", ret)
	}
	e.do("GET", base+"/vat?period=2026-10&excess=50", nil, 200, &ret)
	if ret.Rows["r34"] != "50.00" || ret.Rows["r35"] != "54.65" {
		t.Fatalf("nadmerný odpočet: %+v", ret.Rows)
	}
	e.do("GET", base+"/vat?period=2025-03", nil, 422, nil)
	e.do("GET", base+"/vat?period=x", nil, 422, nil)
	e.do("GET", base+"/vat?period=2026-10&excess=abc", nil, 400, nil)

	// XML exporty: firma nemá DIČ → 422, po doplnení súbor
	e.do("GET", base+"/vat/kv.xml?period=2026-10", nil, 422, nil)
	e.do("PUT", base, `{"name":"Moja firma","vatPayer":true,"icdph":"SK2020372640","dic":"2020372640","city":"Zvolen"}`, 200, nil)
	for _, tt := range []struct{ path, want string }{
		{"/vat/kv.xml?period=2026-10", `<A1 Odb="SK2020372640"`},
		{"/vat/dph.xml?period=2026-10", `<r04>104.65</r04>`},
	} {
		res, err := e.client.Get(e.srv.URL + base + tt.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), tt.want) ||
			!strings.Contains(res.Header.Get("Content-Disposition"), "attachment") {
			t.Errorf("%s: %d %s\n%s", tt.path, res.StatusCode, res.Header.Get("Content-Disposition"), body)
		}
	}
	e.do("GET", base+"/vat/iny.xml?period=2026-10", nil, 404, nil)
}
