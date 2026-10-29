//go:build sqlite

// Package sqlite je úložisko nad SQLite (ovládač modernc.org/sqlite — čisté Go,
// bez C kompilátora, takže sa dá krížovo kompilovať pre Windows/macOS/Linux).
//
// Kompiluje sa s tagom: go build -tags sqlite
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

//go:embed schema.sql
var schemaV1 string

//go:embed schema_v2.sql
var schemaV2 string

//go:embed schema_v3.sql
var schemaV3 string

//go:embed schema_v4.sql
var schemaV4 string

//go:embed schema_v5.sql
var schemaV5 string

//go:embed schema_v6.sql
var schemaV6 string

//go:embed schema_v7.sql
var schemaV7 string

//go:embed schema_v8.sql
var schemaV8 string

//go:embed schema_v9.sql
var schemaV9 string

//go:embed schema_v10.sql
var schemaV10 string

//go:embed schema_v11.sql
var schemaV11 string

//go:embed schema_v12.sql
var schemaV12 string

//go:embed schema_v13.sql
var schemaV13 string

//go:embed schema_v14.sql
var schemaV14 string

//go:embed schema_v15.sql
var schemaV15 string

//go:embed schema_v16.sql
var schemaV16 string

//go:embed schema_v17.sql
var schemaV17 string

//go:embed schema_v18.sql
var schemaV18 string

//go:embed schema_v19.sql
var schemaV19 string

//go:embed schema_v20.sql
var schemaV20 string

//go:embed schema_v21.sql
var schemaV21 string

//go:embed schema_v22.sql
var schemaV22 string

// migrations[i] prevedie schému z verzie i na i+1. Existujúce migrácie sa
// nikdy nemenia — zmena schémy je vždy nová migrácia na konci.
var migrations = []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12, schemaV13, schemaV14, schemaV15, schemaV16, schemaV17, schemaV18, schemaV19, schemaV20, schemaV21, schemaV22}

// Store je SQLite úložisko.
type Store struct{ db *sql.DB }

// Open otvorí (alebo vytvorí) databázu a spustí migrácie.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite má jedného zapisovateľa; jedno spojenie vylúči konflikty zámkov.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	var v int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("databáza je z novšej verzie programu (schéma %d)", v)
	}
	for i := v; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrácia %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// ---- firmy ----

const companyCols = "id, name, street, city, zip, ico, dic, icdph, iban, email, phone, vat_payer, default_due_days, units_json, number_formats_json, tax_office, vat_period"

func scanCompany(r interface{ Scan(...any) error }) (store.Company, error) {
	var c store.Company
	var units, formats string
	err := r.Scan(&c.ID, &c.Name, &c.Street, &c.City, &c.Zip, &c.ICO, &c.DIC, &c.ICDPH, &c.IBAN,
		&c.Email, &c.Phone, &c.VATPayer, &c.DefaultDueDays, &units, &formats, &c.TaxOffice, &c.VATPeriod)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal([]byte(units), &c.Units); err != nil {
		return c, fmt.Errorf("merné jednotky: %w", err)
	}
	if err := json.Unmarshal([]byte(formats), &c.NumberFormats); err != nil {
		return c, fmt.Errorf("číselné rady: %w", err)
	}
	if len(c.Units) == 0 {
		c.Units = nil
	}
	if len(c.NumberFormats) == 0 {
		c.NumberFormats = nil
	}
	return c, nil
}

// codeLists zakóduje číselníky firmy do JSON stĺpcov.
func codeLists(c store.Company) (string, string, error) {
	units := c.Units
	if units == nil {
		units = []string{}
	}
	u, err := json.Marshal(units)
	if err != nil {
		return "", "", err
	}
	formats := c.NumberFormats
	if formats == nil {
		formats = map[string]string{}
	}
	f, err := json.Marshal(formats)
	return string(u), string(f), err
}

