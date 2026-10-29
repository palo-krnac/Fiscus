package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/asset"
	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// AssetItem je položka registra so stavom v zvolenom roku.
type AssetItem struct {
	asset.Asset
	Period   int          `json:"period"`   // doba odpisovania v rokoch
	Year     asset.Year   `json:"thisYear"` // odpis / vyradenie v roku
	Opening  money.Dec    `json:"opening"`  // zostatková cena na začiatku roka
	Residual money.Dec    `json:"residual"` // zostatková cena na konci roka
	Plan     []asset.Year `json:"plan"`
}

// AssetTotals sú súčty registra za rok.
type AssetTotals struct {
	Claimed    money.Dec `json:"claimed"`    // uplatnené odpisy
	Disposal   money.Dec `json:"disposal"`   // zostatková cena vyradeného majetku
	Expense    money.Dec `json:"expense"`    // daňový výdavok spolu
	Tangible   money.Dec `json:"tangible"`   // ZC hmotného majetku na konci roka
	Intangible money.Dec `json:"intangible"` // ZC nehmotného majetku na konci roka
}

// AssetRegister je register majetku za rok.
type AssetRegister struct {
	Year   int         `json:"year"`
	Items  []AssetItem `json:"items"`
	Totals AssetTotals `json:"totals"`
	// Posted je suma odpisov zaúčtovaná v uzávierke roka (nula = nezaúčtované).
	Posted money.Dec `json:"posted"`
}

// depreciationKey je kľúč uzávierkovej operácie s odpismi z registra.
func depreciationKey(year int) string { return closingPrefix(year) + "dm" }

// registerTotals spočíta odpisy a zostatkové ceny majetku za rok.
func registerTotals(list []asset.Asset, year int) AssetTotals {
	var t AssetTotals
	for i := range list {
		x := &list[i]
		y := x.InYear(year)
		t.Claimed, t.Disposal = t.Claimed.Add(y.Claimed), t.Disposal.Add(y.Disposal)
		zc := x.ResidualAt(year)
		if x.Kind == asset.Intangible {
			t.Intangible = t.Intangible.Add(zc)
		} else {
			t.Tangible = t.Tangible.Add(zc)
		}
	}
	t.Expense = t.Claimed.Add(t.Disposal)
	return t
}

// Assets vráti register majetku so stavom v roku (0 = tento rok).
func (a *App) Assets(ctx context.Context, companyID int64, year int) (AssetRegister, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return AssetRegister{}, err
	}
	if year == 0 {
		year = a.Today().Year()
	}
	list, err := a.Store.ListAssets(ctx, companyID)
	if err != nil {
		return AssetRegister{}, err
	}
	out := AssetRegister{Year: year, Items: make([]AssetItem, 0, len(list)), Totals: registerTotals(list, year)}
	for _, x := range list {
		plan, _ := x.Schedule()
		opening := x.ResidualAt(year - 1)
		if x.Acquired.Year() == year {
			opening = x.Cost // zaradený v roku: na začiatku je vstupná cena
		}
		out.Items = append(out.Items, AssetItem{Asset: x, Period: x.Period(), Year: x.InYear(year),
			Opening: opening, Residual: x.ResidualAt(year), Plan: plan})
	}
	overrides, err := a.Store.ListJournalOverrides(ctx, companyID)
	if err != nil {
		return AssetRegister{}, err
	}
	if o, ok := overrides[depreciationKey(year)]; ok {
		for _, l := range o.Lines {
			out.Posted = out.Posted.Add(l.Amount)
		}
	}
	return out, nil
}

// nextAssetNumber navrhne inventárne číslo M001, M002…
func nextAssetNumber(list []asset.Asset) string {
	n := 0
	for _, x := range list {
		if rest, ok := strings.CutPrefix(x.Number, "M"); ok {
			if i, err := strconv.Atoi(rest); err == nil && i > n {
				n = i
			}
		}
	}
	return fmt.Sprintf("M%03d", n+1)
}

