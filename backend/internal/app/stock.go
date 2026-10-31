package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Sklad: skladové položky sú položky cenníka označené ako zásoba (tovar,
// materiál, výrobky). Príjemky a inventúrne rozdiely zadáva používateľ,
// výdajku k vystavenej faktúre (a príjemku vrátenia k dobropisu) vytvorí
// Fiškus sám z riadkov, ktoré vznikli z cenníka. Ocenenie je vážený
// aritmetický priemer.

// Warehouse je sklad firmy.
type Warehouse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// StockItem je položka cenníka vedená na sklade.
type StockItem struct {
	ItemID int64          `json:"itemId"`
	Kind   stock.ItemKind `json:"kind"`
	Min    money.Dec      `json:"min"` // minimálna zásoba (upozornenie)
}

// StockConfig sú sklady a skladové položky firmy (nastavenie stock.<cid>).
type StockConfig struct {
	Warehouses []Warehouse `json:"warehouses"`
	Items      []StockItem `json:"items"`
	// Method je spôsob ocenenia úbytku (vážený priemer alebo FIFO).
	Method stock.Method `json:"method"`
	// FeedHash je odtlačok tajného odkazu na zásoby pre e-shop (tajné,
	// cez API sa nevracia); FeedCreated je čas vytvorenia.
	FeedHash    string `json:"feedHash,omitempty"`
	FeedCreated int64  `json:"feedCreated,omitempty"`
	// Orders: e-shop smie cez odkaz posielať objednávky (výdajky).
	Orders bool `json:"orders"`
}

func stockKey(companyID int64) string { return fmt.Sprintf("stock.%d", companyID) }

var stockMu sync.Mutex

func (a *App) loadStock(ctx context.Context, companyID int64) (StockConfig, error) {
	var c StockConfig
	raw, err := a.Store.GetSetting(ctx, stockKey(companyID))
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return c, err
	default:
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return c, err
		}
	}
	if len(c.Warehouses) == 0 {
		c.Warehouses = []Warehouse{{Code: "HL", Name: "Hlavný sklad"}}
	}
	if c.Items == nil {
		c.Items = []StockItem{}
	}
	if !c.Method.Valid() {
		c.Method = stock.Average
	}
	return c, nil
}

func (a *App) saveStockConfig(ctx context.Context, companyID int64, c StockConfig) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, stockKey(companyID), string(b))
}

func (c StockConfig) item(id int64) (StockItem, bool) {
	for _, it := range c.Items {
		if it.ItemID == id {
			return it, true
		}
	}
	return StockItem{}, false
}

func (c StockConfig) warehouse(code string) bool {
	for _, w := range c.Warehouses {
		if w.Code == code {
			return true
		}
	}
	return false
}

// StockCard je skladová karta na zobrazenie.
type StockCard struct {
	stock.Key
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	Unit     string         `json:"unit"`
	Kind     stock.ItemKind `json:"kind"`
	Quantity money.Dec      `json:"quantity"`
	Value    money.Dec      `json:"value"`
	AvgCost  money.Dec      `json:"avgCost"`
	Min      money.Dec      `json:"min"`
	Low      bool           `json:"low"` // pod minimálnou zásobou
	Price    money.Dec      `json:"price"`
	Archived bool           `json:"archived,omitempty"`
}

// StockView je prehľad skladu.
type StockView struct {
	Warehouses  []Warehouse     `json:"warehouses"`
	Items       []StockItem     `json:"items"`
	Cards       []StockCard     `json:"cards"`
	Docs        []stock.Doc     `json:"docs"`
	Problems    []stock.Problem `json:"problems"`
	Value       money.Dec       `json:"value"` // hodnota zásob spolu
	Kinds       any             `json:"kinds"`
	ItemKinds   any             `json:"itemKinds"`
	Catalog     []catalog.Item  `json:"catalog"`
	Method      stock.Method    `json:"method"`
	Methods     any             `json:"methods"`
	FeedActive  bool            `json:"feedActive"`
	Orders      bool            `json:"orders"`
	FeedCreated int64           `json:"feedCreated,omitempty"`
}

