//go:build sqlite

package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/sqlite"
)

func init() { openSQLite = openWithBackups }

// verifier overí súbor databázy pred obnovou (databáza ešte nie je otvorená).
type verifier struct{}

func (verifier) Snapshot(context.Context, string) error { return backup.ErrNoBackup }
func (verifier) Verify(ctx context.Context, path string) error {
	return (&sqlite.Store{}).Verify(ctx, path)
}

// openWithBackups otvorí SQLite a ak je zadaný priečinok záloh, zálohuje
// pri štarte, každých 24 hodín a pri ukončení programu. Pred otvorením
// vykoná obnovu zo zálohy, ak bola naplánovaná v programe alebo zadaná
// parametrom -restore.
func openWithBackups(path, backupDir, restoreFile string) (store.Store, *backup.Manager, string, func(), error) {
	pw := os.Getenv("FISKUS_BACKUP_PASSWORD")
	var m *backup.Manager
	off := ""
	switch {
	case backupDir == "":
		off = "Zálohy sú vypnuté. Spustite Fiškus s parametrom -backup-dir <priečinok> a heslom v premennej FISKUS_BACKUP_PASSWORD."
	case pw == "":
		off = "Chýba heslo záloh v premennej FISKUS_BACKUP_PASSWORD — bez neho sa zálohy nevytvárajú."
		log.Print("upozornenie: FISKUS_BACKUP_PASSWORD nie je nastavené, zálohy sú vypnuté")
	default:
		m = &backup.Manager{Dir: backupDir, Password: pw, Policy: backup.DefaultPolicy, Source: verifier{}}
	}

	ctx := context.Background()
	if restoreFile != "" {
		if pw == "" {
			return nil, nil, "", nil, errNoPassword
		}
		rm := &backup.Manager{Password: pw, Source: verifier{}}
		if err := rm.Restore(ctx, restoreFile, path); err != nil {
			return nil, nil, "", nil, err
		}
		log.Printf("obnovené zo zálohy %s (pôvodná databáza: %s.pred-obnovou)", restoreFile, path)
	} else if m != nil {
		if b, ok, err := m.ApplyPending(ctx, path); err != nil {
			return nil, nil, "", nil, err
		} else if ok {
			log.Printf("obnovené zo zálohy %s (pôvodná databáza: %s.pred-obnovou)", b.Path, path)
		}
	}

	s, err := sqlite.Open(path)
	if err != nil {
		return nil, nil, "", nil, err
	}
	if m == nil {
		return s, nil, off, func() {}, nil
	}
	m.Source = s
	run := func(reason string) {
		info, err := m.Create(ctx, time.Now())
		if err != nil {
			log.Printf("záloha (%s) zlyhala: %v", reason, err)
			return
		}
		log.Printf("záloha (%s): %s", reason, info.Path)
	}
	run("štart")
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				run("denná")
			case <-done:
				return
			}
		}
	}()
	return s, m, "", func() { close(done); run("ukončenie") }, nil
}
