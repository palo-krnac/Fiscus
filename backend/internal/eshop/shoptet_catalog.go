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

// Shoptet: cenník a zákazníci podľa schémy openapi.yaml.
//   - produkty: zoznam (/api/products) + detail s variantmi (cena,
//     includingVat, vatRate, visible)
//   - cena sa mení v predvolenom cenníku: PATCH /api/price-lists/{id}/prices
//     s vatRate a priceWithoutVat (neplatiteľ DPH: iba price)
//   - názov a viditeľnosť: PATCH /api/products/code/{code}
//   - nový produkt: POST /api/products (skrytý, v prvej kategórii — povinná)

type stInfo struct {
	vatPayer  bool
	pricelist int64
	category  string
}

func (s *shoptet) eshopInfo(ctx context.Context) (*stInfo, error) {
	if s.info != nil {
		return s.info, nil
	}
	var d struct {
		Settings struct {
			VATPayer         bool `json:"vatPayer"`
			DefaultPricelist struct {
				ID int64 `json:"id"`
			} `json:"defaultPricelist"`
		} `json:"settings"`
	}
	if err := s.get(ctx, "/api/eshop", &d); err != nil {
		return nil, err
	}
	s.info = &stInfo{vatPayer: d.Settings.VATPayer, pricelist: d.Settings.DefaultPricelist.ID}
	return s.info, nil
}

type stVariant struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Price        num    `json:"price"`
	IncludingVat bool   `json:"includingVat"`
	VatRate      num    `json:"vatRate"`
	Visible      bool   `json:"visible"`
}

type stProduct struct {
	GUID       string      `json:"guid"`
	Name       string      `json:"name"`
	Visibility string      `json:"visibility"`
	Variants   []stVariant `json:"variants"`
}

func (v stVariant) product(p stProduct, vatPayer bool) Product {
	rate := int(v.VatRate.dec().RoundTo(0).Raw() / money.FromInt(1).Raw())
	if !vatPayer {
		rate = 0
	}
	name := p.Name
	if len(p.Variants) > 1 {
		name = joinNonEmpty(" ", p.Name, v.Name)
	}
	out := Product{SKU: strings.TrimSpace(v.Code), Name: strings.TrimSpace(name), Rate: rate, Active: p.Visibility == "visible" && v.Visible}
	price := v.Price.dec()
	switch {
	case rate == 0:
		out.Net, out.Gross = price, price
	case v.IncludingVat:
		out.Gross = price
		out.Net = price.MulRatio(money.FromInt(100), money.FromInt(int64(100+rate))).RoundCents()
	default:
		out.Net = price
		out.Gross = price.MulRatio(money.FromInt(int64(100+rate)), money.FromInt(100)).RoundCents()
	}
	return out
}

func (s *shoptet) Products(ctx context.Context) ([]Product, error) {
	info, err := s.eshopInfo(ctx)
	if err != nil {
		return nil, err
	}
	var out []Product
	for page := 1; len(out) < MaxProducts; page++ {
		var d struct {
			Products []struct {
				GUID string `json:"guid"`
			} `json:"products"`
			Paginator struct {
				PageCount int `json:"pageCount"`
			} `json:"paginator"`
		}
		if err := s.get(ctx, "/api/products?itemsPerPage=1000&page="+strconv.Itoa(page), &d); err != nil {
			return nil, err
		}
		for _, p := range d.Products {
			var det stProduct
			if err := s.get(ctx, "/api/products/"+url.PathEscape(p.GUID), &det); err != nil {
				return nil, err
			}
			for _, v := range det.Variants {
				if strings.TrimSpace(v.Code) != "" {
					out = append(out, v.product(det, info.vatPayer))
				}
			}
		}
		if len(d.Products) == 0 || page >= d.Paginator.PageCount {
			break
		}
	}
	return out, nil
}

