//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

const apiKeyCols = "id, user_id, name, prefix, hash, read_only, created_at, last_used_at, revoked"

func scanAPIKey(r interface{ Scan(...any) error }) (auth.APIKey, error) {
	var k auth.APIKey
	err := r.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.Hash, &k.ReadOnly, &k.CreatedAt, &k.LastUsedAt, &k.Revoked)
	return k, err
}

func (s *Store) CreateAPIKey(ctx context.Context, k *auth.APIKey) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO api_keys (user_id, name, prefix, hash, read_only, created_at, last_used_at, revoked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, k.UserID, k.Name, k.Prefix, k.Hash, k.ReadOnly, k.CreatedAt, k.LastUsedAt, k.Revoked)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return store.ErrDuplicate
		}
		return err
	}
	k.ID, err = res.LastInsertId()
	return err
}

func (s *Store) ListAPIKeys(ctx context.Context, userID int64) ([]auth.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+apiKeyCols+" FROM api_keys WHERE user_id = ? ORDER BY id DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []auth.APIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) GetAPIKeyByHash(ctx context.Context, hash string) (auth.APIKey, error) {
	k, err := scanAPIKey(s.db.QueryRowContext(ctx, "SELECT "+apiKeyCols+" FROM api_keys WHERE hash = ?", hash))
	if errors.Is(err, sql.ErrNoRows) {
		return k, store.ErrNotFound
	}
	return k, err
}

func (s *Store) TouchAPIKey(ctx context.Context, id int64, at int64) error {
	res, err := s.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at = ? WHERE id = ?", at, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) RevokeAPIKey(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, "UPDATE api_keys SET revoked = 1 WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
