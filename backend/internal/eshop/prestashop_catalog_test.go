package eshop

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

const psProductXML = `<?xml version="1.0" encoding="UTF-8"?>
<prestashop xmlns:xlink="http://www.w3.org/1999/xlink">
<product>
	<id><![CDATA[40]]></id>
	<manufacturer_name><![CDATA[ABB]]></manufacturer_name>
	<quantity><![CDATA[5]]></quantity>
	<id_tax_rules_group><![CDATA[1]]></id_tax_rules_group>
	<price><![CDATA[10.000000]]></price>
	<active><![CDATA[1]]></active>
	<name><language id="1" xlink:href="http://x/api/languages/1"><![CDATA[Kábel]]></language></name>
	<description><language id="1"><![CDATA[<p>Popis & viac</p>]]></language></description>
	<associations><categories><category xlink:href="http://x/api/categories/2"><id><![CDATA[2]]></id></category></categories></associations>
</product>
</prestashop>`

func TestPsRewrite(t *testing.T) {
	out, err := psRewrite([]byte(psProductXML), map[string]string{"price": "11.000000", "id_tax_rules_group": "3"}, psReadOnly, "Kábel CYKY")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"<price>11.000000</price>", "<id_tax_rules_group>3</id_tax_rules_group>", `<language id="1">Kábel CYKY</language>`,
		"<active>1</active>", "&lt;p&gt;Popis &amp; viac&lt;/p&gt;", "<category><id>2</id></category>"} {
		if !strings.Contains(s, want) {
			t.Errorf("chýba %q v %s", want, s)
		}
	}
	for _, bad := range []string{"manufacturer_name", "quantity", "xlink", "Kábel</"} {
		if strings.Contains(s, bad) {
			t.Errorf("nemá obsahovať %q: %s", bad, s)
		}
	}
	// viac jazykov: názov ostane
	two := strings.Replace(psProductXML, `<![CDATA[Kábel]]></language></name>`, `<![CDATA[Kábel]]></language><language id="2"><![CDATA[Cable]]></language></name>`, 1)
	out, _ = psRewrite([]byte(two), nil, psReadOnly, "Nový")
	if strings.Contains(string(out), "Nový") || !strings.Contains(string(out), "Cable") {
		t.Errorf("viacjazyčný názov sa nemá meniť: %s", out)
	}
}

