package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/stock"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/table"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// ItemRow je položka cenníka s cenami bez DPH a s DPH pre editor faktúry.
type ItemRow struct {
	catalog.Item
	catalog.Prices
	// Do zoznamu: stav na sklade (nil = nevedie sa alebo bez práva) a hlavný obrázok.
	Stock   *money.Dec `json:"stock,omitempty"`
	ImageID int64      `json:"imageId,omitempty"`
}

func itemRow(it catalog.Item) ItemRow { return ItemRow{Item: it, Prices: it.Prices()} }

// Items vráti cenník firmy.
func (a *App) Items(ctx context.Context, companyID int64) ([]ItemRow, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	list, err := a.Store.ListItems(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]ItemRow, len(list))
	for i, it := range list {
		out[i] = itemRow(it)
		if imgs, err := a.Store.ListItemImages(ctx, companyID, it.ID); err == nil && len(imgs) > 0 {
			out[i].ImageID = imgs[0].ID
		}
	}
	// stav na sklade spolu za všetky sklady (iba kto vidí sklad)
	if a.Require(ctx, companyID, auth.PermPurchasesRead) == nil {
		cfg, err := a.loadStock(ctx, companyID)
		if err != nil {
			return nil, err
		}
		if len(cfg.Items) > 0 {
			docs, err := a.Store.ListStockDocs(ctx, companyID)
			if err != nil {
				return nil, err
			}
			cards, _, _ := stock.Valuate(docs, cfg.Method)
			qty := map[int64]money.Dec{}
			for k, c := range cards {
				qty[k.ItemID] = qty[k.ItemID].Add(c.Quantity)
			}
			for i := range out {
				if _, ok := cfg.item(out[i].ID); ok {
					q := qty[out[i].ID]
					out[i].Stock = &q
				}
			}
		}
	}
	return out, nil
}

// SaveItem overí a uloží položku cenníka.
func (a *App) SaveItem(ctx context.Context, companyID int64, it catalog.Item) (ItemRow, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return ItemRow{}, err
	}
	it.Normalize()
	if err := it.Validate(); err != nil {
		return itemRow(it), err
	}
	var before any
	if it.ID != 0 {
		old, err := a.Store.GetItem(ctx, companyID, it.ID)
		if err != nil {
			return itemRow(it), err
		}
		before = old
	}
	if err := a.Store.SaveItem(ctx, companyID, &it); errors.Is(err, store.ErrDuplicate) {
		return itemRow(it), &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "code",
			Message: "Položka s kódom " + it.Code + " už v cenníku je"}}}
	} else if err != nil {
		return itemRow(it), err
	}
	if before == nil {
		a.record(ctx, companyID, audit.CatCatalog, "create", "item", it.ID, "Nová položka cenníka "+it.Name, nil, it)
	} else {
		a.record(ctx, companyID, audit.CatCatalog, "update", "item", it.ID, "Úprava položky cenníka "+it.Name, before, it)
	}
	return itemRow(it), nil
}

// DeleteItem zmaže položku cenníka. Faktúry s ňou sa nemenia (položky sú v nich skopírované).
func (a *App) DeleteItem(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	it, err := a.Store.GetItem(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteItem(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatCatalog, "delete", "item", id, "Zmazaná položka cenníka "+it.Name, it, nil)
	return nil
}

// ItemImport je náhľad alebo výsledok importu cenníka.
type ItemImport struct {
	Rows    []catalog.ImportRow `json:"rows"`
	Created int                 `json:"created"`
	Updated int                 `json:"updated"`
	Skipped int                 `json:"skipped"`
	Done    bool                `json:"done"` // false = iba náhľad
}

// MaxImportBytes je najväčší súbor importu.
const MaxImportBytes = 5 << 20

// ImportItems načíta cenník zo súboru CSV alebo XLSX. Položka s rovnakým
// kódom (bez kódu s rovnakým názvom) sa prepíše, ostatné pribudnú; riadky
// s chybou sa preskočia. S apply=false vráti iba náhľad.
func (a *App) ImportItems(ctx context.Context, companyID int64, data []byte, apply bool) (ItemImport, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return ItemImport{}, err
	}
	bad := func(msg string) error {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "file", Message: msg}}}
	}
	rows, err := table.Read(data)
	if err != nil {
		return ItemImport{}, bad(err.Error())
	}
	parsed, err := catalog.ParseItems(rows, vat.Rate23)
	if err != nil {
		return ItemImport{}, bad(err.Error())
	}
	existing, err := a.Store.ListItems(ctx, companyID)
	if err != nil {
		return ItemImport{}, err
	}
	catalog.Match(parsed, existing)
	res := ItemImport{Rows: parsed, Done: apply}
	for _, r := range parsed {
		switch r.Action {
		case "create":
			res.Created++
		case "update":
			res.Updated++
		default:
			res.Skipped++
		}
	}
	if !apply {
		return res, nil
	}
	for i := range res.Rows {
		r := &res.Rows[i]
		if r.Action == "skip" {
			continue
		}
		if err := a.Store.SaveItem(ctx, companyID, &r.Item); err != nil {
			return res, err
		}
	}
	a.record(ctx, companyID, audit.CatCatalog, "import", "item", 0,
		fmt.Sprintf("Import cenníka: %d nových, %d zmenených, %d preskočených", res.Created, res.Updated, res.Skipped), nil, nil)
	return res, nil
}
