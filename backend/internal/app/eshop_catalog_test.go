package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/eshop"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// wooCatalog je falošný WooCommerce s cenníkom (ceny bez DPH).
type wooCatalog struct {
	mu     sync.Mutex
	writes []string // "PUT /…" alebo "POST /…"
	prices map[string]string
}

func (f *wooCatalog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodGet {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		f.writes = append(f.writes, r.Method+" "+r.URL.Path)
		if sku, _ := b["sku"].(string); sku != "" {
			f.prices[sku] = b["regular_price"].(string)
		}
		io.WriteString(w, `{}`)
		return
	}
	w.Header().Set("X-WP-TotalPages", "1")
	switch r.URL.Path {
	case "/wp-json/wc/v3/settings/tax/woocommerce_prices_include_tax":
		io.WriteString(w, `{"value":"no"}`)
	case "/wp-json/wc/v3/taxes":
		io.WriteString(w, `[{"country":"SK","rate":"23.0000","class":"standard"},{"country":"SK","rate":"5.0000","class":"super-reduced"}]`)
	case "/wp-json/wc/v3/products":
		switch sku := r.URL.Query().Get("sku"); sku {
		case "":
			io.WriteString(w, `[{"id":1,"type":"simple","sku":"REL24","name":"Relé 24 V","regular_price":"10.00","tax_class":"","status":"publish"},
 {"id":2,"type":"simple","sku":"KNIHA","name":"Norma","regular_price":"20.00","tax_class":"super-reduced","status":"publish"},
 {"id":3,"type":"simple","sku":"DIVNA","name":"Divná daň","regular_price":"1.00","tax_class":"neznama","status":"publish"}]`)
		case "REL24", "KNIHA":
			io.WriteString(w, `[{"id":1,"type":"simple","sku":"`+sku+`","status":"publish"}]`)
		default:
			io.WriteString(w, `[]`)
		}
	case "/wp-json/wc/v3/customers":
		io.WriteString(w, `[{"email":"eva@example.sk","first_name":"Eva","last_name":"Malá","billing":{"company":"Malá s.r.o.","address_1":"Nová 2","city":"Nitra","postcode":"94901","country":"SK"}}]`)
	default:
		http.NotFound(w, r)
	}
}

func TestEshopCatalog(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	shop := &wooCatalog{prices: map[string]string{}}
	srv := httptest.NewServer(shop)
	defer srv.Close()
	a.HTTP = eshop.Client(true)
	set, err := a.SaveEshops(ctx, c.ID, []EshopConn{{Platform: eshop.WooCommerce, URL: srv.URL, Key: "k", Secret: "s", Manage: true}})
	if err != nil {
		t.Fatal(err)
	}
	id := set.Conns[0].ID
	if !set.Conns[0].Manage {
		t.Fatal("riadenie sa neuložilo")
	}

	// prenos z obchodu: náhľad, potom zápis
	rep, err := a.ImportEshopCatalog(ctx, c.ID, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Items.New != 2 || rep.Items.Skipped != 1 || rep.Contacts.New != 1 {
		t.Fatalf("náhľad %+v", rep)
	}
	if items, _ := a.Store.ListItems(ctx, c.ID); len(items) != 0 {
		t.Fatal("náhľad zapisoval")
	}
	if _, err := a.ImportEshopCatalog(ctx, c.ID, id, true); err != nil {
		t.Fatal(err)
	}
	items, _ := a.Store.ListItems(ctx, c.ID)
	if len(items) != 2 {
		t.Fatalf("cenník %+v", items)
	}
	for _, it := range items {
		if it.Code == "KNIHA" && (it.Rate != vat.Rate5 || !it.Price.Equal(m.FromInt(20))) {
			t.Errorf("kniha %+v", it)
		}
	}

	// odoslanie: prvé pošle všetko, druhé nič, zmena ceny iba ju, nová položka sa založí
	push := func() EshopCatalogReport {
		t.Helper()
		shop.writes = nil
		r, err := a.PushEshopCatalog(ctx, c.ID, id, false)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := push(); r.Updated != 2 || r.Created != 0 || len(r.Errors) != 0 {
		t.Errorf("prvé odoslanie %+v", r)
	}
	if r := push(); r.Updated != 0 || r.Unchanged != 2 || len(shop.writes) != 0 {
		t.Errorf("bez zmien %+v %v", r, shop.writes)
	}
	for _, it := range items {
		if it.Code == "REL24" {
			it.Price = m.FromInt(11)
			if _, err := a.SaveItem(ctx, c.ID, it); err != nil {
				t.Fatal(err)
			}
		}
	}
	// nová skladová položka sa založí, služba nie
	novy, err := a.SaveItem(ctx, c.ID, catalog.Item{Code: "NOVY", Name: "Nový", Unit: "ks", Price: m.FromInt(3), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveItem(ctx, c.ID, catalog.Item{Code: "MONTAZ", Name: "Montáž", Unit: "hod", Price: m.FromInt(30), PriceMode: vat.PriceNet, Rate: vat.Rate23}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}},
		Items: []StockItem{{ItemID: novy.ID, Kind: stock.Goods}}}); err != nil {
		t.Fatal(err)
	}
	r := push()
	if r.Updated != 1 || r.Created != 1 || r.Unchanged != 1 || r.NotInShop != 1 || shop.prices["NOVY"] != "3.00" || shop.prices["MONTAZ"] != "" {
		t.Errorf("zmeny %+v %v %v", r, shop.writes, shop.prices)
	}
	if r := push(); r.Unchanged != 4 || len(shop.writes) != 0 {
		t.Errorf("služba sa skúša znova %+v %v", r, shop.writes)
	}

	// riadenie každú hodinu: nič nové → nič sa nepošle
	shop.writes = nil
	a.SyncAllEshops(ctx)
	if len(shop.writes) != 0 {
		t.Errorf("automatické odoslanie bez zmien zapisovalo: %v", shop.writes)
	}
	got, _ := a.Eshops(ctx, c.ID)
	if got.Conns[0].LastError != "" || got.Conns[0].LastPush == "" {
		t.Errorf("stav %+v", got.Conns[0])
	}
}
