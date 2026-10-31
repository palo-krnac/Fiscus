package eshop

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// PrestaShop: cenník a zákazníci. Cena produktu je v PrestaShope vždy bez
// DPH, sadzbu určuje skupina daňových pravidiel (id_tax_rules_group, sadzba
// pre SK). Úprava (PUT) prijíma iba celý produkt v XML — Fiškus ho načíta,
// zmení iba cenu, skupinu dane, aktívnosť a názov (iba pri jednom jazyku)
// a vynechá polia, ktoré sa zapisovať nedajú (manufacturer_name, quantity).
// Pri kombinácii (variante) sa mení iba príplatok k cene produktu.

type psTax struct {
	groupRate map[string]int
	rateGroup map[int]string
}

func (p *presta) taxes(ctx context.Context, refs psRefs) (*psTax, error) {
	t := &psTax{groupRate: map[string]int{"0": 0}, rateGroup: map[int]string{}}
	var tx struct {
		Taxes []struct {
			ID   int64 `json:"id"`
			Rate num   `json:"rate"`
		} `json:"taxes"`
	}
	if err := p.get(ctx, "taxes", url.Values{"display": {"[id,rate]"}}, &tx); err != nil {
		return nil, err
	}
	rate := map[string]int{}
	for _, x := range tx.Taxes {
		rate[strconv.FormatInt(x.ID, 10)] = int(x.Rate.dec().RoundTo(0).Raw() / money.FromInt(1).Raw())
	}
	var gr struct {
		Groups []struct {
			ID int64 `json:"id"`
		} `json:"tax_rule_groups"`
	}
	if err := p.get(ctx, "tax_rule_groups", url.Values{"display": {"[id]"}, "filter[active]": {"[1]"}, "filter[deleted]": {"[0]"}}, &gr); err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, g := range gr.Groups {
		active[strconv.FormatInt(g.ID, 10)] = true
	}
	var rules struct {
		Rules []struct {
			Group   string `json:"id_tax_rules_group"`
			Country string `json:"id_country"`
			Tax     string `json:"id_tax"`
		} `json:"tax_rules"`
	}
	if err := p.get(ctx, "tax_rules", url.Values{"display": {"[id_tax_rules_group,id_country,id_tax]"}}, &rules); err != nil {
		return nil, err
	}
	sk := map[string]bool{}
	for _, r := range rules.Rules {
		if !active[r.Group] {
			continue
		}
		isSK := refs.country[r.Country] == "SK"
		if _, seen := t.groupRate[r.Group]; !seen || (isSK && !sk[r.Group]) {
			t.groupRate[r.Group], sk[r.Group] = rate[r.Tax], isSK
		}
	}
	for g, r := range t.groupRate {
		if old, ok := t.rateGroup[r]; !ok || (sk[g] && !sk[old]) || (sk[g] == sk[old] && len(g) < len(old)) {
			t.rateGroup[r] = g
		}
	}
	return t, nil
}

func psRate(t *psTax, group string) int {
	if r, ok := t.groupRate[group]; ok {
		return r
	}
	return -1
}

func psProduct(sku, name string, net money.Dec, rate int, active bool) Product {
	pr := Product{SKU: strings.TrimSpace(sku), Name: strings.TrimSpace(name), Rate: rate, Active: active, Net: net.RoundCents(), Gross: net.RoundCents()}
	if rate > 0 {
		pr.Gross = net.MulRatio(money.FromInt(int64(100+rate)), money.FromInt(100)).RoundCents()
	}
	return pr
}

