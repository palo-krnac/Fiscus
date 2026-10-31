package eshop

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// PrestaShop Webservice (/api), výstup JSON, prihlásenie kľúčom cez HTTP
// Basic (kľúč ako meno, prázdne heslo). Zápis (stav zásob) prijíma iba XML.
// https://devdocs.prestashop-project.org/8/webservice/

type presta struct{ c Conn }

func (p *presta) auth(r *http.Request) { r.SetBasicAuth(p.c.Secret, "") }

// get načíta zdroj vo formáte JSON; PrestaShop pri prázdnom výsledku vracia [].
func (p *presta) get(ctx context.Context, resource string, q url.Values, out any) error {
	q.Set("output_format", "JSON")
	var raw json.RawMessage
	if _, err := call(ctx, p.c.HTTP, http.MethodGet, p.c.URL+"/api/"+resource+"?"+q.Encode(), nil, &raw, p.auth); err != nil {
		return err
	}
	if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] == '[' {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (p *presta) Check(ctx context.Context) (string, error) {
	var d struct {
		Configurations []struct {
			Value string `json:"value"`
		} `json:"configurations"`
	}
	if err := p.get(ctx, "configurations", url.Values{"filter[name]": {"[PS_SHOP_NAME]"}, "display": {"[value]"}}, &d); err != nil {
		// kľúč nemusí mať právo na konfiguráciu — stačí, že vidí objednávky
		var o struct{}
		if err2 := p.get(ctx, "orders", url.Values{"limit": {"1"}}, &o); err2 != nil {
			return "", err2
		}
		return p.c.URL, nil
	}
	if len(d.Configurations) > 0 && d.Configurations[0].Value != "" {
		return d.Configurations[0].Value, nil
	}
	return p.c.URL, nil
}

// psText je viacjazyčný text: buď reťazec, alebo [{id, value}].
type psText string

func (t *psText) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = psText(s)
		return nil
	}
	var list []struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(b, &list); err != nil {
		return nil // neznámy tvar: bez textu
	}
	if len(list) > 0 {
		*t = psText(list[0].Value)
	}
	return nil
}

type psOrder struct {
	ID                 int64  `json:"id"`
	Reference          string `json:"reference"`
	IDAddressDelivery  string `json:"id_address_delivery"`
	IDAddressInvoice   string `json:"id_address_invoice"`
	IDCustomer         string `json:"id_customer"`
	IDCurrency         string `json:"id_currency"`
	CurrentState       string `json:"current_state"`
	Payment            string `json:"payment"`
	TotalPaidTaxIncl   num    `json:"total_paid_tax_incl"`
	TotalShippingExcl  num    `json:"total_shipping_tax_excl"`
	TotalShippingIncl  num    `json:"total_shipping_tax_incl"`
	TotalDiscountsExcl num    `json:"total_discounts_tax_excl"`
	TotalDiscountsIncl num    `json:"total_discounts_tax_incl"`
	TotalWrappingExcl  num    `json:"total_wrapping_tax_excl"`
	TotalWrappingIncl  num    `json:"total_wrapping_tax_incl"`
	DateAdd            string `json:"date_add"`
	Associations       struct {
		OrderRows []struct {
			ProductName      string `json:"product_name"`
			ProductReference string `json:"product_reference"`
			ProductQuantity  num    `json:"product_quantity"`
			UnitPriceExcl    num    `json:"unit_price_tax_excl"`
			UnitPriceIncl    num    `json:"unit_price_tax_incl"`
		} `json:"order_rows"`
	} `json:"associations"`
}

type psAddress struct {
	ID          int64  `json:"id"`
	Firstname   string `json:"firstname"`
	Lastname    string `json:"lastname"`
	Company     string `json:"company"`
	Address1    string `json:"address1"`
	Address2    string `json:"address2"`
	Postcode    string `json:"postcode"`
	City        string `json:"city"`
	IDCountry   string `json:"id_country"`
	Phone       string `json:"phone"`
	PhoneMobile string `json:"phone_mobile"`
	VATNumber   string `json:"vat_number"`
	DNI         string `json:"dni"`
}

// psRefs sú číselníky potrebné na objednávky (stavy, krajiny, meny).
type psRefs struct {
	paid, cancelled map[string]bool
	country, curr   map[string]string
}

