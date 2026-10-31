package eshop

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// WooCommerce: cenník a zákazníci. Ceny sú v obchode s DPH alebo bez
// (nastavenie woocommerce_prices_include_tax), sadzba DPH je trieda dane
// (tax_class) — sadzby tried sa načítajú z /taxes (pre SK, inak prvá).

type wooTax struct {
	loaded    bool
	inclTax   bool
	classRate map[string]int // trieda → % ("" = standard)
	rateClass map[int]string
}

func (w *woo) taxes(ctx context.Context) (*wooTax, error) {
	if w.tax != nil && w.tax.loaded {
		return w.tax, nil
	}
	t := &wooTax{classRate: map[string]int{}, rateClass: map[int]string{}}
	var set struct {
		Value string `json:"value"`
	}
	if _, err := call(ctx, w.c.HTTP, http.MethodGet, w.api("/settings/tax/woocommerce_prices_include_tax"), nil, &set, w.auth); err != nil {
		return nil, err
	}
	t.inclTax = set.Value == "yes"
	var rates []struct {
		Country string `json:"country"`
		Rate    num    `json:"rate"`
		Class   string `json:"class"`
	}
	if _, err := call(ctx, w.c.HTTP, http.MethodGet, w.api("/taxes?per_page=100"), nil, &rates, w.auth); err != nil {
		return nil, err
	}
	// sadzba triedy: slovenská, inak sadzba bez štátu, inak prvá
	prio := map[string]int{}
	for _, r := range rates {
		class := r.Class
		if class == "standard" {
			class = ""
		}
		p := 1
		switch strings.ToUpper(r.Country) {
		case "SK":
			p = 3
		case "":
			p = 2
		}
		if p > prio[class] {
			prio[class] = p
			t.classRate[class] = int(r.Rate.dec().RoundTo(0).Raw() / money.FromInt(1).Raw())
		}
	}
	for c, r := range t.classRate {
		// viac tried s rovnakou sadzbou: prednosť má štandardná, inak abecedne prvá
		if old, ok := t.rateClass[r]; !ok || c == "" || (old != "" && c < old) {
			t.rateClass[r] = c
		}
	}
	if _, ok := t.classRate["zero-rate"]; !ok {
		t.classRate["zero-rate"] = 0
		if _, ok := t.rateClass[0]; !ok {
			t.rateClass[0] = "zero-rate"
		}
	}
	t.loaded = true
	w.tax = t
	return t, nil
}

type wooProduct struct {
	ID           int64  `json:"id"`
	ParentID     int64  `json:"parent_id"`
	Type         string `json:"type"`
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	RegularPrice num    `json:"regular_price"`
	TaxClass     string `json:"tax_class"`
	TaxStatus    string `json:"tax_status"`
	Status       string `json:"status"`
	Attributes   []struct {
		Option string `json:"option"`
	} `json:"attributes"`
}

func (w *woo) product(t *wooTax, p wooProduct, parent *wooProduct) Product {
	class := p.TaxClass
	if class == "parent" && parent != nil {
		class = parent.TaxClass
	}
	rate, ok := t.classRate[class]
	if p.TaxStatus == "none" {
		rate, ok = 0, true
	}
	if !ok {
		rate = -1
	}
	name := p.Name
	if parent != nil {
		var opts []string
		for _, a := range p.Attributes {
			opts = append(opts, a.Option)
		}
		name = joinNonEmpty(" ", parent.Name, strings.Join(opts, ", "))
	}
	out := Product{SKU: strings.TrimSpace(p.SKU), Name: strings.TrimSpace(name), Rate: rate, Active: p.Status == "publish"}
	price := p.RegularPrice.dec()
	switch {
	case rate < 0:
		out.Net, out.Gross = price, price
	case t.inclTax:
		out.Gross = price
		out.Net = price.MulRatio(money.FromInt(100), money.FromInt(int64(100+rate))).RoundCents()
	default:
		out.Net = price
		out.Gross = price.MulRatio(money.FromInt(int64(100+rate)), money.FromInt(100)).RoundCents()
	}
	return out
}

