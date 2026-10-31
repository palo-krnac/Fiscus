package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func copyStockDoc(d stock.Doc) stock.Doc {
	d.Lines = append([]stock.Line(nil), d.Lines...)
	return d
}

func (s *Store) ListStockDocs(_ context.Context, companyID int64) ([]stock.Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []stock.Doc{}
	for _, d := range s.stockDocs {
		if d.CompanyID == companyID {
			out = append(out, copyStockDoc(d))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetStockDoc(_ context.Context, companyID, id int64) (stock.Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.stockDocs[id]
	if !ok || d.CompanyID != companyID {
		return stock.Doc{}, store.ErrNotFound
	}
	return copyStockDoc(d), nil
}

func (s *Store) SaveStockDoc(_ context.Context, companyID int64, d *stock.Doc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stockDocs == nil {
		s.stockDocs = map[int64]stock.Doc{}
	}
	if d.ID != 0 {
		if old, ok := s.stockDocs[d.ID]; !ok || old.CompanyID != companyID {
			return store.ErrNotFound
		}
	} else {
		s.nextID++
		d.ID = s.nextID
	}
	d.CompanyID = companyID
	s.stockDocs[d.ID] = copyStockDoc(*d)
	return nil
}

func (s *Store) DeleteStockDoc(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.stockDocs[id]; !ok || d.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.stockDocs, id)
	return nil
}
