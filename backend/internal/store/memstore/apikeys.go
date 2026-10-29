package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) CreateAPIKey(_ context.Context, k *auth.APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.apiKeys == nil {
		s.apiKeys = map[int64]auth.APIKey{}
	}
	for _, x := range s.apiKeys {
		if x.Hash == k.Hash {
			return store.ErrDuplicate
		}
	}
	k.ID = s.id()
	s.apiKeys[k.ID] = *k
	return nil
}

func (s *Store) ListAPIKeys(_ context.Context, userID int64) ([]auth.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []auth.APIKey{}
	for _, k := range s.apiKeys {
		if k.UserID == userID {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (s *Store) GetAPIKeyByHash(_ context.Context, hash string) (auth.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range s.apiKeys {
		if k.Hash == hash {
			return k, nil
		}
	}
	return auth.APIKey{}, store.ErrNotFound
}

func (s *Store) TouchAPIKey(_ context.Context, id int64, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.apiKeys[id]
	if !ok {
		return store.ErrNotFound
	}
	k.LastUsedAt = at
	s.apiKeys[id] = k
	return nil
}

func (s *Store) RevokeAPIKey(_ context.Context, userID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.apiKeys[id]
	if !ok || k.UserID != userID {
		return store.ErrNotFound
	}
	k.Revoked = true
	s.apiKeys[id] = k
	return nil
}

func (s *Store) GetAPIKey(_ context.Context, id int64) (auth.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.apiKeys[id]
	if !ok {
		return auth.APIKey{}, store.ErrNotFound
	}
	return k, nil
}

func (s *Store) RotateAPIKey(_ context.Context, id int64, oldHash, newHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.apiKeys[id]
	if !ok || k.Hash != oldHash || k.Revoked {
		return store.ErrNotFound
	}
	k.Hash = newHash
	s.apiKeys[id] = k
	return nil
}
