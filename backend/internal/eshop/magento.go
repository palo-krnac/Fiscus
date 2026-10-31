package eshop

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Magento 2 / Adobe Commerce REST API (/rest/V1), prihlásenie tokenom
// integrácie (Authorization: Bearer). Dátumy objednávok sú v UTC.
// https://developer.adobe.com/commerce/webapi/rest/

type magento struct {
	c   Conn
	tax *mgTax
}

func (m *magento) auth(r *http.Request) { r.Header.Set("Authorization", "Bearer "+m.c.Secret) }

func (m *magento) api(path string) string { return m.c.URL + "/rest/V1" + path }

func (m *magento) Check(ctx context.Context) (string, error) {
	var res struct {
		TotalCount int `json:"total_count"`
	}
	q := url.Values{"searchCriteria[pageSize]": {"1"}, "fields": {"total_count"}}
	if _, err := call(ctx, m.c.HTTP, http.MethodGet, m.api("/orders?"+q.Encode()), nil, &res, m.auth); err != nil {
		return "", err
	}
	var stores []struct {
		Name string `json:"name"`
	}
	if _, err := call(ctx, m.c.HTTP, http.MethodGet, m.api("/store/storeViews"), nil, &stores, m.auth); err == nil && len(stores) > 1 {
		return stores[1].Name, nil // [0] je správa (admin)
	}
	return m.c.URL, nil
}

type mgAddr struct {
	Firstname string   `json:"firstname"`
	Lastname  string   `json:"lastname"`
	Company   string   `json:"company"`
	Street    []string `json:"street"`
	City      string   `json:"city"`
	Postcode  string   `json:"postcode"`
	CountryID string   `json:"country_id"`
	Telephone string   `json:"telephone"`
	Email     string   `json:"email"`
	VATID     string   `json:"vat_id"`
}

func (a *mgAddr) address() Address {
	if a == nil {
		return Address{}
	}
	return Address{Name: joinNonEmpty(" ", a.Firstname, a.Lastname), Company: strings.TrimSpace(a.Company), Street: joinNonEmpty(", ", a.Street...),
		City: strings.TrimSpace(a.City), Zip: strings.TrimSpace(a.Postcode), Country: strings.ToUpper(a.CountryID), Email: strings.TrimSpace(a.Email),
		Phone: strings.TrimSpace(a.Telephone), ICDPH: strings.TrimSpace(a.VATID)}
}

type mgOrder struct {
	EntityID          int64   `json:"entity_id"`
	IncrementID       string  `json:"increment_id"`
	CreatedAt         string  `json:"created_at"`
	State             string  `json:"state"`
	Status            string  `json:"status"`
	OrderCurrencyCode string  `json:"order_currency_code"`
	GrandTotal        num     `json:"grand_total"`
	TotalPaid         num     `json:"total_paid"`
	TotalDue          num     `json:"total_due"`
	CustomerEmail     string  `json:"customer_email"`
	CustomerTaxvat    string  `json:"customer_taxvat"`
	ShippingDesc      string  `json:"shipping_description"`
	ShippingAmount    num     `json:"shipping_amount"`
	ShippingTax       num     `json:"shipping_tax_amount"`
	ShippingDiscount  num     `json:"shipping_discount_amount"`
	BillingAddress    *mgAddr `json:"billing_address"`
	Payment           struct {
		Method string `json:"method"`
	} `json:"payment"`
	Items []struct {
		ItemID         int64  `json:"item_id"`
		ParentItemID   *int64 `json:"parent_item_id"`
		SKU            string `json:"sku"`
		Name           string `json:"name"`
		QtyOrdered     num    `json:"qty_ordered"`
		RowTotal       num    `json:"row_total"`
		TaxAmount      num    `json:"tax_amount"`
		DiscountAmount num    `json:"discount_amount"`
	} `json:"items"`
	Extension struct {
		Assignments []struct {
			Shipping struct {
				Address *mgAddr `json:"address"`
			} `json:"shipping"`
		} `json:"shipping_assignments"`
	} `json:"extension_attributes"`
}

const mgPage = 50

