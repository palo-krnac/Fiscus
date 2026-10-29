package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) AddEInvoice(_ context.Context, companyID int64, e *store.EInvoice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = s.id()
	e.CompanyID = companyID
	c := *e
	c.Data = append([]byte(nil), e.Data...)
	s.einvoices = append(s.einvoices, c)
	return nil
}

func (s *Store) ListEInvoices(_ context.Context, companyID int64, f store.EInvoiceFilter) ([]store.EInvoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []store.EInvoice{}
	for _, e := range s.einvoices {
		if e.CompanyID != companyID || (f.Direction != "" && e.Direction != f.Direction) ||
			(f.DocID != 0 && e.DocID != f.DocID) || (f.MessageID != "" && e.MessageID != f.MessageID) {
			continue
		}
		e.Data = nil
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.After(out[j].Created)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *Store) GetEInvoice(_ context.Context, companyID, id int64) (store.EInvoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.einvoices {
		if e.ID == id && e.CompanyID == companyID {
			e.Data = append([]byte(nil), e.Data...)
			return e, nil
		}
	}
	return store.EInvoice{}, store.ErrNotFound
}
