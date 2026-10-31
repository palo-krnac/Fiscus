package eshop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Shoptet API (https://api.myshoptet.com), súkromný API token
// (Shoptet-Private-API-Token, balík Premium). Zoznam objednávok vracia
// iba súhrn, položky sú v detaile objednávky. Zásoby: PATCH
// /api/stocks/{id}/movements s realStock (fyzický stav na sklade).
// https://api.docs.shoptet.com/shoptet-api/openapi

// ShoptetAPI je adresa Shoptet API (v testoch falošný server).
var ShoptetAPI = "https://api.myshoptet.com"

type shoptet struct{ c Conn }

func (s *shoptet) auth(r *http.Request) { r.Header.Set("Shoptet-Private-API-Token", s.c.Secret) }

// stErr je chyba v odpovedi Shoptetu.
type stErr struct {
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
	Instance  string `json:"instance"`
}

func (s *shoptet) get(ctx context.Context, path string, out any) error {
	var res struct {
		Data   jsonRaw `json:"data"`
		Errors []stErr `json:"errors"`
	}
	if _, err := call(ctx, s.c.HTTP, http.MethodGet, ShoptetAPI+path, nil, &res, s.auth); err != nil {
		return err
	}
	if len(res.Errors) > 0 && len(res.Data) == 0 {
		return errors.New("Shoptet: " + res.Errors[0].Message)
	}
	return res.Data.decode(out)
}

func (s *shoptet) Check(ctx context.Context) (string, error) {
	var d struct {
		Contact struct {
			EshopName string `json:"eshopName"`
		} `json:"contactInformation"`
		URL string `json:"url"`
	}
	if err := s.get(ctx, "/api/eshop", &d); err != nil {
		if errors.Is(err, ErrAuth) {
			return "", err
		}
		// bez práva na údaje e-shopu stačí, že token vidí objednávky
		var o struct{}
		if err2 := s.get(ctx, "/api/orders?itemsPerPage=1", &o); err2 != nil {
			return "", err2
		}
		return s.c.URL, nil
	}
	return firstOf(d.Contact.EshopName, d.URL, s.c.URL), nil
}

// stTime je formát času Shoptetu (2017-12-12T22:08:01+0100).
const stTime = "2006-01-02T15:04:05-0700"

func parseStTime(v string) time.Time {
	for _, l := range []string{stTime, time.RFC3339} {
		if t, err := time.Parse(l, v); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

type stPrice struct {
	WithVat    num    `json:"withVat"`
	WithoutVat num    `json:"withoutVat"`
	Vat        num    `json:"vat"`
	VatRate    num    `json:"vatRate"`
	Currency   string `json:"currencyCode"`
}

type stAddr struct {
	Company     string `json:"company"`
	FullName    string `json:"fullName"`
	Street      string `json:"street"`
	HouseNumber string `json:"houseNumber"`
	City        string `json:"city"`
	Zip         string `json:"zip"`
	CountryCode string `json:"countryCode"`
	CompanyID   string `json:"companyId"`
	VatID       string `json:"vatId"`
	TaxID       string `json:"taxId"`
}

func (a *stAddr) address(email, phone string) Address {
	if a == nil {
		return Address{Email: email, Phone: phone}
	}
	out := Address{Name: strings.TrimSpace(a.FullName), Company: strings.TrimSpace(a.Company), Street: joinNonEmpty(" ", a.Street, a.HouseNumber),
		City: strings.TrimSpace(a.City), Zip: strings.TrimSpace(a.Zip), Country: strings.ToUpper(a.CountryCode), Email: email, Phone: phone,
		ICO: strings.TrimSpace(a.CompanyID)}
	// DIČ (iba číslice) a IČ DPH (s kódom štátu) — Shoptet ich má vo vatId a taxId
	for _, v := range []string{a.VatID, a.TaxID} {
		v = strings.ReplaceAll(strings.TrimSpace(v), " ", "")
		switch {
		case v == "":
		case len(v) > 2 && v[0] >= 'A' && v[0] <= 'Z' && v[1] >= 'A' && v[1] <= 'Z':
			if out.ICDPH == "" {
				out.ICDPH = v
			}
		default:
			if out.DIC == "" {
				out.DIC = v
			}
		}
	}
	return out
}

type stOrder struct {
	Code         string `json:"code"`
	CreationTime string `json:"creationTime"`
	Status       struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	} `json:"status"`
	Paid            *bool   `json:"paid"`
	Email           string  `json:"email"`
	Phone           string  `json:"phone"`
	Price           stPrice `json:"price"`
	BillingAddress  *stAddr `json:"billingAddress"`
	DeliveryAddress *stAddr `json:"deliveryAddress"`
	Items           []struct {
		ItemType  string  `json:"itemType"`
		Code      string  `json:"code"`
		Name      string  `json:"name"`
		Variant   string  `json:"variantName"`
		Amount    num     `json:"amount"`
		ItemPrice stPrice `json:"itemPrice"`
	} `json:"items"`
}

func (s *shoptet) Orders(ctx context.Context, since time.Time) ([]Order, error) {
	var out []Order
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "itemsPerPage": {"50"}}
		if !since.IsZero() {
			q.Set("creationTimeFrom", since.UTC().Format(stTime))
		}
		var d struct {
			Orders []struct {
				Code string `json:"code"`
			} `json:"orders"`
			Paginator struct {
				PageCount int `json:"pageCount"`
			} `json:"paginator"`
		}
		if err := s.get(ctx, "/api/orders?"+q.Encode(), &d); err != nil {
			return nil, err
		}
		for _, o := range d.Orders {
			var det struct {
				Order stOrder `json:"order"`
			}
			if err := s.get(ctx, "/api/orders/"+url.PathEscape(o.Code), &det); err != nil {
				return nil, err
			}
			out = append(out, det.Order.order())
		}
		if len(d.Orders) == 0 || page >= d.Paginator.PageCount || len(out) >= MaxOrders {
			return out, nil
		}
	}
}

