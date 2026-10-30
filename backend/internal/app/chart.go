package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/chart"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// ChartBook je účtový rozvrh firmy so strediskami a zákazkami (uložené spolu).
type ChartBook struct {
	Accounts []chart.Account `json:"accounts"`
	Centers  []chart.Center  `json:"centers"`
	Jobs     []chart.Job     `json:"jobs"`
}

// ChartView je účtový rozvrh na zobrazenie s číselníkmi.
type ChartView struct {
	ChartBook
	Classes []string `json:"classes"`
	Types   any      `json:"types"`
	// Framework sú čísla syntetických účtov rámcovej osnovy (nedajú sa zmazať).
	Framework []string `json:"framework"`
}

func chartKey(companyID int64) string { return fmt.Sprintf("chart.%d", companyID) }

// loadChart načíta rozvrh; bez uloženého je predvyplnený osnovou.
func (a *App) loadChart(ctx context.Context, companyID int64) (ChartBook, error) {
	raw, err := a.Store.GetSetting(ctx, chartKey(companyID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return ChartBook{Accounts: chart.Default().Accounts, Centers: []chart.Center{}, Jobs: []chart.Job{}}, nil
	}
	if err != nil {
		return ChartBook{}, err
	}
	var b ChartBook
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return b, err
	}
	c := chart.Chart{Accounts: b.Accounts}
	c.Seed() // nová verzia osnovy
	b.Accounts = c.Accounts
	if b.Centers == nil {
		b.Centers = []chart.Center{}
	}
	if b.Jobs == nil {
		b.Jobs = []chart.Job{}
	}
	return b, nil
}

func (a *App) saveChart(ctx context.Context, companyID int64, b ChartBook) error {
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, chartKey(companyID), string(raw))
}

// Chart vráti účtový rozvrh, strediská a zákazky firmy.
func (a *App) Chart(ctx context.Context, companyID int64) (ChartView, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return ChartView{}, err
	}
	b, err := a.loadChart(ctx, companyID)
	if err != nil {
		return ChartView{}, err
	}
	v := ChartView{ChartBook: b, Classes: chart.Classes, Types: chart.Types}
	for _, acc := range chart.Default().Accounts {
		v.Framework = append(v.Framework, acc.Number)
	}
	return v, nil
}

// editChart načíta rozvrh, zavolá úpravu a uloží ho (pod zámkom).
func (a *App) editChart(ctx context.Context, companyID int64, edit func(*ChartBook) (string, error)) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	a.chartMu.Lock()
	defer a.chartMu.Unlock()
	b, err := a.loadChart(ctx, companyID)
	if err != nil {
		return err
	}
	summary, err := edit(&b)
	if err != nil {
		if errors.Is(err, chart.ErrNotFound) {
			return store.ErrNotFound
		}
		return err
	}
	if err := a.saveChart(ctx, companyID, b); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "chart", "chart", 0, summary, nil, nil)
	return nil
}

// SaveAccount pridá (old = "") alebo upraví účet rozvrhu.
func (a *App) SaveAccount(ctx context.Context, companyID int64, old string, acc chart.Account) (chart.Account, error) {
	var out chart.Account
	err := a.editChart(ctx, companyID, func(b *ChartBook) (string, error) {
		c := chart.Chart{Accounts: b.Accounts}
		saved, err := c.Save(acc, old)
		if err != nil {
			return "", err
		}
		b.Accounts, out = c.Accounts, saved
		what := "Pridaný účet "
		if old != "" {
			what = "Upravený účet "
		}
		return what + chart.Display(saved.Number) + " " + saved.Name, nil
	})
	return out, err
}

// DeleteAccount zmaže analytický účet alebo skryje syntetický účet osnovy.
func (a *App) DeleteAccount(ctx context.Context, companyID int64, number string) error {
	return a.editChart(ctx, companyID, func(b *ChartBook) (string, error) {
		c := chart.Chart{Accounts: b.Accounts}
		if err := c.Delete(number); err != nil {
			return "", err
		}
		b.Accounts = c.Accounts
		return "Zmazaný (skrytý) účet " + chart.Display(number), nil
	})
}

// SaveCenter pridá (old = "") alebo upraví stredisko.
func (a *App) SaveCenter(ctx context.Context, companyID int64, old string, c chart.Center) (chart.Center, error) {
	var out chart.Center
	err := a.editChart(ctx, companyID, func(b *ChartBook) (string, error) {
		d := chart.Dims{Centers: b.Centers, Jobs: b.Jobs}
		saved, err := d.SaveCenter(c, old)
		if err != nil {
			return "", err
		}
		b.Centers, out = d.Centers, saved
		return "Stredisko " + saved.Code + " " + saved.Name, nil
	})
	return out, err
}

// DeleteCenter zmaže stredisko.
func (a *App) DeleteCenter(ctx context.Context, companyID int64, code string) error {
	return a.editChart(ctx, companyID, func(b *ChartBook) (string, error) {
		d := chart.Dims{Centers: b.Centers, Jobs: b.Jobs}
		if err := d.DeleteCenter(code); err != nil {
			return "", err
		}
		b.Centers = d.Centers
		return "Zmazané stredisko " + code, nil
	})
}

// SaveJob pridá (old = "") alebo upraví zákazku.
func (a *App) SaveJob(ctx context.Context, companyID int64, old string, j chart.Job) (chart.Job, error) {
	var out chart.Job
	err := a.editChart(ctx, companyID, func(b *ChartBook) (string, error) {
		d := chart.Dims{Centers: b.Centers, Jobs: b.Jobs}
		saved, err := d.SaveJob(j, old)
		if err != nil {
			return "", err
		}
		b.Jobs, out = d.Jobs, saved
		return "Zákazka " + saved.Code + " " + saved.Name, nil
	})
	return out, err
}

// DeleteJob zmaže zákazku.
func (a *App) DeleteJob(ctx context.Context, companyID int64, code string) error {
	return a.editChart(ctx, companyID, func(b *ChartBook) (string, error) {
		d := chart.Dims{Centers: b.Centers, Jobs: b.Jobs}
		if err := d.DeleteJob(code); err != nil {
			return "", err
		}
		b.Jobs = d.Jobs
		return "Zmazaná zákazka " + code, nil
	})
}
