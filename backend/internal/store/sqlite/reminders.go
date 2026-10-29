//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func scanReminder(r interface{ Scan(...any) error }) (reminder.Reminder, error) {
	var x reminder.Reminder
	var id, companyID int64
	var data string
	if err := r.Scan(&id, &companyID, &data); err != nil {
		return x, err
	}
	if err := json.Unmarshal([]byte(data), &x); err != nil {
		return x, err
	}
	x.ID, x.CompanyID = id, companyID
	return x, nil
}

func (s *Store) ListReminders(ctx context.Context, companyID int64, year int) ([]reminder.Reminder, error) {
	q := "SELECT id, company_id, data FROM reminders WHERE company_id = ?"
	args := []any{companyID}
	if year != 0 {
		q += " AND substr(date, 1, 4) = ?"
		args = append(args, fmtYear(year))
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY date DESC, id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []reminder.Reminder{}
	for rows.Next() {
		x, err := scanReminder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetReminder(ctx context.Context, companyID, id int64) (reminder.Reminder, error) {
	x, err := scanReminder(s.db.QueryRowContext(ctx, "SELECT id, company_id, data FROM reminders WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return x, store.ErrNotFound
	}
	return x, err
}

func (s *Store) SaveReminder(ctx context.Context, companyID int64, r *reminder.Reminder) error {
	r.CompanyID = companyID
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if r.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO reminders (company_id, number, date, data) VALUES (?, ?, ?, ?)",
			companyID, r.Number, r.Date.String(), string(data))
		if err != nil {
			return err
		}
		r.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE reminders SET number = ?, date = ?, data = ? WHERE id = ? AND company_id = ?",
		r.Number, r.Date.String(), string(data), r.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteReminder(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM reminders WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
