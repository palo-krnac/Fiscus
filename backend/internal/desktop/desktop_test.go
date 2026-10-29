package desktop

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDataDir(t *testing.T) {
	config := func() (string, error) { return "/cfg", nil }
	home := func() (string, error) { return "/home/palo", nil }
	tests := []struct {
		goos, xdg, want string
	}{
		{"windows", "", filepath.Join("/cfg", "Fiskus")},
		{"darwin", "/x", filepath.Join("/cfg", "Fiskus")},
		{"linux", "", filepath.Join("/home/palo", ".local", "share", "fiskus")},
		{"linux", "/data", filepath.Join("/data", "fiskus")},
		{"linux", "relatívna", filepath.Join("/home/palo", ".local", "share", "fiskus")},
	}
	for _, tt := range tests {
		got, err := dataDir(tt.goos, func(string) string { return tt.xdg }, config, home)
		if err != nil || got != tt.want {
			t.Errorf("%s %q: %s %v", tt.goos, tt.xdg, got, err)
		}
	}
	if _, err := dataDir("windows", nil, func() (string, error) { return "", errors.New("x") }, home); err == nil {
		t.Error("chyba priečinka")
	}
}

func TestOpenCommand(t *testing.T) {
	for goos, want := range map[string]string{"windows": "rundll32", "darwin": "open", "linux": "xdg-open", "freebsd": "xdg-open"} {
		c := OpenCommand(goos, "http://x")
		if c[0] != want || c[len(c)-1] != "http://x" {
			t.Errorf("%s: %v", goos, c)
		}
	}
}

func TestRunning(t *testing.T) {
	fiskus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/capabilities" {
			w.Write([]byte(`{"mode":"desktop"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer fiskus.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) }))
	defer other.Close()
	ctx := context.Background()
	if !Running(ctx, strings.TrimPrefix(fiskus.URL, "http://")) {
		t.Error("Fiškus beží")
	}
	if Running(ctx, strings.TrimPrefix(other.URL, "http://")) || Running(ctx, "127.0.0.1:1") {
		t.Error("iný program alebo nič")
	}
}

func TestWatchdog(t *testing.T) {
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	w := &Watchdog{Idle: 90 * time.Second, Start: 5 * time.Minute, Now: func() time.Time { return now }}
	steps := []struct {
		after   time.Duration
		ping    bool
		expired bool
	}{
		{2 * time.Minute, false, false}, // prehliadač sa ešte otvára
		{4 * time.Minute, true, false},  // okno sa ozvalo
		{80 * time.Second, false, false},
		{20 * time.Second, false, true}, // 100 s ticho → koniec
		{0, true, false},                // obnovené okno
	}
	for i, s := range steps {
		now = now.Add(s.after)
		if s.ping {
			w.Ping()
		}
		if got := w.Expired(); got != s.expired {
			t.Fatalf("krok %d: %v", i, got)
		}
	}
	// bez ozvania po štarte
	w2 := &Watchdog{Idle: time.Minute, Start: 5 * time.Minute, Now: func() time.Time { return now }}
	w2.init()
	now = now.Add(6 * time.Minute)
	if !w2.Expired() {
		t.Fatal("bez okna po štarte")
	}
	// Quit ukončí čakanie
	w3 := NewWatchdog()
	w3.Quit()
	w3.Quit() // druhé volanie nepanikári
	if got := w3.Wait(context.Background(), time.Hour); got != "ukončené používateľom" {
		t.Fatal(got)
	}
}
