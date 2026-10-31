package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/eshop"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// fakeWoo je falošný WooCommerce: objednávky sa dajú meniť medzi načítaniami.
type fakeWoo struct {
	mu     sync.Mutex
	orders []map[string]any
	stock  map[string]float64 // kód → nastavené množstvo
	skus   map[string]int     // kódy, ktoré obchod pozná → ID produktu
}

func (f *fakeWoo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if k, s, ok := r.BasicAuth(); !ok || k != "ck_test" || s != "cs_test" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/wp-json/wc/v3/orders":
		w.Header().Set("X-WP-TotalPages", "1")
		json.NewEncoder(w).Encode(f.orders)
	case r.URL.Path == "/wp-json/wc/v3/products" && r.Method == http.MethodGet:
		sku := strings.ToUpper(r.URL.Query().Get("sku")) // MySQL porovnáva bez ohľadu na veľkosť písmen
		if id, ok := f.skus[sku]; ok {
			json.NewEncoder(w).Encode([]map[string]any{{"id": id, "type": "simple", "sku": sku}})
			return
		}
		io.WriteString(w, "[]")
	case strings.HasPrefix(r.URL.Path, "/wp-json/wc/v3/products/") && r.Method == http.MethodPut:
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		for sku, id := range f.skus {
			if r.URL.Path == "/wp-json/wc/v3/products/"+jsonNum(id) {
				f.stock[sku] = body["stock_quantity"].(float64)
			}
		}
		io.WriteString(w, "{}")
	default:
		http.NotFound(w, r)
	}
}

func jsonNum(i int) string { return strconv.Itoa(i) }

func wooOrder(id int, status, currency string, lines []map[string]any) map[string]any {
	paid := ""
	if status == "processing" || status == "completed" {
		paid = "2026-10-20T09:00:00"
	}
	return map[string]any{"id": id, "number": jsonNum(id), "status": status, "currency": currency,
		"date_created_gmt": "2026-10-20T08:00:00", "date_paid_gmt": paid, "total": "30.75", "payment_method": "bacs",
		"billing": map[string]any{"first_name": "Ján", "last_name": "Novák", "company": "Novák s.r.o.", "address_1": "Hlavná 1",
			"city": "Žilina", "postcode": "010 01", "country": "SK", "email": "jan@novak.sk"},
		"shipping":   map[string]any{"first_name": "Ján", "last_name": "Novák", "address_1": "Sklad 5", "city": "Martin", "postcode": "036 01", "country": "SK"},
		"line_items": lines, "shipping_lines": []map[string]any{{"method_title": "Kuriér", "total": "5.00", "total_tax": "1.15"}},
		"meta_data": []map[string]any{{"key": "_billing_ico", "value": "36000001"}}}
}

