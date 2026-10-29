//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

const bankCols = `id, company_id, account, date, amount, currency, direction, vs, ss, ks,
	counter_name, counter_iban, message, bank_ref, fingerprint, status, match_json, note`

func scanBankTx(r interface{ Scan(...any) error }) (bank.Transaction, error) {
	var t bank.Transaction
	var date, match string
	var amount int64
	err := r.Scan(&t.ID, &t.CompanyID, &t.Account, &date, &amount, &t.Currency, &t.Direction, &t.VS, &t.SS, &t.KS,
		&t.CounterName, &t.CounterIBAN, &t.Message, &t.BankRef, &t.Fingerprint, &t.Status, &match, &t.Note)
	if err != nil {
		return t, err
	}
	t.Amount = money.FromRaw(amount)
	if t.Date, err = invoice.ParseDate(date); err != nil {
		return t, err
	}
	if match != "" {
		t.Match = &bank.Match{}
		if err := json.Unmarshal([]byte(match), t.Match); err != nil {
			return t, err
		}
	}
	return t, nil
}

func matchJSON(m *bank.Match) (string, error) {
	if m == nil {
		return "", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func (s *Store) SaveBankTransactions(ctx context.Context, companyID int64, txs []bank.Transaction) ([]bank.Transaction, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var out []bank.Transaction
	dup := 0
	for _, t := range txs {
		if t.Status == "" {
			t.Status = bank.StatusOpen
		}
		m, err := matchJSON(t.Match)
		if err != nil {
			return nil, 0, err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO bank_transactions (company_id, account, date, amount, currency, direction,
			vs, ss, ks, counter_name, counter_iban, message, bank_ref, fingerprint, status, match_json, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(company_id, fingerprint) DO NOTHING`,
			companyID, t.Account, t.Date.String(), t.Amount.Raw(), t.Currency, t.Direction, t.VS, t.SS, t.KS,
			t.CounterName, t.CounterIBAN, t.Message, t.BankRef, t.Fingerprint, t.Status, m, t.Note)
		if err != nil {
			return nil, 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			dup++
			continue
		}
		t.ID, _ = res.LastInsertId()
		t.CompanyID = companyID
		out = append(out, t)
	}
	return out, dup, tx.Commit()
}

func (s *Store) ListBankTransactions(ctx context.Context, companyID int64, f store.BankFilter) ([]bank.Transaction, error) {
	q := "SELECT " + bankCols + " FROM bank_transactions WHERE company_id = ?"
	args := []any{companyID}
	if f.Year != 0 {
		q += " AND substr(date, 1, 4) = ?"
		args = append(args, fmtYear(f.Year))
	}
	if f.Status != "" {
		q += " AND status = ?"
		args = append(args, f.Status)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY date DESC, id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []bank.Transaction{}
	for rows.Next() {
		t, err := scanBankTx(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func fmtYear(y int) string {
	b := []byte{'0', '0', '0', '0'}
	for i := 3; i >= 0; i-- {
		b[i] = byte('0' + y%10)
		y /= 10
	}
	return string(b)
}

func (s *Store) GetBankTransaction(ctx context.Context, companyID, id int64) (bank.Transaction, error) {
	t, err := scanBankTx(s.db.QueryRowContext(ctx,
		"SELECT "+bankCols+" FROM bank_transactions WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return t, store.ErrNotFound
	}
	return t, err
}

func (s *Store) UpdateBankTransaction(ctx context.Context, companyID int64, t bank.Transaction) error {
	m, err := matchJSON(t.Match)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE bank_transactions SET status = ?, match_json = ?, note = ?
		WHERE id = ? AND company_id = ?`, t.Status, m, t.Note, t.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
