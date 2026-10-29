//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

const purchaseCols = `id, company_id, number, supplier_number, variable_symbol, status, contact_id,
	supplier_json, supplier_iban, issue_date, delivery_date, received_date, due_date, paid_date,
	payment_method, reverse_charge, no_deduction, description, note, rates_json, summary_json, supply_kind, fx_json, expense`

func scanPurchase(r interface{ Scan(...any) error }) (purchase.Purchase, error) {
	var p purchase.Purchase
	var supplier, rates, summary, fxJSON string
	var dates [5]string
	err := r.Scan(&p.ID, &p.CompanyID, &p.Number, &p.SupplierNumber, &p.VariableSymbol, &p.Status, &p.ContactID,
		&supplier, &p.SupplierIBAN, &dates[0], &dates[1], &dates[2], &dates[3], &dates[4],
		&p.PaymentMethod, &p.ReverseCharge, &p.NoDeduction, &p.Description, &p.Note, &rates, &summary, &p.SupplyKind, &fxJSON, &p.Expense)
	if err != nil {
		return p, err
	}
	for i, dst := range []*invoice.Date{&p.IssueDate, &p.DeliveryDate, &p.ReceivedDate, &p.DueDate, &p.PaidDate} {
		if dates[i] != "" {
			if *dst, err = invoice.ParseDate(dates[i]); err != nil {
				return p, err
			}
		}
	}
	for _, j := range []struct {
		src string
		dst any
	}{{supplier, &p.Supplier}, {rates, &p.Rates}, {summary, &p.Summary}} {
		if err := json.Unmarshal([]byte(j.src), j.dst); err != nil {
			return p, err
		}
	}
	if fxJSON != "" {
		var f purchaseFX
		if err := json.Unmarshal([]byte(fxJSON), &f); err != nil {
			return p, fmt.Errorf("kurz: %w", err)
		}
		p.Currency, p.FXRate, p.FXDate, p.FXManual, p.Local = f.Currency, f.Rate, f.Date, f.Manual, f.Local
	}
	return p, nil
}

// purchaseFX je mena, kurz a sumy v eurách prijatej faktúry v cudzej mene.
type purchaseFX struct {
	Currency string            `json:"currency"`
	Rate     money.Dec         `json:"rate"`
	Date     invoice.Date      `json:"date"`
	Manual   bool              `json:"manual"`
	Local    *purchase.Amounts `json:"local,omitempty"`
}

func (s *Store) ListPurchases(ctx context.Context, companyID int64, f store.PurchaseFilter) ([]purchase.Purchase, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+purchaseCols+" FROM purchases WHERE company_id = ?", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []purchase.Purchase{}
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		if f.Year != 0 && p.ReceivedDate.Year() != f.Year {
			continue
		}
		if store.Matches(f.Search, p.Number, p.SupplierNumber, p.Supplier.Name, p.Supplier.ICO) {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ReceivedDate != b.ReceivedDate {
			return a.ReceivedDate.After(b.ReceivedDate)
		}
		return a.ID > b.ID
	})
	return out, rows.Err()
}

func (s *Store) GetPurchase(ctx context.Context, companyID, id int64) (purchase.Purchase, error) {
	p, err := scanPurchase(s.db.QueryRowContext(ctx,
		"SELECT "+purchaseCols+" FROM purchases WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return p, store.ErrNotFound
	}
	return p, err
}

func (s *Store) SavePurchase(ctx context.Context, companyID int64, p *purchase.Purchase) error {
	supplier, err := json.Marshal(p.Supplier)
	if err != nil {
		return err
	}
	rates := p.Rates
	if rates == nil {
		rates = []purchase.RateRow{}
	}
	ratesJSON, err := json.Marshal(rates)
	if err != nil {
		return err
	}
	summary, err := json.Marshal(p.Summary)
	if err != nil {
		return err
	}
	fxJSON := ""
	if p.Foreign() {
		b, err := json.Marshal(purchaseFX{p.Currency, p.FXRate, p.FXDate, p.FXManual, p.Local})
		if err != nil {
			return err
		}
		fxJSON = string(b)
	}
	vals := []any{p.Number, p.SupplierNumber, p.VariableSymbol, string(p.Status), p.ContactID,
		string(supplier), p.SupplierIBAN, p.IssueDate.String(), p.DeliveryDate.String(), p.ReceivedDate.String(),
		p.DueDate.String(), p.PaidDate.String(), string(p.PaymentMethod), p.ReverseCharge, p.NoDeduction,
		p.Description, p.Note, string(ratesJSON), string(summary), string(p.SupplyKind), fxJSON, string(p.Expense)}
	if p.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO purchases (company_id, number, supplier_number, variable_symbol,
			status, contact_id, supplier_json, supplier_iban, issue_date, delivery_date, received_date, due_date,
			paid_date, payment_method, reverse_charge, no_deduction, description, note, rates_json, summary_json, supply_kind, fx_json, expense)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, append([]any{companyID}, vals...)...)
		if err != nil {
			return err
		}
		if p.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		res, err := s.db.ExecContext(ctx, `UPDATE purchases SET number = ?, supplier_number = ?, variable_symbol = ?,
			status = ?, contact_id = ?, supplier_json = ?, supplier_iban = ?, issue_date = ?, delivery_date = ?,
			received_date = ?, due_date = ?, paid_date = ?, payment_method = ?, reverse_charge = ?, no_deduction = ?,
			description = ?, note = ?, rates_json = ?, summary_json = ?, supply_kind = ?, fx_json = ?, expense = ?
			WHERE id = ? AND company_id = ?`, append(vals, p.ID, companyID)...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound
		}
	}
	p.CompanyID = companyID
	return nil
}

func (s *Store) DeletePurchase(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM purchases WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