func (s *Store) ListCompanies(ctx context.Context) ([]store.Company, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+companyCols+" FROM companies ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.Company
	for rows.Next() {
		c, err := scanCompany(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CountCompanies(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM companies").Scan(&n)
	return n, err
}

func (s *Store) CreateCompany(ctx context.Context, c store.Company) (store.Company, error) {
	units, formats, err := codeLists(c)
	if err != nil {
		return c, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO companies (name, street, city, zip, ico, dic, icdph, iban, email, phone, vat_payer, default_due_days,
		 units_json, number_formats_json, tax_office, vat_period)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Street, c.City, c.Zip, c.ICO, c.DIC, c.ICDPH, c.IBAN, c.Email, c.Phone, c.VATPayer, c.DefaultDueDays,
		units, formats, c.TaxOffice, c.VATPeriod)
	if err != nil {
		return c, err
	}
	c.ID, err = res.LastInsertId()
	return c, err
}

func (s *Store) GetCompany(ctx context.Context, id int64) (store.Company, error) {
	c, err := scanCompany(s.db.QueryRowContext(ctx, "SELECT "+companyCols+" FROM companies WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, store.ErrNotFound
	}
	return c, err
}

func (s *Store) UpdateCompany(ctx context.Context, c store.Company) error {
	units, formats, err := codeLists(c)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE companies SET name = ?, street = ?, city = ?, zip = ?,
		ico = ?, dic = ?, icdph = ?, iban = ?, email = ?, phone = ?, vat_payer = ?, default_due_days = ?,
		units_json = ?, number_formats_json = ?, tax_office = ?, vat_period = ?
		WHERE id = ?`,
		c.Name, c.Street, c.City, c.Zip, c.ICO, c.DIC, c.ICDPH, c.IBAN, c.Email, c.Phone, c.VATPayer, c.DefaultDueDays,
		units, formats, c.TaxOffice, c.VATPeriod, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---- adresár ----

const contactCols = `id, company_id, name, street, city, zip, country, ico, dic, icdph,
	email, phone, iban, due_days, note`

func scanContact(r interface{ Scan(...any) error }) (store.Contact, error) {
	var c store.Contact
	err := r.Scan(&c.ID, &c.CompanyID, &c.Name, &c.Street, &c.City, &c.Zip, &c.Country, &c.ICO, &c.DIC,
		&c.ICDPH, &c.Email, &c.Phone, &c.IBAN, &c.DueDays, &c.Note)
	return c, err
}

func (s *Store) ListContacts(ctx context.Context, companyID int64, search string) ([]store.Contact, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+contactCols+" FROM contacts WHERE company_id = ?", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []store.Contact{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		// filtrovanie a zoradenie v Go — SQLite nevie slovenskú diakritiku
		if store.Matches(search, c.Name, c.ICO, c.City) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := store.SearchKey(out[i].Name), store.SearchKey(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, rows.Err()
}

func (s *Store) GetContact(ctx context.Context, companyID, id int64) (store.Contact, error) {
	c, err := scanContact(s.db.QueryRowContext(ctx,
		"SELECT "+contactCols+" FROM contacts WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return c, store.ErrNotFound
	}
	return c, err
}

func (s *Store) SaveContact(ctx context.Context, companyID int64, c *store.Contact) error {
	vals := []any{c.Name, c.Street, c.City, c.Zip, c.Country, c.ICO, c.DIC, c.ICDPH,
		c.Email, c.Phone, c.IBAN, c.DueDays, c.Note}
	if c.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO contacts (company_id, name, street, city, zip, country,
			ico, dic, icdph, email, phone, iban, due_days, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, append([]any{companyID}, vals...)...)
		if err != nil {
			return err
		}
		if c.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		res, err := s.db.ExecContext(ctx, `UPDATE contacts SET name = ?, street = ?, city = ?, zip = ?, country = ?,
			ico = ?, dic = ?, icdph = ?, email = ?, phone = ?, iban = ?, due_days = ?, note = ?
			WHERE id = ? AND company_id = ?`, append(vals, c.ID, companyID)...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound
		}
	}
	c.CompanyID = companyID
	return nil
}

func (s *Store) DeleteContact(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM contacts WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---- faktúry ----

const invoiceCols = `id, company_id, kind, status, number, variable_symbol,
	customer_name, customer_street, customer_city, customer_zip, customer_country,
	customer_ico, customer_dic, customer_icdph, customer_email,
	issue_date, delivery_date, due_date, paid_date,
	currency, price_mode, reverse_charge, note, summary_json, contact_id, non_vat_payer, payment_method, correction_json, supply_kind, advance_json, fx_json, payments_json`

func scanInvoice(r interface{ Scan(...any) error }) (invoice.Invoice, error) {
	var inv invoice.Invoice
	var issue, delivery, due, paid, summary, correction, advance, fxJSON, payments string
	c := &inv.Customer
	err := r.Scan(&inv.ID, &inv.CompanyID, &inv.Kind, &inv.Status, &inv.Number, &inv.VariableSymbol,
		&c.Name, &c.Street, &c.City, &c.Zip, &c.Country, &c.ICO, &c.DIC, &c.ICDPH, &c.Email,
		&issue, &delivery, &due, &paid,
		&inv.Currency, &inv.PriceMode, &inv.ReverseCharge, &inv.Note, &summary, &inv.ContactID, &inv.NonVATPayer, &inv.PaymentMethod, &correction, &inv.SupplyKind, &advance, &fxJSON, &payments)
	if err != nil {
		return inv, err
	}
	for _, d := range []struct {
		src string
		dst *invoice.Date
	}{{issue, &inv.IssueDate}, {delivery, &inv.DeliveryDate}, {due, &inv.DueDate}, {paid, &inv.PaidDate}} {
		if d.src != "" {
			if *d.dst, err = invoice.ParseDate(d.src); err != nil {
				return inv, err
			}
		}
	}
	if err := json.Unmarshal([]byte(summary), &inv.Summary); err != nil {
		return inv, err
	}
	if correction != "" {
		inv.Correction = &invoice.Correction{}
		if err := json.Unmarshal([]byte(correction), inv.Correction); err != nil {
			return inv, fmt.Errorf("údaje dobropisu: %w", err)
		}
	}
	if advance != "" {
		inv.Advance = &invoice.Advance{}
		if err := json.Unmarshal([]byte(advance), inv.Advance); err != nil {
			return inv, fmt.Errorf("údaje zálohy: %w", err)
		}
	}
	if payments != "" {
		if err := json.Unmarshal([]byte(payments), &inv.Payments); err != nil {
			return inv, fmt.Errorf("úhrady: %w", err)
		}
	}
	inv.SyncPaid()
	if fxJSON != "" {
		var f fxData
		if err := json.Unmarshal([]byte(fxJSON), &f); err != nil {
			return inv, fmt.Errorf("kurz: %w", err)
		}
		inv.FXRate, inv.FXDate, inv.FXManual, inv.Local = f.Rate, f.Date, f.Manual, f.Local
	}
	return inv, nil
}

// fxData je kurz a sumy v eurách faktúry v cudzej mene.
type fxData struct {
	Rate   money.Dec    `json:"rate"`
	Date   invoice.Date `json:"date"`
	Manual bool         `json:"manual"`
	Local  *vat.Summary `json:"local,omitempty"`
}

func (s *Store) loadLines(ctx context.Context, inv *invoice.Invoice) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT description, quantity, unit, unit_price, vat_rate, amount
		 FROM invoice_lines WHERE invoice_id = ? ORDER BY position`, inv.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	inv.Lines = nil
	for rows.Next() {
		var l invoice.Line
		var q, p, a int64
		var r int
		if err := rows.Scan(&l.Description, &q, &l.Unit, &p, &r, &a); err != nil {
			return err
		}
		l.Quantity, l.UnitPrice, l.Amount, l.Rate = money.FromRaw(q), money.FromRaw(p), money.FromRaw(a), vat.Rate(r)
		inv.Lines = append(inv.Lines, l)
	}
	return rows.Err()
}

func (s *Store) ListInvoices(ctx context.Context, companyID int64, f store.InvoiceFilter) ([]invoice.Invoice, error) {
	q := "SELECT " + invoiceCols + " FROM invoices WHERE company_id = ?"
	args := []any{companyID}
	if f.Year != 0 {
		q += " AND substr(issue_date, 1, 4) = ?"
		args = append(args, fmt.Sprintf("%04d", f.Year))
	}
	if f.Status != "" {
		q += " AND status = ?"
		args = append(args, string(f.Status))
	}
	// Hľadanie sa robí v Go: lower() v SQLite nepozná slovenskú diakritiku.
	q += " ORDER BY issue_date DESC, id DESC"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []invoice.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if !store.Matches(f.Search, inv.Number, inv.Customer.Name, inv.Customer.ICO) {
			continue
		}
		out = append(out, inv)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.loadLines(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) GetInvoice(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	inv, err := scanInvoice(s.db.QueryRowContext(ctx,
		"SELECT "+invoiceCols+" FROM invoices WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return inv, store.ErrNotFound
	}
	if err != nil {
		return inv, err
	}
	return inv, s.loadLines(ctx, &inv)
}

func (s *Store) SaveInvoice(ctx context.Context, companyID int64, inv *invoice.Invoice) error {
	summary, err := json.Marshal(inv.Summary)
	if err != nil {
		return err
	}
	correction := ""
	if inv.Correction != nil {
		b, err := json.Marshal(inv.Correction)
		if err != nil {
			return err
		}
		correction = string(b)
	}
	advance := ""
	if inv.Advance != nil {
		b, err := json.Marshal(inv.Advance)
		if err != nil {
			return err
		}
		advance = string(b)
	}
	fxJSON := ""
	if inv.Foreign() {
		b, err := json.Marshal(fxData{inv.FXRate, inv.FXDate, inv.FXManual, inv.Local})
		if err != nil {
			return err
		}
		fxJSON = string(b)
	}
	payments := ""
	if len(inv.Payments) > 0 {
		b, err := json.Marshal(inv.Payments)
		if err != nil {
			return err
		}
		payments = string(b)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	c := inv.Customer
	vals := []any{string(inv.Kind), string(inv.Status), inv.Number, inv.VariableSymbol,
		c.Name, c.Street, c.City, c.Zip, c.Country, c.ICO, c.DIC, c.ICDPH, c.Email,
		inv.IssueDate.String(), inv.DeliveryDate.String(), inv.DueDate.String(), inv.PaidDate.String(),
		inv.Currency, string(inv.PriceMode), inv.ReverseCharge, inv.Note, string(summary), inv.ContactID, inv.NonVATPayer, string(inv.PaymentMethod), correction,
		string(inv.SupplyKind), advance, fxJSON, payments}

	if inv.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO invoices (company_id, kind, status, number, variable_symbol,
			customer_name, customer_street, customer_city, customer_zip, customer_country,
			customer_ico, customer_dic, customer_icdph, customer_email,
			issue_date, delivery_date, due_date, paid_date,
			currency, price_mode, reverse_charge, note, summary_json, contact_id, non_vat_payer, payment_method, correction_json, supply_kind, advance_json, fx_json, payments_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append([]any{companyID}, vals...)...)
		if err != nil {
			return err
		}
		if inv.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE invoices SET kind = ?, status = ?, number = ?, variable_symbol = ?,
			customer_name = ?, customer_street = ?, customer_city = ?, customer_zip = ?, customer_country = ?,
			customer_ico = ?, customer_dic = ?, customer_icdph = ?, customer_email = ?,
			issue_date = ?, delivery_date = ?, due_date = ?, paid_date = ?,
			currency = ?, price_mode = ?, reverse_charge = ?, note = ?, summary_json = ?,
			contact_id = ?, non_vat_payer = ?, payment_method = ?, correction_json = ?, supply_kind = ?, advance_json = ?, fx_json = ?, payments_json = ?
			WHERE id = ? AND company_id = ?`,
			append(vals, inv.ID, companyID)...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound // neexistuje alebo patrí inej firme
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM invoice_lines WHERE invoice_id = ?", inv.ID); err != nil {
			return err
		}
	}
	for i, l := range inv.Lines {
		if _, err := tx.ExecContext(ctx, `INSERT INTO invoice_lines
			(invoice_id, position, description, quantity, unit, unit_price, vat_rate, amount)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			inv.ID, i, l.Description, l.Quantity.Raw(), l.Unit, l.UnitPrice.Raw(), int(l.Rate), l.Amount.Raw()); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	inv.CompanyID = companyID
	return nil
}

func (s *Store) NextSequence(ctx context.Context, companyID int64, series string, year int) (int, error) {
	if series == "" || year < 2000 {
		return 0, fmt.Errorf("neplatný rad %q/%d", series, year)
	}
	var v int
	err := s.db.QueryRowContext(ctx, `INSERT INTO sequences (company_id, series, year, value) VALUES (?, ?, ?, 1)
		ON CONFLICT (company_id, series, year) DO UPDATE SET value = value + 1
		RETURNING value`, companyID, series, year).Scan(&v)
	return v, err
}

// ---- zálohy (implementuje backup.Source) ----

// Snapshot vytvorí konzistentnú kópiu databázy aj počas behu programu.
func (s *Store) Snapshot(ctx context.Context, dst string) error {
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dst)
	return err
}

// Verify otvorí súbor databázy a spustí PRAGMA integrity_check.
func (s *Store) Verify(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var res string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("integrity_check: %s", res)
	}
	var v int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v == 0 {
		return errors.New("súbor neobsahuje databázu Fiskus")
	}
	return nil
}
