package eshop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestShoptet(t *testing.T) {
	var patch map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Shoptet-Private-API-Token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		switch r.URL.Path {
		case "/api/eshop":
			io.WriteString(w, `{"data":{"contactInformation":{"eshopName":"Môj Shoptet"}},"errors":null}`)
		case "/api/orders":
			if q.Get("creationTimeFrom") != "2026-10-01T00:00:00+0000" || q.Get("itemsPerPage") != "50" {
				t.Errorf("parametre %v", q)
			}
			if q.Get("page") == "2" {
				io.WriteString(w, `{"data":{"orders":[{"code":"2026000002"}],"paginator":{"totalCount":2,"page":2,"pageCount":2}},"errors":null}`)
				return
			}
			io.WriteString(w, `{"data":{"orders":[{"code":"2026000001"}],"paginator":{"totalCount":2,"page":1,"pageCount":2}},"errors":null}`)
		case "/api/orders/2026000001":
			io.WriteString(w, `{"data":{"order":{"code":"2026000001","creationTime":"2026-10-05T10:00:00+0200","status":{"id":-2,"name":"Vybavená"},"paid":true,
 "email":"jan@firma.sk","phone":"+421900111222","price":{"withVat":"28.30","withoutVat":"23.00","vat":"5.30","currencyCode":"EUR"},
 "billingAddress":{"company":"Firma s.r.o.","fullName":"Ján Novák","street":"Hlavná","houseNumber":"12","city":"Žilina","zip":"010 01","countryCode":"SK",
  "companyId":"36000001","vatId":"SK2020000001","taxId":"2020000001"},
 "deliveryAddress":{"fullName":"Ján Novák","street":"Sklad","houseNumber":"5","city":"Martin","zip":"036 01","countryCode":"SK"},
 "items":[{"itemType":"product","code":"REL24","name":"Relé","variantName":"24 V","amount":"2.000","itemPrice":{"withVat":"12.30","withoutVat":"10.00","vat":"2.30","vatRate":"23.00"}},
  {"itemType":"shipping","name":"Packeta","amount":"1","itemPrice":{"withVat":"4.92","withoutVat":"4.00","vat":"0.92","vatRate":"23.00"}},
  {"itemType":"billing","name":"Dobierka","amount":"1","itemPrice":{"withVat":"1.23","withoutVat":"1.00","vat":"0.23","vatRate":"23.00"}},
  {"itemType":"discount-coupon","code":"ZLAVA","name":"Kupón","amount":"1","itemPrice":{"withVat":"-2.46","withoutVat":"-2.00","vat":"-0.46","vatRate":"23.00"}}]}},"errors":null}`)
		case "/api/orders/2026000002":
			io.WriteString(w, `{"data":{"order":{"code":"2026000002","creationTime":"2026-10-06T10:00:00+0200","status":{"id":-4,"name":"Stornovaná"},"paid":null,
 "price":{"withVat":"10","withoutVat":"10","vat":"0","currencyCode":"CZK"},"billingAddress":{"fullName":"Eva"},"items":[]}},"errors":null}`)
		case "/api/stocks":
			io.WriteString(w, `{"data":{"stocks":[{"id":1,"title":"Hlavný"}],"defaultStockId":1},"errors":null}`)
		case "/api/stocks/1/movements":
			json.NewDecoder(r.Body).Decode(&patch)
			items := patch["data"].([]any)
			if items[0].(map[string]any)["productCode"] == "NIE" {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"data":null,"errors":[{"errorCode":"invalid-request-data","message":"Product with code \"NIE\" not found.","instance":"data[0].productCode"}]}`)
				return
			}
			io.WriteString(w, `{"data":null,"errors":null}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := ShoptetAPI
	ShoptetAPI = srv.URL
	defer func() { ShoptetAPI = old }()
	c, err := New(Conn{Platform: Shoptet, URL: "https://mojobchod.sk", Secret: "tok", HTTP: Client(true)})
	if err != nil {
		t.Fatal(err)
	}
	if name, err := c.Check(context.Background()); err != nil || name != "Môj Shoptet" {
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
	if !o.Paid || o.Payment != "cod" || o.Billing.Street != "Hlavná 12" || o.Billing.ICO != "36000001" || o.Billing.ICDPH != "SK2020000001" ||
		o.Billing.DIC != "2020000001" || o.Billing.Email != "jan@firma.sk" || o.Shipping.City != "Martin" || o.Created.Hour() != 8 {
		t.Errorf("objednávka %+v", o)
	}
	if len(o.Lines) != 4 || o.Lines[0].Name != "Relé 24 V" || !o.Lines[0].Net.Equal(money.FromInt(20)) || !o.Lines[0].Tax.Equal(money.MustParse("4.6")) ||
		!o.Lines[1].Shipping || o.Lines[3].SKU != "" || !o.Lines[3].Net.Equal(money.FromInt(-2)) {
		t.Errorf("riadky %+v", o.Lines)
	}
	if !orders[1].Cancelled || orders[1].Paid || orders[1].Currency != "CZK" {
		t.Errorf("stornovaná %+v", orders[1])
	}
	if err := c.SetStock(context.Background(), "REL24", 7); err != nil {
		t.Fatal(err)
	}
	if it := patch["data"].([]any)[0].(map[string]any); it["realStock"] != float64(7) || it["productCode"] != "REL24" {
		t.Errorf("PATCH %v", patch)
	}
	if err := c.SetStock(context.Background(), "NIE", 1); err != ErrNoSKU {
		t.Errorf("neznámy kód: %v", err)
	}
}
