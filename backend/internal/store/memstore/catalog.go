package memstore

import (
	"context"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListItems(_ context.Context, companyID int64) ([]catalog.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []catalog.Item{}
	for _, it := range s.items {
		if it.CompanyID == companyID {
			it.Params = append([]catalog.Param(nil), it.Params...)
			out = append(out, it)
		}
	}
	sortItems(out)
	return out, nil
}

// sortItems zoradí položky podľa názvu bez ohľadu na diakritiku a veľkosť písmen.
func sortItems(list []catalog.Item) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := store.SearchKey(list[i].Name), store.SearchKey(list[j].Name)
		if a != b {
			return a < b
		}
		return list[i].ID < list[j].ID
	})
}

func (s *Store) GetItem(_ context.Context, companyID, id int64) (catalog.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok || it.CompanyID != companyID {
		return catalog.Item{}, store.ErrNotFound
	}
	it.Params = append([]catalog.Param(nil), it.Params...)
	return it, nil
}

func (s *Store) SaveItem(_ context.Context, companyID int64, it *catalog.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[int64]catalog.Item{}
	}
	if it.ID != 0 {
		if old, ok := s.items[it.ID]; !ok || old.CompanyID != companyID {
			return store.ErrNotFound
		}
	}
	if it.Code != "" {
		for _, o := range s.items {
			if o.CompanyID == companyID && o.ID != it.ID && strings.EqualFold(o.Code, it.Code) {
				return store.ErrDuplicate
			}
		}
	}
	if it.ID == 0 {
		it.ID = s.id()
	}
	it.CompanyID = companyID
	cp := *it
	cp.Params = append([]catalog.Param(nil), it.Params...)
	s.items[it.ID] = cp
	return nil
}

func (s *Store) DeleteItem(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it, ok := s.items[id]; !ok || it.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.items, id)
	for k, im := range s.images {
		if im.ItemID == id {
			delete(s.images, k)
		}
	}
	return nil
}
