package backup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Source vie vytvoriť konzistentnú kópiu databázy do súboru
// (pre SQLite: VACUUM INTO) a overiť integritu súboru databázy.
type Source interface {
	Snapshot(ctx context.Context, dstPath string) error
	Verify(ctx context.Context, path string) error
}

// Manager spravuje zálohy v jednom priečinku.
type Manager struct {
	Dir      string
	Password string
	Policy   Policy
	Source   Source
}

const (
	filePrefix = "fiskus-"
	fileExt    = ".fbk"
	timeLayout = "20060102-150405"
)

// Info opisuje jednu zálohu.
type Info struct {
	Path string    `json:"path"`
	Time time.Time `json:"time"`
	Size int64     `json:"size"`
}

// Create vytvorí, overí, zašifruje a uloží zálohu, potom spustí rotáciu.
func (m *Manager) Create(ctx context.Context, now time.Time) (Info, error) {
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return Info{}, err
	}
	tmp, err := os.MkdirTemp("", "fiskus-backup-")
	if err != nil {
		return Info{}, err
	}
	defer os.RemoveAll(tmp)

	snap := filepath.Join(tmp, "snapshot.db")
	if err := m.Source.Snapshot(ctx, snap); err != nil {
		return Info{}, fmt.Errorf("vytvorenie kópie: %w", err)
	}
	if err := m.Source.Verify(ctx, snap); err != nil {
		return Info{}, fmt.Errorf("kópia databázy je poškodená: %w", err)
	}
	plain, err := os.ReadFile(snap)
	if err != nil {
		return Info{}, err
	}

	var buf bytes.Buffer
	if err := Encrypt(&buf, plain, m.Password); err != nil {
		return Info{}, err
	}
	// Overenie zapísanej zálohy: dešifrovať a porovnať.
	back, err := Decrypt(buf.Bytes(), m.Password)
	if err != nil || !bytes.Equal(back, plain) {
		return Info{}, fmt.Errorf("overenie zálohy zlyhalo: %v", err)
	}

	name := filePrefix + now.UTC().Format(timeLayout) + fileExt
	final := filepath.Join(m.Dir, name)
	part := final + ".part"
	if err := os.WriteFile(part, buf.Bytes(), 0o600); err != nil {
		return Info{}, err
	}
	if err := os.Rename(part, final); err != nil { // atomické — žiadna polovičná záloha
		return Info{}, err
	}
	if _, err := m.Prune(); err != nil {
		return Info{}, fmt.Errorf("rotácia záloh: %w", err)
	}
	return Info{Path: final, Time: now.UTC().Truncate(time.Second), Size: int64(buf.Len())}, nil
}

// List vráti zálohy, najnovšie prvé.
func (m *Manager) List() ([]Info, error) {
	entries, err := os.ReadDir(m.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, filePrefix) || !strings.HasSuffix(n, fileExt) {
			continue
		}
		t, err := time.Parse(timeLayout, strings.TrimSuffix(strings.TrimPrefix(n, filePrefix), fileExt))
		if err != nil {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{Path: filepath.Join(m.Dir, n), Time: t, Size: fi.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

// Prune zmaže zálohy, ktoré politika neponecháva. Vráti zmazané.
func (m *Manager) Prune() ([]Info, error) {
	list, err := m.List()
	if err != nil {
		return nil, err
	}
	times := make([]time.Time, len(list))
	for i, b := range list {
		times[i] = b.Time
	}
	keep := m.Policy.Keep(times)
	var removed []Info
	for _, b := range list {
		if !keep[b.Time] {
			if err := os.Remove(b.Path); err != nil {
				return removed, err
			}
			removed = append(removed, b)
		}
	}
	return removed, nil
}

// Restore dešifruje zálohu, overí ju a zapíše databázu do dstPath.
// Existujúci súbor sa najprv premenuje na .pred-obnovou, nič sa nestratí.
func (m *Manager) Restore(ctx context.Context, backupPath, dstPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	plain, err := Decrypt(data, m.Password)
	if err != nil {
		return err
	}
	tmp := dstPath + ".obnova"
	if err := os.WriteFile(tmp, plain, 0o600); err != nil {
		return err
	}
	if err := m.Source.Verify(ctx, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("záloha obsahuje poškodenú databázu: %w", err)
	}
	if _, err := os.Stat(dstPath); err == nil {
		if err := os.Rename(dstPath, dstPath+".pred-obnovou"); err != nil {
			return err
		}
	}
	return os.Rename(tmp, dstPath)
}

// ErrNoBackup — záloha s daným názvom neexistuje (alebo názov nie je platný).
var ErrNoBackup = fmt.Errorf("záloha neexistuje")

// restoreMarker je súbor s názvom zálohy, ktorá sa obnoví pri ďalšom štarte.
const restoreMarker = "obnovit-pri-starte.txt"

// Find vráti zálohu podľa názvu súboru. Názov nesmie obsahovať cestu.
func (m *Manager) Find(name string) (Info, error) {
	if name != filepath.Base(name) || !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, fileExt) {
		return Info{}, ErrNoBackup
	}
	list, err := m.List()
	if err != nil {
		return Info{}, err
	}
	for _, b := range list {
		if filepath.Base(b.Path) == name {
			return b, nil
		}
	}
	return Info{}, ErrNoBackup
}

// RequestRestore naplánuje obnovu zo zálohy pri ďalšom spustení programu
// (počas behu je databáza otvorená a prepísať sa nedá).
func (m *Manager) RequestRestore(name string) error {
	if _, err := m.Find(name); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.Dir, restoreMarker), []byte(name+"\n"), 0o600)
}

// PendingRestore vráti názov zálohy naplánovanej na obnovu ("" = žiadna).
func (m *Manager) PendingRestore() string {
	b, err := os.ReadFile(filepath.Join(m.Dir, restoreMarker))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// CancelRestore zruší naplánovanú obnovu.
func (m *Manager) CancelRestore() error {
	err := os.Remove(filepath.Join(m.Dir, restoreMarker))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ApplyPending vykoná naplánovanú obnovu do dbPath (volá sa pri štarte pred
// otvorením databázy). Vráti obnovenú zálohu; ok = false, ak nebola naplánovaná.
func (m *Manager) ApplyPending(ctx context.Context, dbPath string) (Info, bool, error) {
	name := m.PendingRestore()
	if name == "" {
		return Info{}, false, nil
	}
	b, err := m.Find(name)
	if err != nil {
		return Info{}, true, fmt.Errorf("naplánovaná obnova %s: %w", name, err)
	}
	if err := m.Restore(ctx, b.Path, dbPath); err != nil {
		return b, true, err
	}
	return b, true, m.CancelRestore()
}
