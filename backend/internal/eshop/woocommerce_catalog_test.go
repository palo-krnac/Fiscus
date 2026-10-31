package eshop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestWooCatalog(t *testing.T) {
	type req struct {
		method, path string
		body         map[string]any
	}
	var writes []req
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			writes = append(writes, req{r.Method, r.URL.Path, b})
			io.WriteString(w, `{"id":99}`)
			return
		}
		w.Header().Set("X-WP-TotalPages", "1")
		switch r.URL.Path {
		case "/wp-json/wc/v3/settings/tax/woocommerce_prices_include_tax":
			io.WriteString(w, `{"id":"woocommerce_prices_include_tax","value":"yes"}`)
		case "/wp-json/wc/v3/taxes":
			io.WriteString(w, `[{"country":"CZ","rate":"21.0000","class":"standard"},{"country":"SK","rate":"23.0000","class":"standard"},
 {"country":"SK","rate":"19.0000","class":"reduced-rate"},{"country":"SK","rate":"5.0000","class":"super-reduced"}]`)
		case "/wp-json/wc/v3/products":
			switch r.URL.Query().Get("sku") {
			case "":
				io.WriteString(w, `[{"id":1,"type":"simple","sku":"REL24","name":"Relé 24 V","regular_price":"12.30","tax_class":"","status":"publish"},
 {"id":2,"type":"variable","sku":"","name":"Tričko","tax_class":"reduced-rate","status":"publish"},
 {"id":3,"type":"simple","sku":"","name":"Bez kódu","regular_price":"1"},
 {"id":4,"type":"simple","sku":"KNIHA","name":"Norma STN","regular_price":"10.50","tax_class":"super-reduced","status":"draft"}]`)
			case "REL24":
				io.WriteString(w, `[{"id":1,"type":"simple","sku":"REL24","status":"publish"}]`)
			case "TR-M":
				io.WriteString(w, `[{"id":21,"parent_id":2,"type":"variation","sku":"TR-M","status":"publish"}]`)
			case "KNIHA":
				io.WriteString(w, `[{"id":4,"type":"simple","sku":"KNIHA","status":"draft"}]`)
			default:
				io.WriteString(w, `[]`)
			}
		case "/wp-json/wc/v3/products/2/variations":
			io.WriteString(w, `[{"id":21,"sku":"TR-M","regular_price":"11.90","tax_class":"parent","status":"publish","attributes":[{"name":"Veľkosť","option":"M"}]}]`)
		case "/wp-json/wc/v3/customers":
			io.WriteString(w, `[{"email":"eva@example.sk","first_name":"Eva","last_name":"Malá","billing":{"company":"Malá s.r.o.","address_1":"Nová 2","city":"Nitra","postcode":"94901","country":"SK","email":""}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, _ := New(Conn{Platform: WooCommerce, URL: srv.URL, Key: "k", Secret: "s", HTTP: Client(true)})
	cat := c.(Catalog)
	ps, err := cat.Products(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 3 {
		t.Fatalf("produkty %+v", ps)
	}
	want := []Product{
		{SKU: "REL24", Name: "Relé 24 V", Net: money.FromInt(10), Gross: money.MustParse("12.30"), Rate: 23, Active: true},
		{SKU: "TR-M", Name: "Tričko M", Net: money.FromInt(10), Gross: money.MustParse("11.90"), Rate: 19, Active: true},
		{SKU: "KNIHA", Name: "Norma STN", Net: money.FromInt(10), Gross: money.MustParse("10.50"), Rate: 5, Active: false},
	}
	for i, w := range want {
		g := ps[i]
		if g.SKU != w.SKU || g.Name != w.Name || !g.Net.Equal(w.Net) || !g.Gross.Equal(w.Gross) || g.Rate != w.Rate || g.Active != w.Active {
			t.Errorf("produkt %d: %+v, chcem %+v", i, g, w)
		}
	}
	cs, err := cat.Customers(context.Background())
	if err != nil || len(cs) != 1 || cs[0].Company != "Malá s.r.o." || cs[0].Email != "eva@example.sk" || cs[0].Name != "Eva Malá" {
		t.Errorf("zákazníci %+v %v", cs, err)
	}

	tests := []struct {
		p       Product
		created bool
		path    string
		check   func(map[string]any) bool
		errText string
	}{
		{Product{SKU: "REL24", Name: "Relé 24V DIN", Net: money.FromInt(11), Gross: money.MustParse("13.53"), Rate: 23, Active: true}, false, "PUT /wp-json/wc/v3/products/1",
			func(b map[string]any) bool {
				return b["regular_price"] == "13.53" && b["tax_class"] == "" && b["name"] == "Relé 24V DIN" && b["status"] == "publish"
			}, ""},
		{Product{SKU: "TR-M", Name: "x", Net: money.FromInt(10), Gross: money.MustParse("11.90"), Rate: 19, Active: false}, false, "PUT /wp-json/wc/v3/products/2/variations/21",
			func(b map[string]any) bool {
				return b["tax_class"] == "reduced-rate" && b["status"] == "private" && b["name"] == nil
			}, ""},
		{Product{SKU: "KNIHA", Name: "Norma", Net: money.FromInt(10), Gross: money.MustParse("10.50"), Rate: 5, Active: true}, false, "PUT /wp-json/wc/v3/products/4",
			func(b map[string]any) bool { return b["status"] == nil }, ""}, // koncept ostane konceptom
		{Product{SKU: "NOVY", Name: "Nový", Net: money.FromInt(1), Gross: money.MustParse("1.23"), Rate: 23, Active: true, Create: true}, true, "POST /wp-json/wc/v3/products",
			func(b map[string]any) bool {
				return b["status"] == "draft" && b["sku"] == "NOVY" && b["type"] == "simple"
			}, ""},
		{Product{SKU: "STARY", Name: "Vyradený", Rate: 23, Active: false, Create: true}, false, "", nil, "v obchode nie je"},
		{Product{SKU: "SLUZBA", Name: "Montáž", Rate: 23, Active: true}, false, "", nil, "v obchode nie je"},
		{Product{SKU: "NOVY2", Name: "x", Rate: 10, Active: true, Create: true}, false, "", nil, "triedu dane so sadzbou 10"},
	}
	for _, tt := range tests {
		writes = nil
		created, err := cat.PushProduct(context.Background(), tt.p)
		if tt.errText != "" {
			if err == nil || !strings.Contains(err.Error(), tt.errText) {
				t.Errorf("%s: chyba %v", tt.p.SKU, err)
			}
			continue
		}
		if err != nil || created != tt.created {
			t.Errorf("%s: %v %v", tt.p.SKU, created, err)
			continue
		}
		if tt.path == "" {
			if len(writes) != 0 {
				t.Errorf("%s: nemal sa zapisovať: %+v", tt.p.SKU, writes)
			}
			continue
		}
		if len(writes) != 1 || writes[0].method+" "+writes[0].path != tt.path || !tt.check(writes[0].body) {
			t.Errorf("%s: zápis %+v", tt.p.SKU, writes)
		}
	}
}
