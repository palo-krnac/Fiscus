//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/logbook"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListVehicles(ctx context.Context, companyID int64) ([]logbook.Vehicle, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM vehicles WHERE company_id = ? ORDER BY id", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []logbook.Vehicle{}
	for rows.Next() {
		var v logbook.Vehicle
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return nil, err
		}
		v.ID, v.CompanyID = id, companyID
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetVehicle(ctx context.Context, companyID, id int64) (logbook.Vehicle, error) {
	var v logbook.Vehicle
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM vehicles WHERE id = ? AND company_id = ?", id, companyID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return v, store.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return v, err
	}
	v.ID, v.CompanyID = id, companyID
	return v, nil
}

func (s *Store) SaveVehicle(ctx context.Context, companyID int64, v *logbook.Vehicle) error {
	v.CompanyID = companyID
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if v.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO vehicles (company_id, data) VALUES (?, ?)", companyID, string(data))
		if err != nil {
			return err
		}
		v.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE vehicles SET data = ? WHERE id = ? AND company_id = ?", string(data), v.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteVehicle(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM vehicles WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// záznamy knihy jázd (jazdy a tankovania) v tabuľke logbook

func (s *Store) listLog(ctx context.Context, kind string, companyID, vehicleID int64, year int, scan func(id, vehicleID int64, data string) error) error {
	q := "SELECT id, vehicle_id, data FROM logbook WHERE company_id = ? AND kind = ?"
	args := []any{companyID, kind}
	if vehicleID != 0 {
		q += " AND vehicle_id = ?"
		args = append(args, vehicleID)
	}
	if year != 0 {
		q += " AND substr(date, 1, 4) = ?"
		args = append(args, fmtYear(year))
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY date, id", args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, vid int64
		var data string
		if err := rows.Scan(&id, &vid, &data); err != nil {
			return err
		}
		if err := scan(id, vid, data); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) getLog(ctx context.Context, kind string, companyID, id int64, into any) error {
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM logbook WHERE id = ? AND company_id = ? AND kind = ?", id, companyID, kind).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), into)
}

func (s *Store) saveLog(ctx context.Context, kind string, companyID int64, id *int64, vehicleID int64, date string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if *id == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO logbook (company_id, vehicle_id, kind, date, data) VALUES (?, ?, ?, ?, ?)",
			companyID, vehicleID, kind, date, string(data))
		if err != nil {
			return err
		}
		*id, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE logbook SET vehicle_id = ?, date = ?, data = ? WHERE id = ? AND company_id = ? AND kind = ?",
		vehicleID, date, string(data), *id, companyID, kind)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) deleteLog(ctx context.Context, kind string, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM logbook WHERE id = ? AND company_id = ? AND kind = ?", id, companyID, kind)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ListTrips(ctx context.Context, companyID, vehicleID int64, year int) ([]logbook.Trip, error) {
	out := []logbook.Trip{}
	err := s.listLog(ctx, "trip", companyID, vehicleID, year, func(id, vid int64, data string) error {
		var t logbook.Trip
		if err := json.Unmarshal([]byte(data), &t); err != nil {
			return err
		}
		t.ID, t.CompanyID, t.VehicleID = id, companyID, vid
		out = append(out, t)
		return nil
	})
	return out, err
}

func (s *Store) GetTrip(ctx context.Context, companyID, id int64) (logbook.Trip, error) {
	var t logbook.Trip
	if err := s.getLog(ctx, "trip", companyID, id, &t); err != nil {
		return t, err
	}
	t.ID, t.CompanyID = id, companyID
	return t, nil
}

func (s *Store) SaveTrip(ctx context.Context, companyID int64, t *logbook.Trip) error {
	t.CompanyID = companyID
	return s.saveLog(ctx, "trip", companyID, &t.ID, t.VehicleID, t.Date.String(), t)
}

func (s *Store) DeleteTrip(ctx context.Context, companyID, id int64) error {
	return s.deleteLog(ctx, "trip", companyID, id)
}

func (s *Store) ListFuel(ctx context.Context, companyID, vehicleID int64, year int) ([]logbook.Fuel, error) {
	out := []logbook.Fuel{}
	err := s.listLog(ctx, "fuel", companyID, vehicleID, year, func(id, vid int64, data string) error {
		var f logbook.Fuel
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return err
		}
		f.ID, f.CompanyID, f.VehicleID = id, companyID, vid
		out = append(out, f)
		return nil
	})
	return out, err
}

func (s *Store) GetFuel(ctx context.Context, companyID, id int64) (logbook.Fuel, error) {
	var f logbook.Fuel
	if err := s.getLog(ctx, "fuel", companyID, id, &f); err != nil {
		return f, err
	}
	f.ID, f.CompanyID = id, companyID
	return f, nil
}

func (s *Store) SaveFuel(ctx context.Context, companyID int64, f *logbook.Fuel) error {
	f.CompanyID = companyID
	return s.saveLog(ctx, "fuel", companyID, &f.ID, f.VehicleID, f.Date.String(), f)
}

func (s *Store) DeleteFuel(ctx context.Context, companyID, id int64) error {
	return s.deleteLog(ctx, "fuel", companyID, id)
}
