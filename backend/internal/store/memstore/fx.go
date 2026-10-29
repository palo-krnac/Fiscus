package memstore

import (
	"context"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

type fxKey struct{ currency, date string }

func (s *Store) SaveRates(_ context.Context, rates []fx.Rate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rates == nil {
		s.rates = map[fxKey]fx.Rate{}
	}
	for _, r := range rates {
		k := fxKey{r.Currency, r.Date}
		if old, ok := s.rates[k]; ok && old.Source == fx.SourceManual && r.Source != fx.SourceManual {
			continue // ručne zadaný kurz má prednosť
		}
		s.rates[k] = r
	}
	return nil
}

func (s *Store) RateBefore(_ context.Context, currency, taxPoint string) (fx.Rate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best fx.Rate
	found := false
	for _, r := range s.rates {
		if r.Currency == currency && r.Date < taxPoint && (!found || r.Date > best.Date) {
			best, found = r, true
		}
	}
	if !found {
		return best, store.ErrNotFound
	}
	return best, nil
}

func (s *Store) ListRates(_ context.Context, f fx.Filter) ([]fx.Rate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []fx.Rate{}
	year := ""
	if f.Year != 0 {
		year = itoa4(f.Year) + "-"
	}
	for _, r := range s.rates {
		if (f.Currency == "" || r.Currency == f.Currency) && (year == "" || strings.HasPrefix(r.Date, year)) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].Currency < out[j].Currency
	})
	return out, nil
}

func (s *Store) DeleteRate(_ context.Context, currency, date string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := fxKey{currency, date}
	if _, ok := s.rates[k]; !ok {
		return store.ErrNotFound
	}
	delete(s.rates, k)
	return nil
}

func itoa4(y int) string {
	b := []byte{'0', '0', '0', '0'}
	for i := 3; i >= 0; i-- {
		b[i] = byte('0' + y%10)
		y /= 10
	}
	return string(b)
}
