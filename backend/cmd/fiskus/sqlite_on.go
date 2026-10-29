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

// openWithBackups otvorí SQLite a ak je zadaný priečinok záloh, zálohuje
// pri štarte, každých 24 hodín a pri ukončení programu.
func openWithBackups(path, backupDir string) (store.Store, func(), error) {
	s, err := sqlite.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if backupDir == "" {
		return s, func() {}, nil
	}
	pw := os.Getenv("FISKUS_BACKUP_PASSWORD")
	if pw == "" {
		log.Print("upozornenie: FISKUS_BACKUP_PASSWORD nie je nastavené, zálohy sú vypnuté")
		return s, func() {}, nil
	}
	m := &backup.Manager{Dir: backupDir, Password: pw, Policy: backup.DefaultPolicy, Source: s}
	run := func(reason string) {
		info, err := m.Create(context.Background(), time.Now())
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
	return s, func() { close(done); run("ukončenie") }, nil
}
