package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// cloneQuote oddelí položky, aby volajúci nemenil uložené dáta.
func cloneQuote(q quote.Quote) quote.Quote {
	q.Lines = append([]invoice.Line(nil), q.Lines...)
	return q
}

func (s *Store) ListQuotes(_ context.Context, companyID int64, year int) ([]quote.Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []quote.Quote{}
	for _, q := range s.quotes {
		if q.CompanyID == companyID && (year == 0 || q.IssueDate.Year() == year) {
			out = append(out, cloneQuote(q))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IssueDate != out[j].IssueDate {
			return out[i].IssueDate.After(out[j].IssueDate)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *Store) GetQuote(_ context.Context, companyID, id int64) (quote.Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.quotes[id]
	if !ok || q.CompanyID != companyID {
		return quote.Quote{}, store.ErrNotFound
	}
	return cloneQuote(q), nil
}

func (s *Store) SaveQuote(_ context.Context, companyID int64, q *quote.Quote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.quotes == nil {
		s.quotes = map[int64]quote.Quote{}
	}
	if q.ID == 0 {
		q.ID = s.id()
	} else if old, ok := s.quotes[q.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	q.CompanyID = companyID
	s.quotes[q.ID] = cloneQuote(*q)
	return nil
}
