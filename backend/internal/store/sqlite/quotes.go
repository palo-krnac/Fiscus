//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func scanQuote(r interface{ Scan(...any) error }) (quote.Quote, error) {
	var q quote.Quote
	var id, companyID int64
	var data string
	if err := r.Scan(&id, &companyID, &data); err != nil {
		return q, err
	}
	if err := json.Unmarshal([]byte(data), &q); err != nil {
		return q, err
	}
	q.ID, q.CompanyID = id, companyID
	return q, nil
}

func (s *Store) ListQuotes(ctx context.Context, companyID int64, year int) ([]quote.Quote, error) {
	q := "SELECT id, company_id, data FROM quotes WHERE company_id = ?"
	args := []any{companyID}
	if year != 0 {
		q += " AND substr(issue_date, 1, 4) = ?"
		args = append(args, fmtYear(year))
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY issue_date DESC, id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []quote.Quote{}
	for rows.Next() {
		x, err := scanQuote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	q, err := scanQuote(s.db.QueryRowContext(ctx, "SELECT id, company_id, data FROM quotes WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return q, store.ErrNotFound
	}
	return q, err
}

func (s *Store) SaveQuote(ctx context.Context, companyID int64, q *quote.Quote) error {
	q.CompanyID = companyID
	data, err := json.Marshal(q)
	if err != nil {
		return err
	}
	issue := ""
	if !q.IssueDate.IsZero() {
		issue = q.IssueDate.String()
	}
	if q.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO quotes (company_id, number, status, issue_date, data) VALUES (?, ?, ?, ?, ?)",
			companyID, q.Number, string(q.Status), issue, string(data))
		if err != nil {
			return err
		}
		q.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE quotes SET number = ?, status = ?, issue_date = ?, data = ? WHERE id = ? AND company_id = ?",
		q.Number, string(q.Status), issue, string(data), q.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