// Stock vráti stav skladu, doklady a problémy ocenenia.
func (a *App) Stock(ctx context.Context, companyID int64) (StockView, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesRead); err != nil {
		return StockView{}, err
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return StockView{}, err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return StockView{}, err
	}
	items, err := a.Store.ListItems(ctx, companyID)
	if err != nil {
		return StockView{}, err
	}
	byID := map[int64]catalog.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	cards, costed, probs := stock.Valuate(docs, cfg.Method)
	v := StockView{Warehouses: cfg.Warehouses, Items: cfg.Items, Docs: costed, Problems: probs, Kinds: stock.Kinds,
		ItemKinds: stock.ItemKinds, Catalog: items, Cards: []StockCard{}, Method: cfg.Method, Methods: stock.Methods,
		FeedActive: cfg.FeedHash != "", FeedCreated: cfg.FeedCreated, Orders: cfg.Orders}
	if v.Problems == nil {
		v.Problems = []stock.Problem{}
	}
	seen := map[stock.Key]bool{}
	add := func(k stock.Key, c *stock.Card) {
		seen[k] = true
		si, _ := cfg.item(k.ItemID)
		it := byID[k.ItemID]
		sc := StockCard{Key: k, Code: it.Code, Name: it.Name, Unit: it.Unit, Kind: si.Kind, Min: si.Min, Price: it.Price, Archived: it.Archived}
		if c != nil {
			sc.Quantity, sc.Value, sc.AvgCost = c.Quantity, c.Value, c.AvgCost
		}
		sc.Low = si.Min.Sign() > 0 && sc.Quantity.Cmp(si.Min) < 0
		v.Value = v.Value.Add(sc.Value)
		v.Cards = append(v.Cards, sc)
	}
	for _, w := range cfg.Warehouses {
		for _, si := range cfg.Items {
			k := stock.Key{Warehouse: w.Code, ItemID: si.ItemID}
			if c := cards[k]; c != nil || w.Code == cfg.Warehouses[0].Code {
				add(k, c)
			}
		}
	}
	for k, c := range cards { // pohyby na položkách, ktoré už nie sú skladové
		if !seen[k] {
			add(k, c)
		}
	}
	sortCards(v.Cards)
	return v, nil
}

func sortCards(cs []StockCard) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0; j-- {
			a, b := cs[j-1], cs[j]
			ka, kb := store.SearchKey(a.Name), store.SearchKey(b.Name)
			if ka < kb || (ka == kb && a.Warehouse <= b.Warehouse) {
				break
			}
			cs[j-1], cs[j] = b, a
		}
	}
}

// StockCardMoves vráti pohyby na skladovej karte.
func (a *App) StockCardMoves(ctx context.Context, companyID int64, warehouse string, itemID int64) ([]stock.Move, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesRead); err != nil {
		return nil, err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return nil, err
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return nil, err
	}
	cards, _, _ := stock.Valuate(docs, cfg.Method)
	if c := cards[stock.Key{Warehouse: warehouse, ItemID: itemID}]; c != nil {
		return c.Moves, nil
	}
	return []stock.Move{}, nil
}

