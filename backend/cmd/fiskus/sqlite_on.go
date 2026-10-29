//go:build sqlite

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/app"
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

// openWithBackups otvorí SQLite a zálohuje pri štarte, každých 24 hodín a
// pri ukončení programu — keď je nastavený priečinok aj heslo. Priečinok a
// heslo sa dajú zadať parametrom -backup-dir a premennou
// FISKUS_BACKUP_PASSWORD, inak v programe (súbor fiskus-zalohy.json vedľa
// databázy). Pred otvorením vykoná obnovu zo zálohy, ak bola naplánovaná v
// programe alebo zadaná parametrom -restore.
func openWithBackups(path string, o backupOpts) (store.Store, *backup.Manager, *app.BackupSetup, func(), error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	setup := &app.BackupSetup{ConfigPath: backup.ConfigPath(abs), DefaultDir: o.Dir, DirFixed: o.DirFixed, Home: o.Home}
	if setup.DefaultDir == "" {
		setup.DefaultDir = filepath.Join(filepath.Dir(abs), "zalohy")
	}
	cfg, err := backup.LoadConfig(setup.ConfigPath)
	if err != nil {
		log.Printf("nastavenie záloh %s sa nedá načítať: %v", setup.ConfigPath, err)
	}
	dir, pw := setup.DefaultDir, cfg.Password
	if !o.DirFixed && cfg.Dir != "" {
		dir = cfg.Dir
	}
	if env := os.Getenv("FISKUS_BACKUP_PASSWORD"); env != "" {
		pw, setup.PasswordFixed = env, true
	}
	m := &backup.Manager{Dir: dir, Password: pw, Policy: backup.DefaultPolicy, Source: verifier{}}
	if !m.Ready() {
		log.Print("upozornenie: heslo záloh nie je nastavené, zálohy sa nevytvárajú (nastavíte ho v programe v časti Zálohy)")
	}

	ctx := context.Background()
	if o.Restore != "" {
		if pw == "" {
			return nil, nil, nil, nil, errNoPassword
		}
		rm := &backup.Manager{Password: pw, Source: verifier{}}
		if err := rm.Restore(ctx, o.Restore, path); err != nil {
			return nil, nil, nil, nil, err
		}
		log.Printf("obnovené zo zálohy %s (pôvodná databáza: %s.pred-obnovou)", o.Restore, path)
	} else if m.Ready() {
		if b, ok, err := m.ApplyPending(ctx, path); err != nil {
			return nil, nil, nil, nil, err
		} else if ok {
			log.Printf("obnovené zo zálohy %s (pôvodná databáza: %s.pred-obnovou)", b.Path, path)
		}
	}

	s, err := sqlite.Open(path)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	m.Source = s
	run := func(reason string) {
		if !m.Ready() {
			return
		}
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
	return s, m, setup, func() { close(done); run("ukončenie") }, nil
}
