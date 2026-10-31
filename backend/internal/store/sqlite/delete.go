//go:build sqlite

package sqlite

import (
	"context"
	"fmt"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// DeleteCompany zmaže firmu a všetky riadky so stĺpcom company_id (tabuľky
// sa zistia zo schémy, aby nová tabuľka nezostala zabudnutá), riadky
// faktúr a nastavenia firmy (kľúče „….<id>“).
func (s *Store) DeleteCompany(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM companies WHERE id = ?", id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT m.name FROM sqlite_master m JOIN pragma_table_info(m.name) p
		WHERE m.type = 'table' AND p.name = 'company_id' AND m.name <> 'companies' ORDER BY m.name`)
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, t)
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, "DELETE FROM invoice_lines WHERE invoice_id IN (SELECT id FROM invoices WHERE company_id = ?)", id); err != nil {
		return err
	}
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q WHERE company_id = ?", t), id); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE key LIKE ?", "%."+strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM companies WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}
