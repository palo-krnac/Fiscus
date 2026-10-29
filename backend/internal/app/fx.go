package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// ErrRatesOff — sťahovanie kurzov je vypnuté (program beží bez internetu).
var ErrRatesOff = errors.New("sťahovanie kurzov ECB je vypnuté — kurz zadajte ručne")

// fetchECB stiahne súbor kurzov ECB.
func (a *App) fetchECB(ctx context.Context) ([]fx.Rate, error) {
	if a.ECBURL == "" {
		return nil, ErrRatesOff
	}
	if a.FetchRates != nil {
		return a.FetchRates(ctx, a.ECBURL)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.ECBURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kurzy ECB: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kurzy ECB: server vrátil %s", res.Status)
	}
	return fx.ParseECB(io.LimitReader(res.Body, 8<<20))
}

// RatesUpdate je výsledok aktualizácie kurzov.
type RatesUpdate struct {
	Count  int    `json:"count"`
	Latest string `json:"latest"` // posledný deň vyhlásenia v súbore
}

// UpdateRates stiahne kurzy ECB (posledných 90 dní) a uloží ich. Ručne
// zadané kurzy ostanú. Spúšťa ju program raz denne alebo správca.
func (a *App) UpdateRates(ctx context.Context) (RatesUpdate, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return RatesUpdate{}, err
	}
	rates, err := a.fetchECB(ctx)
	if err != nil {
		return RatesUpdate{}, err
	}
	if err := a.Store.SaveRates(ctx, rates); err != nil {
		return RatesUpdate{}, err
	}
	res := RatesUpdate{Count: len(rates)}
	for _, r := range rates {
		res.Latest = max(res.Latest, r.Date)
	}
	return res, nil
}

// Rates vráti kurzy (čítať ich môže každý prihlásený).
func (a *App) Rates(ctx context.Context, f fx.Filter) ([]fx.Rate, error) {
	return a.Store.ListRates(ctx, f)
}

// SetRate uloží ručne zadaný kurz (napr. keď ECB kurz nevyhlásila).
func (a *App) SetRate(ctx context.Context, r fx.Rate) (fx.Rate, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return r, err
	}
	r.Source = fx.SourceManual
	if err := r.Validate(); err != nil {
		return r, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "rate", Message: err.Error()}}}
	}
	if err := a.Store.SaveRates(ctx, []fx.Rate{r}); err != nil {
		return r, err
	}
	a.record(ctx, 0, audit.CatSettings, "update", "fx_rate", 0,
		fmt.Sprintf("Ručný kurz %s k %s: %s", r.Currency, r.Date, r.Rate), nil, r)
	return r, nil
}

// DeleteRate zmaže kurz (napr. chybne zadaný ručný kurz).
func (a *App) DeleteRate(ctx context.Context, currency, date string) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	if err := a.Store.DeleteRate(ctx, currency, date); err != nil {
		return err
	}
	a.record(ctx, 0, audit.CatSettings, "delete", "fx_rate", 0, fmt.Sprintf("Zmazaný kurz %s k %s", currency, date), nil, nil)
	return nil
}

// applyRate doplní kurz ECB faktúre v cudzej mene: kurz vyhlásený pred dňom
// vzniku daňovej povinnosti (dátum dodania). Ručne zadaný kurz ostane.
func (a *App) applyRate(ctx context.Context, inv *invoice.Invoice) error {
	if !inv.Foreign() || (inv.FXManual && inv.FXRate.Sign() > 0) {
		return nil
	}
	inv.FXManual = false
	point := inv.DeliveryDate
	if point.IsZero() {
		point = inv.IssueDate
	}
	if point.IsZero() {
		point = a.Today()
	}
	r, err := a.Store.RateBefore(ctx, inv.Currency, point.String())
	switch {
	case errors.Is(err, store.ErrNotFound):
		inv.FXRate, inv.FXDate = money.Zero, invoice.Date{}
		return nil
	case err != nil:
		return err
	}
	inv.FXRate = r.Rate
	inv.FXDate, err = invoice.ParseDate(r.Date)
	return err
}
