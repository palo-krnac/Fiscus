package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestConfigPath(t *testing.T) {
	tests := map[string]string{
		filepath.Join("data", "fiskus.db"): filepath.Join("data", "fiskus-zalohy.json"),
		"firma":                            "firma-zalohy.json",
	}
	for in, want := range tests {
		if got := ConfigPath(in); got != want {
			t.Errorf("ConfigPath(%q) = %q, chcem %q", in, got, want)
		}
	}
}

func TestLoadSaveConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fiskus-zalohy.json")
	if c, err := LoadConfig(path); err != nil || c != (Config{}) {
		t.Fatalf("chýbajúci súbor: %+v %v", c, err)
	}
	want := Config{Dir: "/z", Password: "tajné heslo"}
	if err := SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	if c, err := LoadConfig(path); err != nil || c != want {
		t.Fatalf("načítanie: %+v %v", c, err)
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("práva %v", fi.Mode().Perm())
	}
	os.WriteFile(path, []byte("{"), 0o600)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("poškodený súbor prešiel")
	}
}

func TestCheckDir(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "subor")
	os.WriteFile(file, nil, 0o600)
	tests := []struct {
		name string
		dir  string
		ok   bool
	}{
		{"nový priečinok", filepath.Join(base, "a", "b"), true},
		{"relatívna cesta", "zalohy", false},
		{"pod súborom", filepath.Join(file, "x"), false},
	}
	for _, tt := range tests {
		if err := CheckDir(tt.dir); (err == nil) != tt.ok {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(base, "a", "b", ".fiskus-test-*")); len(left) != 0 {
		t.Fatal("ostal skúšobný súbor")
	}
}

func TestCloudFolders(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{"OneDrive - Firma", "Dropbox", filepath.Join("Google Drive", "My Drive"), "Documents"} {
		os.MkdirAll(filepath.Join(home, d), 0o700)
	}
	got := CloudFolders(home)
	want := []string{
		filepath.Join(home, "OneDrive - Firma", "Fiškus zálohy"),
		filepath.Join(home, "Dropbox", "Fiškus zálohy"),
		filepath.Join(home, "Google Drive", "My Drive", "Fiškus zálohy"),
	}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %s", i, got[i])
		}
	}
	if CloudFolders("") != nil {
		t.Fatal("bez domova")
	}
}

func TestManagerNotReady(t *testing.T) {
	m := &Manager{Policy: DefaultPolicy}
	if m.Ready() {
		t.Fatal("prázdny je pripravený")
	}
	if _, err := m.Create(t.Context(), time.Now()); err != ErrNotReady {
		t.Fatalf("%v", err)
	}
	if l, err := m.List(); err != nil || l != nil || m.PendingRestore() != "" {
		t.Fatalf("%v %v", l, err)
	}
	m.Configure(t.TempDir(), "heslo")
	if !m.Ready() || m.Folder() == "" {
		t.Fatal("po nastavení")
	}
}
