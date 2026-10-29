package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

type fakeDB struct{}

func (fakeDB) Snapshot(_ context.Context, dst string) error {
	return os.WriteFile(dst, []byte("DB:obsah"), 0o600)
}
func (fakeDB) Verify(_ context.Context, path string) error {
	b, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(b, []byte("DB:")) {
		return errors.New("poškodená")
	}
	return nil
}

func TestBackups(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	local := as(auth.Local)

	// vypnuté zálohy
	a.BackupsOff = "Ukážkové dáta v pamäti sa nezálohujú."
	st, err := a.BackupStatus(local)
	if err != nil || st.Enabled || st.Reason == "" {
		t.Fatalf("vypnuté: %+v %v", st, err)
	}
	if _, err := a.CreateBackup(local); !errors.Is(err, ErrBackupsOff) {
		t.Fatalf("záloha bez nastavenia: %v", err)
	}

	a.Backups = &backup.Manager{Dir: filepath.Join(t.TempDir(), "zalohy"), Password: "heslo", Policy: backup.DefaultPolicy, Source: fakeDB{}}
	it, err := a.CreateBackup(local)
	if err != nil || it.Size == 0 {
		t.Fatalf("záloha: %+v %v", it, err)
	}
	st, _ = a.BackupStatus(local)
	if !st.Enabled || len(st.Backups) != 1 || st.Backups[0].Name != it.Name {
		t.Fatalf("stav: %+v", st)
	}
	f, _, err := a.OpenBackup(local, it.Name)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(f)
	f.Close()
	if plain, err := backup.Decrypt(data, "heslo"); err != nil || string(plain) != "DB:obsah" {
		t.Fatalf("stiahnutá záloha: %v", err)
	}
	if _, _, err := a.OpenBackup(local, "../fiskus.db"); !errors.Is(err, backup.ErrNoBackup) {
		t.Fatalf("cesta mimo priečinka: %v", err)
	}
	if err := a.RequestRestore(local, it.Name); err != nil {
		t.Fatal(err)
	}
	if st, _ := a.BackupStatus(local); st.Pending != it.Name {
		t.Fatalf("naplánovaná obnova: %+v", st)
	}
	if err := a.CancelRestore(local); err != nil {
		t.Fatal(err)
	}

	// iba správca inštalácie
	user := as(auth.Actor{UserID: 5, Name: "Jana", Channel: auth.ChannelWeb})
	if _, err := a.BackupStatus(user); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bežný používateľ: %v", err)
	}
	ro := auth.Local
	ro.ReadOnly = true
	if _, err := a.CreateBackup(as(ro)); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("kľúč na čítanie: %v", err)
	}
	list, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatBackups})
	if len(list) != 4 { // záloha, stiahnutie, plán, zrušenie
		t.Fatalf("audit záloh: %d", len(list))
	}
}
