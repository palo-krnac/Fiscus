// Package desktop je spúšťač desktopovej verzie z inštalátora: dáta
// v priečinku používateľa, otvorenie prehliadača, jedna bežiaca inštancia
// a ukončenie programu po zatvorení posledného okna.
package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Addr je predvolená adresa desktopovej verzie spustenej z inštalátora
// (menej obsadený port než 8080, ktorý používajú vývojárske nástroje).
const Addr = "127.0.0.1:48080"

// DataDir vráti priečinok s dátami používateľa:
// Windows %APPDATA%\Fiskus, macOS ~/Library/Application Support/Fiskus,
// Linux $XDG_DATA_HOME/fiskus (predvolene ~/.local/share/fiskus).
func DataDir() (string, error) {
	return dataDir(runtime.GOOS, os.Getenv, os.UserConfigDir, os.UserHomeDir)
}

func dataDir(goos string, env func(string) string, config, home func() (string, error)) (string, error) {
	if goos == "windows" || goos == "darwin" {
		d, err := config()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "Fiskus"), nil
	}
	// XDG cesty sú unixové: path.IsAbs, aby sa správanie dalo overiť aj na Windows
	if x := env("XDG_DATA_HOME"); x != "" && path.IsAbs(x) {
		return filepath.Join(x, "fiskus"), nil
	}
	h, err := home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".local", "share", "fiskus"), nil
}

// OpenCommand vráti príkaz, ktorým systém otvorí adresu v predvolenom prehliadači.
func OpenCommand(goos, url string) []string {
	switch goos {
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	case "darwin":
		return []string{"open", url}
	default:
		return []string{"xdg-open", url}
	}
}

// Open otvorí adresu v prehliadači.
func Open(url string) error {
	c := OpenCommand(runtime.GOOS, url)
	return exec.Command(c[0], c[1:]...).Start()
}

// Running zistí, či na adrese už beží Fiškus (druhé spustenie z ikony
// iba otvorí okno bežiaceho programu).
func Running(ctx context.Context, addr string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/capabilities", nil)
	if err != nil {
		return false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var c struct {
		Mode string `json:"mode"`
	}
	return res.StatusCode == http.StatusOK && json.NewDecoder(res.Body).Decode(&c) == nil && c.Mode != ""
}

// ErrBusy: port obsadil iný program.
var ErrBusy = errors.New("port je obsadený iným programom")

// Watchdog ukončí program, keď je zatvorené posledné okno: otvorená
// stránka sa ozýva (Ping) a po Idle bez ozvania sa zavolá Stop. Do prvého
// ozvania platí dlhšia lehota Start (prehliadač sa ešte otvára).
type Watchdog struct {
	Idle  time.Duration
	Start time.Duration
	Now   func() time.Time
	// Resident: program má ikonu v oblasti oznámení — po zatvorení okna
	// beží ďalej a ukončí sa iba cez Ukončiť (ikona alebo menu).
	Resident bool

	mu     sync.Mutex
	last   time.Time
	pinged bool
	quit   chan struct{}
	once   sync.Once
}

// NewWatchdog vytvorí strážcu s bežnými lehotami.
func NewWatchdog() *Watchdog {
	return &Watchdog{Idle: 90 * time.Second, Start: 5 * time.Minute, Now: time.Now}
}

func (w *Watchdog) init() {
	w.once.Do(func() {
		w.quit = make(chan struct{})
		if w.Now == nil {
			w.Now = time.Now
		}
		w.last = w.Now()
	})
}

// Ping zaznamená, že je otvorené okno programu.
func (w *Watchdog) Ping() {
	w.init()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.last, w.pinged = w.Now(), true
}

// Quit požiada o ukončenie (tlačidlo „Ukončiť Fiškus“).
func (w *Watchdog) Quit() {
	w.init()
	select {
	case <-w.quit:
	default:
		close(w.quit)
	}
}

// Expired vráti true, ak sa okno dlho neozvalo.
func (w *Watchdog) Expired() bool {
	w.init()
	if w.Resident {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	limit := w.Idle
	if !w.pinged {
		limit = w.Start
	}
	return w.Now().Sub(w.last) > limit
}

// Wait čaká, kým sa má program ukončiť (zatvorené okná, Quit alebo ctx).
func (w *Watchdog) Wait(ctx context.Context, every time.Duration) string {
	w.init()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return "prerušené"
		case <-w.quit:
			return "ukončené používateľom"
		case <-t.C:
			if w.Expired() {
				return "zatvorené okno programu"
			}
		}
	}
}

// LogFile otvorí denník programu v priečinku dát (aplikácia z inštalátora
// nemá konzolu).
func LogFile(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("priečinok dát %s: %w", dir, err)
	}
	return os.OpenFile(filepath.Join(dir, "fiskus.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}