func (s *shoptet) Customers(ctx context.Context) ([]Address, error) {
	var out []Address
	for page := 1; len(out) < MaxProducts; page++ {
		var d struct {
			Customers []struct {
				GUID string `json:"guid"`
			} `json:"customers"`
			Paginator struct {
				PageCount int `json:"pageCount"`
			} `json:"paginator"`
		}
		if err := s.get(ctx, "/api/customers?itemsPerPage=1000&page="+strconv.Itoa(page), &d); err != nil {
			return nil, err
		}
		for _, c := range d.Customers {
			var det struct {
				Customer struct {
					BillingAddress *stAddr `json:"billingAddress"`
					Accounts       []struct {
						FullName    string `json:"fullName"`
						Email       string `json:"email"`
						Phone       string `json:"phone"`
						MainAccount bool   `json:"mainAccount"`
					} `json:"accounts"`
				} `json:"customer"`
			}
			if err := s.get(ctx, "/api/customers/"+url.PathEscape(c.GUID), &det); err != nil {
				return nil, err
			}
			email, phone, name := "", "", ""
			for i, a := range det.Customer.Accounts {
				if a.MainAccount || i == 0 {
					email, phone, name = a.Email, a.Phone, a.FullName
				}
			}
			a := det.Customer.BillingAddress.address(strings.TrimSpace(email), strings.TrimSpace(phone))
			a.Name = firstOf(a.Name, name)
			out = append(out, a)
		}
		if len(d.Customers) == 0 || page >= d.Paginator.PageCount {
			break
		}
	}
	return out, nil
}

// write pošle zápis a vráti chybu, ak Shoptet záznam odmietol.
func (s *shoptet) write(ctx context.Context, method, path string, body any) error {
	var res struct {
		Errors []stErr `json:"errors"`
	}
	if _, err := call(ctx, s.c.HTTP, method, ShoptetAPI+path, body, &res, s.auth); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		return errors.New("Shoptet: " + res.Errors[0].Message)
	}
	return nil
}

func (s *shoptet) setPrice(ctx context.Context, info *stInfo, p Product) error {
	row := map[string]any{"code": p.SKU}
	if info.vatPayer {
		row["vatRate"] = fmt.Sprintf("%d.00", p.Rate)
		row["priceWithoutVat"] = map[string]any{"price": p.Net.RoundCents().String()}
	} else {
		row["price"] = map[string]any{"price": p.Gross.RoundCents().String()}
	}
	return s.write(ctx, http.MethodPatch, fmt.Sprintf("/api/price-lists/%d/prices", info.pricelist), map[string]any{"data": []any{row}})
}

func (s *shoptet) PushProduct(ctx context.Context, p Product) (bool, error) {
	info, err := s.eshopInfo(ctx)
	if err != nil {
		return false, err
	}
	if !info.vatPayer && p.Rate != 0 {
		p.Gross = p.Net.MulRatio(money.FromInt(int64(100+p.Rate)), money.FromInt(100)).RoundCents()
	}
	var cur stProduct
	err = s.get(ctx, "/api/products/code/"+url.PathEscape(p.SKU), &cur)
	switch {
	case err == nil:
		if err := s.setPrice(ctx, info, p); err != nil {
			return false, err
		}
		upd := map[string]any{}
		if len(cur.Variants) <= 1 && cur.Name != p.Name {
			upd["name"] = p.Name // názov patrí produktu — mení sa iba pri produkte bez variantov
		}
		if !p.Active && cur.Visibility == "visible" {
			if len(cur.Variants) <= 1 {
				upd["visibility"] = "hidden"
			} else {
				upd["variants"] = []any{map[string]any{"code": p.SKU, "visible": false}}
			}
		} // skrytý produkt ostane skrytý — zverejňuje sa v obchode
		if len(upd) == 0 {
			return false, nil
		}
		if _, ok := upd["name"]; !ok {
			upd["name"] = cur.Name // názov je pri úprave povinný
		}
		return false, s.write(ctx, http.MethodPatch, "/api/products/code/"+url.PathEscape(p.SKU), map[string]any{"data": upd})
	case !errors.Is(err, errNotFound):
		return false, err
	}
	if !p.Active || !p.Create {
		return false, ErrNoSKU
	}
	if info.category == "" {
		var cats struct {
			Categories []struct {
				GUID string `json:"guid"`
			} `json:"categories"`
		}
		if err := s.get(ctx, "/api/categories?itemsPerPage=1", &cats); err != nil {
			return false, err
		}
		if len(cats.Categories) == 0 {
			return false, errors.New("Shoptet: obchod nemá žiadnu kategóriu — produkt sa nedá založiť")
		}
		info.category = cats.Categories[0].GUID
	}
	body := map[string]any{"data": map[string]any{"name": p.Name, "visibility": "hidden", "defaultCategoryGuid": info.category,
		"variants": []any{map[string]any{"code": p.SKU, "price": p.Gross.RoundCents().String()}}}}
	if err := s.write(ctx, http.MethodPost, "/api/products", body); err != nil {
		return false, err
	}
	// sadzba DPH sa pri založení nezadáva — nastaví ju cenník
	return true, s.setPrice(ctx, info, p)
}
