package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) AddEmail(_ context.Context, companyID int64, e *store.Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = s.id()
	e.CompanyID = companyID
	s.emails = append(s.emails, *e)
	return nil
}

func (s *Store) ListEmails(_ context.Context, companyID int64, f store.EmailFilter) ([]store.Email, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []store.Email{}
	for _, e := range s.emails {
		if e.CompanyID != companyID || (f.Year != 0 && e.Sent.Year() != f.Year) ||
			(f.DocKind != "" && e.DocKind != f.DocKind) || (f.DocID != 0 && e.DocID != f.DocID) {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Sent.Equal(out[j].Sent) {
			return out[i].Sent.After(out[j].Sent)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}
