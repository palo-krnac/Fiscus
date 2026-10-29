//go:build sqlite

package sqlite

import (
	"context"
	"encoding/json"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListJournalOverrides(ctx context.Context, companyID int64) (map[string]store.JournalOverride, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT source_key, lines_json, note FROM journal_overrides WHERE company_id = ?", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]store.JournalOverride{}
	for rows.Next() {
		var o store.JournalOverride
		var lines string
		if err := rows.Scan(&o.Key, &lines, &o.Note); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(lines), &o.Lines); err != nil {
			return nil, err
		}
		out[o.Key] = o
	}
	return out, rows.Err()
}

func (s *Store) SaveJournalOverride(ctx context.Context, companyID int64, o store.JournalOverride) error {
	lines, err := json.Marshal(o.Lines)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO journal_overrides (company_id, source_key, lines_json, note) VALUES (?, ?, ?, ?)
		ON CONFLICT (company_id, source_key) DO UPDATE SET lines_json = excluded.lines_json, note = excluded.note`,
		companyID, o.Key, string(lines), o.Note)
	return err
}

func (s *Store) DeleteJournalOverride(ctx context.Context, companyID int64, key string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM journal_overrides WHERE company_id = ? AND source_key = ?", companyID, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
