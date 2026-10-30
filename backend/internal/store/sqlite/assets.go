//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/asset"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func scanAsset(r interface{ Scan(...any) error }) (asset.Asset, error) {
	var a asset.Asset
	var id, companyID int64
	var data string
	if err := r.Scan(&id, &companyID, &data); err != nil {
		return a, err
	}
	if err := json.Unmarshal([]byte(data), &a); err != nil {
		return a, err
	}
	a.ID, a.CompanyID = id, companyID
	return a, nil
}

func (s *Store) ListAssets(ctx context.Context, companyID int64) ([]asset.Asset, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, company_id, data FROM assets WHERE company_id = ? ORDER BY acquired, id", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []asset.Asset{}
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAsset(ctx context.Context, companyID, id int64) (asset.Asset, error) {
	a, err := scanAsset(s.db.QueryRowContext(ctx, "SELECT id, company_id, data FROM assets WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return a, store.ErrNotFound
	}
	return a, err
}

func (s *Store) SaveAsset(ctx context.Context, companyID int64, a *asset.Asset) error {
	a.CompanyID = companyID
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	acquired := ""
	if !a.Acquired.IsZero() {
		acquired = a.Acquired.String()
	}
	if a.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO assets (company_id, acquired, data) VALUES (?, ?, ?)", companyID, acquired, string(data))
		if err != nil {
			return err
		}
		a.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE assets SET acquired = ?, data = ? WHERE id = ? AND company_id = ?", acquired, string(data), a.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAsset(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM assets WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
