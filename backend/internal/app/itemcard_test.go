package app

import (
	"bytes"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// pngHeader je začiatok súboru PNG (stačí na rozpoznanie typu).
var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func TestItemCard(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	row, err := a.SaveItem(ctx, c.ID, catalog.Item{Code: "REL24", Name: "Relé 24 V", Unit: "ks", Price: m.MustParse("12"), PriceMode: vat.PriceNet, Rate: vat.Rate23,
		EAN: "5901234123457", Brand: "Finder", Params: []catalog.Param{{Name: "Napätie cievky", Value: "24", Unit: "V"}}})
	if err != nil {
		t.Fatal(err)
	}
	id := row.ID
	// bez skladu: karta iba s údajmi
	card, err := a.ItemCard(ctx, c.ID, id)
	if err != nil || card.Tracked || card.Item.Brand != "Finder" || len(card.Item.Params) != 1 || len(card.Warehouses) != 0 {
		t.Fatalf("karta %+v %v", card, err)
	}
	// sklad v dvoch skladoch: príjem, presun
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}, {Code: "AUTO", Name: "Auto"}},
		Items: []StockItem{{ItemID: id, Kind: stock.Goods, Min: m.MustParse("15")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-01"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: id, Quantity: m.MustParse("20"), UnitCost: m.MustParse("7.50")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Transfer, Date: invoice.MustDate("2026-10-05"), Warehouse: "HL", To: "AUTO",
		Lines: []stock.Line{{ItemID: id, Quantity: m.MustParse("8")}}}); err != nil {
		t.Fatal(err)
	}
	card, err = a.ItemCard(ctx, c.ID, id)
	if err != nil || !card.Tracked || card.Kind != stock.Goods || !card.Quantity.Equal(m.FromInt(20)) || !card.Value.Equal(m.FromInt(150)) || card.Below {
		t.Fatalf("sklad %+v %v", card, err)
	}
	if len(card.Warehouses) != 2 || !card.Warehouses[0].Quantity.Equal(m.FromInt(12)) || !card.Warehouses[1].Quantity.Equal(m.FromInt(8)) {
		t.Errorf("sklady %+v", card.Warehouses)
	}
	if len(card.Moves) != 3 || card.Moves[0].Date.String() != "2026-10-05" || card.Moves[2].Kind != stock.Receipt {
		t.Errorf("pohyby %+v", card.Moves)
	}
	// zoznam cenníka: stav a hlavný obrázok
	img1, err := a.AddItemImage(ctx, c.ID, id, "predok.png", pngHeader)
	if err != nil || img1.ContentType != "image/png" || img1.Data != nil {
		t.Fatalf("obrázok %+v %v", img1, err)
	}
	img2, _ := a.AddItemImage(ctx, c.ID, id, "zadok.png", pngHeader)
	rows, _ := a.Items(ctx, c.ID)
	if rows[0].Stock == nil || !rows[0].Stock.Equal(m.FromInt(20)) || rows[0].ImageID != img1.ID {
		t.Errorf("riadok cenníka %+v", rows[0])
	}
	if list, err := a.OrderItemImages(ctx, c.ID, id, []int64{img2.ID, img1.ID}); err != nil || list[0].ID != img2.ID {
		t.Errorf("poradie %+v %v", list, err)
	}
	if _, err := a.OrderItemImages(ctx, c.ID, id, []int64{img2.ID}); !fieldErr(err, "ids") {
		t.Errorf("neúplné poradie: %v", err)
	}
	got, err := a.ItemImage(ctx, c.ID, id, img1.ID)
	if err != nil || !bytes.Equal(got.Data, pngHeader) {
		t.Errorf("dáta %v", err)
	}
	// typ podľa obsahu, veľkosť, počet
	tests := []struct {
		name string
		data []byte
	}{
		{"nie obrázok", []byte("%PDF-1.7 tajné")},
		{"prázdny", nil},
		{"veľký", append(append([]byte{}, pngHeader...), make([]byte, MaxItemImageSize)...)},
	}
	for _, tt := range tests {
		if _, err := a.AddItemImage(ctx, c.ID, id, tt.name, tt.data); !fieldErr(err, "image") {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
	for i := 0; i < MaxItemImages-2; i++ {
		if _, err := a.AddItemImage(ctx, c.ID, id, "x.png", pngHeader); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.AddItemImage(ctx, c.ID, id, "11.png", pngHeader); !fieldErr(err, "image") {
		t.Errorf("11. obrázok: %v", err)
	}
	if err := a.DeleteItemImage(ctx, c.ID, id+1, img1.ID); err == nil {
		t.Error("cudzia položka")
	}
	if err := a.DeleteItemImage(ctx, c.ID, id, img1.ID); err != nil {
		t.Fatal(err)
	}
	// pod minimom
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Issue, Date: invoice.MustDate("2026-10-10"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: id, Quantity: m.MustParse("10")}}}); err != nil {
		t.Fatal(err)
	}
	if card, _ := a.ItemCard(ctx, c.ID, id); !card.Below || len(card.Images) != MaxItemImages-1 {
		t.Errorf("pod minimom %v, obrázkov %d", card.Below, len(card.Images))
	}
}

func TestSetItemStock(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	row, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "K", Name: "Kábel", Unit: "m", Price: m.MustParse("1"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	tests := []struct {
		name    string
		in      ItemStock
		tracked bool
		field   string
	}{
		{"zaradiť ako tovar", ItemStock{Tracked: true, Min: m.MustParse("100")}, true, ""},
		{"zmena na materiál", ItemStock{Tracked: true, Kind: stock.Material, Min: m.MustParse("50")}, true, ""},
		{"záporné minimum", ItemStock{Tracked: true, Kind: stock.Material, Min: m.MustParse("-1")}, true, "min"},
		{"zlý druh", ItemStock{Tracked: true, Kind: "x"}, true, "kind"},
		{"vyradiť bez pohybov", ItemStock{}, false, ""},
	}
	for _, tt := range tests {
		card, err := a.SetItemStock(ctx, c.ID, row.ID, tt.in)
		if tt.field != "" {
			if !fieldErr(err, tt.field) {
				t.Errorf("%s: %v", tt.name, err)
			}
			continue
		}
		if err != nil || card.Tracked != tt.tracked {
			t.Errorf("%s: %+v %v", tt.name, card, err)
		}
	}
	// s pohybom sa vyradiť nedá
	a.SetItemStock(ctx, c.ID, row.ID, ItemStock{Tracked: true})
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-01"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: row.ID, Quantity: m.MustParse("5"), UnitCost: m.MustParse("1")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetItemStock(ctx, c.ID, row.ID, ItemStock{}); !fieldErr(err, "items") {
		t.Errorf("vyradenie s pohybmi: %v", err)
	}
}
