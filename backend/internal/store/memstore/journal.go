package memstore

import (
	"context"

	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListJournalOverrides(_ context.Context, companyID int64) (map[string]store.JournalOverride, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]store.JournalOverride{}
	for k, o := range s.journal[companyID] {
		o.Lines = append([]journal.Line(nil), o.Lines...)
		out[k] = o
	}
	return out, nil
}

func (s *Store) SaveJournalOverride(_ context.Context, companyID int64, o store.JournalOverride) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal == nil {
		s.journal = map[int64]map[string]store.JournalOverride{}
	}
	if s.journal[companyID] == nil {
		s.journal[companyID] = map[string]store.JournalOverride{}
	}
	o.Lines = append([]journal.Line(nil), o.Lines...)
	s.journal[companyID][o.Key] = o
	return nil
}

func (s *Store) DeleteJournalOverride(_ context.Context, companyID int64, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.journal[companyID][key]; !ok {
		return store.ErrNotFound
	}
	delete(s.journal[companyID], key)
	return nil
}
