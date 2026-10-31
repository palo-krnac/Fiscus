package eshop

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Shopify Admin GraphQL API, prihlásenie tokenom vlastnej aplikácie
// (X-Shopify-Access-Token). https://shopify.dev/docs/api/admin-graphql

// ShopifyVersion je verzia Admin API.
const ShopifyVersion = "2025-01"

type shopify struct{ c Conn }

type gqlError struct {
	Message string `json:"message"`
}

func (s *shopify) gql(ctx context.Context, query string, vars map[string]any, out any) error {
	var res struct {
		Data   jsonRaw    `json:"data"`
		Errors []gqlError `json:"errors"`
	}
	body := map[string]any{"query": query, "variables": vars}
	if _, err := call(ctx, s.c.HTTP, http.MethodPost, s.c.URL+"/admin/api/"+ShopifyVersion+"/graphql.json", body, &res,
		func(r *http.Request) { r.Header.Set("X-Shopify-Access-Token", s.c.Secret) }); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		msg := res.Errors[0].Message
		if strings.Contains(strings.ToLower(msg), "access denied") {
			return fmt.Errorf("%w (%s)", ErrAuth, msg)
		}
		return errors.New("Shopify: " + msg)
	}
	return res.Data.decode(out)
}

func (s *shopify) Check(ctx context.Context) (string, error) {
	var d struct {
		Shop struct {
			Name string `json:"name"`
		} `json:"shop"`
	}
	if err := s.gql(ctx, `{ shop { name } }`, nil, &d); err != nil {
		return "", err
	}
	return d.Shop.Name, nil
}

type shMoney struct {
	ShopMoney struct {
		Amount num `json:"amount"`
	} `json:"shopMoney"`
}

type shTax struct {
	PriceSet shMoney `json:"priceSet"`
}

type shAddr struct {
	Name        string `json:"name"`
	Company     string `json:"company"`
	Address1    string `json:"address1"`
	Address2    string `json:"address2"`
	City        string `json:"city"`
	Zip         string `json:"zip"`
	CountryCode string `json:"countryCodeV2"`
	Phone       string `json:"phone"`
}

func (a *shAddr) address(email string) Address {
	if a == nil {
		return Address{Email: email}
	}
	return Address{Name: strings.TrimSpace(a.Name), Company: strings.TrimSpace(a.Company), Street: joinNonEmpty(", ", a.Address1, a.Address2),
		City: strings.TrimSpace(a.City), Zip: strings.TrimSpace(a.Zip), Country: strings.ToUpper(a.CountryCode), Email: email, Phone: strings.TrimSpace(a.Phone)}
}

type shOrder struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	CreatedAt              string   `json:"createdAt"`
	ProcessedAt            string   `json:"processedAt"`
	CancelledAt            *string  `json:"cancelledAt"`
	DisplayFinancialStatus string   `json:"displayFinancialStatus"`
	CurrencyCode           string   `json:"currencyCode"`
	TaxesIncluded          bool     `json:"taxesIncluded"`
	Email                  string   `json:"email"`
	Note                   string   `json:"note"`
	PaymentGatewayNames    []string `json:"paymentGatewayNames"`
	TotalPriceSet          shMoney  `json:"totalPriceSet"`
	BillingAddress         *shAddr  `json:"billingAddress"`
	ShippingAddress        *shAddr  `json:"shippingAddress"`
	LineItems              struct {
		Nodes []struct {
			Name               string  `json:"name"`
			SKU                string  `json:"sku"`
			Quantity           num     `json:"quantity"`
			DiscountedTotalSet shMoney `json:"discountedTotalSet"`
			TaxLines           []shTax `json:"taxLines"`
		} `json:"nodes"`
	} `json:"lineItems"`
	ShippingLines struct {
		Nodes []struct {
			Title              string  `json:"title"`
			DiscountedPriceSet shMoney `json:"discountedPriceSet"`
			TaxLines           []shTax `json:"taxLines"`
		} `json:"nodes"`
	} `json:"shippingLines"`
}

const shOrdersQuery = `query($q: String, $after: String) {
  orders(first: 50, after: $after, query: $q, sortKey: CREATED_AT) {
    pageInfo { hasNextPage endCursor }
    nodes {
      id name createdAt processedAt cancelledAt displayFinancialStatus currencyCode taxesIncluded email note paymentGatewayNames
      totalPriceSet { shopMoney { amount } }
      billingAddress { name company address1 address2 city zip countryCodeV2 phone }
      shippingAddress { name company address1 address2 city zip countryCodeV2 phone }
      lineItems(first: 250) { nodes { name sku quantity discountedTotalSet { shopMoney { amount } } taxLines { priceSet { shopMoney { amount } } } } }
      shippingLines(first: 10) { nodes { title discountedPriceSet { shopMoney { amount } } taxLines { priceSet { shopMoney { amount } } } } }
    }
  }
}`

