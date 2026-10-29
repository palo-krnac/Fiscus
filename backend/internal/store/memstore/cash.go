package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListCashDocuments(_ context.Context, companyID int64, year int) ([]cash.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []cash.Document{}
	for _, d := range s.cashDocs {
		if d.CompanyID == companyID && (year == 0 || d.Date.Year() == year) {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date.After(out[j].Date)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *Store) GetCashDocument(_ context.Context, companyID, id int64) (cash.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.cashDocs[id]
	if !ok || d.CompanyID != companyID {
		return cash.Document{}, store.ErrNotFound
	}
	return d, nil
}

func (s *Store) SaveCashDocument(_ context.Context, companyID int64, d *cash.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cashDocs == nil {
		s.cashDocs = map[int64]cash.Document{}
	}
	if d.ID == 0 {
		d.ID = s.id()
	} else if old, ok := s.cashDocs[d.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	d.CompanyID = companyID
	s.cashDocs[d.ID] = *d
	return nil
}
