//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/storetest"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "fiskus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) store.Store { return open(t) })
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fiskus.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCompany(context.Background(), store.Company{Name: "Alfa"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path) // migrácia sa nesmie spustiť znova
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if n, _ := s.CountCompanies(context.Background()); n != 1 {
		t.Fatalf("po znovuotvorení %d firiem", n)
	}
}

func TestBackupRestoreRealDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fiskus.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.CreateCompany(ctx, store.Company{Name: "Pred zálohou"})

	m := &backup.Manager{Dir: filepath.Join(dir, "zalohy"), Password: "heslo", Policy: backup.DefaultPolicy, Source: s}
	info, err := m.Create(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.CreateCompany(ctx, store.Company{Name: "Po zálohe"})
	s.Close()

	if err := m.Restore(ctx, info.Path, dbPath); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, _ := s.ListCompanies(ctx)
	if len(list) != 1 || list[0].Name != "Pred zálohou" {
		t.Fatalf("po obnove: %+v", list)
	}
}

// Databáza vytvorená staršou verziou programu sa pri otvorení zmigruje
// a dáta zostanú zachované.
func TestUpgradeFromV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stara.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		schemaV1,
		"PRAGMA user_version = 1",
		"INSERT INTO companies (name, vat_payer) VALUES ('Stará firma', 1)",
		`INSERT INTO invoices (company_id, kind, status, number, customer_name, issue_date)
		 VALUES (1, 'invoice', 'issued', 'FA20250001', 'Odberateľ', '2025-12-01')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := s.GetCompany(ctx, 1)
	if err != nil || c.Name != "Stará firma" || c.DefaultDueDays != 14 {
		t.Fatalf("firma po migrácii: %+v %v", c, err)
	}
	list, err := s.ListInvoices(ctx, 1, store.InvoiceFilter{})
	if err != nil || len(list) != 1 || list[0].Number != "FA20250001" || list[0].NonVATPayer {
		t.Fatalf("faktúry po migrácii: %+v %v", list, err)
	}
	var v int
	_ = s.db.QueryRow("PRAGMA user_version").Scan(&v)
	if v != len(migrations) {
		t.Fatalf("verzia schémy %d", v)
	}
}

// Audit sa nedá zmeniť ani zmazať ani priamo v databáze (triggery).
func TestAuditIsAppendOnly(t *testing.T) {
	s := open(t)
	e := audit.New(auth.Local, 1, audit.CatInvoices, "issue", "invoice", 1, "Vystavená faktúra", nil, nil)
	if err := s.AppendAudit(context.Background(), &e); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE audit_log SET summary = 'podvrh'"); err == nil {
		t.Fatal("audit sa dal zmeniť")
	}
	if _, err := s.db.Exec("DELETE FROM audit_log"); err == nil {
		t.Fatal("audit sa dal zmazať")
	}
}
