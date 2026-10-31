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

func TestShopifyCatalog(t *testing.T) {
	var calls []string
	var lastVars map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		q := req.Query
		switch {
		case strings.Contains(q, "taxesIncluded"):
			io.WriteString(w, `{"data":{"shop":{"taxesIncluded":true}}}`)
		case strings.Contains(q, "productVariants(first: 250"):
			io.WriteString(w, `{"data":{"productVariants":{"pageInfo":{"hasNextPage":false},"nodes":[
 {"sku":"REL24","title":"Default Title","price":"12.30","taxable":true,"product":{"title":"Relé","status":"ACTIVE","hasOnlyDefaultVariant":true}},
 {"sku":"TR-M","title":"M","price":"10.00","taxable":false,"product":{"title":"Tričko","status":"DRAFT","hasOnlyDefaultVariant":false}},
 {"sku":"","title":"x","price":"1","taxable":true,"product":{"title":"Bez kódu","status":"ACTIVE","hasOnlyDefaultVariant":true}}]}}}`)
		case strings.Contains(q, "customers("):
			io.WriteString(w, `{"data":{"customers":{"pageInfo":{"hasNextPage":false},"nodes":[{"email":"eva@x.sk","firstName":"Eva","lastName":"Malá","phone":null,
 "defaultAddress":{"company":"Malá s.r.o.","address1":"Nová 2","city":"Nitra","zip":"94901","countryCodeV2":"SK","phone":"0900"}}]}}}`)
		case strings.Contains(q, "productVariants(first: 5"):
			if req.Variables["q"] == `sku:"REL24"` {
				io.WriteString(w, `{"data":{"productVariants":{"nodes":[{"id":"gid://shopify/ProductVariant/1","sku":"REL24","product":{"id":"gid://shopify/Product/1","status":"ACTIVE","hasOnlyDefaultVariant":true}}]}}}`)
				return
			}
			io.WriteString(w, `{"data":{"productVariants":{"nodes":[]}}}`)
		case strings.Contains(q, "productVariantsBulkUpdate"):
			calls, lastVars = append(calls, "update"), req.Variables
			io.WriteString(w, `{"data":{"productVariantsBulkUpdate":{"userErrors":[]}}}`)
		case strings.Contains(q, "productUpdate"):
			calls = append(calls, "product:"+req.Variables["in"].(map[string]any)["title"].(string))
			io.WriteString(w, `{"data":{"productUpdate":{"userErrors":[]}}}`)
		case strings.Contains(q, "productCreate"):
			calls = append(calls, "create")
			io.WriteString(w, `{"data":{"productCreate":{"product":{"id":"gid://shopify/Product/9"},"userErrors":[]}}}`)
		case strings.Contains(q, "productVariantsBulkCreate"):
			calls, lastVars = append(calls, "variant"), req.Variables
			io.WriteString(w, `{"data":{"productVariantsBulkCreate":{"userErrors":[]}}}`)
		default:
			io.WriteString(w, `{"errors":[{"message":"?"}]}`)
		}
	}))
	defer srv.Close()
	c, _ := New(Conn{Platform: Shopify, URL: srv.URL, Secret: "t", HTTP: Client(true)})
	cat := c.(Catalog)
	ps, err := cat.Products(context.Background())
	if err != nil || len(ps) != 2 {
		t.Fatalf("%+v %v", ps, err)
	}
	if ps[0].Name != "Relé" || ps[0].Rate != 23 || !ps[0].Net.Equal(money.FromInt(10)) || !ps[0].Active ||
		ps[1].Name != "Tričko M" || ps[1].Rate != 0 || !ps[1].Net.Equal(money.FromInt(10)) || ps[1].Active {
		t.Errorf("produkty %+v", ps)
	}
	cs, err := cat.Customers(context.Background())
	if err != nil || len(cs) != 1 || cs[0].Name != "Eva Malá" || cs[0].Company != "Malá s.r.o." || cs[0].Phone != "0900" {
		t.Errorf("zákazníci %+v %v", cs, err)
	}
	created, err := cat.PushProduct(context.Background(), Product{SKU: "REL24", Name: "Relé DIN", Net: money.FromInt(11), Gross: money.MustParse("13.53"), Rate: 23, Active: true})
	v := lastVars["v"].([]any)[0].(map[string]any)
	if err != nil || created || strings.Join(calls, ",") != "update,product:Relé DIN" || v["price"] != "13.53" || v["taxable"] != true {
		t.Errorf("úprava %v %v %v %v", created, err, calls, v)
	}
	calls = nil
	created, err = cat.PushProduct(context.Background(), Product{SKU: "NOVY", Name: "Nový", Net: money.FromInt(1), Gross: money.MustParse("1.23"), Rate: 23, Active: true, Create: true})
	v = lastVars["v"].([]any)[0].(map[string]any)
	if err != nil || !created || strings.Join(calls, ",") != "create,variant" || v["inventoryItem"].(map[string]any)["sku"] != "NOVY" {
		t.Errorf("nový %v %v %v %v", created, err, calls, v)
	}
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "KNIHA", Rate: 5, Active: true}); err == nil || !strings.Contains(err.Error(), "znížené") {
		t.Errorf("znížená sadzba: %v", err)
	}
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "SLUZBA", Rate: 23, Active: true}); err != ErrNoSKU {
		t.Errorf("služba: %v", err)
	}
}

