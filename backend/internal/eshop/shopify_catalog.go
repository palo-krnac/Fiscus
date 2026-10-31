package eshop

import (
	"context"
	"errors"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Shopify: cenník a zákazníci. Shopify nemá sadzbu DPH pri produkte, iba
// príznak „zdaniteľný“ (sadzbu určuje krajina obchodu), preto zdaniteľný
// produkt = základná sadzba (23 %), nezdaniteľný = 0 %. Ceny sú s DPH
// alebo bez podľa nastavenia obchodu (shop.taxesIncluded).

// ShopifyStandardRate je sadzba DPH zdaniteľných produktov.
var ShopifyStandardRate = 23

func (s *shopify) taxesIncluded(ctx context.Context) (bool, error) {
	var d struct {
		Shop struct {
			TaxesIncluded bool `json:"taxesIncluded"`
		} `json:"shop"`
	}
	err := s.gql(ctx, `{ shop { taxesIncluded } }`, nil, &d)
	return d.Shop.TaxesIncluded, err
}

func shopifyPrices(price money.Dec, rate int, incl bool) (net, gross money.Dec) {
	if incl {
		return price.MulRatio(money.FromInt(100), money.FromInt(int64(100+rate))).RoundCents(), price
	}
	return price, price.MulRatio(money.FromInt(int64(100+rate)), money.FromInt(100)).RoundCents()
}

func (s *shopify) Products(ctx context.Context) ([]Product, error) {
	incl, err := s.taxesIncluded(ctx)
	if err != nil {
		return nil, err
	}
	q := `query($after: String) { productVariants(first: 250, after: $after) {
  pageInfo { hasNextPage endCursor }
  nodes { sku title price taxable product { title status hasOnlyDefaultVariant } } } }`
	vars := map[string]any{}
	var out []Product
	for len(out) < MaxProducts {
		var d struct {
			ProductVariants struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					SKU     string `json:"sku"`
					Title   string `json:"title"`
					Price   num    `json:"price"`
					Taxable bool   `json:"taxable"`
					Product struct {
						Title       string `json:"title"`
						Status      string `json:"status"`
						OnlyDefault bool   `json:"hasOnlyDefaultVariant"`
					} `json:"product"`
				} `json:"nodes"`
			} `json:"productVariants"`
		}
		if err := s.gql(ctx, q, vars, &d); err != nil {
			return nil, err
		}
		for _, v := range d.ProductVariants.Nodes {
			if strings.TrimSpace(v.SKU) == "" {
				continue
			}
			rate := 0
			if v.Taxable {
				rate = ShopifyStandardRate
			}
			name := v.Product.Title
			if !v.Product.OnlyDefault {
				name = joinNonEmpty(" ", v.Product.Title, v.Title)
			}
			p := Product{SKU: strings.TrimSpace(v.SKU), Name: strings.TrimSpace(name), Rate: rate, Active: v.Product.Status == "ACTIVE"}
			p.Net, p.Gross = shopifyPrices(v.Price.dec(), rate, incl)
			out = append(out, p)
		}
		if !d.ProductVariants.PageInfo.HasNextPage {
			break
		}
		vars["after"] = d.ProductVariants.PageInfo.EndCursor
	}
	return out, nil
}

func (s *shopify) Customers(ctx context.Context) ([]Address, error) {
	q := `query($after: String) { customers(first: 250, after: $after) {
  pageInfo { hasNextPage endCursor }
  nodes { email firstName lastName phone defaultAddress { company address1 address2 city zip countryCodeV2 phone } } } }`
	vars := map[string]any{}
	var out []Address
	for len(out) < MaxProducts {
		var d struct {
			Customers struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					Email     string  `json:"email"`
					FirstName string  `json:"firstName"`
					LastName  string  `json:"lastName"`
					Phone     string  `json:"phone"`
					Address   *shAddr `json:"defaultAddress"`
				} `json:"nodes"`
			} `json:"customers"`
		}
		if err := s.gql(ctx, q, vars, &d); err != nil {
			return nil, err
		}
		for _, c := range d.Customers.Nodes {
			a := c.Address.address(strings.TrimSpace(c.Email))
			a.Name = joinNonEmpty(" ", c.FirstName, c.LastName)
			a.Phone = firstOf(a.Phone, c.Phone)
			out = append(out, a)
		}
		if !d.Customers.PageInfo.HasNextPage {
			break
		}
		vars["after"] = d.Customers.PageInfo.EndCursor
	}
	return out, nil
}

