package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func cloneTx(t bank.Transaction) bank.Transaction {
	if t.Match != nil {
		m := *t.Match
		t.Match = &m
	}
	return t
}

func (s *Store) SaveBankTransactions(_ context.Context, companyID int64, txs []bank.Transaction) ([]bank.Transaction, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bankTx == nil {
		s.bankTx = map[int64]bank.Transaction{}
	}
	known := map[string]bool{}
	for _, t := range s.bankTx {
		if t.CompanyID == companyID {
			known[t.Fingerprint] = true
		}
	}
	var out []bank.Transaction
	dup := 0
	for _, t := range txs {
		if known[t.Fingerprint] {
			dup++
			continue
		}
		known[t.Fingerprint] = true
		t.ID, t.CompanyID = s.id(), companyID
		if t.Status == "" {
			t.Status = bank.StatusOpen
		}
		s.bankTx[t.ID] = cloneTx(t)
		out = append(out, cloneTx(t))
	}
	return out, dup, nil
}

func (s *Store) ListBankTransactions(_ context.Context, companyID int64, f store.BankFilter) ([]bank.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []bank.Transaction{}
	for _, t := range s.bankTx {
		if t.CompanyID != companyID || (f.Year != 0 && t.Date.Year() != f.Year) || (f.Status != "" && t.Status != f.Status) {
			continue
		}
		out = append(out, cloneTx(t))
	}
	sortTx(out)
	return out, nil
}

func sortTx(out []bank.Transaction) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date.After(out[j].Date)
		}
		return out[i].ID > out[j].ID
	})
}

func (s *Store) GetBankTransaction(_ context.Context, companyID, id int64) (bank.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.bankTx[id]
	if !ok || t.CompanyID != companyID {
		return bank.Transaction{}, store.ErrNotFound
	}
	return cloneTx(t), nil
}

func (s *Store) UpdateBankTransaction(_ context.Context, companyID int64, t bank.Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.bankTx[t.ID]
	if !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	old.Status, old.Match, old.Note = t.Status, t.Match, t.Note
	s.bankTx[t.ID] = cloneTx(old)
	return nil
}