func (p *presta) refs(ctx context.Context) (psRefs, error) {
	r := psRefs{paid: map[string]bool{}, cancelled: map[string]bool{}, country: map[string]string{}, curr: map[string]string{}}
	var st struct {
		States []struct {
			ID   int64  `json:"id"`
			Paid string `json:"paid"`
			Name psText `json:"name"`
		} `json:"order_states"`
	}
	if err := p.get(ctx, "order_states", url.Values{"display": {"full"}}, &st); err != nil {
		return r, err
	}
	for _, s := range st.States {
		id := strconv.FormatInt(s.ID, 10)
		r.paid[id] = s.Paid == "1"
		n := strings.ToLower(string(s.Name))
		// 6 = zrušená, 7 = vrátené peniaze v predvolenej inštalácii
		if id == "6" || id == "7" || strings.Contains(n, "zruš") || strings.Contains(n, "storn") || strings.Contains(n, "cancel") || strings.Contains(n, "refund") {
			r.cancelled[id] = true
		}
	}
	var co struct {
		Countries []struct {
			ID  int64  `json:"id"`
			ISO string `json:"iso_code"`
		} `json:"countries"`
	}
	if err := p.get(ctx, "countries", url.Values{"display": {"[id,iso_code]"}}, &co); err != nil {
		return r, err
	}
	for _, c := range co.Countries {
		r.country[strconv.FormatInt(c.ID, 10)] = strings.ToUpper(c.ISO)
	}
	var cu struct {
		Currencies []struct {
			ID  int64  `json:"id"`
			ISO string `json:"iso_code"`
		} `json:"currencies"`
	}
	if err := p.get(ctx, "currencies", url.Values{"display": {"[id,iso_code]"}}, &cu); err != nil {
		return r, err
	}
	for _, c := range cu.Currencies {
		r.curr[strconv.FormatInt(c.ID, 10)] = strings.ToUpper(c.ISO)
	}
	return r, nil
}

// psIDs je filter zoznamu ID: [1|2|3].
func psIDs(ids map[string]bool) string {
	var out []string
	for id := range ids {
		if _, err := strconv.Atoi(id); err == nil && id != "0" {
			out = append(out, id)
		}
	}
	return "[" + strings.Join(out, "|") + "]"
}

const psPage = 100

func (p *presta) Orders(ctx context.Context, since time.Time) ([]Order, error) {
	refs, err := p.refs(ctx)
	if err != nil {
		return nil, err
	}
	loc := local()
	var out []Order
	for offset := 0; ; offset += psPage {
		q := url.Values{"display": {"full"}, "sort": {"[date_add_ASC]"}, "limit": {fmt.Sprintf("%d,%d", offset, psPage)}}
		if !since.IsZero() {
			q.Set("date", "1")
			q.Set("filter[date_add]", "["+since.In(loc).Format("2006-01-02 15:04:05")+",2999-12-31 23:59:59]")
		}
		var d struct {
			Orders []psOrder `json:"orders"`
		}
		if err := p.get(ctx, "orders", q, &d); err != nil {
			return nil, err
		}
		if len(d.Orders) == 0 {
			return out, nil
		}
		addrIDs, custIDs := map[string]bool{}, map[string]bool{}
		for _, o := range d.Orders {
			addrIDs[o.IDAddressInvoice], addrIDs[o.IDAddressDelivery], custIDs[o.IDCustomer] = true, true, true
		}
		var ad struct {
			Addresses []psAddress `json:"addresses"`
		}
		if err := p.get(ctx, "addresses", url.Values{"display": {"full"}, "filter[id]": {psIDs(addrIDs)}}, &ad); err != nil {
			return nil, err
		}
		addrs := map[string]psAddress{}
		for _, a := range ad.Addresses {
			addrs[strconv.FormatInt(a.ID, 10)] = a
		}
		var cu struct {
			Customers []struct {
				ID    int64  `json:"id"`
				Email string `json:"email"`
			} `json:"customers"`
		}
		if err := p.get(ctx, "customers", url.Values{"display": {"[id,email]"}, "filter[id]": {psIDs(custIDs)}}, &cu); err != nil {
			return nil, err
		}
		mails := map[string]string{}
		for _, c := range cu.Customers {
			mails[strconv.FormatInt(c.ID, 10)] = c.Email
		}
		for _, o := range d.Orders {
			out = append(out, o.order(refs, addrs, mails[o.IDCustomer], loc))
		}
		if len(d.Orders) < psPage || len(out) >= MaxOrders {
			return out, nil
		}
	}
}

func (a psAddress) address(refs psRefs, email string) Address {
	out := Address{Name: joinNonEmpty(" ", a.Firstname, a.Lastname), Company: strings.TrimSpace(a.Company), Street: joinNonEmpty(", ", a.Address1, a.Address2),
		City: strings.TrimSpace(a.City), Zip: strings.TrimSpace(a.Postcode), Country: refs.country[a.IDCountry], Email: email,
		Phone: firstOf(a.Phone, a.PhoneMobile), ICDPH: strings.TrimSpace(a.VATNumber)}
	if d := strings.TrimSpace(a.DNI); d != "" {
		out.ICO = d // pole „DNI“ slovenské a české obchody používajú na IČO
	}
	return out
}

