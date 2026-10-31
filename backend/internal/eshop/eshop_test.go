package eshop

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		p       Platform
		in      string
		want    string
		wantErr bool
	}{
		{WooCommerce, "https://obchod.sk/", "https://obchod.sk", false},
		{WooCommerce, "obchod.sk/shop", "https://obchod.sk/shop", false},
		{WooCommerce, "http://obchod.sk", "", true},
		{WooCommerce, "https://user:pass@obchod.sk", "", true},
		{WooCommerce, "https://obchod.sk/?x=1", "", true},
		{WooCommerce, "", "", true},
		{Shopify, "mojobchod", "https://mojobchod.myshopify.com", false},
		{Shopify, "mojobchod.myshopify.com", "https://mojobchod.myshopify.com", false},
		{WooCommerce, "http://127.0.0.1:8080", "http://127.0.0.1:8080", false},
	}
	for _, tt := range tests {
		got, err := NormalizeURL(tt.p, tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("NormalizeURL(%s, %q) = %q, %v; chcem %q, chyba %v", tt.p, tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestBlockedIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true}, {"10.1.2.3", true}, {"192.168.0.10", true}, {"169.254.169.254", true},
		{"::1", true}, {"fd00::1", true}, {"0.0.0.0", true}, {"93.184.216.34", false}, {"2a00:1450::1", false},
	}
	for _, tt := range tests {
		if got := blockedIP(net.ParseIP(tt.ip)); got != tt.want {
			t.Errorf("blockedIP(%s) = %v", tt.ip, got)
		}
	}
}

func TestClientRefusesPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, err := Client(false).Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "vnútornej sieti") {
		t.Fatalf("chcem odmietnutie vnútornej siete, mám %v", err)
	}
	if _, err := Client(true).Get(srv.URL); err != nil {
		t.Fatal(err)
	}
}

const wooOrders = `[{"id":727,"number":"727","status":"processing","currency":"EUR",
 "date_created_gmt":"2026-10-02T08:15:00","date_paid_gmt":"2026-10-02T08:20:00","total":"30.75","payment_method":"cod","customer_note":"Prosím rýchlo",
 "billing":{"first_name":"Ján","last_name":"Novák","company":"Novák s.r.o.","address_1":"Hlavná 1","address_2":"","city":"Žilina","postcode":"010 01","country":"SK","email":"jan@novak.sk","phone":"+421900111222"},
 "shipping":{"first_name":"Ján","last_name":"Novák","company":"","address_1":"Sklad 5","address_2":"","city":"Martin","postcode":"036 01","country":"SK"},
 "line_items":[{"name":"Kábel CYKY","sku":"CYKY3","quantity":2,"total":"20.00","total_tax":"4.60"}],
 "shipping_lines":[{"method_title":"Kuriér","total":"5.00","total_tax":"1.15"}],
 "fee_lines":[],
 "meta_data":[{"key":"_billing_ico","value":"36000001"},{"key":"_billing_dic_dph","value":"SK2020000001"},{"key":"other","value":{"x":1}}]},
 {"id":728,"number":"728","status":"cancelled","currency":"EUR","date_created_gmt":"2026-10-03T08:15:00","date_paid_gmt":null,"total":"10.00",
  "billing":{},"shipping":{},"line_items":[],"shipping_lines":[],"fee_lines":[],"meta_data":[]}]`