func TestPrestaCatalog(t *testing.T) {
	var puts, posts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			if r.Method == http.MethodPut {
				puts = append(puts, r.URL.Path+" "+string(b))
			} else {
				posts = append(posts, string(b))
			}
			return
		}
		switch r.URL.Path {
		case "/api/order_states", "/api/currencies":
			io.WriteString(w, `[]`)
		case "/api/countries":
			io.WriteString(w, `{"countries":[{"id":37,"iso_code":"SK"},{"id":16,"iso_code":"CZ"}]}`)
		case "/api/taxes":
			io.WriteString(w, `{"taxes":[{"id":1,"rate":"23.000"},{"id":2,"rate":"21.000"},{"id":3,"rate":"5.000"}]}`)
		case "/api/tax_rule_groups":
			io.WriteString(w, `{"tax_rule_groups":[{"id":1},{"id":3}]}`)
		case "/api/tax_rules":
			io.WriteString(w, `{"tax_rules":[{"id_tax_rules_group":"1","id_country":"16","id_tax":"2"},{"id_tax_rules_group":"1","id_country":"37","id_tax":"1"},{"id_tax_rules_group":"3","id_country":"37","id_tax":"3"}]}`)
		case "/api/products":
			switch {
			case q.Get("filter[reference]") == "[CYKY]":
				io.WriteString(w, `{"products":[{"id":40}]}`)
			case q.Get("filter[id]") == "[41]":
				io.WriteString(w, `{"products":[{"price":"10.000000","id_tax_rules_group":"1"}]}`)
			case q.Get("filter[reference]") != "":
				io.WriteString(w, `[]`)
			case q.Get("limit") == "0,100":
				io.WriteString(w, `{"products":[{"id":40,"reference":"CYKY","name":[{"id":"1","value":"Kábel"}],"price":"10.000000","id_tax_rules_group":"1","active":"1"},
 {"id":41,"reference":"","name":"Tričko","price":"10.000000","id_tax_rules_group":"1","active":"1"}]}`)
			default:
				io.WriteString(w, `[]`)
			}
		case "/api/combinations":
			switch {
			case q.Get("filter[reference]") == "[TR-M]":
				io.WriteString(w, `{"combinations":[{"id":9,"id_product":"41"}]}`)
			case q.Get("filter[reference]") != "":
				io.WriteString(w, `[]`)
			case q.Get("limit") == "0,100":
				io.WriteString(w, `{"combinations":[{"id_product":"41","reference":"TR-M","price":"2.000000"}]}`)
			default:
				io.WriteString(w, `[]`)
			}
		case "/api/products/40":
			io.WriteString(w, psProductXML)
		case "/api/combinations/9":
			io.WriteString(w, `<prestashop><combination><id>9</id><price>2.000000</price><quantity>3</quantity></combination></prestashop>`)
		case "/api/languages":
			io.WriteString(w, `{"languages":[{"id":1},{"id":2}]}`)
		case "/api/configurations":
			io.WriteString(w, `{"configurations":[{"value":"2"}]}`)
		case "/api/addresses":
			io.WriteString(w, `{"addresses":[{"id":7,"id_customer":"3","firstname":"Ján","lastname":"Novák","company":"Novák s.r.o.","address1":"Hlavná 1","postcode":"01001","city":"Žilina","id_country":"37","dni":"36000001"}]}`)
		case "/api/customers":
			if q.Get("limit") == "0,100" {
				io.WriteString(w, `{"customers":[{"id":3,"firstname":"Ján","lastname":"Novák","email":"jan@x.sk","company":""}]}`)
				return
			}
			io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, _ := New(Conn{Platform: PrestaShop, URL: srv.URL, Secret: "K", HTTP: Client(true)})
	cat := c.(Catalog)
	ps, err := cat.Products(context.Background())
	if err != nil || len(ps) != 2 {
		t.Fatalf("%+v %v", ps, err)
	}
	if ps[0].Rate != 23 || !ps[0].Gross.Equal(money.MustParse("12.30")) || ps[1].SKU != "TR-M" || !ps[1].Net.Equal(money.FromInt(12)) || ps[1].Name != "Tričko (TR-M)" {
		t.Errorf("produkty %+v", ps)
	}
	cs, err := cat.Customers(context.Background())
	if err != nil || len(cs) != 1 || cs[0].Company != "Novák s.r.o." || cs[0].ICO != "36000001" || cs[0].Email != "jan@x.sk" {
		t.Errorf("zákazníci %+v %v", cs, err)
	}
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "CYKY", Name: "Kábel CYKY", Net: money.FromInt(11), Rate: 5, Active: false}); err != nil {
		t.Fatal(err)
	}
	if len(puts) != 1 || !strings.HasPrefix(puts[0], "/api/products/40 ") || !strings.Contains(puts[0], "<price>11.00</price>") ||
		!strings.Contains(puts[0], "<id_tax_rules_group>3</id_tax_rules_group>") || !strings.Contains(puts[0], "<active>0</active>") {
		t.Errorf("úprava %v", puts)
	}
	puts = nil
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "TR-M", Net: money.FromInt(13), Rate: 23, Active: true}); err != nil {
		t.Fatal(err)
	}
	if len(puts) != 1 || !strings.Contains(puts[0], "<price>3.00</price>") || strings.Contains(puts[0], "quantity") {
		t.Errorf("variant %v", puts)
	}
	created, err := cat.PushProduct(context.Background(), Product{SKU: "NOVY", Name: "Nová zásuvka", Net: money.FromInt(5), Rate: 23, Active: true, Create: true})
	if err != nil || !created || len(posts) != 1 || !strings.Contains(posts[0], "<link_rewrite><language id=\"1\">nova-zasuvka</language><language id=\"2\">nova-zasuvka</language></link_rewrite>") ||
		!strings.Contains(posts[0], "<active>0</active>") || !strings.Contains(posts[0], "<id_category_default>2</id_category_default>") {
		t.Errorf("nový %v %v %v", created, err, posts)
	}
	if _, err := cat.PushProduct(context.Background(), Product{SKU: "X", Rate: 19, Active: true, Create: true}); err == nil {
		t.Error("chýbajúca sadzba")
	}
}
