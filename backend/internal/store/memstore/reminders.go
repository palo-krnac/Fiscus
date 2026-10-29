package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func cloneReminder(r reminder.Reminder) reminder.Reminder {
	r.Items = append([]reminder.Item(nil), r.Items...)
	return r
}

func (s *Store) ListReminders(_ context.Context, companyID int64, year int) ([]reminder.Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []reminder.Reminder{}
	for _, r := range s.reminders {
		if r.CompanyID == companyID && (year == 0 || r.Date.Year() == year) {
			out = append(out, cloneReminder(r))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date.After(out[j].Date)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *Store) GetReminder(_ context.Context, companyID, id int64) (reminder.Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reminders[id]
	if !ok || r.CompanyID != companyID {
		return reminder.Reminder{}, store.ErrNotFound
	}
	return cloneReminder(r), nil
}

func (s *Store) SaveReminder(_ context.Context, companyID int64, r *reminder.Reminder) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reminders == nil {
		s.reminders = map[int64]reminder.Reminder{}
	}
	if r.ID == 0 {
		r.ID = s.id()
	} else if old, ok := s.reminders[r.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	r.CompanyID = companyID
	s.reminders[r.ID] = cloneReminder(*r)
	return nil
}

func (s *Store) DeleteReminder(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.reminders[id]; !ok || r.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.reminders, id)
	return nil
}

func (s *Store) GetSetting(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.settings[key]
	if !ok {
		return "", store.ErrNotFound
	}
	return v, nil
}

func (s *Store) SetSetting(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settings == nil {
		s.settings = map[string]string{}
	}
	s.settings[key] = value
	return nil
}
