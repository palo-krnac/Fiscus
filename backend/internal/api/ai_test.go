package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestExtractAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	calls := 0
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("x-api-key") != "sk-ant-test" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","name":"zapis_fakturu","input":{"supplierName":"Dodávateľ s.r.o.","ico":"36123456",
			"invoiceNumber":"2026123","issueDate":"2026-10-20","rates":[{"vatRate":23,"base":"100","vat":"23"}],"total":"123"}}]}`))
	}))
	defer claude.Close()
	e.app.AIBaseURL = claude.URL
	path := fmt.Sprintf("/api/companies/%d/purchases/extract?name=f.pdf", e.cid)
	pdf := "%PDF-1.7\n%%EOF"

	var st struct {
		KeySet bool `json:"keySet"`
		Model  string
	}
	e.do("GET", "/api/ai/settings", nil, 200, &st)
	if st.KeySet || st.Model == "" {
		t.Fatalf("nastavenie: %+v", st)
	}
	e.do("POST", path, pdf, 409, nil) // bez kľúča
	e.do("PUT", "/api/ai/settings", `{"apiKey":"sk-ant-test","model":""}`, 200, &st)
	if !st.KeySet {
		t.Fatal("kľúč sa neuložil")
	}
	var d struct {
		Purchase struct {
			SupplierNumber string `json:"supplierNumber"`
			VariableSymbol string `json:"variableSymbol"`
			Summary        struct{ Total string }
		}
		Warnings []string
	}
	e.do("POST", path, pdf, 200, &d)
	if d.Purchase.SupplierNumber != "2026123" || d.Purchase.VariableSymbol != "2026123" || d.Purchase.Summary.Total != "123.00" {
		t.Fatalf("koncept: %+v", d)
	}
	e.do("POST", path, "obyčajný text", 422, nil)
	e.do("PUT", "/api/ai/settings", `{"apiKey":"zly"}`, 200, nil)
	e.do("POST", path, pdf, 502, nil)
	// kľúč sa nikdy nevracia
	var raw map[string]any
	e.do("GET", "/api/ai/settings", nil, 200, &raw)
	if _, has := raw["apiKey"]; has {
		t.Fatalf("kľúč v odpovedi: %v", raw)
	}
	if calls != 2 {
		t.Fatalf("volania API: %d", calls)
	}
	_ = bytes.MinRead
}
