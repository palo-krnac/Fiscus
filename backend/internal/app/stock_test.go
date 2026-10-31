package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestStock(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	relay, err := a.SaveItem(ctx, c.ID, catalog.Item{Name: "Relé 24 V", Unit: "ks", Price: m.MustParse("12"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if err != nil {
		t.Fatal(err)
	}
	work, _ := a.SaveItem(ctx, c.ID, catalog.Item{Name: "Montáž", Unit: "hod", Price: m.MustParse("40"), PriceMode: vat.PriceNet, Rate: vat.Rate23})

	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "hl", Name: "Hlavný sklad"}},
		Items: []StockItem{{ItemID: relay.ID, Kind: stock.Goods, Min: m.MustParse("5")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "x"}},
		Items: []StockItem{{ItemID: 999, Kind: stock.Goods}}}); !fieldErr(err, "items[0].itemId") {
		t.Errorf("neznáma položka: %v", err)
	}
	rec := stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-01"), Warehouse: "HL", Partner: "Elektro veľkoobchod",
		Lines: []stock.Line{{ItemID: relay.ID, Quantity: m.MustParse("20"), UnitCost: m.MustParse("7.50")}}}
	r, err := a.SaveStockDoc(ctx, c.ID, rec)
	if err != nil || r.Number != "PR2026001" || r.Lines[0].Name != "Relé 24 V" || r.Lines[0].Total.String() != "150.00" {
		t.Fatalf("príjemka: %+v %v", r, err)
	}
	bad := rec
	bad.Lines = []stock.Line{{ItemID: work.ID, Quantity: m.MustParse("1"), UnitCost: m.MustParse("1")}}
	if _, err := a.SaveStockDoc(ctx, c.ID, bad); !fieldErr(err, "lines[0].itemId") {
		t.Errorf("neskladová položka: %v", err)
	}

	// faktúra s relé z cenníka → výdajka 18 ks, montáž sa nevydáva
	d := validDraft()
	d.Lines = []invoice.Line{
		{Description: "Relé 24 V", Quantity: m.MustParse("18"), Unit: "ks", UnitPrice: m.MustParse("12"), Rate: vat.Rate23, ItemID: relay.ID},
		{Description: "Montáž", Quantity: m.MustParse("2"), Unit: "hod", UnitPrice: m.MustParse("40"), Rate: vat.Rate23, ItemID: work.ID},
	}
	dr, err := a.SaveDraft(ctx, c.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Stock(ctx, c.ID); len(v.Docs) != 1 {
		t.Fatalf("koncept faktúry nevydáva: %+v", v.Docs)
	}
	inv, err := a.Issue(ctx, c.ID, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	v, err := a.Stock(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Docs) != 2 || v.Docs[1].Number != "VY2026001" || !v.Docs[1].Auto || v.Docs[1].LinkNumber != inv.Number || v.Docs[1].Lines[0].Total.String() != "135.00" {
		t.Fatalf("výdajka z faktúry: %+v", v.Docs)
	}
	if len(v.Cards) != 1 || v.Cards[0].Quantity.String() != "2.00" || v.Cards[0].Value.String() != "15.00" || !v.Cards[0].Low {
		t.Fatalf("karta: %+v", v.Cards)
	}
	if err := a.DeleteStockDoc(ctx, c.ID, v.Docs[1].ID); !fieldErr(err, "kind") {
		t.Errorf("výdajka z faktúry sa ručne nemaže: %v", err)
	}

	// účtovanie: príjemka 132/131, výdajka 504/132
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ps := range book.Postings {
		if ps.Doc == "stock" {
			got[ps.Number] = ps.Template + " " + lineStr(ps.Lines)
		}
	}
	if got["PR2026001"] != "SK-PRIJEM-TOVAR 132 MD 150.00, 131 D 150.00" || got["VY2026001"] != "SK-VYDAJ-TOVAR 504 MD 135.00, 132 D 135.00" {
		t.Errorf("účtovanie skladu: %v", got)
	}

	// dobropis vráti 3 ks priemernou cenou, storno faktúry zruší výdajku
	cn, err := a.CreateCreditNote(ctx, c.ID, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	cn.Lines = cn.Lines[:1]
	cn.Lines[0].Quantity = m.MustParse("-3")
	cn.Correction.Reason = "Vrátenie tovaru"
	if cn, err = a.SaveDraft(ctx, c.ID, cn); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Issue(ctx, c.ID, cn.ID); err != nil {
		t.Fatal(err)
	}
	v, _ = a.Stock(ctx, c.ID)
	if len(v.Docs) != 3 || v.Docs[2].Kind != stock.Receipt || !v.Docs[2].Return || v.Docs[2].Lines[0].Total.String() != "22.50" || v.Cards[0].Quantity.String() != "5.00" {
		t.Fatalf("vrátenie: %+v %+v", v.Docs, v.Cards)
	}

	// výkaz o majetku (jednoduché účtovníctvo): tovar zo skladu
	st, err := a.StatementsFor(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if st.Facts.Goods.String() != "37.50" {
		t.Errorf("zásoby vo výkaze: %+v", st.Facts)
	}

	// manko pri inventúre
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Count, Date: invoice.MustDate("2026-10-31"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: relay.ID, Quantity: m.MustParse("-1")}}}); err != nil {
		t.Fatal(err)
	}
	moves, err := a.StockCardMoves(ctx, c.ID, "HL", relay.ID)
	if err != nil || len(moves) != 4 || moves[3].Balance.String() != "4.00" || moves[3].Total.String() != "-7.50" {
		t.Fatalf("pohyby: %+v %v", moves, err)
	}
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}}}); !fieldErr(err, "items") {
		t.Errorf("vyradenie položky s pohybmi: %v", err)
	}
}
