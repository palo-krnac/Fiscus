package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
)

// BackupItem je záloha v zozname.
type BackupItem struct {
	Name string    `json:"name"`
	Time time.Time `json:"time"`
	Size int64     `json:"size"`
}

// BackupStatus je stav zálohovania inštalácie.
type BackupStatus struct {
	Enabled bool          `json:"enabled"`
	Reason  string        `json:"reason,omitempty"` // prečo sú zálohy vypnuté
	Dir     string        `json:"dir,omitempty"`
	Policy  backup.Policy `json:"policy"`
	Backups []BackupItem  `json:"backups"`
	Pending string        `json:"pending,omitempty"` // záloha naplánovaná na obnovu
}

// ErrBackupsOff — zálohovanie nie je zapnuté.
var ErrBackupsOff = errors.New("zálohovanie nie je zapnuté")

func requireSuperAdmin(ctx context.Context) error {
	act := auth.ActorFrom(ctx)
	if !act.SuperAdmin && act.Channel != auth.ChannelSystem {
		return ErrForbidden
	}
	if act.ReadOnly {
		return ErrReadOnly
	}
	return nil
}

func item(i backup.Info) BackupItem {
	return BackupItem{Name: filepath.Base(i.Path), Time: i.Time, Size: i.Size}
}

// BackupStatus vráti stav záloh (iba správca inštalácie).
func (a *App) BackupStatus(ctx context.Context) (BackupStatus, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return BackupStatus{}, err
	}
	st := BackupStatus{Backups: []BackupItem{}, Reason: a.BackupsOff}
	if a.Backups == nil {
		if st.Reason == "" {
			st.Reason = "Zálohy nie sú nastavené."
		}
		return st, nil
	}
	m := a.Backups
	st.Enabled, st.Dir, st.Policy, st.Reason, st.Pending = true, m.Dir, m.Policy, "", m.PendingRestore()
	list, err := m.List()
	if err != nil {
		return st, err
	}
	for _, b := range list {
		st.Backups = append(st.Backups, item(b))
	}
	return st, nil
}

// CreateBackup vytvorí zálohu hneď.
func (a *App) CreateBackup(ctx context.Context) (BackupItem, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return BackupItem{}, err
	}
	if a.Backups == nil {
		return BackupItem{}, ErrBackupsOff
	}
	b, err := a.Backups.Create(ctx, time.Now())
	if err != nil {
		a.record(ctx, 0, audit.CatBackups, "backup_failed", "backup", 0, "Záloha zlyhala: "+err.Error(), nil, nil)
		return BackupItem{}, err
	}
	it := item(b)
	a.record(ctx, 0, audit.CatBackups, "backup", "backup", 0, "Ručná záloha "+it.Name, nil, it)
	return it, nil
}

// OpenBackup otvorí šifrovaný súbor zálohy na stiahnutie.
func (a *App) OpenBackup(ctx context.Context, name string) (*os.File, BackupItem, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return nil, BackupItem{}, err
	}
	if a.Backups == nil {
		return nil, BackupItem{}, ErrBackupsOff
	}
	b, err := a.Backups.Find(name)
	if err != nil {
		return nil, BackupItem{}, err
	}
	f, err := os.Open(b.Path)
	if err != nil {
		return nil, BackupItem{}, err
	}
	it := item(b)
	a.record(ctx, 0, audit.CatBackups, "download", "backup", 0, "Stiahnutá záloha "+it.Name, nil, nil)
	return f, it, nil
}

// RequestRestore naplánuje obnovu zo zálohy pri ďalšom spustení.
func (a *App) RequestRestore(ctx context.Context, name string) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	if a.Backups == nil {
		return ErrBackupsOff
	}
	if err := a.Backups.RequestRestore(name); err != nil {
		return err
	}
	a.record(ctx, 0, audit.CatBackups, "restore_request", "backup", 0,
		"Naplánovaná obnova zo zálohy "+name+" pri ďalšom spustení", nil, nil)
	return nil
}

// CancelRestore zruší naplánovanú obnovu.
func (a *App) CancelRestore(ctx context.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	if a.Backups == nil {
		return ErrBackupsOff
	}
	if err := a.Backups.CancelRestore(); err != nil {
		return err
	}
	a.record(ctx, 0, audit.CatBackups, "restore_cancel", "backup", 0, "Zrušená naplánovaná obnova zo zálohy", nil, nil)
	return nil
}
