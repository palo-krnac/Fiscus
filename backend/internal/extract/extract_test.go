package extract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

var pdf = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF")

func fakeAPI(t *testing.T, status int, reply string, check func(map[string]any)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "sk-test" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("požiadavka: %s %v", r.URL.Path, r.Header)
		}
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		if check != nil {
			check(body)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const okReply = `{"content":[{"type":"tool_use","name":"zapis_fakturu","input":{
	"supplierName":"Elektro Sklad s.r.o.","street":"Priemyselná 5","city":"Zvolen","zip":"960 01","country":"sk",
	"ico":"36 123 456","icdph":"sk2020123456","iban":"SK31 1200 0000 1987 4263 7541","invoiceNumber":"FV-2026/0815",
	"issueDate":"2026-10-20","deliveryDate":"","dueDate":"2026-11-03","currency":"EUR","description":"Káble a svorky",
	"rates":[{"vatRate":23,"base":"100.00","vat":"23.00"},{"vatRate":5,"base":"10,00","vat":"0,50"}],"total":"133.50"}}],
	"usage":{"input_tokens":1500,"output_tokens":200}}`

func TestExtract(t *testing.T) {
	srv := fakeAPI(t, 200, okReply, func(body map[string]any) {
		msgs := body["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		doc := content[0].(map[string]any)
		src := doc["source"].(map[string]any)
		if doc["type"] != "document" || src["media_type"] != "application/pdf" || body["model"] != DefaultModel ||
			body["tool_choice"].(map[string]any)["name"] != "zapis_fakturu" {
			t.Errorf("telo: %v", body)
		}
	})
	c := Client{Settings: Settings{APIKey: "sk-test"}, BaseURL: srv.URL}
	r, err := c.Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	p, warn := r.Purchase(invoice.MustDate("2026-10-29"))
	s := p.Supplier
	if s.Name != "Elektro Sklad s.r.o." || s.ICO != "36123456" || s.ICDPH != "SK2020123456" || s.Country != "SK" || p.SupplierIBAN != "SK3112000000198742637541" ||
		p.SupplierNumber != "FV-2026/0815" || p.VariableSymbol != "20260815" || p.IssueDate.String() != "2026-10-20" ||
		p.DeliveryDate.String() != "2026-10-20" || p.ReceivedDate.String() != "2026-10-29" || p.DueDate.String() != "2026-11-03" {
		t.Errorf("koncept: %+v", p)
	}
	if len(p.Rates) != 2 || p.Summary.Total.String() != "133.50" || len(warn) != 0 {
		t.Errorf("sumy: %+v %+v %v", p.Rates, p.Summary, warn)
	}
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		reply  string
		key    string
		data   []byte
		want   string
	}{
		{"bez kľúča", 200, okReply, "", pdf, "nie je nastavené"},
		{"zlý súbor", 200, okReply, "sk-test", []byte("ahoj"), "PDF alebo obrázok"},
		{"zlý kľúč", 401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`, "sk-test", pdf, "odmietlo kľúč"},
		{"limit", 429, `{}`, "sk-test", pdf, "preťažené"},
		{"iná chyba", 400, `{"type":"error","error":{"type":"invalid_request_error","message":"PDF je poškodené"}}`, "sk-test", pdf, "PDF je poškodené"},
		{"bez nástroja", 200, `{"content":[{"type":"text","text":"neviem"}]}`, "sk-test", pdf, "nepodarilo"},
	}
	for _, tt := range tests {
		srv := fakeAPI(t, tt.status, tt.reply, nil)
		_, err := Client{Settings: Settings{APIKey: tt.key}, BaseURL: srv.URL}.Extract(context.Background(), tt.data)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

func TestPurchaseWarnings(t *testing.T) {
	r := Result{SupplierName: "X", InvoiceNumber: "A", Currency: "CZK", Total: "999",
		Rates: []Rate{{Rate: 23, Base: "100", VAT: "23"}, {Rate: 21, Base: "10", VAT: "2.10"}, {Rate: 5, Base: "veľa", VAT: "1"}}}
	_, warn := r.Purchase(invoice.MustDate("2026-10-29"))
	all := strings.Join(warn, "|")
	for _, w := range []string{"Dátum vyhotovenia", "CZK", "21 %", "Nečitateľná", "nesedí"} {
		if !strings.Contains(all, w) {
			t.Errorf("chýba upozornenie %q v %q", w, all)
		}
	}
	if _, warn := (Result{SupplierName: "Y"}).Purchase(invoice.MustDate("2026-10-29")); !strings.Contains(strings.Join(warn, "|"), "Rekapitulácia") {
		t.Error("bez rekapitulácie")
	}
	for _, mt := range [][]byte{pdf, {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}} {
		if _, err := MediaType(mt); err != nil {
			t.Error(err)
		}
	}
}
