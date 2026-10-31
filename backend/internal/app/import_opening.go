package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/chart"
	"github.com/palo-krnac/fiscus/backend/internal/ledger"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/table"
	"github.com/palo-krnac/fiscus/backend/internal/tabimport"
)

// OpeningImport sú počiatočné stavy načítané z tabuľky (konečná súvaha
// alebo obratová predvaha z iného programu) — návrh do editora, nič sa
// neukladá. Účet, ktorý nie je v rozvrhu, ostane v návrhu s upozornením.
type OpeningImport struct {
	Lines    []ledger.Line `json:"lines"`
	Warnings []string      `json:"warnings"`
	Debit    money.Dec     `json:"debit"`
	Credit   money.Dec     `json:"credit"`
	Balanced bool          `json:"balanced"`
}

// ImportOpening prečíta zostatky účtov z tabuľky CSV/Excel.
func (a *App) ImportOpening(ctx context.Context, companyID int64, data []byte) (OpeningImport, error) {
	out := OpeningImport{Lines: []ledger.Line{}, Warnings: []string{}}
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return out, err
	}
	rows, err := table.Read(data)
	if err != nil {
		return out, mfaFieldErr("file", err.Error())
	}
	bs, err := tabimport.ParseBalances(rows)
	if err != nil {
		return out, mfaFieldErr("file", err.Error())
	}
	cb, err := a.loadChart(ctx, companyID)
	if err != nil {
		return out, err
	}
	c := chart.Chart{Accounts: cb.Accounts}
	for _, b := range bs {
		if b.Err != "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Riadok %d: %s", b.Line, b.Err))
			continue
		}
		num := chart.NormalizeNumber(b.Account)
		acc, ok := c.Find(num)
		// 211000 z iného programu = syntetický účet 211
		if !ok && len(num) > 3 && strings.Trim(num[3:], "0") == "" {
			if s, found := c.Find(num[:3]); found {
				num, acc, ok = num[:3], s, true
			}
		}
		switch {
		case !ok:
			out.Warnings = append(out.Warnings, fmt.Sprintf("Riadok %d: účet %s (%s) nie je v rozvrhu — založte ho v Účtovom rozvrhu alebo zmeňte riadok", b.Line, chart.Display(num), b.Name))
		case !acc.Active:
			out.Warnings = append(out.Warnings, fmt.Sprintf("Riadok %d: účet %s je skrytý — zobrazte ho v Účtovom rozvrhu", b.Line, chart.Display(num)))
		}
		name := b.Name
		if ok {
			name = acc.Name
		}
		out.Lines = append(out.Lines, ledger.Line{Account: num, Name: name, Debit: b.Debit, Credit: b.Credit})
		out.Debit, out.Credit = out.Debit.Add(b.Debit), out.Credit.Add(b.Credit)
	}
	out.Balanced = out.Debit.Equal(out.Credit)
	if !out.Balanced {
		out.Warnings = append(out.Warnings, fmt.Sprintf("Strany sa nerovnajú: MD %s, D %s — zostatky sú asi neúplné", out.Debit.Display(), out.Credit.Display()))
	}
	return out, nil
}