type shUserErrors struct {
	UserErrors []gqlError `json:"userErrors"`
}

func (u shUserErrors) err() error {
	if len(u.UserErrors) > 0 {
		return errors.New("Shopify: " + u.UserErrors[0].Message)
	}
	return nil
}

func (s *shopify) PushProduct(ctx context.Context, p Product) (bool, error) {
	if p.Rate != 0 && p.Rate != ShopifyStandardRate {
		return false, errors.New("Shopify pozná iba zdaniteľný (základná sadzba) a nezdaniteľný produkt — znížené sadzby nastavte v Shopify cez daňové výnimky")
	}
	incl, err := s.taxesIncluded(ctx)
	if err != nil {
		return false, err
	}
	price := p.Net
	if incl {
		price = p.Gross
	}
	var d struct {
		ProductVariants struct {
			Nodes []struct {
				ID      string `json:"id"`
				SKU     string `json:"sku"`
				Product struct {
					ID          string `json:"id"`
					Status      string `json:"status"`
					OnlyDefault bool   `json:"hasOnlyDefaultVariant"`
				} `json:"product"`
			} `json:"nodes"`
		} `json:"productVariants"`
	}
	q := `query($q: String) { productVariants(first: 5, query: $q) { nodes { id sku product { id status hasOnlyDefaultVariant } } } }`
	if err := s.gql(ctx, q, map[string]any{"q": "sku:" + quoteSearch(p.SKU)}, &d); err != nil {
		return false, err
	}
	for _, v := range d.ProductVariants.Nodes {
		if !strings.EqualFold(v.SKU, p.SKU) {
			continue
		}
		var up struct {
			R shUserErrors `json:"productVariantsBulkUpdate"`
		}
		mut := `mutation($pid: ID!, $v: [ProductVariantsBulkInput!]!) { productVariantsBulkUpdate(productId: $pid, variants: $v) { userErrors { message } } }`
		vin := []map[string]any{{"id": v.ID, "price": price.RoundCents().String(), "taxable": p.Rate > 0}}
		if err := s.gql(ctx, mut, map[string]any{"pid": v.Product.ID, "v": vin}, &up); err != nil {
			return false, err
		}
		if err := up.R.err(); err != nil {
			return false, err
		}
		// názov a stav patria produktu — menia sa iba pri produkte bez variantov
		if v.Product.OnlyDefault {
			in := map[string]any{"id": v.Product.ID, "title": p.Name}
			switch {
			case !p.Active:
				in["status"] = "ARCHIVED"
			case v.Product.Status == "ARCHIVED":
				in["status"] = "ACTIVE"
			} // koncept (DRAFT) ostane konceptom
			var pu struct {
				R shUserErrors `json:"productUpdate"`
			}
			if err := s.gql(ctx, `mutation($in: ProductInput!) { productUpdate(input: $in) { userErrors { message } } }`, map[string]any{"in": in}, &pu); err != nil {
				return false, err
			}
			return false, pu.R.err()
		}
		return false, nil
	}
	if !p.Active || !p.Create {
		return false, ErrNoSKU
	}
	var cr struct {
		R struct {
			Product *struct {
				ID string `json:"id"`
			} `json:"product"`
			shUserErrors
		} `json:"productCreate"`
	}
	if err := s.gql(ctx, `mutation($p: ProductCreateInput!) { productCreate(product: $p) { product { id } userErrors { message } } }`,
		map[string]any{"p": map[string]any{"title": p.Name, "status": "DRAFT"}}, &cr); err != nil {
		return false, err
	}
	if err := cr.R.err(); err != nil {
		return false, err
	}
	if cr.R.Product == nil {
		return false, errors.New("Shopify produkt nezaložil")
	}
	var vc struct {
		R shUserErrors `json:"productVariantsBulkCreate"`
	}
	mut := `mutation($pid: ID!, $v: [ProductVariantsBulkInput!]!) { productVariantsBulkCreate(productId: $pid, variants: $v, strategy: REMOVE_STANDALONE_VARIANT) { userErrors { message } } }`
	vin := []map[string]any{{"price": price.RoundCents().String(), "taxable": p.Rate > 0, "inventoryItem": map[string]any{"sku": p.SKU, "tracked": true}}}
	if err := s.gql(ctx, mut, map[string]any{"pid": cr.R.Product.ID, "v": vin}, &vc); err != nil {
		return true, err
	}
	return true, vc.R.err()
}