func (p *presta) Products(ctx context.Context) ([]Product, error) {
	refs, err := p.refs(ctx)
	if err != nil {
		return nil, err
	}
	t, err := p.taxes(ctx, refs)
	if err != nil {
		return nil, err
	}
	type prod struct {
		ID        int64  `json:"id"`
		Reference string `json:"reference"`
		Name      psText `json:"name"`
		Price     num    `json:"price"`
		TaxGroup  string `json:"id_tax_rules_group"`
		Active    string `json:"active"`
	}
	byID := map[string]prod{}
	var out []Product
	for offset := 0; len(out) < MaxProducts; offset += psPage {
		var d struct {
			Products []prod `json:"products"`
		}
		q := url.Values{"display": {"[id,reference,name,price,id_tax_rules_group,active]"}, "limit": {fmt.Sprintf("%d,%d", offset, psPage)}}
		if err := p.get(ctx, "products", q, &d); err != nil {
			return nil, err
		}
		for _, x := range d.Products {
			byID[strconv.FormatInt(x.ID, 10)] = x
			if x.Reference != "" {
				out = append(out, psProduct(x.Reference, string(x.Name), x.Price.dec(), psRate(t, x.TaxGroup), x.Active == "1"))
			}
		}
		if len(d.Products) < psPage {
			break
		}
	}
	for offset := 0; len(out) < MaxProducts; offset += psPage {
		var d struct {
			Combinations []struct {
				IDProduct string `json:"id_product"`
				Reference string `json:"reference"`
				Price     num    `json:"price"`
			} `json:"combinations"`
		}
		q := url.Values{"display": {"[id_product,reference,price]"}, "limit": {fmt.Sprintf("%d,%d", offset, psPage)}}
		if err := p.get(ctx, "combinations", q, &d); err != nil {
			return nil, err
		}
		for _, c := range d.Combinations {
			base, ok := byID[c.IDProduct]
			if !ok || c.Reference == "" {
				continue
			}
			name := string(base.Name) + " (" + c.Reference + ")"
			out = append(out, psProduct(c.Reference, name, base.Price.dec().Add(c.Price.dec()), psRate(t, base.TaxGroup), base.Active == "1"))
		}
		if len(d.Combinations) < psPage {
			break
		}
	}
	return out, nil
}

func (p *presta) Customers(ctx context.Context) ([]Address, error) {
	refs, err := p.refs(ctx)
	if err != nil {
		return nil, err
	}
	var addrs struct {
		Addresses []struct {
			psAddress
			IDCustomer string `json:"id_customer"`
			Deleted    string `json:"deleted"`
		} `json:"addresses"`
	}
	if err := p.get(ctx, "addresses", url.Values{"display": {"full"}, "filter[deleted]": {"[0]"}}, &addrs); err != nil {
		return nil, err
	}
	first := map[string]psAddress{}
	for _, a := range addrs.Addresses {
		if _, ok := first[a.IDCustomer]; !ok && a.IDCustomer != "0" {
			first[a.IDCustomer] = a.psAddress
		}
	}
	var out []Address
	for offset := 0; len(out) < MaxProducts; offset += psPage {
		var d struct {
			Customers []struct {
				ID        int64  `json:"id"`
				Firstname string `json:"firstname"`
				Lastname  string `json:"lastname"`
				Email     string `json:"email"`
				Company   string `json:"company"`
			} `json:"customers"`
		}
		q := url.Values{"display": {"[id,firstname,lastname,email,company]"}, "limit": {fmt.Sprintf("%d,%d", offset, psPage)}}
		if err := p.get(ctx, "customers", q, &d); err != nil {
			return nil, err
		}
		for _, c := range d.Customers {
			a := Address{}
			if ad, ok := first[strconv.FormatInt(c.ID, 10)]; ok {
				a = ad.address(refs, "")
			}
			a.Name = firstOf(a.Name, joinNonEmpty(" ", c.Firstname, c.Lastname))
			a.Company = firstOf(a.Company, c.Company)
			a.Email = strings.TrimSpace(c.Email)
			out = append(out, a)
		}
		if len(d.Customers) < psPage {
			break
		}
	}
	return out, nil
}

