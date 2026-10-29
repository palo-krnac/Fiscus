//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

const userCols = "id, email, name, password_hash, active, super_admin, mfa_json"

func scanUser(r interface{ Scan(...any) error }) (auth.User, error) {
	var u auth.User
	var mfa string
	err := r.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Active, &u.SuperAdmin, &mfa)
	if errors.Is(err, sql.ErrNoRows) {
		return u, store.ErrNotFound
	}
	if err == nil && mfa != "" {
		err = json.Unmarshal([]byte(mfa), &u.MFA)
	}
	return u, err
}

// mfaJSON: prázdny reťazec, kým si používateľ 2FA nenastaví.
func mfaJSON(m auth.MFA) (string, error) {
	if !m.Enabled && m.Secret == "" && len(m.Recovery) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func (s *Store) CreateUser(ctx context.Context, u auth.User) (auth.User, error) {
	mfa, err := mfaJSON(u.MFA)
	if err != nil {
		return u, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (email, name, password_hash, active, super_admin, mfa_json)
		VALUES (?, ?, ?, ?, ?, ?)`, strings.TrimSpace(u.Email), u.Name, u.PasswordHash, u.Active, u.SuperAdmin, mfa)
	if isUnique(err) {
		return u, store.ErrDuplicate
	}
	if err != nil {
		return u, err
	}
	u.ID, err = res.LastInsertId()
	return u, err
}

func (s *Store) GetUser(ctx context.Context, id int64) (auth.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE id = ?", id))
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (auth.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userCols+" FROM users WHERE lower(email) = lower(?)", strings.TrimSpace(email)))
}

func (s *Store) UpdateUser(ctx context.Context, u auth.User) error {
	mfa, err := mfaJSON(u.MFA)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET email = ?, name = ?, password_hash = ?, active = ?, super_admin = ?, mfa_json = ?
		WHERE id = ?`, strings.TrimSpace(u.Email), u.Name, u.PasswordHash, u.Active, u.SuperAdmin, mfa, u.ID)
	if isUnique(err) {
		return store.ErrDuplicate
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ListUsers(ctx context.Context) ([]auth.User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userCols+" FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []auth.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) SetMembership(ctx context.Context, m auth.Membership) error {
	if _, err := s.GetUser(ctx, m.UserID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO memberships (user_id, company_id, role) VALUES (?, ?, ?)
		ON CONFLICT (user_id, company_id) DO UPDATE SET role = excluded.role`, m.UserID, m.CompanyID, string(m.Role))
	return err
}

func (s *Store) RemoveMembership(ctx context.Context, userID, companyID int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM memberships WHERE user_id = ? AND company_id = ?", userID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) listMemberships(ctx context.Context, where string, arg int64) ([]auth.Membership, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT user_id, company_id, role FROM memberships WHERE "+where+
		" ORDER BY company_id, user_id", arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []auth.Membership{}
	for rows.Next() {
		var m auth.Membership
		if err := rows.Scan(&m.UserID, &m.CompanyID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) UserMemberships(ctx context.Context, userID int64) ([]auth.Membership, error) {
	return s.listMemberships(ctx, "user_id = ?", userID)
}

func (s *Store) CompanyMembers(ctx context.Context, companyID int64) ([]auth.Membership, error) {
	return s.listMemberships(ctx, "company_id = ?", companyID)
}

func (s *Store) CreateSession(ctx context.Context, ss auth.Session) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)",
		ss.TokenHash, ss.UserID, ss.ExpiresAt)
	return err
}

func (s *Store) GetSession(ctx context.Context, hash string) (auth.Session, error) {
	var ss auth.Session
	err := s.db.QueryRowContext(ctx, "SELECT token_hash, user_id, expires_at FROM sessions WHERE token_hash = ?", hash).
		Scan(&ss.TokenHash, &ss.UserID, &ss.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ss, store.ErrNotFound
	}
	return ss, err
}

func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", hash)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// ---- audit ----

const auditCols = `id, time, company_id, user_id, user_name, channel, ip, category, action,
	entity_type, entity_id, summary, before_json, after_json, prev_hash, hash`

func scanAudit(r interface{ Scan(...any) error }) (audit.Entry, error) {
	var e audit.Entry
	var t, before, after string
	err := r.Scan(&e.ID, &t, &e.CompanyID, &e.UserID, &e.UserName, &e.Channel, &e.IP, &e.Category, &e.Action,
		&e.EntityType, &e.EntityID, &e.Summary, &before, &after, &e.PrevHash, &e.Hash)
	if err != nil {
		return e, err
	}
	if e.Time, err = time.Parse(time.RFC3339Nano, t); err != nil {
		return e, err
	}
	if before != "" {
		e.Before = json.RawMessage(before)
	}
	if after != "" {
		e.After = json.RawMessage(after)
	}
	return e, nil
}

func (s *Store) AppendAudit(ctx context.Context, e *audit.Entry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var prev string
	var lastID int64
	err = tx.QueryRowContext(ctx, "SELECT id, hash FROM audit_log ORDER BY id DESC LIMIT 1").Scan(&lastID, &prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	e.ID = lastID + 1
	e.Time = e.Time.UTC()
	audit.Seal(e, prev)
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_log (`+auditCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Time.Format(time.RFC3339Nano), e.CompanyID, e.UserID, e.UserName, string(e.Channel), e.IP,
		string(e.Category), e.Action, e.EntityType, e.EntityID, e.Summary, string(e.Before), string(e.After),
		e.PrevHash, e.Hash)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListAudit(ctx context.Context, f audit.Filter) ([]audit.Entry, int, error) {
	q := "SELECT " + auditCols + " FROM audit_log WHERE 1 = 1"
	var args []any
	if f.CompanyIDs != nil {
		if len(f.CompanyIDs) == 0 {
			return []audit.Entry{}, 0, nil
		}
		q += " AND company_id IN (?" + strings.Repeat(", ?", len(f.CompanyIDs)-1) + ")"
		for _, id := range f.CompanyIDs {
			args = append(args, id)
		}
	}
	if f.Category != "" {
		q += " AND category = ?"
		args = append(args, string(f.Category))
	}
	if f.UserID != 0 {
		q += " AND user_id = ?"
		args = append(args, f.UserID)
	}
	q += " ORDER BY id DESC"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var all []audit.Entry
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			return nil, 0, err
		}
		// čas a hľadanie bez diakritiky sa filtrujú v Go (rovnaké pravidlá ako v pamäti)
		if f.Match(e) {
			all = append(all, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return append([]audit.Entry{}, f.Page(all)...), len(all), nil
}

func (s *Store) AuditChain(ctx context.Context) ([]audit.Entry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+auditCols+" FROM audit_log ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []audit.Entry
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
