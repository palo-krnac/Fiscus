package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func cloneRecurring(t recurring.Template) recurring.Template {
	t.Lines = append([]invoice.Line(nil), t.Lines...)
	t.History = append([]recurring.Run(nil), t.History...)
	return t
}

func (s *Store) ListRecurring(_ context.Context, companyID int64) ([]recurring.Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []recurring.Template{}
	for _, t := range s.recurring {
		if t.CompanyID == companyID {
			out = append(out, cloneRecurring(t))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) GetRecurring(_ context.Context, companyID, id int64) (recurring.Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.recurring[id]
	if !ok || t.CompanyID != companyID {
		return recurring.Template{}, store.ErrNotFound
	}
	return cloneRecurring(t), nil
}

func (s *Store) SaveRecurring(_ context.Context, companyID int64, t *recurring.Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recurring == nil {
		s.recurring = map[int64]recurring.Template{}
	}
	if t.ID == 0 {
		t.ID = s.id()
	} else if old, ok := s.recurring[t.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	t.CompanyID = companyID
	s.recurring[t.ID] = cloneRecurring(*t)
	return nil
}

func (s *Store) DeleteRecurring(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.recurring[id]; !ok || t.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.recurring, id)
	return nil
}
