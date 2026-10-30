package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
)

// ErrECBRatesOff — sťahovanie sadzieb ECB je vypnuté (program bez internetu).
var ErrECBRatesOff = errors.New("sťahovanie sadzieb ECB je vypnuté — sadzbu zadajte ručne")

// ECBRatesUpdate je výsledok načítania základnej úrokovej sadzby ECB.
type ECBRatesUpdate struct {
	Added  []reminder.Rate `json:"added"`
	Latest reminder.Rate   `json:"latest"` // posledná zmena v tabuľke
}

// UpdateECBRates stiahne zo servera ECB sadzbu hlavných refinančných
// operácií a doplní do Legislatívy nové zmeny (ručne zadané dni ostanú).
// Spúšťa ju program raz za čas alebo správca tlačidlom.
func (a *App) UpdateECBRates(ctx context.Context) (ECBRatesUpdate, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return ECBRatesUpdate{}, err
	}
	if a.ECBRateURL == "" {
		return ECBRatesUpdate{}, ErrECBRatesOff
	}
	p := a.legalParams()
	table := reminder.Sorted(p.ECB)
	src := a.ECBRateURL
	if last := table.Updated(); !last.IsZero() {
		// stačí obdobie od poslednej známej zmeny (s rezervou)
		sep := "&"
		if !strings.Contains(src, "?") {
			sep = "?"
		}
		src += sep + "startPeriod=" + url.QueryEscape(last.AddDays(-30).String())
	}
	fetched, err := a.fetchECBRates(ctx, src)
	if err != nil {
		return ECBRatesUpdate{}, err
	}
	merged, added := reminder.MergeECB(table, fetched)
	out := ECBRatesUpdate{Added: added}
	if out.Added == nil {
		out.Added = []reminder.Rate{}
	}
	if len(merged) > 0 {
		out.Latest = merged[len(merged)-1]
	}
	if len(added) == 0 {
		return out, nil
	}
	before := p
	p.ECB = merged
	if err := a.saveLegal(ctx, p); err != nil {
		return out, err
	}
	a.applyLegal(p)
	parts := make([]string, len(added))
	for i, r := range added {
		parts[i] = r.Rate.Display() + " % od " + r.From.Display()
	}
	a.record(ctx, 0, audit.CatSettings, "update", "legal", 0, "Sadzba ECB zo servera ECB: "+strings.Join(parts, ", "), before.ECB, p.ECB)
	return out, nil
}

func (a *App) fetchECBRates(ctx context.Context, src string) ([]reminder.Rate, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/csv")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sadzby ECB: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sadzby ECB: server vrátil %s", res.Status)
	}
	return reminder.ParseECBData(io.LimitReader(res.Body, 4<<20))
}
