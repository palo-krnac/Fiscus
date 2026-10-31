package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListItemImages(_ context.Context, companyID, itemID int64) ([]store.ItemImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []store.ItemImage{}
	for _, im := range s.images {
		if im.CompanyID == companyID && im.ItemID == itemID {
			im.Data = nil
			out = append(out, im)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetItemImage(_ context.Context, companyID, id int64) (store.ItemImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	im, ok := s.images[id]
	if !ok || im.CompanyID != companyID {
		return store.ItemImage{}, store.ErrNotFound
	}
	im.Data = append([]byte(nil), im.Data...)
	return im, nil
}

func (s *Store) AddItemImage(_ context.Context, img *store.ItemImage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[img.ItemID]
	if !ok || it.CompanyID != img.CompanyID {
		return store.ErrNotFound
	}
	if s.images == nil {
		s.images = map[int64]store.ItemImage{}
	}
	pos := 0
	for _, im := range s.images {
		if im.ItemID == img.ItemID && im.Position >= pos {
			pos = im.Position + 1
		}
	}
	img.ID, img.Position, img.Size = s.id(), pos, len(img.Data)
	cp := *img
	cp.Data = append([]byte(nil), img.Data...)
	s.images[img.ID] = cp
	return nil
}

func (s *Store) SetItemImageOrder(_ context.Context, companyID, itemID int64, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if im, ok := s.images[id]; !ok || im.CompanyID != companyID || im.ItemID != itemID {
			return store.ErrNotFound
		}
	}
	for i, id := range ids {
		im := s.images[id]
		im.Position = i
		s.images[id] = im
	}
	return nil
}

func (s *Store) DeleteItemImage(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	im, ok := s.images[id]
	if !ok || im.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.images, id)
	return nil
}
