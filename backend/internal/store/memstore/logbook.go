package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/logbook"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListVehicles(_ context.Context, companyID int64) ([]logbook.Vehicle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []logbook.Vehicle{}
	for _, v := range s.vehicles {
		if v.CompanyID == companyID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) GetVehicle(_ context.Context, companyID, id int64) (logbook.Vehicle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.vehicles[id]
	if !ok || v.CompanyID != companyID {
		return logbook.Vehicle{}, store.ErrNotFound
	}
	return v, nil
}

func (s *Store) SaveVehicle(_ context.Context, companyID int64, v *logbook.Vehicle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vehicles == nil {
		s.vehicles = map[int64]logbook.Vehicle{}
	}
	if v.ID == 0 {
		v.ID = s.id()
	} else if old, ok := s.vehicles[v.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	v.CompanyID = companyID
	s.vehicles[v.ID] = *v
	return nil
}

func (s *Store) DeleteVehicle(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.vehicles[id]
	if !ok || v.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.vehicles, id)
	return nil
}

func (s *Store) ListTrips(_ context.Context, companyID, vehicleID int64, year int) ([]logbook.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []logbook.Trip{}
	for _, t := range s.trips {
		if t.CompanyID == companyID && (vehicleID == 0 || t.VehicleID == vehicleID) && (year == 0 || t.Date.Year() == year) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetTrip(_ context.Context, companyID, id int64) (logbook.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.trips[id]
	if !ok || t.CompanyID != companyID {
		return logbook.Trip{}, store.ErrNotFound
	}
	return t, nil
}

func (s *Store) SaveTrip(_ context.Context, companyID int64, t *logbook.Trip) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trips == nil {
		s.trips = map[int64]logbook.Trip{}
	}
	if t.ID == 0 {
		t.ID = s.id()
	} else if old, ok := s.trips[t.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	t.CompanyID = companyID
	s.trips[t.ID] = *t
	return nil
}

func (s *Store) DeleteTrip(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.trips[id]
	if !ok || t.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.trips, id)
	return nil
}

func (s *Store) ListFuel(_ context.Context, companyID, vehicleID int64, year int) ([]logbook.Fuel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []logbook.Fuel{}
	for _, f := range s.fuel {
		if f.CompanyID == companyID && (vehicleID == 0 || f.VehicleID == vehicleID) && (year == 0 || f.Date.Year() == year) {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetFuel(_ context.Context, companyID, id int64) (logbook.Fuel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.fuel[id]
	if !ok || f.CompanyID != companyID {
		return logbook.Fuel{}, store.ErrNotFound
	}
	return f, nil
}

func (s *Store) SaveFuel(_ context.Context, companyID int64, f *logbook.Fuel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fuel == nil {
		s.fuel = map[int64]logbook.Fuel{}
	}
	if f.ID == 0 {
		f.ID = s.id()
	} else if old, ok := s.fuel[f.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	f.CompanyID = companyID
	s.fuel[f.ID] = *f
	return nil
}

func (s *Store) DeleteFuel(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.fuel[id]
	if !ok || f.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.fuel, id)
	return nil
}
