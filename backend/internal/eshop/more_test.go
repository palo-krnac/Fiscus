package eshop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestPrestaShop(t *testing.T) {
	var putBody, putPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if k, p, ok := r.BasicAuth(); !ok || k != "PSKEY" || p != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		if r.Method == http.MethodPut {
			putPath = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			return
		}
		if q.Get("output_format") != "JSON" {
			t.Errorf("chýba output_format: %s", r.URL)
		}
		switch r.URL.Path {
		case "/api/configurations":
			io.WriteString(w, `{"configurations":[{"value":"Elektro PS"}]}`)
		case "/api/order_states":
			io.WriteString(w, `{"order_states":[{"id":2,"paid":"1","name":[{"id":"1","value":"Platba prijatá"}]},{"id":6,"paid":"0","name":"Zrušená"},{"id":10,"paid":"0","name":"Čaká na platbu"}]}`)
		case "/api/countries":
			io.WriteString(w, `{"countries":[{"id":37,"iso_code":"SK"}]}`)
		case "/api/currencies":
			io.WriteString(w, `{"currencies":[{"id":1,"iso_code":"EUR"}]}`)
		case "/api/orders":
			if q.Get("limit") != "0,100" {
				io.WriteString(w, `[]`)
				return
			}
			if !strings.HasPrefix(q.Get("filter[date_add]"), "[2026-10-01 02:00:00,") {
				t.Errorf("filter %q", q.Get("filter[date_add]"))
			}
			io.WriteString(w, `{"orders":[{"id":5,"reference":"XKBKNABJK","id_address_delivery":"8","id_address_invoice":"7","id_customer":"3","id_currency":"1",
 "current_state":"2","payment":"Bankový prevod","total_paid_tax_incl":"30.750000","total_shipping_tax_excl":"5.000000","total_shipping_tax_incl":"6.150000",
 "total_discounts_tax_excl":"0.000000","total_discounts_tax_incl":"0.000000","total_wrapping_tax_excl":"0","total_wrapping_tax_incl":"0","date_add":"2026-10-02 10:00:00",
 "associations":{"order_rows":[{"product_name":"Kábel","product_reference":"CYKY3","product_quantity":"2","unit_price_tax_excl":"10.000000","unit_price_tax_incl":"12.300000"}]}},
 {"id":6,"reference":"ZRUS","id_address_delivery":"7","id_address_invoice":"7","id_customer":"3","id_currency":"1","current_state":"6","date_add":"2026-10-03 10:00:00","associations":{}}]}`)
		case "/api/addresses":
			io.WriteString(w, `{"addresses":[{"id":7,"firstname":"Ján","lastname":"Novák","company":"Novák s.r.o.","address1":"Hlavná 1","postcode":"01001","city":"Žilina","id_country":"37","vat_number":"SK2020000001","dni":"36000001"},
 {"id":8,"firstname":"Ján","lastname":"Novák","address1":"Sklad 5","postcode":"03601","city":"Martin","id_country":"37"}]}`)
		case "/api/customers":
			io.WriteString(w, `{"customers":[{"id":3,"email":"jan@novak.sk"}]}`)
		case "/api/products":
			if q.Get("filter[reference]") == "[CYKY3]" {
				io.WriteString(w, `{"products":[{"id":40}]}`)
				return
			}
			io.WriteString(w, `[]`)
		case "/api/combinations":
			if q.Get("filter[reference]") == "[VAR1]" {
				io.WriteString(w, `{"combinations":[{"id":9,"id_product":"41"}]}`)
				return
			}
			io.WriteString(w, `[]`)
		case "/api/stock_availables":
			io.WriteString(w, `{"stock_availables":[{"id":77,"id_product":"`+q.Get("filter[id_product]")[1:3]+`","id_product_attribute":"0","id_shop":"1","id_shop_group":"0","quantity":"3","depends_on_stock":"0","out_of_stock":"2"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, _ := New(Conn{Platform: PrestaShop, URL: srv.URL, Secret: "PSKEY", HTTP: Client(true)})
	if name, err := c.Check(context.Background()); err != nil || name != "Elektro PS" {
		t.Fatalf("Check %q %v", name, err)
	}
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) // 02:00 v Bratislave
	orders, err := c.Orders(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 {
		t.Fatalf("objednávok %d", len(orders))
	}
	o := orders[0]
	if o.Number != "XKBKNABJK" || !o.Paid || o.Currency != "EUR" || o.Billing.ICO != "36000001" || o.Billing.ICDPH != "SK2020000001" ||
		o.Billing.Country != "SK" || o.Billing.Email != "jan@novak.sk" || o.Shipping.City != "Martin" || o.Payment != "transfer" {
		t.Errorf("objednávka %+v", o)
	}
	if len(o.Lines) != 2 || !o.Lines[0].Net.Equal(money.FromInt(20)) || !o.Lines[0].Tax.Equal(money.MustParse("4.6")) || !o.Lines[1].Shipping ||
		!o.Lines[1].Tax.Equal(money.MustParse("1.15")) {
		t.Errorf("riadky %+v", o.Lines)
	}
	if !orders[1].Cancelled || !orders[1].Shipping.IsZero() {
		t.Errorf("zrušená %+v", orders[1])
	}
	if err := c.SetStock(context.Background(), "CYKY3", 12); err != nil {
		t.Fatal(err)
	}
	if putPath != "/api/stock_availables/77" || !strings.Contains(putBody, "<quantity>12</quantity>") || !strings.Contains(putBody, "<id_product>40</id_product>") {
		t.Errorf("PUT %s %s", putPath, putBody)
	}
	if err := c.SetStock(context.Background(), "VAR1", 1); err != nil {
		t.Fatal(err)
	}
	for _, sku := range []string{"NIE", "A|B", "[x]"} {
		if err := c.SetStock(context.Background(), sku, 1); err != ErrNoSKU {
			t.Errorf("%s: %v", sku, err)
		}
	}
}

func TestMagento(t *testing.T) {
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mgtok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/rest/V1/orders" && q.Get("fields") == "total_count":
			io.WriteString(w, `{"total_count":3}`)
		case r.URL.Path == "/rest/V1/store/storeViews":
			io.WriteString(w, `[{"name":"Admin"},{"name":"Magento obchod"}]`)
		case r.URL.Path == "/rest/V1/orders":
			if q.Get("searchCriteria[filter_groups][0][filters][0][value]") != "2026-10-01 00:00:00" {
				t.Errorf("filter %v", q)
			}
			io.WriteString(w, `{"total_count":2,"items":[{"entity_id":12,"increment_id":"000000012","created_at":"2026-10-02 08:00:00","state":"processing",
 "order_currency_code":"EUR","grand_total":30.75,"total_paid":30.75,"total_due":0,"customer_email":"eva@example.sk","shipping_description":"DPD - Kuriér",
 "shipping_amount":5,"shipping_tax_amount":1.15,"shipping_discount_amount":0,
 "billing_address":{"firstname":"Eva","lastname":"Malá","company":"Malá s.r.o.","street":["Nová 2","bl. A"],"city":"Nitra","postcode":"94901","country_id":"SK","telephone":"0900","vat_id":"SK2020000002"},
 "payment":{"method":"cashondelivery"},
 "items":[{"item_id":1,"parent_item_id":null,"sku":"TRIKO-M","name":"Tričko","qty_ordered":2,"row_total":22,"tax_amount":4.6,"discount_amount":2},
  {"item_id":2,"parent_item_id":1,"sku":"TRIKO-M","name":"Tričko M","qty_ordered":2,"row_total":0,"tax_amount":0,"discount_amount":0}],
 "extension_attributes":{"shipping_assignments":[{"shipping":{"address":{"firstname":"Eva","lastname":"Malá","street":["Sklad 1"],"city":"Nitra","postcode":"94901","country_id":"SK"}}}]}},
 {"entity_id":13,"increment_id":"000000013","created_at":"2026-10-03 08:00:00","state":"canceled","order_currency_code":"EUR","grand_total":10,"items":[]}]}`)
		case r.URL.Path == "/rest/V1/stockItems/TRIKO-M":
			io.WriteString(w, `{"item_id":55,"qty":3}`)
		case r.URL.Path == "/rest/V1/products/TRIKO-M/stockItems/55" && r.Method == http.MethodPut:
			json.NewDecoder(r.Body).Decode(&put)
			io.WriteString(w, `55`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, _ := New(Conn{Platform: Magento, URL: srv.URL, Secret: "mgtok", HTTP: Client(true)})
	if name, err := c.Check(context.Background()); err != nil || name != "Magento obchod" {
		t.Fatalf("Check %q %v", name, err)
	}
	orders, err := c.Orders(context.Background(), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	o := orders[0]
	if len(orders) != 2 || o.Number != "000000012" || !o.Paid || o.Payment != "cod" || o.Billing.Street != "Nová 2, bl. A" ||
		o.Billing.Email != "eva@example.sk" || o.Billing.ICDPH != "SK2020000002" || o.Shipping.Street != "Sklad 1" {
		t.Errorf("objednávka %+v", o)
	}
	if len(o.Lines) != 2 || !o.Lines[0].Net.Equal(money.FromInt(20)) || o.Lines[1].Name != "DPD - Kuriér" {
		t.Errorf("riadky %+v", o.Lines)
	}
	if !orders[1].Cancelled || orders[1].Paid {
		t.Errorf("zrušená %+v", orders[1])
	}
	if err := c.SetStock(context.Background(), "TRIKO-M", 4); err != nil {
		t.Fatal(err)
	}
	st, _ := put["stockItem"].(map[string]any)
	if st["qty"] != float64(4) || st["is_in_stock"] != true {
		t.Errorf("PUT %v", put)
	}
	if err := c.SetStock(context.Background(), "NIE", 1); err != ErrNoSKU {
		t.Errorf("neznámy kód: %v", err)
	}
}

func TestDrupalCommerce(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "fiskus" || p != "heslo" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		switch r.URL.Path {
		case "/jsonapi/commerce_store/online":
			io.WriteString(w, `{"data":[{"type":"commerce_store--online","id":"s1","attributes":{"name":"industrio"}}]}`)
		case "/jsonapi/commerce_order/default":
			if q.Get("page[limit]") == "1" {
				io.WriteString(w, `{"data":[]}`)
				return
			}
			if q.Get("page[offset]") == "50" { // odkaz „next“ (skutočný Drupal v ňom filtre zachová)
				io.WriteString(w, `{"data":[{"type":"commerce_order--default","id":"o2","attributes":{"order_number":"8","state":"canceled","placed":"2026-10-04T10:00:00+00:00","total_price":{"number":"5.00","currency_code":"EUR"}},"relationships":{}}],"links":{}}`)
				return
			}
			if q.Get("filter[cart][condition][value]") != "draft" || q.Get("filter[since][condition][value]") != "1790812800" {
				t.Errorf("filter %v", q)
			}
			io.WriteString(w, `{"data":[{"type":"commerce_order--default","id":"o1","attributes":{"order_number":"7","state":"completed","mail":"nakup@firma.sk",
  "placed":"2026-10-03T09:00:00+00:00","total_price":{"number":"30.75","currency_code":"EUR"},"total_paid":{"number":"30.75","currency_code":"EUR"},
  "adjustments":[{"type":"shipping","label":"Kuriér","amount":{"number":"6.15","currency_code":"EUR"},"included":false},{"type":"tax","label":"DPH","amount":{"number":"1.15"},"included":true}]},
  "relationships":{"order_items":{"data":[{"type":"commerce_order_item--default","id":"i1"}]},"billing_profile":{"data":{"type":"profile--customer","id":"p1"}}}}],
 "included":[{"type":"commerce_order_item--default","id":"i1","attributes":{"title":"Relé","quantity":"2.00","total_price":{"number":"24.60"},
   "adjustments":[{"type":"tax","amount":{"number":"4.60"},"included":true}]},"relationships":{"purchased_entity":{"data":{"type":"commerce_product_variation--default","id":"v1"}}}},
  {"type":"commerce_product_variation--default","id":"v1","attributes":{"sku":"REL24"}},
  {"type":"profile--customer","id":"p1","attributes":{"address":{"given_name":"Ján","family_name":"Novák","organization":"Firma s.r.o.","address_line1":"Hlavná 1","locality":"Zvolen","postal_code":"96001","country_code":"SK"},"tax_number":{"value":"SK2020000003"}}}],
 "links":{"next":{"href":"`+srvURL+`/jsonapi/commerce_order/default?page%5Boffset%5D=50&page%5Blimit%5D=50"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	c, _ := New(Conn{Platform: Drupal, URL: srv.URL, Key: "fiskus", Secret: "heslo", HTTP: Client(true)})
	if name, err := c.Check(context.Background()); err != nil || name != "industrio" {
		t.Fatalf("Check %q %v", name, err)
	}
	orders, err := c.Orders(context.Background(), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 {
		t.Fatalf("objednávok %d", len(orders))
	}
	o := orders[0]
	if o.Number != "7" || !o.Paid || o.Billing.Company != "Firma s.r.o." || o.Billing.Email != "nakup@firma.sk" || o.Billing.ICDPH != "SK2020000003" {
		t.Errorf("objednávka %+v", o)
	}
	if len(o.Lines) != 2 || o.Lines[0].SKU != "REL24" || !o.Lines[0].Net.Equal(money.FromInt(20)) ||
		!o.Lines[1].Net.Equal(money.FromInt(5)) || !o.Lines[1].Tax.Equal(money.MustParse("1.15")) {
		t.Errorf("riadky %+v", o.Lines)
	}
	if !orders[1].Cancelled {
		t.Errorf("zrušená %+v", orders[1])
	}
	if err := c.SetStock(context.Background(), "REL24", 1); err != ErrNoStock {
		t.Errorf("zásoby: %v", err)
	}
}