// SaveStockConfig uloží sklady a skladové položky.
func (a *App) SaveStockConfig(ctx context.Context, companyID int64, c StockConfig) (StockConfig, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return c, err
	}
	stockMu.Lock()
	defer stockMu.Unlock()
	old, err := a.loadStock(ctx, companyID)
	if err != nil {
		return c, err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return c, err
	}
	used := map[string]bool{}
	usedItem := map[int64]bool{}
	for _, d := range docs {
		used[d.Warehouse] = true
		for _, l := range d.Lines {
			usedItem[l.ItemID] = true
		}
	}
	codes := map[string]bool{}
	for i := range c.Warehouses {
		w := &c.Warehouses[i]
		w.Code, w.Name = strings.ToUpper(strings.TrimSpace(w.Code)), strings.TrimSpace(w.Name)
		if w.Code == "" || len(w.Code) > 10 || strings.ContainsAny(w.Code, " /") {
			return c, mfaFieldErr(fmt.Sprintf("warehouses[%d].code", i), "Kód skladu: 1 až 10 znakov bez medzier")
		}
		if w.Name == "" {
			return c, mfaFieldErr(fmt.Sprintf("warehouses[%d].name", i), "Zadajte názov skladu")
		}
		if codes[w.Code] {
			return c, mfaFieldErr(fmt.Sprintf("warehouses[%d].code", i), "Sklad "+w.Code+" je dvakrát")
		}
		codes[w.Code] = true
	}
	if len(c.Warehouses) == 0 {
		return c, mfaFieldErr("warehouses", "Firma musí mať aspoň jeden sklad")
	}
	for _, w := range old.Warehouses {
		if !codes[w.Code] && used[w.Code] {
			return c, mfaFieldErr("warehouses", "Sklad "+w.Code+" má doklady — nedá sa zrušiť")
		}
	}
	seen := map[int64]bool{}
	for i := range c.Items {
		it := &c.Items[i]
		if !it.Kind.Valid() {
			return c, mfaFieldErr(fmt.Sprintf("items[%d].kind", i), "Vyberte druh zásoby")
		}
		if it.Min.Sign() < 0 {
			return c, mfaFieldErr(fmt.Sprintf("items[%d].min", i), "Minimálna zásoba nemôže byť záporná")
		}
		if _, err := a.Store.GetItem(ctx, companyID, it.ItemID); err != nil {
			return c, mfaFieldErr(fmt.Sprintf("items[%d].itemId", i), "Položka nie je v cenníku")
		}
		if seen[it.ItemID] {
			return c, mfaFieldErr(fmt.Sprintf("items[%d].itemId", i), "Položka je na sklade dvakrát")
		}
		seen[it.ItemID] = true
	}
	for _, it := range old.Items {
		if !seen[it.ItemID] && usedItem[it.ItemID] {
			return c, mfaFieldErr("items", "Položka s pohybmi na sklade sa nedá vyradiť zo skladu")
		}
	}
	if c.Items == nil {
		c.Items = []StockItem{}
	}
	if c.Method == "" {
		c.Method = old.Method
	}
	if !c.Method.Valid() {
		return c, mfaFieldErr("method", "Vyberte spôsob ocenenia")
	}
	c.FeedHash, c.FeedCreated = old.FeedHash, old.FeedCreated // odkaz pre e-shop sa mení zvlášť
	if err := a.saveStockConfig(ctx, companyID, c); err != nil {
		return c, err
	}
	oldView, newView := old, c
	oldView.FeedHash, newView.FeedHash = "", ""
	a.record(ctx, companyID, audit.CatSettings, "update", "stock", 0,
		fmt.Sprintf("Sklad: %d skladov, %d skladových položiek, ocenenie %s", len(c.Warehouses), len(c.Items), c.Method), oldView, newView)
	c.FeedHash = ""
	return c, nil
}

// checkStockDoc overí doklad a doplní názvy a jednotky z cenníka.
func (a *App) checkStockDoc(ctx context.Context, companyID int64, cfg StockConfig, d *stock.Doc) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if !cfg.warehouse(d.Warehouse) {
		return mfaFieldErr("warehouse", "Sklad "+d.Warehouse+" neexistuje")
	}
	if d.Kind == stock.Transfer && !cfg.warehouse(d.To) {
		return mfaFieldErr("to", "Sklad "+d.To+" neexistuje")
	}
	if !d.Auto {
		switch d.LinkKind {
		case "":
			d.LinkID, d.LinkNumber = 0, ""
		case "purchase":
			p, err := a.Store.GetPurchase(ctx, companyID, d.LinkID)
			if err != nil {
				return mfaFieldErr("linkId", "Prijatá faktúra neexistuje")
			}
			d.LinkNumber = p.Number
			if d.Partner == "" {
				d.Partner = p.Supplier.Name
			}
		default:
			return mfaFieldErr("linkKind", "Skladový doklad sa ručne viaže iba na prijatú faktúru")
		}
	}
	for i := range d.Lines {
		l := &d.Lines[i]
		if _, ok := cfg.item(l.ItemID); !ok {
			return mfaFieldErr(fmt.Sprintf("lines[%d].itemId", i), "Položka nie je vedená na sklade")
		}
		it, err := a.Store.GetItem(ctx, companyID, l.ItemID)
		if err != nil {
			return mfaFieldErr(fmt.Sprintf("lines[%d].itemId", i), "Položka nie je v cenníku")
		}
		l.Name, l.Unit = it.Name, it.Unit
	}
	return nil
}

