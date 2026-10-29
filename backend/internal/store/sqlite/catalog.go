//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

const itemCols = "id, company_id, code, name, unit, price, price_mode, vat_rate, note, archived"

func scanItem(r interface{ Scan(...any) error }) (catalog.Item, error) {
	var it catalog.Item
	var price int64
	var mode string
	var rate int
	err := r.Scan(&it.ID, &it.CompanyID, &it.Code, &it.Name, &it.Unit, &price, &mode, &rate, &it.Note, &it.Archived)
	it.Price, it.PriceMode, it.Rate = money.FromRaw(price), vat.PriceMode(mode), vat.Rate(rate)
	return it, err
}

func (s *Store) ListItems(ctx context.Context, companyID int64) ([]catalog.Item, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+itemCols+" FROM catalog_items WHERE company_id = ?", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// poradie podľa názvu bez diakritiky (SQLite NOCASE nepozná slovenčinu)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := store.SearchKey(out[i].Name), store.SearchKey(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetItem(ctx context.Context, companyID, id int64) (catalog.Item, error) {
	it, err := scanItem(s.db.QueryRowContext(ctx, "SELECT "+itemCols+" FROM catalog_items WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return it, store.ErrNotFound
	}
	return it, err
}

func (s *Store) SaveItem(ctx context.Context, companyID int64, it *catalog.Item) error {
	vals := []any{it.Code, it.Name, it.Unit, it.Price.Raw(), string(it.PriceMode), int(it.Rate), it.Note, it.Archived}
	if it.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO catalog_items (company_id, code, name, unit, price, price_mode, vat_rate, note, archived)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, append([]any{companyID}, vals...)...)
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		if it.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		res, err := s.db.ExecContext(ctx, `UPDATE catalog_items SET code = ?, name = ?, unit = ?, price = ?, price_mode = ?, vat_rate = ?,
			note = ?, archived = ? WHERE id = ? AND company_id = ?`, append(vals, it.ID, companyID)...)
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound
		}
	}
	it.CompanyID = companyID
	return nil
}

func (s *Store) DeleteItem(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM catalog_items WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