func (m *magento) Orders(ctx context.Context, since time.Time) ([]Order, error) {
	var out []Order
	for page := 1; ; page++ {
		q := url.Values{"searchCriteria[pageSize]": {strconv.Itoa(mgPage)}, "searchCriteria[currentPage]": {strconv.Itoa(page)},
			"searchCriteria[sortOrders][0][field]": {"created_at"}, "searchCriteria[sortOrders][0][direction]": {"ASC"}}
		if !since.IsZero() {
			q.Set("searchCriteria[filter_groups][0][filters][0][field]", "created_at")
			q.Set("searchCriteria[filter_groups][0][filters][0][value]", since.UTC().Format("2006-01-02 15:04:05"))
			q.Set("searchCriteria[filter_groups][0][filters][0][condition_type]", "gteq")
		}
		var res struct {
			Items      []mgOrder `json:"items"`
			TotalCount int       `json:"total_count"`
		}
		if _, err := call(ctx, m.c.HTTP, http.MethodGet, m.api("/orders?"+q.Encode()), nil, &res, m.auth); err != nil {
			return nil, err
		}
		for _, o := range res.Items {
			out = append(out, o.order())
		}
		// Magento za poslednou stranou vracia znova poslednú stranu — rozhoduje počet
		if len(res.Items) == 0 || page*mgPage >= res.TotalCount || len(out) >= MaxOrders {
			return out, nil
		}
	}
}

func (o mgOrder) order() Order {
	created, _ := time.Parse("2006-01-02 15:04:05", o.CreatedAt)
	out := Order{ID: strconv.FormatInt(o.EntityID, 10), Number: firstOf(o.IncrementID, strconv.FormatInt(o.EntityID, 10)), Created: created.UTC(),
		Status: o.State, Currency: strings.ToUpper(o.OrderCurrencyCode), Billing: o.BillingAddress.address(), Total: o.GrandTotal.dec(),
		Payment: paymentOf(mgPayment(o.Payment.Method))}
	if out.Billing.Email == "" {
		out.Billing.Email = strings.TrimSpace(o.CustomerEmail)
	}
	if out.Billing.ICDPH == "" {
		out.Billing.ICDPH = strings.TrimSpace(o.CustomerTaxvat)
	}
	if len(o.Extension.Assignments) > 0 {
		out.Shipping = o.Extension.Assignments[0].Shipping.Address.address()
	}
	switch o.State {
	case "canceled", "closed":
		out.Cancelled = true
	}
	// uhradená: zaplatená celá suma (faktúra v Magente) alebo dokončená
	if !out.Cancelled && (o.State == "complete" || (o.TotalPaid.dec().Sign() > 0 && o.TotalDue.dec().Sign() <= 0)) {
		out.Paid, out.PaidAt = true, out.Created
	}
	for _, it := range o.Items {
		if it.ParentItemID != nil {
			continue // podriadok konfigurovateľného produktu: cena je pri rodičovi
		}
		disc := it.DiscountAmount.dec()
		out.Lines = append(out.Lines, Line{SKU: strings.TrimSpace(it.SKU), Name: strings.TrimSpace(it.Name), Quantity: it.QtyOrdered.dec(),
			Net: it.RowTotal.dec().Sub(disc), Tax: it.TaxAmount.dec()})
	}
	if ship := o.ShippingAmount.dec().Sub(o.ShippingDiscount.dec()); !ship.IsZero() || !o.ShippingTax.dec().IsZero() {
		out.Lines = append(out.Lines, Line{Name: firstOf(o.ShippingDesc, "Doprava"), Quantity: money.FromInt(1), Net: ship, Tax: o.ShippingTax.dec(), Shipping: true})
	}
	return out
}

// mgPayment preloží kódy platobných metód Magenta na slová pre paymentOf.
func mgPayment(code string) string {
	switch code {
	case "cashondelivery":
		return "cod"
	case "banktransfer", "checkmo", "purchaseorder":
		return "bank"
	case "free":
		return ""
	}
	return code
}

func (m *magento) SetStock(ctx context.Context, sku string, qty int64) error {
	var item struct {
		ItemID int64 `json:"item_id"`
	}
	path := "/stockItems/" + url.PathEscape(sku)
	if _, err := call(ctx, m.c.HTTP, http.MethodGet, m.api(path), nil, &item, m.auth); err != nil {
		if errors.Is(err, errNotFound) {
			return ErrNoSKU
		}
		return err
	}
	body := map[string]any{"stockItem": map[string]any{"qty": qty, "is_in_stock": qty > 0}}
	_, err := call(ctx, m.c.HTTP, http.MethodPut, m.api(fmt.Sprintf("/products/%s/stockItems/%d", url.PathEscape(sku), item.ItemID)), body, nil, m.auth)
	return err
}