// SaveStockDoc pridá (ID = 0) alebo upraví skladový doklad zadaný ručne.
func (a *App) SaveStockDoc(ctx context.Context, companyID int64, d stock.Doc) (stock.Doc, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return d, err
	}
	stockMu.Lock()
	defer stockMu.Unlock()
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return d, err
	}
	d.Auto = false
	if d.Kind != stock.Receipt {
		d.Return = false
	}
	if err := a.checkStockDoc(ctx, companyID, cfg, &d); err != nil {
		return d, err
	}
	action := "create"
	var before any
	if d.ID != 0 {
		old, err := a.Store.GetStockDoc(ctx, companyID, d.ID)
		if err != nil {
			return d, err
		}
		if old.Auto {
			return d, mfaFieldErr("kind", autoSource(old)+" — mení sa iba spolu s ním")
		}
		if old.Kind != d.Kind {
			return d, mfaFieldErr("kind", "Druh dokladu sa nemení")
		}
		d.Number, action, before = old.Number, "update", old
	} else {
		docs, err := a.Store.ListStockDocs(ctx, companyID)
		if err != nil {
			return d, err
		}
		d.Number = stock.NextNumber(docs, d.Kind, d.Date.Year())
	}
	if err := a.Store.SaveStockDoc(ctx, companyID, &d); err != nil {
		return d, err
	}
	a.record(ctx, companyID, audit.CatSettings, action, "stock_doc", d.ID, stockDocSummary(d), before, d)
	return d, nil
}

func stockDocSummary(d stock.Doc) string {
	label := ""
	for _, k := range stock.Kinds {
		if k.Code == d.Kind {
			label = k.Label
		}
	}
	return fmt.Sprintf("%s %s, %d položiek", label, d.Number, len(d.Lines))
}

// DeleteStockDoc zmaže ručne zadaný skladový doklad.
func (a *App) DeleteStockDoc(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return err
	}
	stockMu.Lock()
	defer stockMu.Unlock()
	d, err := a.Store.GetStockDoc(ctx, companyID, id)
	if err != nil {
		return err
	}
	if d.Auto {
		return mfaFieldErr("kind", autoSource(d)+" — zmaže sa stornom faktúry alebo zrušením objednávky")
	}
	if err := a.Store.DeleteStockDoc(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatSettings, "delete", "stock_doc", id, "Zmazaný "+stockDocSummary(d), d, nil)
	return nil
}

// syncInvoiceStock zosúladí výdajku (faktúra) alebo príjemku vrátenia
// (dobropis) s vystavenou faktúrou: riadky z cenníka so skladovou
// položkou sa vydajú z prvého skladu. Storno a koncept doklad zmažú.
// Chyba skladu nezastaví faktúru — iba sa zapíše do denníka.
func (a *App) syncInvoiceStock(ctx context.Context, companyID int64, inv invoice.Invoice) {
	if err := a.syncInvoiceStockErr(ctx, companyID, inv); err != nil {
		log.Printf("sklad k faktúre %s: %v", inv.Number, err)
	}
}

