//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) GetLogo(ctx context.Context, companyID int64) (store.Logo, error) {
	var l store.Logo
	var updated string
	err := s.db.QueryRowContext(ctx, "SELECT content_type, data, updated FROM company_logos WHERE company_id = ?", companyID).
		Scan(&l.ContentType, &l.Data, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return l, store.ErrNotFound
	}
	if err != nil {
		return l, err
	}
	l.Updated, err = time.Parse(time.RFC3339Nano, updated)
	return l, err
}

func (s *Store) SaveLogo(ctx context.Context, companyID int64, l store.Logo) error {
	if _, err := s.GetCompany(ctx, companyID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO company_logos (company_id, content_type, data, updated) VALUES (?, ?, ?, ?)
		ON CONFLICT(company_id) DO UPDATE SET content_type = excluded.content_type, data = excluded.data, updated = excluded.updated`,
		companyID, l.ContentType, l.Data, l.Updated.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) DeleteLogo(ctx context.Context, companyID int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM company_logos WHERE company_id = ?", companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