func (w *woo) Products(ctx context.Context) ([]Product, error) {
	t, err := w.taxes(ctx)
	if err != nil {
		return nil, err
	}
	var out []Product
	for page := 1; len(out) < MaxProducts; page++ {
		var list []wooProduct
		h, err := call(ctx, w.c.HTTP, http.MethodGet, w.api("/products?per_page=100&page="+strconv.Itoa(page)), nil, &list, w.auth)
		if err != nil {
			return nil, err
		}
		for _, p := range list {
			if p.Type == "variable" {
				var vars []wooProduct
				if _, err := call(ctx, w.c.HTTP, http.MethodGet, w.api(fmt.Sprintf("/products/%d/variations?per_page=100", p.ID)), nil, &vars, w.auth); err != nil {
					return nil, err
				}
				for _, v := range vars {
					if v.SKU != "" {
						out = append(out, w.product(t, v, &p))
					}
				}
				continue
			}
			if p.SKU != "" {
				out = append(out, w.product(t, p, nil))
			}
		}
		pages, _ := strconv.Atoi(h.Get("X-WP-TotalPages"))
		if len(list) == 0 || page >= pages {
			break
		}
	}
	return out, nil
}

func (w *woo) Customers(ctx context.Context) ([]Address, error) {
	var out []Address
	for page := 1; len(out) < MaxProducts; page++ {
		var list []struct {
			Email     string  `json:"email"`
			FirstName string  `json:"first_name"`
			LastName  string  `json:"last_name"`
			Billing   wooAddr `json:"billing"`
		}
		h, err := call(ctx, w.c.HTTP, http.MethodGet, w.api("/customers?per_page=100&role=all&page="+strconv.Itoa(page)), nil, &list, w.auth)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			a := c.Billing.address()
			if a.Name == "" {
				a.Name = joinNonEmpty(" ", c.FirstName, c.LastName)
			}
			a.Email = firstOf(a.Email, c.Email)
			out = append(out, a)
		}
		pages, _ := strconv.Atoi(h.Get("X-WP-TotalPages"))
		if len(list) == 0 || page >= pages {
			break
		}
	}
	return out, nil
}

func (w *woo) PushProduct(ctx context.Context, p Product) (bool, error) {
	t, err := w.taxes(ctx)
	if err != nil {
		return false, err
	}
	class, ok := t.rateClass[p.Rate]
	if !ok {
		return false, fmt.Errorf("obchod nemá triedu dane so sadzbou %d %% — pridajte ju vo WooCommerce → Nastavenia → Dane", p.Rate)
	}
	price := p.Net
	if t.inclTax {
		price = p.Gross
	}
	status := "private"
	if p.Active {
		status = "publish"
	}
	body := map[string]any{"regular_price": price.RoundCents().String(), "tax_class": class, "tax_status": "taxable"}
	var found []wooProduct
	if _, err := call(ctx, w.c.HTTP, http.MethodGet, w.api("/products?"+url.Values{"sku": {p.SKU}}.Encode()), nil, &found, w.auth); err != nil {
		return false, err
	}
	for _, f := range found {
		if !strings.EqualFold(f.SKU, p.SKU) {
			continue
		}
		path := fmt.Sprintf("/products/%d", f.ID)
		if f.Type == "variation" && f.ParentID != 0 {
			path = fmt.Sprintf("/products/%d/variations/%d", f.ParentID, f.ID)
		} else {
			body["name"] = p.Name
		}
		if !p.Active || f.Status == "publish" || f.Status == "private" {
			body["status"] = status // koncept ostane konceptom, kým ho obchod nezverejní
		}
		_, err := call(ctx, w.c.HTTP, http.MethodPut, w.api(path), body, nil, w.auth)
		return false, err
	}
	if !p.Active || !p.Create {
		return false, ErrNoSKU // vyradenú alebo nepredajnú položku nezakladať
	}
	body["name"], body["sku"], body["type"], body["status"] = p.Name, p.SKU, "simple", "draft"
	_, err = call(ctx, w.c.HTTP, http.MethodPost, w.api("/products"), body, nil, w.auth)
	return err == nil, err
}
