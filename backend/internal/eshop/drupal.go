package eshop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Drupal Commerce cez JSON:API (/jsonapi/commerce_order/default),
// prihlásenie HTTP Basic (modul basic_auth). Stav zásob Drupal Commerce
// bez doplnku commerce_stock nevedie — zásoby sa neposielajú.
// https://www.drupal.org/docs/core-modules-and-themes/core-modules/jsonapi-module

type drupal struct{ c Conn }

func (d *drupal) auth(r *http.Request) {
	r.SetBasicAuth(d.c.Key, d.c.Secret)
	r.Header.Set("Accept", "application/vnd.api+json")
}

type jaRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// jaRel je vzťah: jeden odkaz alebo zoznam.
type jaRel struct {
	Data jsonRaw `json:"data"`
}

func (r jaRel) refs() []jaRef {
	var list []jaRef
	if json.Unmarshal(r.Data, &list) == nil {
		return list
	}
	var one jaRef
	if json.Unmarshal(r.Data, &one) == nil && one.ID != "" {
		return []jaRef{one}
	}
	return nil
}

type jaResource struct {
	Type          string           `json:"type"`
	ID            string           `json:"id"`
	Attributes    jsonRaw          `json:"attributes"`
	Relationships map[string]jaRel `json:"relationships"`
}

type dcPrice struct {
	Number       num    `json:"number"`
	CurrencyCode string `json:"currency_code"`
}

type dcAdjustment struct {
	Type     string  `json:"type"`
	Label    string  `json:"label"`
	Amount   dcPrice `json:"amount"`
	Included bool    `json:"included"`
}

type dcOrder struct {
	OrderNumber string         `json:"order_number"`
	State       string         `json:"state"`
	Mail        string         `json:"mail"`
	Placed      jsonRaw        `json:"placed"`
	TotalPrice  *dcPrice       `json:"total_price"`
	TotalPaid   *dcPrice       `json:"total_paid"`
	Adjustments []dcAdjustment `json:"adjustments"`
	DrupalID    int64          `json:"drupal_internal__order_id"`
}

type dcItem struct {
	Title       string         `json:"title"`
	Quantity    num            `json:"quantity"`
	TotalPrice  *dcPrice       `json:"total_price"`
	Adjustments []dcAdjustment `json:"adjustments"`
}

type dcProfile struct {
	Address *struct {
		GivenName    string `json:"given_name"`
		FamilyName   string `json:"family_name"`
		Organization string `json:"organization"`
		AddressLine1 string `json:"address_line1"`
		AddressLine2 string `json:"address_line2"`
		Locality     string `json:"locality"`
		PostalCode   string `json:"postal_code"`
		CountryCode  string `json:"country_code"`
	} `json:"address"`
	TaxNumber *struct {
		Value string `json:"value"`
	} `json:"tax_number"`
}

func (d *drupal) list(ctx context.Context, u string, out any) error {
	_, err := call(ctx, d.c.HTTP, http.MethodGet, u, nil, out, d.auth)
	return err
}

func (d *drupal) Check(ctx context.Context) (string, error) {
	var res struct {
		Data []jaResource `json:"data"`
	}
	if err := d.list(ctx, d.c.URL+"/jsonapi/commerce_order/default?page[limit]=1", &res); err != nil {
		return "", err
	}
	var site struct {
		Data []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if d.list(ctx, d.c.URL+"/jsonapi/commerce_store/online?page[limit]=1", &site) == nil && len(site.Data) > 0 && site.Data[0].Attributes.Name != "" {
		return site.Data[0].Attributes.Name, nil
	}
	return d.c.URL, nil
}

const dcPage = 50

func (d *drupal) Orders(ctx context.Context, since time.Time) ([]Order, error) {
	q := url.Values{
		"include":                           {"order_items,order_items.purchased_entity,billing_profile"},
		"sort":                              {"placed"},
		"page[limit]":                       {strconv.Itoa(dcPage)},
		"filter[cart][condition][path]":     {"state"},
		"filter[cart][condition][operator]": {"<>"},
		"filter[cart][condition][value]":    {"draft"},
	}
	if !since.IsZero() {
		q.Set("filter[since][condition][path]", "placed")
		q.Set("filter[since][condition][operator]", ">=")
		q.Set("filter[since][condition][value]", strconv.FormatInt(since.Unix(), 10))
	}
	next := d.c.URL + "/jsonapi/commerce_order/default?" + q.Encode()
	var out []Order
	for next != "" {
		var res struct {
			Data     []jaResource `json:"data"`
			Included []jaResource `json:"included"`
			Links    struct {
				Next *struct {
					Href string `json:"href"`
				} `json:"next"`
			} `json:"links"`
		}
		if err := d.list(ctx, next, &res); err != nil {
			return nil, err
		}
		inc := map[string]jaResource{}
		for _, r := range res.Included {
			inc[r.Type+"/"+r.ID] = r
		}
		for _, r := range res.Data {
			out = append(out, dcOrderOf(r, inc))
		}
		next = ""
		// ďalšia strana iba na rovnakom serveri (odkaz z odpovede)
		if res.Links.Next != nil && strings.HasPrefix(res.Links.Next.Href, d.c.URL+"/") && len(out) < MaxOrders {
			next = res.Links.Next.Href
		}
	}
	return out, nil
}

// dcTime prečíta čas: ISO 8601 alebo unixové sekundy.
func dcTime(raw jsonRaw) time.Time {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC()
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Unix(n, 0).UTC()
		}
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		return time.Unix(n, 0).UTC()
	}
	return time.Time{}
}

