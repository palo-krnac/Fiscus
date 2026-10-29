// Package memstore je úložisko v pamäti — pre testy a demo režim.
package memstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

type seqKey struct {
	company int64
	series  string
	year    int
}

// Store je úložisko v pamäti, bezpečné pre súbežný prístup.
type Store struct {
	mu        sync.Mutex
	companies []store.Company
	invoices  map[int64]invoice.Invoice
	contacts  map[int64]store.Contact
	seq       map[seqKey]int
	nextID    int64
}

// New vytvorí prázdne úložisko.
func New() *Store {
	return &Store{invoices: map[int64]invoice.Invoice{}, contacts: map[int64]store.Contact{}, seq: map[seqKey]int{}}
}

func (s *Store) id() int64 { s.nextID++; return s.nextID }

// clone vytvorí hlbokú kópiu, aby volajúci nemohol meniť dáta v úložisku.
func clone(in invoice.Invoice) invoice.Invoice {
	b, _ := json.Marshal(in)
	var out invoice.Invoice
	_ = json.Unmarshal(b, &out)
	out.ID, out.CompanyID = in.ID, in.CompanyID
	return out
}

func (s *Store) ListCompanies(context.Context) ([]store.Company, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.Company(nil), s.companies...), nil
}

func (s *Store) CountCompanies(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.companies), nil
}

func (s *Store) CreateCompany(_ context.Context, c store.Company) (store.Company, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = s.id()
	s.companies = append(s.companies, c)
	return c, nil
}

func (s *Store) GetCompany(_ context.Context, id int64) (store.Company, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.companies {
		if c.ID == id {
			return c, nil
		}
	}
	return store.Company{}, store.ErrNotFound
}

func (s *Store) UpdateCompany(_ context.Context, c store.Company) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.companies {
		if s.companies[i].ID == c.ID {
			s.companies[i] = c
			return nil
		}
	}
	return store.ErrNotFound
}

func (s *Store) ListContacts(_ context.Context, companyID int64, search string) ([]store.Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []store.Contact{}
	for _, c := range s.contacts {
		if c.CompanyID == companyID && store.Matches(search, c.Name, c.ICO, c.City) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := store.SearchKey(out[i].Name), store.SearchKey(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetContact(_ context.Context, companyID, id int64) (store.Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.contacts[id]
	if !ok || c.CompanyID != companyID {
		return store.Contact{}, store.ErrNotFound
	}
	return c, nil
}

func (s *Store) SaveContact(_ context.Context, companyID int64, c *store.Contact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == 0 {
		c.ID = s.id()
	} else if old, ok := s.contacts[c.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	c.CompanyID = companyID
	s.contacts[c.ID] = *c
	return nil
}

func (s *Store) DeleteContact(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.contacts[id]
	if !ok || c.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.contacts, id)
	return nil
}

func matches(inv invoice.Invoice, f store.InvoiceFilter) bool {
	if f.Year != 0 && inv.IssueDate.Year() != f.Year {
		return false
	}
	if f.Status != "" && inv.Status != f.Status {
		return false
	}
	return store.Matches(f.Search, inv.Number, inv.Customer.Name, inv.Customer.ICO)
}

func (s *Store) ListInvoices(_ context.Context, companyID int64, f store.InvoiceFilter) ([]invoice.Invoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []invoice.Invoice
	for _, inv := range s.invoices {
		if inv.CompanyID == companyID && matches(inv, f) {
			out = append(out, clone(inv))
		}
	}
	// najnovšie hore; koncepty (bez čísla) podľa ID
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.IssueDate.IsZero() && !b.IssueDate.IsZero() && a.IssueDate != b.IssueDate {
			return a.IssueDate.After(b.IssueDate)
		}
		return a.ID > b.ID
	})
	return out, nil
}

func (s *Store) GetInvoice(_ context.Context, companyID, id int64) (invoice.Invoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.invoices[id]
	if !ok || inv.CompanyID != companyID {
		return invoice.Invoice{}, store.ErrNotFound
	}
	return clone(inv), nil
}

func (s *Store) SaveInvoice(_ context.Context, companyID int64, inv *invoice.Invoice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inv.ID == 0 {
		inv.ID = s.id()
	} else if old, ok := s.invoices[inv.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	inv.CompanyID = companyID
	s.invoices[inv.ID] = clone(*inv)
	return nil
}

func (s *Store) NextSequence(_ context.Context, companyID int64, series string, year int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if series == "" || year < 2000 {
		return 0, fmt.Errorf("neplatný rad %q/%d", series, year)
	}
	k := seqKey{companyID, series, year}
	s.seq[k]++
	return s.seq[k], nil
}

func (s *Store) Close() error { return nil }