func (a *App) syncInvoiceStockErr(ctx context.Context, companyID int64, inv invoice.Invoice) error {
	stockMu.Lock()
	defer stockMu.Unlock()
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return err
	}
	var existing *stock.Doc
	for i := range docs {
		if docs[i].Auto && docs[i].LinkKind == "invoice" && docs[i].LinkID == inv.ID {
			existing = &docs[i]
			break
		}
	}
	var lines []stock.Line
	active := inv.Status == invoice.StatusIssued || inv.Status == invoice.StatusPaid
	if active && (inv.Kind == invoice.KindInvoice || inv.Kind == invoice.KindCreditNote) {
		for _, l := range inv.Lines {
			if l.ItemID == 0 || l.Quantity.IsZero() {
				continue
			}
			if _, ok := cfg.item(l.ItemID); !ok {
				continue
			}
			q := l.Quantity
			if q.Sign() < 0 {
				q = q.Neg() // dobropis má záporné množstvá
			}
			lines = append(lines, stock.Line{ItemID: l.ItemID, Quantity: q})
		}
	}
	if len(lines) == 0 {
		if existing != nil {
			if err := a.Store.DeleteStockDoc(ctx, companyID, existing.ID); err != nil {
				return err
			}
			a.record(ctx, companyID, audit.CatSettings, "delete", "stock_doc", existing.ID, "Zmazaný "+stockDocSummary(*existing)+" (faktúra "+inv.Number+")", nil, nil)
		}
		return nil
	}
	when := inv.DeliveryDate
	if when.IsZero() || inv.Kind == invoice.KindCreditNote {
		when = inv.IssueDate
	}
	d := stock.Doc{Kind: stock.Issue, Date: when, Warehouse: cfg.Warehouses[0].Code, Partner: inv.Customer.Name,
		LinkKind: "invoice", LinkID: inv.ID, LinkNumber: inv.Number, Auto: true, Lines: lines}
	if inv.Kind == invoice.KindCreditNote {
		d.Kind, d.Return, d.Note = stock.Receipt, true, "Vrátenie k dobropisu"
	}
	if existing != nil {
		if existing.Kind == d.Kind {
			d.ID, d.Number, d.Warehouse = existing.ID, existing.Number, existing.Warehouse
		} else if err := a.Store.DeleteStockDoc(ctx, companyID, existing.ID); err != nil {
			return err
		}
	}
	if err := a.checkStockDoc(ctx, companyID, cfg, &d); err != nil {
		return err
	}
	if d.ID == 0 {
		d.Number = stock.NextNumber(docs, d.Kind, d.Date.Year())
	}
	if err := a.Store.SaveStockDoc(ctx, companyID, &d); err != nil {
		return err
	}
	if existing == nil || existing.ID != d.ID {
		a.record(ctx, companyID, audit.CatSettings, "create", "stock_doc", d.ID, stockDocSummary(d)+" z faktúry "+inv.Number, nil, d)
	}
	return nil
}

// stockValues vráti hodnotu zásob k dňu podľa druhu (výkaz o majetku).
func (a *App) stockValues(ctx context.Context, companyID int64, at invoice.Date) (map[stock.ItemKind]money.Dec, error) {
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return nil, err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return nil, err
	}
	var upto []stock.Doc
	for _, d := range docs {
		if !d.Date.After(at) {
			upto = append(upto, d)
		}
	}
	cards, _, _ := stock.Valuate(upto, cfg.Method)
	out := map[stock.ItemKind]money.Dec{}
	for k, c := range cards {
		si, ok := cfg.item(k.ItemID)
		kind := si.Kind
		if !ok {
			kind = stock.Goods
		}
		out[kind] = out[kind].Add(c.Value)
	}
	return out, nil
}

// StockFeedPrefix je začiatok tajného odkazu na zásoby pre e-shop.
const StockFeedPrefix = "fks_"

// ErrBadStockFeed — neplatný alebo zrušený odkaz na zásoby.
var ErrBadStockFeed = errors.New("neplatný alebo zrušený odkaz na zásoby")

