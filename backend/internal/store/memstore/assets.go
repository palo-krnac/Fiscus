package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/asset"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListAssets(_ context.Context, companyID int64) ([]asset.Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []asset.Asset{}
	for _, a := range s.assets {
		if a.CompanyID == companyID {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Acquired != out[j].Acquired {
			return out[i].Acquired.Before(out[j].Acquired)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetAsset(_ context.Context, companyID, id int64) (asset.Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assets[id]
	if !ok || a.CompanyID != companyID {
		return asset.Asset{}, store.ErrNotFound
	}
	return a, nil
}

func (s *Store) SaveAsset(_ context.Context, companyID int64, a *asset.Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assets == nil {
		s.assets = map[int64]asset.Asset{}
	}
	if a.ID == 0 {
		a.ID = s.id()
	} else if old, ok := s.assets[a.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	a.CompanyID = companyID
	s.assets[a.ID] = *a
	return nil
}

func (s *Store) DeleteAsset(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assets[id]
	if !ok || a.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.assets, id)
	return nil
}
