//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) AddEInvoice(ctx context.Context, companyID int64, e *store.EInvoice) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO einvoices (company_id, direction, doc_id, doc_number, partner, peppol_id, file,
		message_id, status, error, created, user_name, data) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		companyID, e.Direction, e.DocID, e.DocNumber, e.Partner, e.PeppolID, e.File, e.MessageID, e.Status, e.Error,
		e.Created.UTC().Format(time.RFC3339Nano), e.User, e.Data)
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	e.CompanyID = companyID
	return nil
}

const einvoiceCols = `id, company_id, direction, doc_id, doc_number, partner, peppol_id, file, message_id, status, error, created, user_name`

func scanEInvoice(r interface{ Scan(...any) error }, extra ...any) (store.EInvoice, error) {
	var e store.EInvoice
	var created string
	dst := append([]any{&e.ID, &e.CompanyID, &e.Direction, &e.DocID, &e.DocNumber, &e.Partner, &e.PeppolID, &e.File,
		&e.MessageID, &e.Status, &e.Error, &created, &e.User}, extra...)
	if err := r.Scan(dst...); err != nil {
		return e, err
	}
	var err error
	e.Created, err = time.Parse(time.RFC3339Nano, created)
	return e, err
}

func (s *Store) ListEInvoices(ctx context.Context, companyID int64, f store.EInvoiceFilter) ([]store.EInvoice, error) {
	q := "SELECT " + einvoiceCols + " FROM einvoices WHERE company_id = ?"
	args := []any{companyID}
	if f.Direction != "" {
		q += " AND direction = ?"
		args = append(args, f.Direction)
	}
	if f.DocID != 0 {
		q += " AND doc_id = ?"
		args = append(args, f.DocID)
	}
	if f.MessageID != "" {
		q += " AND message_id = ?"
		args = append(args, f.MessageID)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY created DESC, id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []store.EInvoice{}
	for rows.Next() {
		e, err := scanEInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetEInvoice(ctx context.Context, companyID, id int64) (store.EInvoice, error) {
	var data []byte
	e, err := scanEInvoice(s.db.QueryRowContext(ctx, "SELECT "+einvoiceCols+", data FROM einvoices WHERE id = ? AND company_id = ?", id, companyID), &data)
	if errors.Is(err, sql.ErrNoRows) {
		return e, store.ErrNotFound
	}
	e.Data = data
	return e, err
}
