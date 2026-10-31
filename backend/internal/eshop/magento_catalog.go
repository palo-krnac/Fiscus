package eshop

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Magento: cenník a zákazníci. Sadzba DPH je trieda dane produktu
// (tax_class_id); sadzbu triedy určia daňové pravidlá a sadzby (pre SK).
// Ceny sa berú bez DPH (predvolené nastavenie Magenta „Catalog Prices:
// Excluding Tax“). Zápis ide do globálneho rozsahu (/rest/all).

type mgTax struct {
	classRate map[string]int
	rateClass map[int]string
}

func (m *magento) taxes(ctx context.Context) (*mgTax, error) {
	if m.tax != nil {
		return m.tax, nil
	}
	t := &mgTax{classRate: map[string]int{}, rateClass: map[int]string{}}
	var rates struct {
		Items []struct {
			ID      int64  `json:"id"`
			Country string `json:"tax_country_id"`
			Rate    num    `json:"rate"`
		} `json:"items"`
	}
	if _, err := call(ctx, m.c.HTTP, http.MethodGet, m.api("/taxRates/search?searchCriteria[pageSize]=500"), nil, &rates, m.auth); err != nil {
		return nil, err
	}
	type rr struct {
		rate int
		sk   bool
	}
	byID := map[int64]rr{}
	for _, r := range rates.Items {
		byID[r.ID] = rr{int(r.Rate.dec().RoundTo(0).Raw() / money.FromInt(1).Raw()), strings.EqualFold(r.Country, "SK")}
	}
	var rules struct {
		Items []struct {
			ProductClasses []int64 `json:"product_tax_class_ids"`
			Rates          []int64 `json:"tax_rate_ids"`
		} `json:"items"`
	}
	if _, err := call(ctx, m.c.HTTP, http.MethodGet, m.api("/taxRules/search?searchCriteria[pageSize]=500"), nil, &rules, m.auth); err != nil {
		return nil, err
	}
	sk := map[string]bool{}
	for _, rule := range rules.Items {
		for _, pc := range rule.ProductClasses {
			class := strconv.FormatInt(pc, 10)
			for _, id := range rule.Rates {
				r, ok := byID[id]
				if !ok || (sk[class] && !r.sk) {
					continue
				}
				if _, seen := t.classRate[class]; !seen || (r.sk && !sk[class]) {
					t.classRate[class], sk[class] = r.rate, r.sk
				}
			}
		}
	}
	t.classRate["0"] = 0 // „None“
	for c, r := range t.classRate {
		if old, ok := t.rateClass[r]; !ok || len(c) < len(old) || (len(c) == len(old) && c < old) {
			t.rateClass[r] = c
		}
	}
	m.tax = t
	return t, nil
}

type mgProduct struct {
	SKU    string `json:"sku"`
	Name   string `json:"name"`
	Price  num    `json:"price"`
	Status int    `json:"status"`
	Type   string `json:"type_id"`
	Custom []struct {
		Code  string `json:"attribute_code"`
		Value any    `json:"value"`
	} `json:"custom_attributes"`
}

func (p mgProduct) taxClass() string {
	for _, c := range p.Custom {
		if c.Code == "tax_class_id" {
			return strings.TrimSpace(fmt.Sprint(c.Value))
		}
	}
	return "0"
}