// SaveAsset uloží položku majetku (ID 0 = nová).
func (a *App) SaveAsset(ctx context.Context, companyID int64, x asset.Asset) (asset.Asset, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return asset.Asset{}, err
	}
	x.Normalize()
	if err := x.Validate(); err != nil {
		return asset.Asset{}, err
	}
	list, err := a.Store.ListAssets(ctx, companyID)
	if err != nil {
		return asset.Asset{}, err
	}
	var before *asset.Asset
	for i := range list {
		if list[i].ID == x.ID && x.ID != 0 {
			before = &list[i]
		}
	}
	if x.ID != 0 && before == nil {
		return asset.Asset{}, store.ErrNotFound
	}
	for _, o := range list {
		if o.ID != x.ID && x.Number != "" && strings.EqualFold(o.Number, x.Number) {
			return asset.Asset{}, mfaFieldErr("number", "Inventárne číslo "+x.Number+" už má iný majetok")
		}
	}
	if x.Number == "" {
		x.Number = nextAssetNumber(list)
	}
	x.PurchaseNo = ""
	if x.PurchaseID != 0 {
		p, err := a.Store.GetPurchase(ctx, companyID, x.PurchaseID)
		if err != nil {
			return asset.Asset{}, mfaFieldErr("purchaseId", "Prijatá faktúra neexistuje")
		}
		x.PurchaseNo = p.Number
	}
	if err := a.Store.SaveAsset(ctx, companyID, &x); err != nil {
		return asset.Asset{}, err
	}
	verb := "Zaradený majetok"
	if before != nil {
		verb = "Upravený majetok"
	}
	a.record(ctx, companyID, audit.CatLedger, "asset", "asset", x.ID,
		fmt.Sprintf("%s %s %s (vstupná cena %s €)", verb, x.Number, x.Name, x.Cost.Display()), before, x)
	return x, nil
}

// DeleteAsset zmaže položku majetku (omylom zadanú; vyradenie sa robí dátumom vyradenia).
func (a *App) DeleteAsset(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	x, err := a.Store.GetAsset(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteAsset(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "asset", "asset", id, "Zmazaný majetok "+x.Number+" "+x.Name, x, nil)
	return nil
}

// PostDepreciation zaúčtuje daňové odpisy a zostatkové ceny vyradeného
// majetku z registra do uzávierky roka (stĺpec e6 – ostatné výdavky).
// Opakované volanie sumu prepíše; nulová suma operáciu zmaže.
func (a *App) PostDepreciation(ctx context.Context, companyID int64, year int) (money.Dec, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return money.Zero, err
	}
	if year < 2000 || year > 2100 {
		return money.Zero, mfaFieldErr("year", "Neplatný rok")
	}
	list, err := a.Store.ListAssets(ctx, companyID)
	if err != nil {
		return money.Zero, err
	}
	t := registerTotals(list, year)
	key := depreciationKey(year)
	if t.Expense.IsZero() {
		overrides, err := a.Store.ListJournalOverrides(ctx, companyID)
		if err != nil {
			return money.Zero, err
		}
		if old, ok := overrides[key]; ok {
			if err := a.Store.DeleteJournalOverride(ctx, companyID, key); err != nil {
				return money.Zero, err
			}
			a.record(ctx, companyID, audit.CatLedger, "closing", "journal", 0, "Zmazaná uzávierková operácia "+key+" (v registri nie sú odpisy)", old, nil)
		}
		return money.Zero, nil
	}
	text := fmt.Sprintf("Daňové odpisy dlhodobého majetku %d", year)
	if !t.Disposal.IsZero() {
		text += " a zostatková cena vyradeného majetku"
	}
	return t.Expense, a.saveClosing(ctx, companyID, key, text, []journal.Line{{Column: journal.ExpOther, Amount: t.Expense}})
}
