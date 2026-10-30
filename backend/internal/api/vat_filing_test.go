package api

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestVATSupplementAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	e.do("PUT", base, `{"name":"Moja firma","vatPayer":true,"icdph":"SK2020372640","dic":"2020372640","city":"Zvolen"}`, 200, nil)
	var draft struct{ ID int64 }
	e.do("POST", base+"/invoices", `{"customer":{"name":"Odberateľ s.r.o.","icdph":"SK2020372640"},"issueDate":"2026-09-10","deliveryDate":"2026-09-10",
	  "lines":[{"description":"Montáž","quantity":"1","unit":"ks","unitPrice":"1000","vatRate":23}]}`, 201, &draft)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, draft.ID), nil, 200, nil)

	// riadne priznanie zaznamenané ako podané
	var filings []struct {
		Kind   string
		Result string
	}
	e.do("POST", base+"/vat/filings?period=2026-09&podane=2026-10-20", nil, 200, &filings)
	if len(filings) != 1 || filings[0].Kind != "riadne" || filings[0].Result != "230.00" {
		t.Fatalf("podanie: %+v", filings)
	}
	// ďalšia faktúra za september → dodatočné priznanie
	e.do("POST", base+"/invoices", `{"customer":{"name":"Odberateľ s.r.o.","icdph":"SK2020372640"},"issueDate":"2026-09-20","deliveryDate":"2026-09-20",
	  "lines":[{"description":"Servis","quantity":"1","unit":"ks","unitPrice":"100","vatRate":23}]}`, 201, &draft)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, draft.ID), nil, 200, nil)
	var ret struct {
		Kind string            `json:"kind"`
		Rows map[string]string `json:"rows"`
	}
	q := "/vat?period=2026-09&druh=dodatocne&zistene=2026-11-03&posledne=" + filings[0].Result
	e.do("GET", base+q, nil, 200, &ret)
	if ret.Kind != "dodatocne" || ret.Rows["r36"] != "23.00" || ret.Rows["r37"] != "23.00" {
		t.Fatalf("dodatočné: %+v", ret)
	}
	e.do("GET", base+"/vat?period=2026-09&druh=dodatocne", nil, 422, nil) // bez dátumu zistenia
	e.do("GET", base+"/vat?period=2026-09&druh=xyz", nil, 422, nil)
	e.do("GET", base+"/vat?period=2026-09&druh=dodatocne&zistene=x", nil, 400, nil)

	res, err := e.client.Get(e.srv.URL + base + "/vat/dph.xml" + strings.TrimPrefix(q, "/vat"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "<ddp>1</ddp>") || !strings.Contains(res.Header.Get("Content-Disposition"), "dph-2026-09-dodatocne.xml") {
		t.Fatalf("XML: %d %s", res.StatusCode, body)
	}
	e.do("GET", base+"/vat/kv.xml"+strings.TrimPrefix(q, "/vat"), nil, 422, nil) // dodatočný KV nie
	e.do("GET", base+"/vat/filings?period=2026-09", nil, 200, &filings)
	if len(filings) != 1 {
		t.Fatalf("zoznam: %+v", filings)
	}
}