func TestWooCommerce(t *testing.T) {
	var put map[string]any
	var putPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if k, s, ok := r.BasicAuth(); r.URL.Path != "/wp-json/" && (!ok || k != "ck_1" || s != "cs_1") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/wp-json/":
			io.WriteString(w, `{"name":"Elektro obchod"}`)
		case r.URL.Path == "/wp-json/wc/v3/orders":
			if r.URL.Query().Get("per_page") == "100" && r.URL.Query().Get("after") != "2026-10-01T00:00:00" {
				t.Errorf("after = %q", r.URL.Query().Get("after"))
			}
			w.Header().Set("X-WP-TotalPages", "1")
			io.WriteString(w, wooOrders)
		case r.URL.Path == "/wp-json/wc/v3/products" && r.Method == http.MethodGet:
			if r.URL.Query().Get("sku") == "VAR1" {
				io.WriteString(w, `[{"id":51,"parent_id":50,"type":"variation","sku":"VAR1"}]`)
				return
			}
			io.WriteString(w, `[]`)
		case r.Method == http.MethodPut:
			putPath = r.URL.Path
			json.NewDecoder(r.Body).Decode(&put)
			io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, err := New(Conn{Platform: WooCommerce, URL: srv.URL, Key: "ck_1", Secret: "cs_1", HTTP: Client(true)})
	if err != nil {
		t.Fatal(err)
	}
	name, err := c.Check(context.Background())
	if err != nil || name != "Elektro obchod" {
		t.Fatalf("Check = %q, %v", name, err)
	}
	orders, err := c.Orders(context.Background(), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 {
		t.Fatalf("objednávok %d", len(orders))
	}
	o := orders[0]
	if !o.Paid || o.Cancelled || o.Number != "727" || o.Billing.Company != "Novák s.r.o." || o.Billing.ICO != "36000001" || o.Payment != "cod" ||
		o.Billing.ICDPH != "SK2020000001" || o.Shipping.City != "Martin" || !o.Total.Equal(money.MustParse("30.75")) {
		t.Errorf("objednávka %+v", o)
	}
	if len(o.Lines) != 2 || o.Lines[0].SKU != "CYKY3" || !o.Lines[0].Net.Equal(money.FromInt(20)) || !o.Lines[1].Shipping {
		t.Errorf("riadky %+v", o.Lines)
	}
	if !orders[1].Cancelled || orders[1].Paid {
		t.Errorf("zrušená %+v", orders[1])
	}
	if err := c.SetStock(context.Background(), "VAR1", 7); err != nil {
		t.Fatal(err)
	}
	if putPath != "/wp-json/wc/v3/products/50/variations/51" || put["stock_quantity"] != float64(7) || put["manage_stock"] != true {
		t.Errorf("PUT %s %v", putPath, put)
	}
	if err := c.SetStock(context.Background(), "NIE", 1); err != ErrNoSKU {
		t.Errorf("neznámy kód: %v", err)
	}
	bad, _ := New(Conn{Platform: WooCommerce, URL: srv.URL, Key: "x", Secret: "y", HTTP: Client(true)})
	if _, err := bad.Check(context.Background()); err != ErrAuth {
		t.Errorf("zlý kľúč: %v", err)
	}
}

func TestShopify(t *testing.T) {
	var lastInput map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Shopify-Access-Token") != "shpat_1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/admin/api/"+ShopifyVersion+"/graphql.json" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		switch {
		case strings.Contains(req.Query, "shop { name }"):
			io.WriteString(w, `{"data":{"shop":{"name":"Môj Shopify"}}}`)
		case strings.Contains(req.Query, "orders("):
			if req.Variables["after"] == nil {
				io.WriteString(w, `{"data":{"orders":{"pageInfo":{"hasNextPage":true,"endCursor":"c1"},"nodes":[
 {"id":"gid://shopify/Order/1","name":"#1001","createdAt":"2026-10-05T10:00:00Z","processedAt":"2026-10-05T10:01:00Z","cancelledAt":null,
  "displayFinancialStatus":"PAID","currencyCode":"EUR","taxesIncluded":true,"email":"eva@example.sk","note":null,"paymentGatewayNames":["shopify_payments"],
  "totalPriceSet":{"shopMoney":{"amount":"24.6"}},
  "billingAddress":{"name":"Eva Malá","company":null,"address1":"Nová 2","address2":null,"city":"Nitra","zip":"94901","countryCodeV2":"SK","phone":null},
  "shippingAddress":null,
  "lineItems":{"nodes":[{"name":"Zásuvka","sku":"ZAS1","quantity":2,"discountedTotalSet":{"shopMoney":{"amount":"24.6"}},"taxLines":[{"priceSet":{"shopMoney":{"amount":"4.6"}}}]}]},
  "shippingLines":{"nodes":[]}}]}}}`)
				return
			}
			io.WriteString(w, `{"data":{"orders":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[
 {"id":"gid://shopify/Order/2","name":"#1002","createdAt":"2026-10-06T10:00:00Z","processedAt":"2026-10-06T10:00:00Z","cancelledAt":"2026-10-06T11:00:00Z",
  "displayFinancialStatus":"PENDING","currencyCode":"CZK","taxesIncluded":false,"email":"","totalPriceSet":{"shopMoney":{"amount":"100"}},
  "billingAddress":null,"shippingAddress":null,"lineItems":{"nodes":[]},"shippingLines":{"nodes":[]}}]}}}`)
		case strings.Contains(req.Query, "productVariants"):
			if req.Variables["q"] != `sku:"ZAS1"` {
				io.WriteString(w, `{"data":{"productVariants":{"nodes":[]},"locations":{"nodes":[{"id":"gid://shopify/Location/9"}]}}}`)
				return
			}
			io.WriteString(w, `{"data":{"productVariants":{"nodes":[{"sku":"ZAS1","inventoryItem":{"id":"gid://shopify/InventoryItem/5"}}]},"locations":{"nodes":[{"id":"gid://shopify/Location/9"}]}}}`)
		case strings.Contains(req.Query, "inventorySetQuantities"):
			lastInput, _ = req.Variables["input"].(map[string]any)
			io.WriteString(w, `{"data":{"inventorySetQuantities":{"userErrors":[]}}}`)
		default:
			io.WriteString(w, `{"errors":[{"message":"unexpected"}]}`)
		}
	}))
	defer srv.Close()
	c, err := New(Conn{Platform: Shopify, URL: srv.URL, Secret: "shpat_1", HTTP: Client(true)})
	if err != nil {
		t.Fatal(err)
	}
	if name, err := c.Check(context.Background()); err != nil || name != "Môj Shopify" {
		t.Fatalf("Check = %q, %v", name, err)
	}
	orders, err := c.Orders(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 {
		t.Fatalf("objednávok %d", len(orders))
	}
	o := orders[0]
	if o.Number != "1001" || !o.Paid || o.Billing.Name != "Eva Malá" || o.Billing.Email != "eva@example.sk" || o.Payment != "card" ||
		!o.Lines[0].Net.Equal(money.FromInt(20)) || !o.Lines[0].Tax.Equal(money.MustParse("4.6")) {
		t.Errorf("objednávka %+v", o)
	}
	if !orders[1].Cancelled || orders[1].Currency != "CZK" {
		t.Errorf("druhá %+v", orders[1])
	}
	if err := c.SetStock(context.Background(), "ZAS1", 3); err != nil {
		t.Fatal(err)
	}
	qs, _ := lastInput["quantities"].([]any)
	if len(qs) != 1 || qs[0].(map[string]any)["quantity"] != float64(3) || qs[0].(map[string]any)["locationId"] != "gid://shopify/Location/9" {
		t.Errorf("vstup %v", lastInput)
	}
	if err := c.SetStock(context.Background(), "NIE", 1); err != ErrNoSKU {
		t.Errorf("neznámy kód: %v", err)
	}
}

func TestPaymentOf(t *testing.T) {
	tests := map[string]string{"": "", "cod": "cod", "Dobierka": "cod", "bacs": "transfer", "Bankový prevod": "transfer",
		"manual": "transfer", "stripe": "card", "gopay": "card", "Hotovosť": "cash"}
	for in, want := range tests {
		if got := paymentOf(in); got != want {
			t.Errorf("paymentOf(%q) = %q, chcem %q", in, got, want)
		}
	}
}
