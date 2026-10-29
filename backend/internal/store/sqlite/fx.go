//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) SaveRates(ctx context.Context, rates []fx.Rate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rates {
		// kurz z ECB neprepíše ručne zadaný
		if _, err := tx.ExecContext(ctx, `INSERT INTO fx_rates (currency, date, rate, source) VALUES (?, ?, ?, ?)
			ON CONFLICT(currency, date) DO UPDATE SET rate = excluded.rate, source = excluded.source
			WHERE fx_rates.source != 'manual' OR excluded.source = 'manual'`,
			r.Currency, r.Date, r.Rate.Raw(), r.Source); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanRate(r interface{ Scan(...any) error }) (fx.Rate, error) {
	var x fx.Rate
	var raw int64
	err := r.Scan(&x.Currency, &x.Date, &raw, &x.Source)
	x.Rate = money.FromRaw(raw)
	return x, err
}

func (s *Store) RateBefore(ctx context.Context, currency, taxPoint string) (fx.Rate, error) {
	x, err := scanRate(s.db.QueryRowContext(ctx,
		"SELECT currency, date, rate, source FROM fx_rates WHERE currency = ? AND date < ? ORDER BY date DESC LIMIT 1", currency, taxPoint))
	if errors.Is(err, sql.ErrNoRows) {
		return x, store.ErrNotFound
	}
	return x, err
}

func (s *Store) ListRates(ctx context.Context, f fx.Filter) ([]fx.Rate, error) {
	q := "SELECT currency, date, rate, source FROM fx_rates WHERE 1 = 1"
	var args []any
	if f.Currency != "" {
		q += " AND currency = ?"
		args = append(args, f.Currency)
	}
	if f.Year != 0 {
		q += " AND date >= ? AND date < ?"
		args = append(args, fmt.Sprintf("%04d-01-01", f.Year), fmt.Sprintf("%04d-01-01", f.Year+1))
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY date DESC, currency", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []fx.Rate{}
	for rows.Next() {
		x, err := scanRate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRate(ctx context.Context, currency, date string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM fx_rates WHERE currency = ? AND date = ?", currency, date)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