// CreateStockFeed vytvorí nový tajný odkaz na zásoby pre e-shop (starý
// prestane platiť); vráti ho iba raz.
func (a *App) CreateStockFeed(ctx context.Context, companyID int64) (string, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return "", err
	}
	stockMu.Lock()
	defer stockMu.Unlock()
	c, err := a.loadStock(ctx, companyID)
	if err != nil {
		return "", err
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := StockFeedPrefix + base64.RawURLEncoding.EncodeToString(b)
	c.FeedHash, c.FeedCreated = auth.TokenHash(token), a.clock().Unix()
	if err := a.saveStockConfig(ctx, companyID, c); err != nil {
		return "", err
	}
	a.record(ctx, companyID, audit.CatSettings, "create", "stock_feed", 0, "Odkaz na zásoby pre e-shop", nil, nil)
	return token, nil
}

// RevokeStockFeed zruší odkaz na zásoby.
func (a *App) RevokeStockFeed(ctx context.Context, companyID int64) error {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return err
	}
	stockMu.Lock()
	defer stockMu.Unlock()
	c, err := a.loadStock(ctx, companyID)
	if err != nil {
		return err
	}
	c.FeedHash, c.FeedCreated = "", 0
	if err := a.saveStockConfig(ctx, companyID, c); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatSettings, "delete", "stock_feed", 0, "Zrušený odkaz na zásoby pre e-shop", nil, nil)
	return nil
}

// FeedItem je položka v zozname zásob pre e-shop.
type FeedItem struct {
	Code       string               `json:"code"`
	Name       string               `json:"name"`
	Unit       string               `json:"unit"`
	Quantity   money.Dec            `json:"quantity"` // spolu vo všetkých skladoch, záporný stav = 0
	Price      money.Dec            `json:"price"`    // predajná cena z cenníka
	PriceGross money.Dec            `json:"priceGross"`
	VATRate    int                  `json:"vatRate"`
	Warehouses map[string]money.Dec `json:"warehouses"`
}

// StockFeed vráti zásoby pre e-shop podľa tajného odkazu (bez prihlásenia).
func (a *App) StockFeed(ctx context.Context, companyID int64, token string) ([]FeedItem, error) {
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if !checkFeed(cfg, token) {
		return nil, ErrBadStockFeed
	}
	return a.feedItems(ctx, companyID, cfg)
}

// feedItems sú skladové položky so stavom zásob (pre e-shop).
func (a *App) feedItems(ctx context.Context, companyID int64, cfg StockConfig) ([]FeedItem, error) {
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return nil, err
	}
	cards, _, _ := stock.Valuate(docs, cfg.Method)
	out := []FeedItem{}
	for _, si := range cfg.Items {
		it, err := a.Store.GetItem(ctx, companyID, si.ItemID)
		if err != nil || it.Archived {
			continue
		}
		pr := it.Prices()
		f := FeedItem{Code: it.Code, Name: it.Name, Unit: it.Unit, Price: pr.Net, PriceGross: pr.Gross, VATRate: int(it.Rate), Warehouses: map[string]money.Dec{}}
		for _, w := range cfg.Warehouses {
			q := money.Zero
			if c := cards[stock.Key{Warehouse: w.Code, ItemID: si.ItemID}]; c != nil && c.Quantity.Sign() > 0 {
				q = c.Quantity
			}
			f.Warehouses[w.Code] = q
			f.Quantity = f.Quantity.Add(q)
		}
		out = append(out, f)
	}
	return out, nil
}

// checkFeed overí tajný odkaz e-shopu.
func checkFeed(cfg StockConfig, token string) bool {
	return strings.HasPrefix(token, StockFeedPrefix) && cfg.FeedHash != "" &&
		subtle.ConstantTimeCompare([]byte(cfg.FeedHash), []byte(auth.TokenHash(token))) == 1
}

// ShopOrderLine je položka objednávky podľa kódu v cenníku.
type ShopOrderLine struct {
	Code     string    `json:"code"`
	Quantity money.Dec `json:"quantity"`
}

// ShopOrder je objednávka z e-shopu.
type ShopOrder struct {
	Order    string          `json:"order"` // číslo objednávky v e-shope
	Date     invoice.Date    `json:"date"`
	Customer string          `json:"customer"`
	Cancel   bool            `json:"cancel"` // zrušená objednávka → výdajka sa zmaže
	Lines    []ShopOrderLine `json:"lines"`
}