func (s *shopify) Orders(ctx context.Context, since time.Time) ([]Order, error) {
	vars := map[string]any{}
	if !since.IsZero() {
		vars["q"] = "created_at:>='" + since.UTC().Format(time.RFC3339) + "'"
	}
	var out []Order
	for {
		var d struct {
			Orders struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []shOrder `json:"nodes"`
			} `json:"orders"`
		}
		if err := s.gql(ctx, shOrdersQuery, vars, &d); err != nil {
			return nil, err
		}
		for _, o := range d.Orders.Nodes {
			out = append(out, o.order())
		}
		if !d.Orders.PageInfo.HasNextPage || len(out) >= MaxOrders {
			return out, nil
		}
		vars["after"] = d.Orders.PageInfo.EndCursor
	}
}

func sumTax(t []shTax) money.Dec {
	s := money.Zero
	for _, x := range t {
		s = s.Add(x.PriceSet.ShopMoney.Amount.dec())
	}
	return s
}

func (o shOrder) order() Order {
	created, _ := time.Parse(time.RFC3339, o.CreatedAt)
	out := Order{ID: o.ID, Number: strings.TrimPrefix(o.Name, "#"), Created: created.UTC(), Status: strings.ToLower(o.DisplayFinancialStatus),
		Currency: strings.ToUpper(o.CurrencyCode), Billing: o.BillingAddress.address(o.Email), Shipping: o.ShippingAddress.address(""),
		Total: o.TotalPriceSet.ShopMoney.Amount.dec(), Note: strings.TrimSpace(o.Note), Cancelled: o.CancelledAt != nil}
	if len(o.PaymentGatewayNames) > 0 {
		out.Payment = paymentOf(o.PaymentGatewayNames[0])
	}
	switch o.DisplayFinancialStatus {
	case "PAID", "PARTIALLY_REFUNDED":
		out.Paid = true
		if t, err := time.Parse(time.RFC3339, o.ProcessedAt); err == nil {
			out.PaidAt = t.UTC()
		} else {
			out.PaidAt = out.Created
		}
	case "REFUNDED", "VOIDED":
		out.Cancelled = true
	}
	// sumy v obchode s cenami s DPH obsahujú daň — Fiškus potrebuje základ
	net := func(total money.Dec, tax money.Dec) money.Dec {
		if o.TaxesIncluded {
			return total.Sub(tax)
		}
		return total
	}
	for _, l := range o.LineItems.Nodes {
		tax := sumTax(l.TaxLines)
		out.Lines = append(out.Lines, Line{SKU: strings.TrimSpace(l.SKU), Name: strings.TrimSpace(l.Name), Quantity: l.Quantity.dec(),
			Net: net(l.DiscountedTotalSet.ShopMoney.Amount.dec(), tax), Tax: tax})
	}
	for _, l := range o.ShippingLines.Nodes {
		tax := sumTax(l.TaxLines)
		out.Lines = append(out.Lines, Line{Name: firstOf(l.Title, "Doprava"), Quantity: money.FromInt(1),
			Net: net(l.DiscountedPriceSet.ShopMoney.Amount.dec(), tax), Tax: tax, Shipping: true})
	}
	return out
}

func (s *shopify) SetStock(ctx context.Context, sku string, qty int64) error {
	var d struct {
		ProductVariants struct {
			Nodes []struct {
				SKU           string `json:"sku"`
				InventoryItem struct {
					ID string `json:"id"`
				} `json:"inventoryItem"`
			} `json:"nodes"`
		} `json:"productVariants"`
		Locations struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"locations"`
	}
	q := `query($q: String) { productVariants(first: 5, query: $q) { nodes { sku inventoryItem { id } } } locations(first: 1) { nodes { id } } }`
	if err := s.gql(ctx, q, map[string]any{"q": "sku:" + quoteSearch(sku)}, &d); err != nil {
		return err
	}
	item := ""
	for _, v := range d.ProductVariants.Nodes {
		if strings.EqualFold(v.SKU, sku) {
			item = v.InventoryItem.ID
		}
	}
	if item == "" {
		return ErrNoSKU
	}
	if len(d.Locations.Nodes) == 0 {
		return errors.New("Shopify: obchod nemá žiadne miesto skladu (location)")
	}
	var m struct {
		InventorySetQuantities struct {
			UserErrors []gqlError `json:"userErrors"`
		} `json:"inventorySetQuantities"`
	}
	mut := `mutation($input: InventorySetQuantitiesInput!) { inventorySetQuantities(input: $input) { userErrors { message } } }`
	in := map[string]any{"name": "available", "reason": "correction", "ignoreCompareQuantity": true,
		"quantities": []map[string]any{{"inventoryItemId": item, "locationId": d.Locations.Nodes[0].ID, "quantity": qty}}}
	if err := s.gql(ctx, mut, map[string]any{"input": in}, &m); err != nil {
		return err
	}
	if ue := m.InventorySetQuantities.UserErrors; len(ue) > 0 {
		return errors.New("Shopify: " + ue[0].Message)
	}
	return nil
}

// quoteSearch dá hodnotu do úvodzoviek pre vyhľadávací dopyt Shopify.
func quoteSearch(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