// psRewrite upraví XML zdroja (prestashop > product|combination > pole):
// set nahradí hodnoty polí, drop polia vynechá, name (ak nie je prázdne)
// nastaví názov, ak má práve jeden jazyk. Atribúty xlink sa vynechajú.
func psRewrite(data []byte, set map[string]string, drop map[string]bool, name string) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	depth, skip := 0, 0
	var nameToks []xml.Token
	inName := false
	clean := func(se xml.StartElement) xml.StartElement {
		var attrs []xml.Attr
		for _, a := range se.Attr {
			if a.Name.Space == "" && a.Name.Local != "xmlns" {
				attrs = append(attrs, a)
			}
		}
		return xml.StartElement{Name: xml.Name{Local: se.Name.Local}, Attr: attrs}
	}
	emit := func(t xml.Token) error { return enc.EncodeToken(t) }
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		tok = xml.CopyToken(tok)
		switch t := tok.(type) {
		case xml.ProcInst, xml.Comment, xml.Directive:
			continue
		case xml.StartElement:
			depth++
			if skip > 0 {
				skip++
				continue
			}
			t = clean(t)
			if depth == 3 && drop[t.Name.Local] {
				skip = 1
				continue
			}
			if depth == 3 && t.Name.Local == "name" && name != "" {
				inName, nameToks = true, []xml.Token{t}
				continue
			}
			if inName {
				nameToks = append(nameToks, t)
				continue
			}
			if err := emit(t); err != nil {
				return nil, err
			}
			if v, ok := set[t.Name.Local]; ok && depth == 3 {
				if err := emit(xml.CharData(v)); err != nil {
					return nil, err
				}
				// pôvodný obsah poľa preskočiť až po jeho koniec
				for d := 1; d > 0; {
					x, err := dec.Token()
					if err != nil {
						return nil, err
					}
					switch x.(type) {
					case xml.StartElement:
						d++
					case xml.EndElement:
						d--
					}
				}
				depth--
				if err := emit(xml.EndElement{Name: t.Name}); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			depth--
			if skip > 0 {
				skip--
				continue
			}
			t = xml.EndElement{Name: xml.Name{Local: t.Name.Local}}
			if inName {
				nameToks = append(nameToks, t)
				if depth == 2 { // koniec <name>
					inName = false
					langs := 0
					for _, x := range nameToks {
						if se, ok := x.(xml.StartElement); ok && se.Name.Local == "language" {
							langs++
						}
					}
					for _, x := range nameToks {
						if cd, ok := x.(xml.CharData); ok && langs == 1 && len(bytes.TrimSpace(cd)) > 0 {
							x = xml.CharData(name)
						}
						if err := emit(x); err != nil {
							return nil, err
						}
					}
				}
				continue
			}
			if err := emit(t); err != nil {
				return nil, err
			}
		case xml.CharData:
			if skip > 0 {
				continue
			}
			if inName {
				nameToks = append(nameToks, t)
				continue
			}
			if err := emit(t); err != nil {
				return nil, err
			}
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var psReadOnly = map[string]bool{"manufacturer_name": true, "quantity": true, "position_in_category": true}

func psPrice(d money.Dec) string { return d.RoundCents().String() }

func (p *presta) PushProduct(ctx context.Context, pr Product) (bool, error) {
	if !psSafe(pr.SKU) {
		return false, ErrNoSKU
	}
	refs, err := p.refs(ctx)
	if err != nil {
		return false, err
	}
	t, err := p.taxes(ctx, refs)
	if err != nil {
		return false, err
	}
	group, ok := t.rateGroup[pr.Rate]
	if !ok {
		return false, fmt.Errorf("obchod nemá skupinu daňových pravidiel so sadzbou %d %% — pridajte ju v PrestaShope (Medzinárodné → Dane → Daňové pravidlá)", pr.Rate)
	}
	var found struct {
		Products []struct {
			ID int64 `json:"id"`
		} `json:"products"`
	}
	if err := p.get(ctx, "products", url.Values{"display": {"[id]"}, "filter[reference]": {"[" + pr.SKU + "]"}}, &found); err != nil {
		return false, err
	}
	if len(found.Products) > 0 {
		path := fmt.Sprintf("/api/products/%d", found.Products[0].ID)
		var raw []byte
		if _, err := send(ctx, p.c.HTTP, http.MethodGet, p.c.URL+path, "", nil, &raw, p.auth); err != nil {
			return false, err
		}
		set := map[string]string{"price": psPrice(pr.Net), "id_tax_rules_group": group}
		if !pr.Active {
			set["active"] = "0" // neaktívny produkt Fiškus nezapína
		}
		body, err := psRewrite(raw, set, psReadOnly, pr.Name)
		if err != nil {
			return false, err
		}
		_, err = send(ctx, p.c.HTTP, http.MethodPut, p.c.URL+path, "application/xml", body, nil, p.auth)
		return false, err
	}
	var cb struct {
		Combinations []struct {
			ID        int64  `json:"id"`
			IDProduct string `json:"id_product"`
		} `json:"combinations"`
	}
	if err := p.get(ctx, "combinations", url.Values{"display": {"[id,id_product]"}, "filter[reference]": {"[" + pr.SKU + "]"}}, &cb); err != nil {
		return false, err
	}
	if len(cb.Combinations) > 0 {
		var base struct {
			Products []struct {
				Price    num    `json:"price"`
				TaxGroup string `json:"id_tax_rules_group"`
			} `json:"products"`
		}
		if err := p.get(ctx, "products", url.Values{"display": {"[price,id_tax_rules_group]"}, "filter[id]": {"[" + cb.Combinations[0].IDProduct + "]"}}, &base); err != nil {
			return false, err
		}
		if len(base.Products) == 0 {
			return false, ErrNoSKU
		}
		if psRate(t, base.Products[0].TaxGroup) != pr.Rate {
			return false, errors.New("variant má inú sadzbu DPH ako produkt — v PrestaShope má sadzbu iba produkt")
		}
		path := fmt.Sprintf("/api/combinations/%d", cb.Combinations[0].ID)
		var raw []byte
		if _, err := send(ctx, p.c.HTTP, http.MethodGet, p.c.URL+path, "", nil, &raw, p.auth); err != nil {
			return false, err
		}
		body, err := psRewrite(raw, map[string]string{"price": psPrice(pr.Net.Sub(base.Products[0].Price.dec()))}, psReadOnly, "")
		if err != nil {
			return false, err
		}
		_, err = send(ctx, p.c.HTTP, http.MethodPut, p.c.URL+path, "application/xml", body, nil, p.auth)
		return false, err
	}
	if !pr.Active || !pr.Create {
		return false, ErrNoSKU
	}
	return true, p.create(ctx, pr, group)
}

// create založí neaktívny produkt v domovskej kategórii.
func (p *presta) create(ctx context.Context, pr Product, group string) error {
	var langs struct {
		Languages []struct {
			ID int64 `json:"id"`
		} `json:"languages"`
	}
	if err := p.get(ctx, "languages", url.Values{"display": {"[id]"}, "filter[active]": {"[1]"}}, &langs); err != nil {
		return err
	}
	if len(langs.Languages) == 0 {
		langs.Languages = append(langs.Languages, struct {
			ID int64 `json:"id"`
		}{1})
	}
	home := "2"
	var cfg struct {
		Configurations []struct {
			Value string `json:"value"`
		} `json:"configurations"`
	}
	if p.get(ctx, "configurations", url.Values{"filter[name]": {"[PS_HOME_CATEGORY]"}, "display": {"[value]"}}, &cfg) == nil &&
		len(cfg.Configurations) > 0 && cfg.Configurations[0].Value != "" {
		home = cfg.Configurations[0].Value
	}
	type lang struct {
		ID   int64  `xml:"id,attr"`
		Text string `xml:",chardata"`
	}
	var names, slugs []lang
	for _, l := range langs.Languages {
		names = append(names, lang{l.ID, pr.Name})
		slugs = append(slugs, lang{l.ID, slug(pr.Name)})
	}
	doc := struct {
		XMLName xml.Name `xml:"prestashop"`
		Product struct {
			Reference   string `xml:"reference"`
			Price       string `xml:"price"`
			TaxGroup    string `xml:"id_tax_rules_group"`
			Active      int    `xml:"active"`
			State       int    `xml:"state"`
			Category    string `xml:"id_category_default"`
			Name        []lang `xml:"name>language"`
			LinkRewrite []lang `xml:"link_rewrite>language"`
			Categories  []struct {
				ID string `xml:"id"`
			} `xml:"associations>categories>category"`
		} `xml:"product"`
	}{}
	d := &doc.Product
	d.Reference, d.Price, d.TaxGroup, d.State, d.Category = pr.SKU, psPrice(pr.Net), group, 1, home
	d.Name, d.LinkRewrite = names, slugs
	d.Categories = append(d.Categories, struct {
		ID string `xml:"id"`
	}{home})
	body, err := xml.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = send(ctx, p.c.HTTP, http.MethodPost, p.c.URL+"/api/products", "application/xml", body, nil, p.auth)
	return err
}

// slug vytvorí adresu produktu z názvu (malé písmená bez diakritiky).
func slug(s string) string {
	r := strings.NewReplacer("á", "a", "ä", "a", "č", "c", "ď", "d", "é", "e", "ě", "e", "í", "i", "ĺ", "l", "ľ", "l", "ň", "n",
		"ó", "o", "ô", "o", "ŕ", "r", "ř", "r", "š", "s", "ť", "t", "ú", "u", "ů", "u", "ý", "y", "ž", "z")
	var b strings.Builder
	dash := false
	for _, c := range r.Replace(strings.ToLower(s)) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