// ErrOrdersOff — e-shop nemá povolené posielať objednávky.
var ErrOrdersOff = errors.New("posielanie objednávok z e-shopu nie je zapnuté")

// ShopOrderIssue zapíše objednávku z e-shopu ako výdajku z prvého skladu
// (rovnaké číslo objednávky výdajku prepíše, zrušená ju zmaže). Položky
// mimo skladu (služby, doprava) sa preskočia.
func (a *App) ShopOrderIssue(ctx context.Context, companyID int64, token string, o ShopOrder) (stock.Doc, error) {
	stockMu.Lock()
	defer stockMu.Unlock()
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return stock.Doc{}, err
	}
	if !checkFeed(cfg, token) {
		return stock.Doc{}, ErrBadStockFeed
	}
	if !cfg.Orders {
		return stock.Doc{}, ErrOrdersOff
	}
	o.Order = strings.TrimSpace(o.Order)
	if o.Order == "" || len(o.Order) > 40 {
		return stock.Doc{}, mfaFieldErr("order", "Zadajte číslo objednávky")
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return stock.Doc{}, err
	}
	var existing *stock.Doc
	for i := range docs {
		if docs[i].LinkKind == "order" && docs[i].LinkNumber == o.Order {
			existing = &docs[i]
		}
	}
	ctx = auth.WithActor(ctx, auth.Actor{Name: "E-shop", Channel: auth.ChannelAPI})
	if o.Cancel {
		if existing == nil {
			return stock.Doc{}, nil
		}
		if err := a.Store.DeleteStockDoc(ctx, companyID, existing.ID); err != nil {
			return stock.Doc{}, err
		}
		a.record(ctx, companyID, audit.CatSettings, "delete", "stock_doc", existing.ID, "Zrušená objednávka z e-shopu "+o.Order, nil, nil)
		return *existing, nil
	}
	items, err := a.Store.ListItems(ctx, companyID)
	if err != nil {
		return stock.Doc{}, err
	}
	byCode := map[string]int64{}
	for _, it := range items {
		if it.Code != "" {
			byCode[strings.ToUpper(it.Code)] = it.ID
		}
	}
	if o.Date.IsZero() {
		o.Date = a.Today()
	}
	d := stock.Doc{Kind: stock.Issue, Date: o.Date, Warehouse: cfg.Warehouses[0].Code, Partner: strings.TrimSpace(o.Customer),
		Note: "Objednávka z e-shopu", LinkKind: "order", LinkNumber: o.Order, Auto: true}
	for i, l := range o.Lines {
		id := byCode[strings.ToUpper(strings.TrimSpace(l.Code))]
		if id == 0 {
			return stock.Doc{}, mfaFieldErr(fmt.Sprintf("lines[%d].code", i), "Kód "+l.Code+" nie je v cenníku")
		}
		if _, ok := cfg.item(id); ok {
			d.Lines = append(d.Lines, stock.Line{ItemID: id, Quantity: l.Quantity})
		}
	}
	if len(d.Lines) == 0 {
		return stock.Doc{}, nil // objednávka bez skladových položiek
	}
	if err := a.checkStockDoc(ctx, companyID, cfg, &d); err != nil {
		return stock.Doc{}, err
	}
	if existing != nil {
		d.ID, d.Number = existing.ID, existing.Number
	} else {
		d.Number = stock.NextNumber(docs, d.Kind, d.Date.Year())
	}
	if err := a.Store.SaveStockDoc(ctx, companyID, &d); err != nil {
		return stock.Doc{}, err
	}
	a.record(ctx, companyID, audit.CatSettings, "create", "stock_doc", d.ID, stockDocSummary(d)+" z objednávky e-shopu "+o.Order, nil, d)
	return d, nil
}

// autoSource popíše, odkiaľ automatický doklad vznikol.
func autoSource(d stock.Doc) string {
	if d.LinkKind == "order" {
		return "Doklad vznikol z objednávky e-shopu " + d.LinkNumber
	}
	return "Doklad vznikol z faktúry " + d.LinkNumber
}
