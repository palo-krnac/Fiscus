//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

const cashCols = "id, company_id, number, kind, date, amount, partner, purpose, note, link_kind, link_id, link_number, cancelled"

func scanCash(r interface{ Scan(...any) error }) (cash.Document, error) {
	var d cash.Document
	var date string
	var amount int64
	err := r.Scan(&d.ID, &d.CompanyID, &d.Number, &d.Kind, &date, &amount, &d.Partner, &d.Purpose, &d.Note,
		&d.LinkKind, &d.LinkID, &d.LinkNumber, &d.Cancelled)
	if err != nil {
		return d, err
	}
	d.Amount = money.FromRaw(amount)
	d.Date, err = invoice.ParseDate(date)
	return d, err
}

func (s *Store) ListCashDocuments(ctx context.Context, companyID int64, year int) ([]cash.Document, error) {
	q := "SELECT " + cashCols + " FROM cash_documents WHERE company_id = ?"
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
	out := []cash.Document{}
	for rows.Next() {
		d, err := scanCash(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetCashDocument(ctx context.Context, companyID, id int64) (cash.Document, error) {
	d, err := scanCash(s.db.QueryRowContext(ctx, "SELECT "+cashCols+" FROM cash_documents WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return d, store.ErrNotFound
	}
	return d, err
}

func (s *Store) SaveCashDocument(ctx context.Context, companyID int64, d *cash.Document) error {
	vals := []any{d.Number, string(d.Kind), d.Date.String(), d.Amount.Raw(), d.Partner, d.Purpose, d.Note,
		d.LinkKind, d.LinkID, d.LinkNumber, d.Cancelled}
	if d.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO cash_documents (company_id, number, kind, date, amount, partner,
			purpose, note, link_kind, link_id, link_number, cancelled) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append([]any{companyID}, vals...)...)
		if err != nil {
			return err
		}
		if d.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		res, err := s.db.ExecContext(ctx, `UPDATE cash_documents SET number = ?, kind = ?, date = ?, amount = ?, partner = ?,
			purpose = ?, note = ?, link_kind = ?, link_id = ?, link_number = ?, cancelled = ? WHERE id = ? AND company_id = ?`,
			append(vals, d.ID, companyID)...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound
		}
	}
	d.CompanyID = companyID
	return nil
}
