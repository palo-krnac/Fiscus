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

func TestShoptetCatalog(t *testing.T) {
	type wr struct {
		method, path string
		body         map[string]any
	}
	var writes []wr
	vatPayer := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			writes = append(writes, wr{r.Method, r.URL.Path, b})
			io.WriteString(w, `{"data":null,"errors":null}`)
			return
		}
		switch r.URL.Path {
		case "/api/eshop":
			v := "true"
			if !vatPayer {
				v = "false"
			}
			io.WriteString(w, `{"data":{"contactInformation":{"eshopName":"Obchod"},"settings":{"vatPayer":`+v+`,"defaultPricelist":{"id":1,"name":"Základný"}}},"errors":null}`)
		case "/api/products":
			io.WriteString(w, `{"data":{"products":[{"guid":"g1"},{"guid":"g2"}],"paginator":{"pageCount":1}},"errors":null}`)
		case "/api/products/g1":
			io.WriteString(w, `{"data":{"guid":"g1","name":"Relé","visibility":"visible","variants":[{"code":"REL24","name":null,"price":"12.30","includingVat":true,"vatRate":"23.00","visible":true}]},"errors":null}`)
		case "/api/products/g2":
			io.WriteString(w, `{"data":{"guid":"g2","name":"Tričko","visibility":"visible","variants":[
 {"code":"TR-M","name":"Veľkosť: M","price":"10.00","includingVat":false,"vatRate":"19.00","visible":true},
 {"code":"TR-L","name":"Veľkosť: L","price":"10.00","includingVat":false,"vatRate":"19.00","visible":false}]},"errors":null}`)
		case "/api/customers":
			io.WriteString(w, `{"data":{"customers":[{"guid":"c1"}],"paginator":{"pageCount":1}},"errors":null}`)
		case "/api/customers/c1":
			io.WriteString(w, `{"data":{"customer":{"billingAddress":{"company":"Firma s.r.o.","fullName":"Ján Novák","street":"Hlavná","houseNumber":"1","city":"Zvolen","zip":"96001","countryCode":"SK","companyId":"36000001","vatId":"SK2020000001"},
 "accounts":[{"fullName":"Ján Novák","email":"jan@firma.sk","phone":"+421900","mainAccount":true}]}},"errors":null}`)
		case "/api/products/code/REL24":
			io.WriteString(w, `{"data":{"guid":"g1","name":"Relé","visibility":"visible","variants":[{"code":"REL24"}]},"errors":null}`)
		case "/api/products/code/TR-M":
			io.WriteString(w, `{"data":{"guid":"g2","name":"Tričko","visibility":"visible","variants":[{"code":"TR-M"},{"code":"TR-L"}]},"errors":null}`)
		case "/api/categories":
			io.WriteString(w, `{"data":{"categories":[{"guid":"cat1","name":"Elektro"}]},"errors":null}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"data":null,"errors":[{"errorCode":"not-found","message":"Not found"}]}`)
		}
	}))
	defer srv.Close()
	old := ShoptetAPI
	ShoptetAPI = srv.URL
	defer func() { ShoptetAPI = old }()
	c, _ := New(Conn{Platform: Shoptet, URL: "https://obchod.sk", Secret: "tok", HTTP: Client(true)})
	cat := c.(Catalog)
	ps, err := cat.Products(context.Background())
	if err != nil || len(ps) != 3 {
		t.Fatalf("%+v %v", ps, err)
	}
	if !ps[0].Net.Equal(money.FromInt(10)) || ps[0].Rate != 23 || ps[0].Name != "Relé" || !ps[0].Active ||
		ps[1].Name != "Tričko Veľkosť: M" || !ps[1].Gross.Equal(money.MustParse("11.90")) || ps[2].Active {
		t.Errorf("produkty %+v", ps)
	}
	cs, err := cat.Customers(context.Background())
	if err != nil || len(cs) != 1 || cs[0].Company != "Firma s.r.o." || cs[0].Street != "Hlavná 1" || cs[0].Email != "jan@firma.sk" || cs[0].ICO != "36000001" {
		t.Errorf("zákazníci %+v %v", cs, err)
	}

	// úprava: cena v predvolenom cenníku + názov
	created, err := cat.PushProduct(context.Background(), Product{SKU: "REL24", Name: "Relé DIN", Net: money.FromInt(11), Gross: money.MustParse("13.53"), Rate: 23, Active: true})
	if err != nil || created || len(writes) != 2 {
		t.Fatalf("úprava %v %v %+v", created, err, writes)
	}
	row := writes[0].body["data"].([]any)[0].(map[string]any)
	if writes[0].path != "/api/price-lists/1/prices" || row["vatRate"] != "23.00" || row["priceWithoutVat"].(map[string]any)["price"] != "11.00" {
		t.Errorf("cena %+v", writes[0])
	}
	if d := writes[1].body["data"].(map[string]any); writes[1].path != "/api/products/code/REL24" || d["name"] != "Relé DIN" || d["visibility"] != nil {
		t.Errorf("názov %+v", writes[1])
	}
	// variant vyradený: iba variant sa skryje, názov produktu ostane
	writes = nil
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "TR-M", Name: "x", Net: money.FromInt(10), Rate: 19, Active: false}); err != nil {
		t.Fatal(err)
	}
	if d := writes[1].body["data"].(map[string]any); d["name"] != "Tričko" || d["variants"].([]any)[0].(map[string]any)["visible"] != false {
		t.Errorf("variant %+v", writes[1])
	}
	// nový: skrytý v prvej kategórii, potom cenník so sadzbou
	writes = nil
	created, err = cat.PushProduct(context.Background(), Product{SKU: "NOVY", Name: "Nový", Net: money.FromInt(5), Gross: money.MustParse("5.25"), Rate: 5, Active: true, Create: true})
	if err != nil || !created || len(writes) != 2 || writes[0].method != http.MethodPost {
		t.Fatalf("nový %v %v %+v", created, err, writes)
	}
	if d := writes[0].body["data"].(map[string]any); d["visibility"] != "hidden" || d["defaultCategoryGuid"] != "cat1" {
		t.Errorf("založenie %+v", d)
	}
	if row := writes[1].body["data"].([]any)[0].(map[string]any); row["vatRate"] != "5.00" {
		t.Errorf("sadzba nového %+v", row)
	}
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "SLUZBA", Rate: 23, Active: true}); err != ErrNoSKU {
		t.Errorf("služba: %v", err)
	}
	// neplatiteľ DPH: iba cena s DPH, bez sadzby
	vatPayer = false
	c2, _ := New(Conn{Platform: Shoptet, URL: "https://obchod.sk", Secret: "tok", HTTP: Client(true)})
	writes = nil
	if _, err := c2.(Catalog).PushProduct(context.Background(), Product{SKU: "REL24", Name: "Relé", Net: money.FromInt(10), Rate: 23, Active: true}); err != nil {
		t.Fatal(err)
	}
	row = writes[0].body["data"].([]any)[0].(map[string]any)
	if row["vatRate"] != nil || row["price"].(map[string]any)["price"] != "12.30" || !strings.HasSuffix(writes[0].path, "/prices") {
		t.Errorf("neplatiteľ %+v", row)
	}
}
