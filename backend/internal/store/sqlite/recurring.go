//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func scanRecurring(r interface{ Scan(...any) error }) (recurring.Template, error) {
	var t recurring.Template
	var id, companyID int64
	var data string
	if err := r.Scan(&id, &companyID, &data); err != nil {
		return t, err
	}
	if err := json.Unmarshal([]byte(data), &t); err != nil {
		return t, err
	}
	t.ID, t.CompanyID = id, companyID
	return t, nil
}

func (s *Store) ListRecurring(ctx context.Context, companyID int64) ([]recurring.Template, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, company_id, data FROM recurring_invoices WHERE company_id = ? ORDER BY id", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []recurring.Template{}
	for rows.Next() {
		t, err := scanRecurring(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetRecurring(ctx context.Context, companyID, id int64) (recurring.Template, error) {
	t, err := scanRecurring(s.db.QueryRowContext(ctx,
		"SELECT id, company_id, data FROM recurring_invoices WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return t, store.ErrNotFound
	}
	return t, err
}

func (s *Store) SaveRecurring(ctx context.Context, companyID int64, t *recurring.Template) error {
	t.CompanyID = companyID
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if t.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO recurring_invoices (company_id, active, next_date, data) VALUES (?, ?, ?, ?)",
			companyID, t.Active, t.NextDate.String(), string(data))
		if err != nil {
			return err
		}
		t.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE recurring_invoices SET active = ?, next_date = ?, data = ? WHERE id = ? AND company_id = ?",
		t.Active, t.NextDate.String(), string(data), t.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteRecurring(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM recurring_invoices WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