func (m *magento) Products(ctx context.Context) ([]Product, error) {
	t, err := m.taxes(ctx)
	if err != nil {
		return nil, err
	}
	var out []Product
	for page := 1; len(out) < MaxProducts; page++ {
		q := url.Values{"searchCriteria[pageSize]": {"100"}, "searchCriteria[currentPage]": {strconv.Itoa(page)}}
		var res struct {
			Items      []mgProduct `json:"items"`
			TotalCount int         `json:"total_count"`
		}
		if _, err := call(ctx, m.c.HTTP, http.MethodGet, m.api("/products?"+q.Encode()), nil, &res, m.auth); err != nil {
			return nil, err
		}
		for _, p := range res.Items {
			if p.Type == "configurable" || p.Type == "bundle" || p.Type == "grouped" || p.SKU == "" {
				continue // cenu majú jednoduché produkty (varianty)
			}
			rate, ok := t.classRate[p.taxClass()]
			if !ok {
				rate = -1
			}
			pr := Product{SKU: p.SKU, Name: strings.TrimSpace(p.Name), Rate: rate, Active: p.Status == 1, Net: p.Price.dec(), Gross: p.Price.dec()}
			if rate > 0 {
				pr.Gross = pr.Net.MulRatio(money.FromInt(int64(100+rate)), money.FromInt(100)).RoundCents()
			}
			out = append(out, pr)
		}
		if len(res.Items) == 0 || page*100 >= res.TotalCount {
			break
		}
	}
	return out, nil
}

func (m *magento) Customers(ctx context.Context) ([]Address, error) {
	var out []Address
	for page := 1; len(out) < MaxProducts; page++ {
		q := url.Values{"searchCriteria[pageSize]": {"100"}, "searchCriteria[currentPage]": {strconv.Itoa(page)}}
		var res struct {
			Items []struct {
				Email     string `json:"email"`
				Firstname string `json:"firstname"`
				Lastname  string `json:"lastname"`
				Taxvat    string `json:"taxvat"`
				Addresses []struct {
					mgAddr
					DefaultBilling bool `json:"default_billing"`
				} `json:"addresses"`
			} `json:"items"`
			TotalCount int `json:"total_count"`
		}
		if _, err := call(ctx, m.c.HTTP, http.MethodGet, m.api("/customers/search?"+q.Encode()), nil, &res, m.auth); err != nil {
			return nil, err
		}
		for _, c := range res.Items {
			var a Address
			for i, ad := range c.Addresses {
				if ad.DefaultBilling || i == 0 {
					x := ad.mgAddr
					a = x.address()
				}
			}
			a.Name = firstOf(a.Name, joinNonEmpty(" ", c.Firstname, c.Lastname))
			a.Email = firstOf(a.Email, c.Email)
			a.ICDPH = firstOf(a.ICDPH, c.Taxvat)
			out = append(out, a)
		}
		if len(res.Items) == 0 || page*100 >= res.TotalCount {
			break
		}
	}
	return out, nil
}

func (m *magento) PushProduct(ctx context.Context, p Product) (bool, error) {
	t, err := m.taxes(ctx)
	if err != nil {
		return false, err
	}
	class, ok := t.rateClass[p.Rate]
	if !ok {
		return false, fmt.Errorf("obchod nemá triedu dane so sadzbou %d %% — pridajte daňové pravidlo v Magente (Stores → Tax Rules)", p.Rate)
	}
	all := m.c.URL + "/rest/all/V1/products"
	var cur struct {
		Status int `json:"status"`
	}
	_, err = call(ctx, m.c.HTTP, http.MethodGet, m.api("/products/"+url.PathEscape(p.SKU)), nil, &cur, m.auth)
	exists := err == nil
	if err != nil && !errors.Is(err, errNotFound) {
		return false, err
	}
	prod := map[string]any{"sku": p.SKU, "name": p.Name, "price": p.Net.RoundCents().String(),
		"custom_attributes": []map[string]any{{"attribute_code": "tax_class_id", "value": class}}}
	if exists {
		if !p.Active {
			prod["status"] = 2
		} // vypnutý produkt v Magente ostane vypnutý — zapína sa v obchode
		_, err := call(ctx, m.c.HTTP, http.MethodPut, all+"/"+url.PathEscape(p.SKU), map[string]any{"product": prod}, nil, m.auth)
		return false, err
	}
	if !p.Active || !p.Create {
		return false, ErrNoSKU
	}
	prod["status"], prod["type_id"], prod["attribute_set_id"], prod["visibility"] = 2, "simple", 4, 4 // vypnutý = koncept
	_, err = call(ctx, m.c.HTTP, http.MethodPost, all, map[string]any{"product": prod}, nil, m.auth)
	return err == nil, err
}
