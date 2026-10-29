package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

func backupField(err error, field string) bool {
	var ve *invoice.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return true
		}
	}
	return false
}

func TestBackupSettings(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	local := as(auth.Local)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	os.MkdirAll(filepath.Join(home, "Dropbox"), 0o700)
	def := filepath.Join(root, "data", "zalohy")
	cfgPath := filepath.Join(root, "data", "fiskus-zalohy.json")
	os.MkdirAll(filepath.Dir(cfgPath), 0o700)
	a.Backups = &backup.Manager{Dir: def, Policy: backup.DefaultPolicy, Source: fakeDB{}}
	a.BackupSetup = &BackupSetup{ConfigPath: cfgPath, DefaultDir: def, Home: home}

	// bez hesla sa nezálohuje, formulár ponúkne Dropbox
	st, err := a.BackupStatus(local)
	if err != nil || st.Enabled || !strings.Contains(st.Reason, "heslo") || st.Settings == nil || st.Settings.PasswordSet ||
		len(st.Settings.Suggestions) != 1 || !strings.Contains(st.Settings.Suggestions[0], "Dropbox") {
		t.Fatalf("bez hesla: %+v %+v %v", st, st.Settings, err)
	}
	if _, err := a.CreateBackup(local); err == nil {
		t.Fatal("záloha bez hesla")
	}

	tests := []struct {
		name  string
		req   BackupSettingsRequest
		field string
	}{
		{"bez hesla", BackupSettingsRequest{}, "password"},
		{"krátke heslo", BackupSettingsRequest{Password: "kratke"}, "password"},
		{"medzera na konci", BackupSettingsRequest{Password: "dlhé heslo "}, "password"},
		{"relatívny priečinok", BackupSettingsRequest{Dir: "zalohy", Password: "dlhé heslo"}, "dir"},
	}
	for _, tt := range tests {
		if _, err := a.SetBackupSettings(local, tt.req); !backupField(err, tt.field) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}

	// heslo v predvolenom priečinku → hneď overovacia záloha, uložené nastavenie
	st, err = a.SetBackupSettings(local, BackupSettingsRequest{Password: "dlhé heslo"})
	if err != nil || !st.Enabled || len(st.Backups) != 1 || !st.Settings.PasswordSet || st.Dir != def {
		t.Fatalf("nastavenie hesla: %+v %v", st, err)
	}
	if cfg, _ := backup.LoadConfig(cfgPath); cfg.Password != "dlhé heslo" || cfg.Dir != "" {
		t.Fatalf("súbor: %+v", cfg)
	}

	// zmena priečinka bez zmeny hesla
	cloud := st.Settings.Suggestions[0]
	st, err = a.SetBackupSettings(local, BackupSettingsRequest{Dir: cloud})
	if err != nil || st.Dir != cloud || len(st.Backups) != 1 {
		t.Fatalf("cloud: %+v %v", st, err)
	}
	if cfg, _ := backup.LoadConfig(cfgPath); cfg.Dir != cloud || cfg.Password != "dlhé heslo" {
		t.Fatalf("súbor: %+v", cfg)
	}

	// priečinok z parametra a heslo z premennej sa v programe nemenia
	a.BackupSetup.DirFixed, a.BackupSetup.PasswordFixed = true, true
	if _, err := a.SetBackupSettings(local, BackupSettingsRequest{Dir: def}); !backupField(err, "dir") {
		t.Fatalf("pevný priečinok: %v", err)
	}
	if _, err := a.SetBackupSettings(local, BackupSettingsRequest{Password: "iné dlhé heslo"}); !backupField(err, "password") {
		t.Fatalf("pevné heslo: %v", err)
	}
	if _, err := a.SetBackupSettings(local, BackupSettingsRequest{}); err != nil {
		t.Fatalf("bez zmeny: %v", err)
	}
	if cfg, _ := backup.LoadConfig(cfgPath); cfg != (backup.Config{}) {
		t.Fatalf("pevné hodnoty sa neukladajú: %+v", cfg)
	}

	// iba správca inštalácie; heslo sa nevracia
	user := as(auth.Actor{UserID: 5, Name: "Jana", Channel: auth.ChannelWeb})
	if _, err := a.SetBackupSettings(user, BackupSettingsRequest{Password: "dlhé heslo 2"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bežný používateľ: %v", err)
	}
	a.BackupSetup = nil
	if _, err := a.SetBackupSettings(local, BackupSettingsRequest{}); !errors.Is(err, ErrBackupsOff) {
		t.Fatalf("bez nastavenia v programe: %v", err)
	}
}