func (o psOrder) order(refs psRefs, addrs map[string]psAddress, email string, loc *time.Location) Order {
	created, _ := time.ParseInLocation("2006-01-02 15:04:05", o.DateAdd, loc)
	out := Order{ID: strconv.FormatInt(o.ID, 10), Number: firstOf(o.Reference, strconv.FormatInt(o.ID, 10)), Created: created.UTC(),
		Status: o.CurrentState, Paid: refs.paid[o.CurrentState], Cancelled: refs.cancelled[o.CurrentState], Currency: refs.curr[o.IDCurrency],
		Billing: addrs[o.IDAddressInvoice].address(refs, email), Total: o.TotalPaidTaxIncl.dec(), Payment: paymentOf(o.Payment)}
	if o.IDAddressDelivery != o.IDAddressInvoice {
		out.Shipping = addrs[o.IDAddressDelivery].address(refs, "")
	}
	if out.Paid {
		out.PaidAt = out.Created
	}
	for _, r := range o.Associations.OrderRows {
		q := r.ProductQuantity.dec()
		net := r.UnitPriceExcl.dec().Mul(q).RoundCents()
		out.Lines = append(out.Lines, Line{SKU: strings.TrimSpace(r.ProductReference), Name: strings.TrimSpace(r.ProductName), Quantity: q,
			Net: net, Tax: r.UnitPriceIncl.dec().Mul(q).RoundCents().Sub(net)})
	}
	extra := func(name string, excl, incl money.Dec, ship bool) {
		if !excl.IsZero() || !incl.IsZero() {
			out.Lines = append(out.Lines, Line{Name: name, Quantity: money.FromInt(1), Net: excl, Tax: incl.Sub(excl), Shipping: ship})
		}
	}
	extra("Doprava", o.TotalShippingExcl.dec(), o.TotalShippingIncl.dec(), true)
	extra("Darčekové balenie", o.TotalWrappingExcl.dec(), o.TotalWrappingIncl.dec(), false)
	extra("Zľava", o.TotalDiscountsExcl.dec().Neg(), o.TotalDiscountsIncl.dec().Neg(), false)
	return out
}

// psSafe: hodnota filtra nesmie obsahovať znaky syntaxe filtra.
func psSafe(s string) bool { return s != "" && !strings.ContainsAny(s, "[]|,%") }

type psStock struct {
	ID                 int64  `json:"id" xml:"id"`
	IDProduct          string `json:"id_product" xml:"id_product"`
	IDProductAttribute string `json:"id_product_attribute" xml:"id_product_attribute"`
	IDShop             string `json:"id_shop" xml:"id_shop"`
	IDShopGroup        string `json:"id_shop_group" xml:"id_shop_group"`
	Quantity           string `json:"quantity" xml:"quantity"`
	DependsOnStock     string `json:"depends_on_stock" xml:"depends_on_stock"`
	OutOfStock         string `json:"out_of_stock" xml:"out_of_stock"`
}

func (p *presta) SetStock(ctx context.Context, sku string, qty int64) error {
	if !psSafe(sku) {
		return ErrNoSKU
	}
	product, attr := "", "0"
	var pr struct {
		Products []struct {
			ID int64 `json:"id"`
		} `json:"products"`
	}
	if err := p.get(ctx, "products", url.Values{"display": {"[id]"}, "filter[reference]": {"[" + sku + "]"}}, &pr); err != nil {
		return err
	}
	if len(pr.Products) > 0 {
		product = strconv.FormatInt(pr.Products[0].ID, 10)
	} else {
		var cb struct {
			Combinations []struct {
				ID        int64  `json:"id"`
				IDProduct string `json:"id_product"`
			} `json:"combinations"`
		}
		if err := p.get(ctx, "combinations", url.Values{"display": {"[id,id_product]"}, "filter[reference]": {"[" + sku + "]"}}, &cb); err != nil {
			return err
		}
		if len(cb.Combinations) == 0 {
			return ErrNoSKU
		}
		product, attr = cb.Combinations[0].IDProduct, strconv.FormatInt(cb.Combinations[0].ID, 10)
	}
	var sa struct {
		Stocks []psStock `json:"stock_availables"`
	}
	if err := p.get(ctx, "stock_availables", url.Values{"display": {"full"}, "filter[id_product]": {"[" + product + "]"},
		"filter[id_product_attribute]": {"[" + attr + "]"}}, &sa); err != nil {
		return err
	}
	if len(sa.Stocks) == 0 {
		return ErrNoSKU
	}
	s := sa.Stocks[0]
	s.Quantity = strconv.FormatInt(qty, 10)
	body, err := xml.Marshal(struct {
		XMLName xml.Name `xml:"prestashop"`
		Stock   psStock  `xml:"stock_available"`
	}{Stock: s})
	if err != nil {
		return err
	}
	_, err = send(ctx, p.c.HTTP, http.MethodPut, fmt.Sprintf("%s/api/stock_availables/%d", p.c.URL, s.ID), "application/xml", body, nil, p.auth)
	return err
}

// local je časové pásmo obchodu (dátumy PrestaShopu a Magenta bez pásma).
func local() *time.Location {
	if loc, err := time.LoadLocation("Europe/Bratislava"); err == nil {
		return loc
	}
	return time.UTC
}