func TestEshopOrders(t *testing.T) {
	a, c := newApp(t, edition.Server)
	shop := &fakeWoo{stock: map[string]float64{}, skus: map[string]int{"CYKY3": 11}}
	srv := httptest.NewServer(shop)
	defer srv.Close()
	a.HTTP = eshop.Client(true)

	cable, err := a.SaveItem(ctx, c.ID, catalog.Item{Code: "cyky3", Name: "Kábel CYKY", Unit: "m", Price: m.MustParse("10"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if err != nil {
		t.Fatal(err)
	}

	// nastavenie: chyby polí, potom platné napojenie
	_, err = a.SaveEshops(ctx, c.ID, []EshopConn{{Platform: eshop.WooCommerce, URL: "http://obchod.sk"}})
	for _, f := range []string{"conns[0].url", "conns[0].key", "conns[0].secret"} {
		if !fieldErr(err, f) {
			t.Errorf("chýba chyba %s: %v", f, err)
		}
	}
	set, err := a.SaveEshops(ctx, c.ID, []EshopConn{{Platform: eshop.WooCommerce, URL: srv.URL, Key: "ck_test", Secret: "cs_test", OnlyPaid: true}})
	if err != nil {
		t.Fatal(err)
	}
	conn := set.Conns[0]
	if !conn.HasKey || !conn.HasSecret || conn.Name != "WooCommerce" || conn.Since.String() != "2026-09-29" {
		t.Errorf("napojenie %+v", conn)
	}
	// tajné údaje sa nevracajú; pri uložení bez nich ostanú
	conn.Name = "Elektro obchod"
	set, err = a.SaveEshops(ctx, c.ID, []EshopConn{conn})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := a.Eshops(ctx, c.ID)
	if b, _ := json.Marshal(got); strings.Contains(string(b), "cs_test") || strings.Contains(string(b), "ck_test") {
		t.Fatalf("tajné údaje v odpovedi: %s", b)
	}
	if name, err := a.TestEshop(ctx, c.ID, conn.ID); err != nil || name == "" {
		t.Fatalf("test spojenia: %q %v", name, err)
	}

	line := func(sku, total, tax string, qty int) map[string]any {
		return map[string]any{"name": "Kábel CYKY", "sku": sku, "quantity": qty, "total": total, "total_tax": tax}
	}
	shop.orders = []map[string]any{
		wooOrder(101, "processing", "EUR", []map[string]any{line("CYKY3", "20.00", "4.60", 2)}),
		wooOrder(102, "on-hold", "EUR", []map[string]any{line("", "10.00", "2.30", 1)}),
		wooOrder(103, "cancelled", "EUR", []map[string]any{line("", "10.00", "2.30", 1)}),
		wooOrder(104, "completed", "CZK", []map[string]any{line("", "10.00", "2.30", 1)}),
		wooOrder(105, "completed", "EUR", []map[string]any{line("", "10.00", "1.00", 1)}), // 10 % neplatí
	}
	rep, err := a.SyncEshopOrders(ctx, c.ID, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Created != 1 || rep.Waiting != 1 || rep.Skipped != 2 || rep.Failed != 1 {
		t.Fatalf("výsledok %+v", rep)
	}
	inv, err := a.Store.GetInvoice(ctx, c.ID, rep.Orders[0].InvoiceID)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != invoice.StatusDraft || inv.Customer.Name != "Novák s.r.o." || inv.Customer.ICO != "36000001" ||
		inv.Attention == nil || inv.Attention.Name != "Ján Novák" || inv.Delivery == nil || inv.Delivery.City != "Martin" ||
		inv.PaymentMethod != invoice.PayTransfer || inv.DeliveryDate.String() != "2026-10-20" || inv.DueDate.String() != "2026-10-29" {
		t.Errorf("faktúra %+v", inv)
	}
	if len(inv.Lines) != 2 || inv.Lines[0].ItemID != cable.ID || inv.Lines[0].Unit != "m" || !inv.Lines[0].Quantity.Equal(m.FromInt(2)) ||
		!inv.Lines[0].UnitPrice.Equal(m.FromInt(10)) || inv.Lines[0].Rate != vat.Rate23 || inv.Lines[1].Description != "Kuriér" {
		t.Errorf("riadky %+v", inv.Lines)
	}
	if inv.Summary.Total.String() != "30.75" {
		t.Errorf("suma %s", inv.Summary.Total)
	}
	if !strings.Contains(rep.Orders[4].Message, "sadzbe DPH") {
		t.Errorf("zlá sadzba: %+v", rep.Orders[4])
	}

	// ďalšie načítanie: prenesené sa nezopakujú, uhradená čakajúca pribudne
	shop.orders[1] = wooOrder(102, "processing", "EUR", []map[string]any{line("", "10.00", "2.30", 1)})
	rep, err = a.SyncEshopOrders(ctx, c.ID, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Created != 1 || rep.Waiting != 0 || rep.Skipped != 0 || rep.Failed != 1 || rep.Orders[0].Order != "102" {
		t.Fatalf("druhé načítanie %+v", rep)
	}
	list, _ := a.Eshops(ctx, c.ID)
	if list.Conns[0].Imported != 4 || list.Conns[0].LastResult == "" {
		t.Errorf("stav %+v", list.Conns[0])
	}

	// zásoby: CYKY3 obchod pozná, druhú položku nie
	relay, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "REL24", Name: "Relé", Unit: "ks", Price: m.MustParse("12"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.PushEshopStock(ctx, c.ID, conn.ID); !fieldErr(err, "stock") {
		t.Errorf("prázdny sklad: %v", err)
	}
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}},
		Items: []StockItem{{ItemID: cable.ID, Kind: stock.Goods}, {ItemID: relay.ID, Kind: stock.Goods}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-01"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: cable.ID, Quantity: m.MustParse("12.5"), UnitCost: m.MustParse("5")}}}); err != nil {
		t.Fatal(err)
	}
	st, err := a.PushEshopStock(ctx, c.ID, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Updated != 1 || len(st.Missing) != 1 || st.Missing[0] != "REL24" || shop.stock["CYKY3"] != 12 {
		t.Errorf("zásoby %+v, v obchode %v", st, shop.stock)
	}
}

func TestEshopServerBlocksPrivate(t *testing.T) {
	a, c := newApp(t, edition.Server)
	srv := httptest.NewServer(&fakeWoo{})
	defer srv.Close()
	set, err := a.SaveEshops(ctx, c.ID, []EshopConn{{Platform: eshop.WooCommerce, URL: srv.URL, Key: "ck_test", Secret: "cs_test"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.TestEshop(ctx, c.ID, set.Conns[0].ID); err == nil || !strings.Contains(err.Error(), "vnútornej sieti") {
		t.Errorf("server nesmie volať vnútornú sieť: %v", err)
	}
}

func TestShopRate(t *testing.T) {
	d := invoice.MustDate("2026-10-01")
	tests := []struct {
		net, tax string
		want     vat.Rate
		ok       bool
	}{
		{"20.00", "4.60", vat.Rate23, true},
		{"0.33", "0.08", vat.Rate23, true}, // zaokrúhlenie dane v riadku
		{"10.00", "1.90", vat.Rate19, true},
		{"10.00", "0.50", vat.Rate5, true},
		{"10.00", "0", vat.Rate0, true},
		{"10.00", "1.00", vat.Rate0, false},
		{"0", "1.00", vat.Rate0, false},
	}
	for _, tt := range tests {
		r, ok := shopRate(m.MustParse(tt.net), m.MustParse(tt.tax), d)
		if r != tt.want || ok != tt.ok {
			t.Errorf("shopRate(%s, %s) = %d %v", tt.net, tt.tax, r, ok)
		}
	}
}

func TestEshopNoStockPlatform(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	set, err := a.SaveEshops(ctx, c.ID, []EshopConn{{Platform: eshop.Drupal, URL: "https://obchod.sk", Key: "fiskus", Secret: "heslo"}})
	if err != nil {
		t.Fatal(err)
	}
	var se *EshopError
	if _, err := a.PushEshopStock(ctx, c.ID, set.Conns[0].ID); !errors.As(err, &se) || !errors.Is(err, eshop.ErrNoStock) {
		t.Errorf("Drupal zásoby: %v", err)
	}
	if _, err := a.SaveEshops(ctx, c.ID, []EshopConn{{Platform: eshop.Drupal, URL: "https://obchod.sk", Secret: "heslo"}}); !fieldErr(err, "conns[0].key") {
		t.Errorf("Drupal bez používateľa: %v", err)
	}
}
