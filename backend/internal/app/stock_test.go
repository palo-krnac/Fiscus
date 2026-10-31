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

func TestStockFIFOTransferFeed(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	it, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "K5", Name: "Kábel", Unit: "m", Price: m.MustParse("2"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	cfg := StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}, {Code: "ZV", Name: "Zvolen"}}, Items: []StockItem{{ItemID: it.ID, Kind: stock.Goods}}, Method: stock.FIFO}
	if _, err := a.SaveStockConfig(ctx, c.ID, cfg); err != nil {
		t.Fatal(err)
	}
	p, _, err := a.SavePurchase(ctx, c.ID, validPurchase())
	if err != nil {
		t.Fatal(err)
	}
	ln := func(q, cost string) []stock.Line {
		return []stock.Line{{ItemID: it.ID, Quantity: m.MustParse(q), UnitCost: m.MustParse(cost)}}
	}
	r1, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-01"), Warehouse: "HL", LinkKind: "purchase", LinkID: p.ID, Lines: ln("100", "1")})
	if err != nil || r1.LinkNumber != p.Number || r1.Partner != p.Supplier.Name {
		t.Fatalf("príjemka k prijatej faktúre: %+v %v", r1, err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-02"), Warehouse: "HL", LinkKind: "purchase", LinkID: 999, Lines: ln("1", "1")}); !fieldErr(err, "linkId") {
		t.Errorf("neexistujúca faktúra: %v", err)
	}
	_, _ = a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-03"), Warehouse: "HL", Lines: ln("50", "1.30")})
	tr, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Transfer, Date: invoice.MustDate("2026-10-05"), Warehouse: "HL", To: "ZV", Lines: ln("120", "0")})
	if err != nil || tr.Number != "PS2026001" {
		t.Fatalf("presun: %+v %v", tr, err)
	}
	v, _ := a.Stock(ctx, c.ID)
	vals := map[string]string{}
	for _, cd := range v.Cards {
		vals[cd.Warehouse] = cd.Quantity.String() + "/" + cd.Value.String()
	}
	// FIFO: do Zvolena 100 × 1,00 + 20 × 1,30 = 126; v HL ostane 30 × 1,30
	if vals["HL"] != "30.00/39.00" || vals["ZV"] != "120.00/126.00" || v.Method != stock.FIFO {
		t.Fatalf("FIFO a presun: %v", vals)
	}
	// prepnutie na priemer prepočíta celú históriu
	cfg.Method = stock.Average
	if _, err := a.SaveStockConfig(ctx, c.ID, cfg); err != nil {
		t.Fatal(err)
	}
	v, _ = a.Stock(ctx, c.ID)
	for _, cd := range v.Cards {
		if cd.Warehouse == "HL" && cd.Value.String() != "33.00" { // 30 × 165/150
			t.Errorf("priemer: %+v", cd)
		}
	}
	// presun sa neúčtuje
	book, _ := a.Ledger(ctx, c.ID, 2026)
	for _, ps := range book.Postings {
		if ps.Number == "PS2026001" {
			t.Errorf("presun v denníku: %+v", ps)
		}
	}
	// odkaz pre e-shop
	if _, err := a.StockFeed(ctx, c.ID, "fks_nic"); err != ErrBadStockFeed {
		t.Errorf("bez odkazu: %v", err)
	}
	token, err := a.CreateStockFeed(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	feed, err := a.StockFeed(ctx, c.ID, token)
	if err != nil || len(feed) != 1 || feed[0].Code != "K5" || feed[0].Quantity.String() != "150.00" || feed[0].Warehouses["ZV"].String() != "120.00" || feed[0].PriceGross.String() != "2.46" {
		t.Fatalf("zásoby pre e-shop: %+v %v", feed, err)
	}
	if v, _ = a.Stock(ctx, c.ID); !v.FeedActive {
		t.Error("odkaz je aktívny")
	}
	saved, _ := a.SaveStockConfig(ctx, c.ID, cfg)
	if saved.FeedHash != "" {
		t.Error("odtlačok odkazu sa nevracia")
	}
	if _, err := a.StockFeed(ctx, c.ID, token); err != nil {
		t.Errorf("uloženie nastavenia nezruší odkaz: %v", err)
	}
	// objednávka z e-shopu: iba so zapnutým povolením, podľa kódu, opakovane bez duplicity
	order := ShopOrder{Order: "E-1001", Customer: "Ján Kupec", Lines: []ShopOrderLine{{Code: "k5", Quantity: m.MustParse("10")}, {Code: "K5", Quantity: m.MustParse("2")}}}
	if _, err := a.ShopOrderIssue(ctx, c.ID, token, order); err != ErrOrdersOff {
		t.Errorf("bez povolenia: %v", err)
	}
	cfg.Orders = true
	if _, err := a.SaveStockConfig(ctx, c.ID, cfg); err != nil {
		t.Fatal(err)
	}
	d1, err := a.ShopOrderIssue(ctx, c.ID, token, order)
	if err != nil || d1.Kind != stock.Issue || len(d1.Lines) != 2 || d1.LinkNumber != "E-1001" {
		t.Fatalf("objednávka: %+v %v", d1, err)
	}
	order.Lines = order.Lines[:1]
	d2, err := a.ShopOrderIssue(ctx, c.ID, token, order)
	if err != nil || d2.ID != d1.ID || len(d2.Lines) != 1 {
		t.Fatalf("opakovaná objednávka: %+v %v", d2, err)
	}
	if _, err := a.ShopOrderIssue(ctx, c.ID, token, ShopOrder{Order: "E-2", Lines: []ShopOrderLine{{Code: "XX", Quantity: m.MustParse("1")}}}); !fieldErr(err, "lines[0].code") {
		t.Errorf("neznámy kód: %v", err)
	}
	if err := a.DeleteStockDoc(ctx, c.ID, d1.ID); !fieldErr(err, "kind") {
		t.Errorf("výdajka z objednávky sa ručne nemaže: %v", err)
	}
	if _, err := a.ShopOrderIssue(ctx, c.ID, token, ShopOrder{Order: "E-1001", Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if v, _ = a.Stock(ctx, c.ID); len(v.Docs) != 3 {
		t.Errorf("po zrušení objednávky: %d dokladov", len(v.Docs))
	}
	if err := a.RevokeStockFeed(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StockFeed(ctx, c.ID, token); err != ErrBadStockFeed {
		t.Errorf("zrušený odkaz: %v", err)
	}
}