func TestMagentoCatalog(t *testing.T) {
	type w struct {
		method, path string
		product      map[string]any
	}
	var writes []w
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			var b struct {
				Product map[string]any `json:"product"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			writes = append(writes, w{r.Method, r.URL.Path, b.Product})
			io.WriteString(rw, `{}`)
			return
		}
		switch r.URL.Path {
		case "/rest/V1/taxRates/search":
			io.WriteString(rw, `{"items":[{"id":1,"tax_country_id":"US","rate":"8.25"},{"id":2,"tax_country_id":"SK","rate":"23"},{"id":3,"tax_country_id":"SK","rate":"5.0000"}]}`)
		case "/rest/V1/taxRules/search":
			io.WriteString(rw, `{"items":[{"product_tax_class_ids":[2],"tax_rate_ids":[1,2]},{"product_tax_class_ids":[7],"tax_rate_ids":[3]}]}`)
		case "/rest/V1/products":
			io.WriteString(rw, `{"total_count":3,"items":[{"sku":"REL24","name":"Relé","price":10,"status":1,"type_id":"simple","custom_attributes":[{"attribute_code":"tax_class_id","value":"2"}]},
 {"sku":"TRIKO","name":"Tričko","price":0,"status":1,"type_id":"configurable"},
 {"sku":"KNIHA","name":"Norma","price":20,"status":2,"type_id":"simple","custom_attributes":[{"attribute_code":"tax_class_id","value":"7"}]}]}`)
		case "/rest/V1/customers/search":
			io.WriteString(rw, `{"total_count":1,"items":[{"email":"jan@x.sk","firstname":"Ján","lastname":"Novák","taxvat":"SK2020000001","addresses":[{"default_billing":true,"company":"Novák s.r.o.","street":["Hlavná 1"],"city":"Žilina","postcode":"01001","country_id":"SK"}]}]}`)
		case "/rest/V1/products/REL24":
			io.WriteString(rw, `{"sku":"REL24","status":1}`)
		default:
			http.NotFound(rw, r)
		}
	}))
	defer srv.Close()
	c, _ := New(Conn{Platform: Magento, URL: srv.URL, Secret: "t", HTTP: Client(true)})
	cat := c.(Catalog)
	ps, err := cat.Products(context.Background())
	if err != nil || len(ps) != 2 || ps[0].Rate != 23 || !ps[0].Gross.Equal(money.MustParse("12.30")) || ps[1].Rate != 5 || ps[1].Active {
		t.Fatalf("produkty %+v %v", ps, err)
	}
	cs, err := cat.Customers(context.Background())
	if err != nil || len(cs) != 1 || cs[0].Company != "Novák s.r.o." || cs[0].Name != "Ján Novák" || cs[0].ICDPH != "SK2020000001" || cs[0].Email != "jan@x.sk" {
		t.Errorf("zákazníci %+v %v", cs, err)
	}
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "REL24", Name: "Relé DIN", Net: money.FromInt(11), Rate: 23, Active: true}); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || writes[0].method != http.MethodPut || writes[0].path != "/rest/all/V1/products/REL24" || writes[0].product["price"] != "11.00" ||
		writes[0].product["status"] != nil {
		t.Errorf("úprava %+v", writes)
	}
	writes = nil
	created, err := cat.PushProduct(context.Background(), Product{SKU: "NOVY", Name: "Nový", Net: money.FromInt(5), Rate: 5, Active: true, Create: true})
	if err != nil || !created || len(writes) != 1 || writes[0].method != http.MethodPost || writes[0].product["status"] != float64(2) {
		t.Errorf("nový %v %v %+v", created, err, writes)
	}
	if ca := writes[0].product["custom_attributes"].([]any)[0].(map[string]any); ca["value"] != "7" {
		t.Errorf("trieda dane %v", ca)
	}
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "X", Rate: 19, Active: true, Create: true}); err == nil {
		t.Error("chýbajúca sadzba 19 %")
	}
}
