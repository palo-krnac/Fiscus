//go:build sqlite

package sqlite

import (
	"context"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) AddEmail(ctx context.Context, companyID int64, e *store.Email) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO emails (company_id, doc_kind, doc_id, doc_number, to_addr, cc_addr, subject, file, sent, user_name, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		companyID, e.DocKind, e.DocID, e.DocNumber, e.To, e.Cc, e.Subject, e.File, e.Sent.UTC().Format(time.RFC3339Nano), e.User, e.Error)
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	e.CompanyID = companyID
	return nil
}

func (s *Store) ListEmails(ctx context.Context, companyID int64, f store.EmailFilter) ([]store.Email, error) {
	q := `SELECT id, company_id, doc_kind, doc_id, doc_number, to_addr, cc_addr, subject, file, sent, user_name, error
		FROM emails WHERE company_id = ?`
	args := []any{companyID}
	if f.Year != 0 {
		// čas je v UTC; rok sa porovná v rozsahu, aby sedel aj okolo polnoci
		q += " AND sent >= ? AND sent < ?"
		args = append(args, time.Date(f.Year, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			time.Date(f.Year+1, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano))
	}
	if f.DocKind != "" {
		q += " AND doc_kind = ?"
		args = append(args, f.DocKind)
	}
	if f.DocID != 0 {
		q += " AND doc_id = ?"
		args = append(args, f.DocID)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY sent DESC, id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []store.Email{}
	for rows.Next() {
		var e store.Email
		var sent string
		if err := rows.Scan(&e.ID, &e.CompanyID, &e.DocKind, &e.DocID, &e.DocNumber, &e.To, &e.Cc, &e.Subject, &e.File, &sent, &e.User, &e.Error); err != nil {
			return nil, err
		}
		if e.Sent, err = time.Parse(time.RFC3339Nano, sent); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
