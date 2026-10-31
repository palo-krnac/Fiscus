package eshop

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// WooCommerce REST API v3 (/wp-json/wc/v3), prihlásenie kľúčom
// consumer key/secret cez HTTP Basic (iba cez https).
// https://woocommerce.github.io/woocommerce-rest-api-docs/

type woo struct {
	c   Conn
	tax *wooTax
}

func (w *woo) auth(r *http.Request) { r.SetBasicAuth(w.c.Key, w.c.Secret) }

func (w *woo) api(path string) string { return w.c.URL + "/wp-json/wc/v3" + path }

func (w *woo) Check(ctx context.Context) (string, error) {
	var orders []wooOrder
	if _, err := call(ctx, w.c.HTTP, http.MethodGet, w.api("/orders?per_page=1"), nil, &orders, w.auth); err != nil {
		return "", err
	}
	var site struct {
		Name string `json:"name"`
	}
	if _, err := call(ctx, w.c.HTTP, http.MethodGet, w.c.URL+"/wp-json/", nil, &site, func(*http.Request) {}); err != nil || site.Name == "" {
		return w.c.URL, nil
	}
	return site.Name, nil
}

type wooAddr struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Company   string `json:"company"`
	Address1  string `json:"address_1"`
	Address2  string `json:"address_2"`
	City      string `json:"city"`
	Postcode  string `json:"postcode"`
	Country   string `json:"country"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
}

type wooMeta struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type wooLine struct {
	Name     string `json:"name"`
	SKU      string `json:"sku"`
	Quantity num    `json:"quantity"`
	Total    num    `json:"total"`
	TotalTax num    `json:"total_tax"`
	// doprava a poplatky
	MethodTitle string `json:"method_title"`
}

type wooOrder struct {
	ID             int64     `json:"id"`
	Number         string    `json:"number"`
	Status         string    `json:"status"`
	Currency       string    `json:"currency"`
	DateCreatedGMT string    `json:"date_created_gmt"`
	DatePaidGMT    string    `json:"date_paid_gmt"`
	Total          num       `json:"total"`
	CustomerNote   string    `json:"customer_note"`
	Billing        wooAddr   `json:"billing"`
	Shipping       wooAddr   `json:"shipping"`
	LineItems      []wooLine `json:"line_items"`
	ShippingLines  []wooLine `json:"shipping_lines"`
	FeeLines       []wooLine `json:"fee_lines"`
	MetaData       []wooMeta `json:"meta_data"`
	PaymentMethod  string    `json:"payment_method"`
}

func (w *woo) Orders(ctx context.Context, since time.Time) ([]Order, error) {
	var out []Order
	for page := 1; ; page++ {
		q := url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}, "orderby": {"date"}, "order": {"asc"}}
		if !since.IsZero() {
			q.Set("after", since.UTC().Format("2006-01-02T15:04:05"))
			q.Set("dates_are_gmt", "true")
		}
		var batch []wooOrder
		h, err := call(ctx, w.c.HTTP, http.MethodGet, w.api("/orders?"+q.Encode()), nil, &batch, w.auth)
		if err != nil {
			return nil, err
		}
		for _, o := range batch {
			out = append(out, o.order())
		}
		pages, _ := strconv.Atoi(h.Get("X-WP-TotalPages"))
		if len(batch) == 0 || page >= pages || len(out) >= MaxOrders {
			return out, nil
		}
	}
}

func wooTime(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func (a wooAddr) address() Address {
	return Address{Name: joinNonEmpty(" ", a.FirstName, a.LastName), Company: strings.TrimSpace(a.Company),
		Street: joinNonEmpty(", ", a.Address1, a.Address2), City: strings.TrimSpace(a.City), Zip: strings.TrimSpace(a.Postcode),
		Country: strings.ToUpper(strings.TrimSpace(a.Country)), Email: strings.TrimSpace(a.Email), Phone: strings.TrimSpace(a.Phone)}
}

// wooMetaKeys sú bežné názvy polí pre IČO, DIČ a IČ DPH zo slovenských
// a českých doplnkov fakturačných údajov.
var wooMetaKeys = map[string][]string{
	"ico":   {"billing_ico", "billing_ic", "billing_company_id", "ico", "ic"},
	"dic":   {"billing_dic", "billing_tax_id", "dic"},
	"icdph": {"billing_dic_dph", "billing_ic_dph", "billing_icdph", "billing_vat_id", "billing_vat_number", "vat_number", "ic_dph", "dic_dph"},
}

func (o wooOrder) meta(field string) string {
	for _, k := range wooMetaKeys[field] {
		for _, m := range o.MetaData {
			if strings.EqualFold(strings.TrimPrefix(m.Key, "_"), k) {
				if s, ok := m.Value.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func (o wooOrder) order() Order {
	out := Order{ID: strconv.FormatInt(o.ID, 10), Number: firstOf(o.Number, strconv.FormatInt(o.ID, 10)), Status: o.Status,
		Created: wooTime(o.DateCreatedGMT), PaidAt: wooTime(o.DatePaidGMT), Currency: strings.ToUpper(o.Currency),
		Billing: o.Billing.address(), Shipping: o.Shipping.address(), Total: o.Total.dec(), Note: strings.TrimSpace(o.CustomerNote),
		Payment: paymentOf(o.PaymentMethod)}
	out.Billing.ICO, out.Billing.DIC, out.Billing.ICDPH = o.meta("ico"), o.meta("dic"), o.meta("icdph")
	switch o.Status {
	case "processing", "completed":
		out.Paid = !out.PaidAt.IsZero() || o.Status == "completed"
	case "cancelled", "refunded", "failed", "trash":
		out.Cancelled = true
	}
	if out.Paid && out.PaidAt.IsZero() {
		out.PaidAt = out.Created
	}
	for _, l := range o.LineItems {
		out.Lines = append(out.Lines, Line{SKU: strings.TrimSpace(l.SKU), Name: strings.TrimSpace(l.Name), Quantity: l.Quantity.dec(), Net: l.Total.dec(), Tax: l.TotalTax.dec()})
	}
	for _, l := range o.ShippingLines {
		out.Lines = append(out.Lines, Line{Name: firstOf(l.MethodTitle, "Doprava"), Quantity: money.FromInt(1), Net: l.Total.dec(), Tax: l.TotalTax.dec(), Shipping: true})
	}
	for _, l := range o.FeeLines {
		out.Lines = append(out.Lines, Line{Name: firstOf(l.Name, "Poplatok"), Quantity: money.FromInt(1), Net: l.Total.dec(), Tax: l.TotalTax.dec()})
	}
	return out
}

func (w *woo) SetStock(ctx context.Context, sku string, qty int64) error {
	var found []struct {
		ID       int64  `json:"id"`
		ParentID int64  `json:"parent_id"`
		Type     string `json:"type"`
		SKU      string `json:"sku"`
	}
	if _, err := call(ctx, w.c.HTTP, http.MethodGet, w.api("/products?"+url.Values{"sku": {sku}}.Encode()), nil, &found, w.auth); err != nil {
		return err
	}
	for _, p := range found {
		if !strings.EqualFold(p.SKU, sku) {
			continue
		}
		path := fmt.Sprintf("/products/%d", p.ID)
		if p.Type == "variation" && p.ParentID != 0 {
			path = fmt.Sprintf("/products/%d/variations/%d", p.ParentID, p.ID)
		}
		body := map[string]any{"manage_stock": true, "stock_quantity": qty}
		_, err := call(ctx, w.c.HTTP, http.MethodPut, w.api(path), body, nil, w.auth)
		return err
	}
	return ErrNoSKU
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