func (o stOrder) order() Order {
	out := Order{ID: o.Code, Number: o.Code, Created: parseStTime(o.CreationTime), Status: o.Status.Name,
		Currency: strings.ToUpper(o.Price.Currency), Total: o.Price.WithVat.dec(),
		Billing: o.BillingAddress.address(strings.TrimSpace(o.Email), strings.TrimSpace(o.Phone)), Shipping: o.DeliveryAddress.address("", "")}
	n := strings.ToLower(o.Status.Name)
	out.Cancelled = strings.Contains(n, "storn") || strings.Contains(n, "zruš") || strings.Contains(n, "cancel")
	if o.Paid != nil && *o.Paid && !out.Cancelled {
		out.Paid, out.PaidAt = true, out.Created
	}
	for _, it := range o.Items {
		q := it.Amount.dec()
		if q.IsZero() {
			q = money.FromInt(1)
		}
		net := it.ItemPrice.WithoutVat.dec().Mul(q).RoundCents()
		gross := it.ItemPrice.WithVat.dec().Mul(q).RoundCents()
		switch it.ItemType {
		case "product-set-item":
			continue // súčasť sady: cena je pri sade
		case "billing":
			out.Payment = paymentOf(it.Name)
		}
		if it.ItemType == "billing" && net.IsZero() && gross.IsZero() {
			continue
		}
		name := it.Name
		if it.Variant != "" {
			name += " " + it.Variant
		}
		l := Line{SKU: strings.TrimSpace(it.Code), Name: strings.TrimSpace(name), Quantity: q, Net: net, Tax: gross.Sub(net), Shipping: it.ItemType == "shipping"}
		if it.ItemType != "product" && it.ItemType != "bazar" && it.ItemType != "product-set" {
			l.SKU = "" // doprava, platba, zľavy nie sú položky cenníka
			l.Quantity = money.FromInt(1)
		}
		out.Lines = append(out.Lines, l)
	}
	return out
}

func (s *shoptet) SetStock(ctx context.Context, sku string, qty int64) error {
	var st struct {
		DefaultStockID json.Number `json:"defaultStockId"`
	}
	if err := s.get(ctx, "/api/stocks", &st); err != nil {
		return err
	}
	body := map[string]any{"data": []map[string]any{{"productCode": sku, "realStock": qty}}}
	var res struct {
		Errors []stErr `json:"errors"`
	}
	_, err := call(ctx, s.c.HTTP, http.MethodPatch, ShoptetAPI+"/api/stocks/"+url.PathEscape(st.DefaultStockID.String())+"/movements", body, &res, s.auth)
	if err != nil {
		// jediný záznam s chybou → 400; neznámy kód je najčastejší dôvod
		if l := strings.ToLower(err.Error()); strings.Contains(l, "400") && (strings.Contains(l, "not found") || strings.Contains(l, "exist") || strings.Contains(l, "productcode")) {
			return ErrNoSKU
		}
		return err
	}
	if len(res.Errors) > 0 {
		return ErrNoSKU
	}
	return nil
}
