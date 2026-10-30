package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

// davServer je najjednoduchší WebDAV (PUT, GET, DELETE, PROPFIND) s heslom.
type davServer struct {
	mu    sync.Mutex
	files map[string][]byte
	pass  string
}

func (d *davServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if u, p, ok := r.BasicAuth(); !ok || u != "jana" || p != d.pass {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		d.files[path.Base(r.URL.Path)] = b
		w.WriteHeader(http.StatusCreated)
	case http.MethodGet:
		b, ok := d.files[path.Base(r.URL.Path)]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(b)
	case http.MethodDelete:
		delete(d.files, path.Base(r.URL.Path))
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`)
		for n, b := range d.files {
			fmt.Fprintf(w, `<d:response><d:href>/dav/%s</d:href><d:propstat><d:prop><d:getcontentlength>%d</d:getcontentlength></d:prop></d:propstat></d:response>`, n, len(b))
		}
		fmt.Fprint(w, `</d:multistatus>`)
	}
}

func TestBackupRemote(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	local := as(auth.Local)
	root := t.TempDir()
	dir := filepath.Join(root, "zalohy")
	cfgPath := filepath.Join(root, "fiskus-zalohy.json")
	if err := backup.SaveConfig(cfgPath, backup.Config{Password: "dlhé heslo"}); err != nil {
		t.Fatal(err)
	}
	dav := &davServer{files: map[string][]byte{}, pass: "tajne-dav"}
	srv := httptest.NewServer(dav)
	defer srv.Close()
	a.Backups = &backup.Manager{Dir: dir, Password: "dlhé heslo", Policy: backup.DefaultPolicy, Source: fakeDB{}, Client: srv.Client()}
	a.BackupSetup = &BackupSetup{ConfigPath: cfgPath, DefaultDir: dir}
	if _, err := a.CreateBackup(local); err != nil {
		t.Fatal(err)
	}

	st, err := a.BackupStatus(local)
	if err != nil || st.Remote.Kind != "" {
		t.Fatalf("bez cloudu: %+v %v", st.Remote, err)
	}
	if _, err := a.RemoteBackups(local); !errors.Is(err, backup.ErrNoRemote) {
		t.Errorf("zoznam bez cloudu: %v", err)
	}

	good := BackupRemoteRequest{Kind: "webdav", URL: srv.URL + "/dav", User: "jana", Secret: "tajne-dav"}
	tests := []struct {
		name  string
		mod   func(*BackupRemoteRequest)
		field string
	}{
		{"neznámy druh", func(r *BackupRemoteRequest) { r.Kind = "ftp" }, "kind"},
		{"http mimo localhost", func(r *BackupRemoteRequest) { r.URL = "http://cloud.example.com/dav" }, "url"},
		{"bez používateľa", func(r *BackupRemoteRequest) { r.User = "" }, "user"},
		{"bez hesla", func(r *BackupRemoteRequest) { r.Secret = "" }, "secret"},
		{"S3 bez bucketu", func(r *BackupRemoteRequest) { r.Kind = "s3"; r.Region = "eu-central-1" }, "bucket"},
		{"zlé heslo", func(r *BackupRemoteRequest) { r.Secret = "zle" }, "url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := good
			tt.mod(&req)
			_, err := a.SetBackupRemote(local, req)
			if !backupField(err, tt.field) {
				t.Fatalf("čakám chybu poľa %s: %v", tt.field, err)
			}
			if strings.Contains(fmt.Sprint(err), "tajne-dav") {
				t.Errorf("chyba prezrádza heslo: %v", err)
			}
		})
	}
	if a.Backups.RemoteConfig().Enabled() {
		t.Fatal("chybné nastavenie sa uložilo")
	}

	// iba správca inštalácie
	if _, err := a.SetBackupRemote(as(auth.Actor{UserID: 5, Name: "Jana", Channel: auth.ChannelWeb}), good); !errors.Is(err, ErrForbidden) {
		t.Errorf("bežný používateľ: %v", err)
	}

	st, err = a.SetBackupRemote(local, good)
	if err != nil {
		t.Fatal(err)
	}
	if st.Remote.Kind != "webdav" || !st.Remote.SecretSet || st.Remote.State.Count != 1 || st.Remote.State.LastError != "" {
		t.Fatalf("po nastavení: %+v", st.Remote)
	}
	if b, _ := json.Marshal(st); strings.Contains(string(b), "tajne-dav") {
		t.Fatal("stav záloh vracia heslo cloudu")
	}
	cfg, err := backup.LoadConfig(cfgPath)
	if err != nil || cfg.Password != "dlhé heslo" || cfg.Remote.Secret != "tajne-dav" {
		t.Fatalf("uložené nastavenie: %v", err)
	}
	if fi, _ := os.Stat(cfgPath); fi.Mode().Perm() != 0o600 {
		t.Errorf("práva súboru %v", fi.Mode().Perm())
	}

	// zmena bez hesla ponechá uložené heslo; zmena hesla záloh ponechá cloud
	if _, err := a.SetBackupRemote(local, BackupRemoteRequest{Kind: "webdav", URL: srv.URL + "/dav/", User: "jana"}); err != nil {
		t.Fatalf("ponechanie hesla: %v", err)
	}
	if _, err := a.SetBackupSettings(local, BackupSettingsRequest{Dir: dir, Password: "nové dlhé heslo"}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := backup.LoadConfig(cfgPath); cfg.Remote.Kind != "webdav" || cfg.Remote.Secret != "tajne-dav" {
		t.Fatalf("zmena hesla záloh zmazala cloud: %+v", cfg.Remote.Kind)
	}

	list, err := a.RemoteBackups(local)
	if err != nil || len(list) == 0 {
		t.Fatalf("zoznam v cloude: %v %v", list, err)
	}
	// stiahnutie zálohy z cloudu (napr. po strate disku)
	for _, b := range list {
		os.Remove(filepath.Join(dir, b.Name))
	}
	it, err := a.FetchRemoteBackup(local, list[0].Name)
	if err != nil || it.Name != list[0].Name {
		t.Fatalf("stiahnutie: %+v %v", it, err)
	}
	if _, err := os.Stat(filepath.Join(dir, it.Name)); err != nil {
		t.Error(err)
	}

	// výpadok cloudu: ručná záloha prejde, chyba je v stave
	dav.mu.Lock()
	dav.pass = "zmenene"
	dav.mu.Unlock()
	if _, err := a.CreateBackup(local); err != nil {
		t.Fatalf("lokálna záloha pri výpadku cloudu: %v", err)
	}
	st, _ = a.BackupStatus(local)
	if !strings.Contains(st.Remote.State.LastError, "prístup zamietnutý") {
		t.Errorf("stav po výpadku: %+v", st.Remote.State)
	}
	if _, err := a.RemoteBackups(local); !errors.Is(err, ErrCloud) {
		t.Errorf("zoznam pri výpadku: %v", err)
	}

	// vypnutie
	st, err = a.SetBackupRemote(local, BackupRemoteRequest{})
	if err != nil || st.Remote.Kind != "" {
		t.Fatalf("vypnutie: %+v %v", st.Remote, err)
	}
	if cfg, _ := backup.LoadConfig(cfgPath); cfg.Remote.Enabled() || cfg.Remote.Secret != "" || cfg.Password == "" {
		t.Fatalf("po vypnutí: kind=%q", cfg.Remote.Kind)
	}
}
