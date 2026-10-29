package memstore

import (
	"context"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) GetLogo(_ context.Context, companyID int64) (store.Logo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.logos[companyID]
	if !ok {
		return store.Logo{}, store.ErrNotFound
	}
	l.Data = append([]byte(nil), l.Data...)
	return l, nil
}

func (s *Store) SaveLogo(_ context.Context, companyID int64, l store.Logo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, c := range s.companies {
		found = found || c.ID == companyID
	}
	if !found {
		return store.ErrNotFound
	}
	if s.logos == nil {
		s.logos = map[int64]store.Logo{}
	}
	l.Data = append([]byte(nil), l.Data...)
	s.logos[companyID] = l
	return nil
}

func (s *Store) DeleteLogo(_ context.Context, companyID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.logos[companyID]; !ok {
		return store.ErrNotFound
	}
	delete(s.logos, companyID)
	return nil
}
