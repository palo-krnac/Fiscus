package app

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Skladová karta produktu: údaje z cenníka, obrázky, parametre, stav po
// skladoch a všetky pohyby (príjemky, výdajky, presuny, inventúra).

// Limity obrázkov produktu.
const (
	MaxItemImages    = 10
	MaxItemImageSize = 2 << 20
)

// WarehouseStock je stav produktu v jednom sklade.
type WarehouseStock struct {
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	Quantity money.Dec `json:"quantity"`
	Value    money.Dec `json:"value"`
	AvgCost  money.Dec `json:"avgCost"`
}

// CardMove je pohyb na karte aj so skladom.
type CardMove struct {
	stock.Move
	Warehouse string `json:"warehouse"`
}

// ItemCard je skladová karta produktu.
type ItemCard struct {
	Item       ItemRow           `json:"item"`
	Images     []store.ItemImage `json:"images"`
	Tracked    bool              `json:"tracked"` // vedie sa na sklade
	Kind       stock.ItemKind    `json:"kind,omitempty"`
	Min        money.Dec         `json:"min"`
	Below      bool              `json:"below"` // pod minimálnou zásobou
	Quantity   money.Dec         `json:"quantity"`
	Value      money.Dec         `json:"value"`
	Warehouses []WarehouseStock  `json:"warehouses"`
	Moves      []CardMove        `json:"moves"`
	Method     stock.Method      `json:"method"`
}

// ItemCard vráti skladovú kartu produktu.
func (a *App) ItemCard(ctx context.Context, companyID, id int64) (ItemCard, error) {
	var out ItemCard
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return out, err
	}
	it, err := a.Store.GetItem(ctx, companyID, id)
	if err != nil {
		return out, err
	}
	out.Item = itemRow(it)
	if out.Images, err = a.Store.ListItemImages(ctx, companyID, id); err != nil {
		return out, err
	}
	out.Warehouses, out.Moves = []WarehouseStock{}, []CardMove{}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return out, err
	}
	out.Method = cfg.Method
	si, tracked := cfg.item(id)
	out.Tracked = tracked
	if !tracked || a.Require(ctx, companyID, auth.PermPurchasesRead) != nil {
		return out, nil
	}
	out.Kind, out.Min = si.Kind, si.Min
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return out, err
	}
	cards, _, _ := stock.Valuate(docs, cfg.Method)
	for _, w := range cfg.Warehouses {
		ws := WarehouseStock{Code: w.Code, Name: w.Name}
		if c := cards[stock.Key{Warehouse: w.Code, ItemID: id}]; c != nil {
			ws.Quantity, ws.Value, ws.AvgCost = c.Quantity, c.Value, c.AvgCost
			for _, m := range c.Moves {
				out.Moves = append(out.Moves, CardMove{Move: m, Warehouse: w.Code})
			}
		}
		out.Quantity, out.Value = out.Quantity.Add(ws.Quantity), out.Value.Add(ws.Value)
		out.Warehouses = append(out.Warehouses, ws)
	}
	// pohyby všetkých skladov podľa dátumu (najnovšie hore)
	sortMoves(out.Moves)
	out.Below = !out.Min.IsZero() && out.Quantity.Cmp(out.Min) < 0
	return out, nil
}

func sortMoves(m []CardMove) {
	for i := 1; i < len(m); i++ {
		for j := i; j > 0 && (m[j].Date.After(m[j-1].Date) || (m[j].Date == m[j-1].Date && m[j].DocID > m[j-1].DocID)); j-- {
			m[j], m[j-1] = m[j-1], m[j]
		}
	}
}

var imageTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// AddItemImage pridá obrázok produktu (JPEG, PNG alebo WebP do 2 MB).
func (a *App) AddItemImage(ctx context.Context, companyID, itemID int64, name string, data []byte) (store.ItemImage, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return store.ItemImage{}, err
	}
	it, err := a.Store.GetItem(ctx, companyID, itemID)
	if err != nil {
		return store.ItemImage{}, err
	}
	if len(data) == 0 {
		return store.ItemImage{}, mfaFieldErr("image", "Súbor je prázdny")
	}
	if len(data) > MaxItemImageSize {
		return store.ItemImage{}, mfaFieldErr("image", "Obrázok je príliš veľký (najviac 2 MB)")
	}
	ct := http.DetectContentType(data) // podľa obsahu, nie podľa mena súboru
	if !imageTypes[ct] {
		return store.ItemImage{}, mfaFieldErr("image", "Obrázok musí byť JPEG, PNG alebo WebP")
	}
	list, err := a.Store.ListItemImages(ctx, companyID, itemID)
	if err != nil {
		return store.ItemImage{}, err
	}
	if len(list) >= MaxItemImages {
		return store.ItemImage{}, mfaFieldErr("image", "Produkt môže mať najviac 10 obrázkov")
	}
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 100 {
		name = string(r[:100])
	}
	if name == "" {
		name = "obrazok"
	}
	img := store.ItemImage{CompanyID: companyID, ItemID: itemID, Name: name, ContentType: ct, Data: data, Created: a.now()}
	if err := a.Store.AddItemImage(ctx, &img); err != nil {
		return store.ItemImage{}, err
	}
	a.record(ctx, companyID, audit.CatCatalog, "update", "item_image", itemID, "Obrázok produktu "+it.Name+": "+name, nil, nil)
	img.Data = nil
	return img, nil
}

// ItemImage vráti obrázok s dátami.
func (a *App) ItemImage(ctx context.Context, companyID, itemID, id int64) (store.ItemImage, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return store.ItemImage{}, err
	}
	return a.itemImage(ctx, companyID, itemID, id)
}

// itemImage vráti obrázok, iba ak patrí danej položke.
func (a *App) itemImage(ctx context.Context, companyID, itemID, id int64) (store.ItemImage, error) {
	im, err := a.Store.GetItemImage(ctx, companyID, id)
	if err == nil && im.ItemID != itemID {
		return store.ItemImage{}, store.ErrNotFound
	}
	return im, err
}

// DeleteItemImage zmaže obrázok produktu.
func (a *App) DeleteItemImage(ctx context.Context, companyID, itemID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	im, err := a.itemImage(ctx, companyID, itemID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteItemImage(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatCatalog, "delete", "item_image", im.ItemID, "Zmazaný obrázok produktu: "+im.Name, nil, nil)
	return nil
}

// OrderItemImages nastaví poradie obrázkov (prvý je hlavný).
func (a *App) OrderItemImages(ctx context.Context, companyID, itemID int64, ids []int64) ([]store.ItemImage, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return nil, err
	}
	list, err := a.Store.ListItemImages(ctx, companyID, itemID)
	if err != nil {
		return nil, err
	}
	if len(ids) != len(list) {
		return nil, mfaFieldErr("ids", "Poradie musí obsahovať všetky obrázky produktu")
	}
	if err := a.Store.SetItemImageOrder(ctx, companyID, itemID, ids); err != nil {
		return nil, err
	}
	return a.Store.ListItemImages(ctx, companyID, itemID)
}

// ItemStock je nastavenie skladu pre jednu položku (zo skladovej karty).
type ItemStock struct {
	Tracked bool           `json:"tracked"`
	Kind    stock.ItemKind `json:"kind"`
	Min     money.Dec      `json:"min"`
}

// SetItemStock zaradí položku na sklad, zmení jej druh a minimum, alebo ju
// zo skladu vyradí (iba ak nemá pohyby — to stráži SaveStockConfig).
func (a *App) SetItemStock(ctx context.Context, companyID, itemID int64, in ItemStock) (ItemCard, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return ItemCard{}, err
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return ItemCard{}, err
	}
	items := make([]StockItem, 0, len(cfg.Items)+1)
	for _, it := range cfg.Items {
		if it.ItemID != itemID {
			items = append(items, it)
		}
	}
	if in.Tracked {
		if in.Kind == "" {
			in.Kind = stock.Goods
		}
		items = append(items, StockItem{ItemID: itemID, Kind: in.Kind, Min: in.Min})
	}
	cfg.Items = items
	if _, err := a.SaveStockConfig(ctx, companyID, cfg); err != nil {
		var ve *invoice.ValidationError
		if errors.As(err, &ve) {
			// chyby zoznamu items[i].x sa na karte ukážu pri poli x
			for i := range ve.Fields {
				f := &ve.Fields[i]
				if j := strings.LastIndexByte(f.Field, '.'); j >= 0 && strings.HasPrefix(f.Field, "items") {
					f.Field = f.Field[j+1:]
				}
			}
		}
		return ItemCard{}, err
	}
	return a.ItemCard(ctx, companyID, itemID)
}
