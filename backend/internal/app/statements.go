package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Statements je účtovná závierka v jednoduchom účtovníctve za rok:
// Výkaz o príjmoch a výdavkoch (Úč FO 1-01) a Výkaz o majetku a záväzkoch
// (Úč FO 2-01), bežné a predchádzajúce obdobie.
type Statements struct {
	Year         int                 `json:"year"`
	Income       []journal.Row       `json:"income"`
	Assets       []journal.Row       `json:"assets"`
	Inputs       journal.AssetInputs `json:"inputs"`
	Facts        journal.AssetFacts  `json:"facts"`
	PrevEmpty    bool                `json:"previousEmpty"` // prvá závierka: stĺpce predchádzajúceho obdobia sa nevypĺňajú
	Unclassified int                 `json:"unclassified"`  // nerozpísané pohyby v denníku (výkaz nie je úplný)
	TaxB         journal.TaxB        `json:"taxB"`          // podklady pre DP FO typ B, VI. oddiel
}

func statementsKey(companyID int64, year int) string {
	return fmt.Sprintf("statements.%d.%d", companyID, year)
}

func (a *App) statementInputs(ctx context.Context, companyID int64, year int) (journal.AssetInputs, bool, error) {
	var in journal.AssetInputs
	v, err := a.Store.GetSetting(ctx, statementsKey(companyID, year))
	if errors.Is(err, store.ErrNotFound) {
		return in, false, nil
	}
	if err != nil {
		return in, false, err
	}
	return in, true, json.Unmarshal([]byte(v), &in)
}

// assetFacts zistí hodnoty z dokladov k 31. 12. roka.
func (a *App) assetFacts(ctx context.Context, companyID int64, book JournalBook) (journal.AssetFacts, error) {
	var f journal.AssetFacts
	items, err := a.OpenItemsAt(ctx, companyID, invoice.MustDate(fmt.Sprintf("%d-12-31", book.Year)))
	if err != nil {
		return f, err
	}
	for _, it := range items.Receivables {
		if it.RemainEUR.Sign() >= 0 {
			f.Receivables = f.Receivables.Add(it.RemainEUR)
		} else {
			f.Payables = f.Payables.Add(it.RemainEUR.Neg()) // dobropis na vrátenie je záväzok
		}
	}
	f.Payables = f.Payables.Add(items.PayablesTotal)
	for _, b := range book.Balances {
		if b.Means == journal.Cash {
			f.Cash = f.Cash.Add(b.Closing)
		} else {
			f.Bank = f.Bank.Add(b.Closing)
		}
	}
	f.Transit = book.Totals.Columns[journal.TransitOut].Sub(book.Totals.Columns[journal.TransitIn])
	return f, nil
}

// StatementsFor zostaví výkazy za rok.
func (a *App) StatementsFor(ctx context.Context, companyID int64, year int) (Statements, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return Statements{}, err
	}
	if year == 0 {
		year = a.Today().Year()
	}
	cur, err := a.Journal(ctx, companyID, year)
	if err != nil {
		return Statements{}, err
	}
	prev, err := a.Journal(ctx, companyID, year-1)
	if err != nil {
		return Statements{}, err
	}
	curFacts, err := a.assetFacts(ctx, companyID, cur)
	if err != nil {
		return Statements{}, err
	}
	prevFacts, err := a.assetFacts(ctx, companyID, prev)
	if err != nil {
		return Statements{}, err
	}
	curIn, _, err := a.statementInputs(ctx, companyID, year)
	if err != nil {
		return Statements{}, err
	}
	prevIn, prevSaved, err := a.statementInputs(ctx, companyID, year-1)
	if err != nil {
		return Statements{}, err
	}
	prevEmpty := !prevSaved && len(prev.Entries) == 0 && prevFacts == (journal.AssetFacts{})
	curVals := journal.AssetValues{Inputs: curIn, Facts: curFacts}
	prevVals := journal.AssetValues{Inputs: prevIn, Facts: prevFacts}
	return Statements{
		TaxB:         journal.TaxReturnB(cur.Totals, curVals, prevVals),
		Year:         year,
		Income:       journal.IncomeStatement(cur.Totals, prev.Totals),
		Assets:       journal.AssetStatement(curVals, prevVals),
		Inputs:       curIn,
		Facts:        curFacts,
		PrevEmpty:    prevEmpty,
		Unclassified: cur.Totals.Open,
	}, nil
}

// SetStatementInputs uloží ručne zadané riadky výkazu o majetku a záväzkoch.
func (a *App) SetStatementInputs(ctx context.Context, companyID int64, year int, in journal.AssetInputs) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	if year < 2000 || year > 2100 {
		return mfaFieldErr("year", "Neplatný rok")
	}
	for _, v := range []money.Dec{in.IntangibleDM, in.TangibleDM, in.FinancialDM, in.Material, in.Goods, in.OtherStock, in.OtherCashEq,
		in.AdjustAsset, in.Reserves, in.OtherLiab, in.Loans, in.AdjustLiab, in.OtherRecv} {
		if !v.Equal(v.RoundCents()) {
			return mfaFieldErr("inputs", "Sumy zadajte najviac na centy")
		}
	}
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if err := a.Store.SetSetting(ctx, statementsKey(companyID, year), string(b)); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "statements", "statements", 0, fmt.Sprintf("Výkaz o majetku a záväzkoch %d: ručne zadané riadky", year), nil, in)
	return nil
}