func (p dcPrice) dec() money.Dec { return p.Number.dec() }

func dcOrderOf(r jaResource, inc map[string]jaResource) Order {
	var o dcOrder
	_ = r.Attributes.decode(&o)
	out := Order{ID: r.ID, Number: firstOf(o.OrderNumber, strconv.FormatInt(o.DrupalID, 10), r.ID), Created: dcTime(o.Placed), Status: o.State,
		Cancelled: o.State == "canceled"}
	if o.TotalPrice != nil {
		out.Total, out.Currency = o.TotalPrice.dec(), strings.ToUpper(o.TotalPrice.CurrencyCode)
	}
	if o.State == "completed" || (o.TotalPaid != nil && out.Total.Sign() > 0 && o.TotalPaid.dec().Cmp(out.Total) >= 0) {
		out.Paid, out.PaidAt = !out.Cancelled, out.Created
	}
	out.Billing.Email = strings.TrimSpace(o.Mail)
	for _, ref := range r.Relationships["billing_profile"].refs() {
		var p dcProfile
		_ = inc[ref.Type+"/"+ref.ID].Attributes.decode(&p)
		if a := p.Address; a != nil {
			out.Billing = Address{Name: joinNonEmpty(" ", a.GivenName, a.FamilyName), Company: strings.TrimSpace(a.Organization),
				Street: joinNonEmpty(", ", a.AddressLine1, a.AddressLine2), City: strings.TrimSpace(a.Locality), Zip: strings.TrimSpace(a.PostalCode),
				Country: strings.ToUpper(a.CountryCode), Email: strings.TrimSpace(o.Mail)}
		}
		if p.TaxNumber != nil {
			out.Billing.ICDPH = strings.TrimSpace(p.TaxNumber.Value)
		}
	}
	for _, ref := range r.Relationships["order_items"].refs() {
		item := inc[ref.Type+"/"+ref.ID]
		var it dcItem
		_ = item.Attributes.decode(&it)
		total, tax, included := money.Zero, money.Zero, false
		if it.TotalPrice != nil {
			total = it.TotalPrice.dec()
		}
		for _, adj := range it.Adjustments {
			switch adj.Type {
			case "tax":
				tax = tax.Add(adj.Amount.dec())
				included = included || adj.Included
			case "promotion", "custom":
				if !adj.Included {
					total = total.Add(adj.Amount.dec())
				}
			}
		}
		net := total
		if included {
			net = total.Sub(tax)
		}
		sku := ""
		for _, pe := range item.Relationships["purchased_entity"].refs() {
			var v struct {
				SKU string `json:"sku"`
			}
			_ = inc[pe.Type+"/"+pe.ID].Attributes.decode(&v)
			sku = v.SKU
		}
		out.Lines = append(out.Lines, Line{SKU: strings.TrimSpace(sku), Name: strings.TrimSpace(it.Title), Quantity: it.Quantity.dec(), Net: net, Tax: tax})
	}
	// úpravy objednávky: doprava, zľavy a daň z nich
	shipping := -1
	extraTax, extraIncluded := money.Zero, false
	for _, adj := range o.Adjustments {
		switch adj.Type {
		case "shipping":
			out.Lines = append(out.Lines, Line{Name: firstOf(adj.Label, "Doprava"), Quantity: money.FromInt(1), Net: adj.Amount.dec(), Shipping: true})
			shipping = len(out.Lines) - 1
		case "promotion":
			if !adj.Included {
				out.Lines = append(out.Lines, Line{Name: firstOf(adj.Label, "Zľava"), Quantity: money.FromInt(1), Net: adj.Amount.dec()})
			}
		case "tax":
			extraTax = extraTax.Add(adj.Amount.dec())
			extraIncluded = extraIncluded || adj.Included
		}
	}
	if !extraTax.IsZero() && shipping >= 0 {
		out.Lines[shipping].Tax = extraTax
		if extraIncluded {
			out.Lines[shipping].Net = out.Lines[shipping].Net.Sub(extraTax)
		}
	}
	return out
}

func (d *drupal) SetStock(context.Context, string, int64) error { return ErrNoStock }
