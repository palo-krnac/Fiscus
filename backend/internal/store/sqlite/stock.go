//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func scanStockDoc(r interface{ Scan(...any) error }) (stock.Doc, error) {
	var d stock.Doc
	var id, cid int64
	var raw string
	if err := r.Scan(&id, &cid, &raw); err != nil {
		return d, err
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return d, err
	}
	d.ID, d.CompanyID = id, cid
	if d.Lines == nil {
		d.Lines = []stock.Line{}
	}
	return d, nil
}

func (s *Store) ListStockDocs(ctx context.Context, companyID int64) ([]stock.Doc, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, company_id, data_json FROM stock_docs WHERE company_id = ? ORDER BY date, id", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []stock.Doc{}
	for rows.Next() {
		d, err := scanStockDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetStockDoc(ctx context.Context, companyID, id int64) (stock.Doc, error) {
	d, err := scanStockDoc(s.db.QueryRowContext(ctx, "SELECT id, company_id, data_json FROM stock_docs WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return d, store.ErrNotFound
	}
	return d, err
}

func (s *Store) SaveStockDoc(ctx context.Context, companyID int64, d *stock.Doc) error {
	d.CompanyID = companyID
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if d.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO stock_docs (company_id, number, kind, date, data_json) VALUES (?, ?, ?, ?, ?)",
			companyID, d.Number, string(d.Kind), d.Date.String(), string(raw))
		if err != nil {
			return err
		}
		d.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE stock_docs SET number = ?, kind = ?, date = ?, data_json = ? WHERE id = ? AND company_id = ?",
		d.Number, string(d.Kind), d.Date.String(), string(raw), d.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteStockDoc(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM stock_docs WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
