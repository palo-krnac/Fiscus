//go:build sqlite

package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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
