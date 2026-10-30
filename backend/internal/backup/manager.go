package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Source vie vytvoriť konzistentnú kópiu databázy do súboru
// (pre SQLite: VACUUM INTO) a overiť integritu súboru databázy.
type Source interface {
	Snapshot(ctx context.Context, dstPath string) error
	Verify(ctx context.Context, path string) error
}

// Manager spravuje zálohy v jednom priečinku. Priečinok a heslo sa dajú
// zmeniť za behu (Configure); Dir a Password sa po spustení priamo nemenia.
type Manager struct {
	mu       sync.RWMutex
	Dir      string
	Password string
	Policy   Policy
	Source   Source
	// Client je HTTP klient pre cloud (nil = predvolený; v testoch falošný server).
	Client *http.Client

	remote      RemoteConfig
	remoteState RemoteState
}

// SetRemote nastaví kópiu záloh v cloude (prázdne Kind = vypnutá).
func (m *Manager) SetRemote(c RemoteConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c.Kind != m.remote.Kind || c.URL != m.remote.URL || c.Bucket != m.remote.Bucket || c.Prefix != m.remote.Prefix {
		m.remoteState = RemoteState{}
	}
	m.remote = c
}

// RemoteConfig vráti nastavenie cloudu (aj s tajným údajom — nikdy do API).
func (m *Manager) RemoteConfig() RemoteConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.remote
}

// RemoteState vráti výsledok poslednej kópie do cloudu.
func (m *Manager) RemoteState() RemoteState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.remoteState
}

// Remote vráti klienta cloudu; nil, ak cloud nie je nastavený.
func (m *Manager) Remote() (Remote, error) {
	c := m.RemoteConfig()
	if !c.Enabled() {
		return nil, nil
	}
	return NewRemote(c, m.Client)
}

// UploadRemote skopíruje lokálnu zálohu do cloudu a v cloude zmaže zálohy,
// ktoré politika neponecháva. Výsledok si zapamätá pre stav záloh.
func (m *Manager) UploadRemote(ctx context.Context, b Info) error {
	r, err := m.Remote()
	if err != nil || r == nil {
		return err
	}
	data, err := os.ReadFile(b.Path)
	if err == nil {
		var n int
		n, err = SyncRemote(ctx, r, m.Policy, filepath.Base(b.Path), data)
		if err == nil {
			m.mu.Lock()
			m.remoteState = RemoteState{LastOK: time.Now().UTC(), LastName: filepath.Base(b.Path), Count: n}
			m.mu.Unlock()
			return nil
		}
	}
	m.mu.Lock()
	m.remoteState.LastError, m.remoteState.ErrorAt = err.Error(), time.Now().UTC()
	m.mu.Unlock()
	return err
}

// ErrRemoteCopy — lokálna záloha je v poriadku, ale kópia do cloudu zlyhala.
var ErrRemoteCopy = errors.New("kópia do cloudu zlyhala")

// FetchRemote stiahne zálohu z cloudu do lokálneho priečinka (napr. na novom
// počítači), overí ju heslom záloh a vráti ju. Potom sa dá obnoviť ako každá iná.
func (m *Manager) FetchRemote(ctx context.Context, name string) (Info, error) {
	dir, password := m.conf()
	if dir == "" || password == "" {
		return Info{}, ErrNotReady
	}
	if name != filepath.Base(name) || !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, fileExt) {
		return Info{}, ErrNoBackup
	}
	t, err := time.Parse(timeLayout, strings.TrimSuffix(strings.TrimPrefix(name, filePrefix), fileExt))
	if err != nil {
		return Info{}, ErrNoBackup
	}
	r, err := m.Remote()
	if err != nil {
		return Info{}, err
	}
	if r == nil {
		return Info{}, ErrNoRemote
	}
	data, err := r.Get(ctx, name)
	if err != nil {
		return Info{}, err
	}
	if _, err := Decrypt(data, password); err != nil {
		return Info{}, fmt.Errorf("záloha z cloudu sa nedá otvoriť heslom záloh: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Info{}, err
	}
	final := filepath.Join(dir, name)
	if err := os.WriteFile(final+".part", data, 0o600); err != nil {
		return Info{}, err
	}
	if err := os.Rename(final+".part", final); err != nil {
		return Info{}, err
	}
	return Info{Path: final, Time: t, Size: int64(len(data))}, nil
}

// ErrNoRemote — kópia do cloudu nie je nastavená.
var ErrNoRemote = errors.New("kópia záloh do cloudu nie je nastavená")

// Configure nastaví priečinok a heslo (napr. po zmene v programe).
func (m *Manager) Configure(dir, password string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Dir, m.Password = dir, password
}

// conf vráti aktuálny priečinok a heslo.
func (m *Manager) conf() (string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Dir, m.Password
}

// Settings vráti priečinok a heslo (heslo iba pre nastavenie v programe, nikdy do API).
func (m *Manager) Settings() (dir, password string) { return m.conf() }

// Folder vráti priečinok záloh.
func (m *Manager) Folder() string {
	dir, _ := m.conf()
	return dir
}

// Ready: zálohy sa dajú vytvárať (je priečinok aj heslo).
func (m *Manager) Ready() bool {
	dir, pw := m.conf()
	return dir != "" && pw != ""
}

// ErrNotReady — chýba priečinok alebo heslo záloh.
var ErrNotReady = fmt.Errorf("zálohy nie sú nastavené — chýba priečinok alebo heslo")

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
	dir, password := m.conf()
	if dir == "" || password == "" {
		return Info{}, ErrNotReady
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
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
	if err := Encrypt(&buf, plain, password); err != nil {
		return Info{}, err
	}
	// Overenie zapísanej zálohy: dešifrovať a porovnať.
	back, err := Decrypt(buf.Bytes(), password)
	if err != nil || !bytes.Equal(back, plain) {
		return Info{}, fmt.Errorf("overenie zálohy zlyhalo: %v", err)
	}

	name := filePrefix + now.UTC().Format(timeLayout) + fileExt
	final := filepath.Join(dir, name)
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
	info := Info{Path: final, Time: now.UTC().Truncate(time.Second), Size: int64(buf.Len())}
	if err := m.UploadRemote(ctx, info); err != nil {
		// lokálna záloha je hotová; chybu cloudu vráti volajúcemu na zápis do logu
		return info, fmt.Errorf("%w: %v", ErrRemoteCopy, err)
	}
	return info, nil
}

// List vráti zálohy, najnovšie prvé.
func (m *Manager) List() ([]Info, error) {
	dir := m.Folder()
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
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
		out = append(out, Info{Path: filepath.Join(dir, n), Time: t, Size: fi.Size()})
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
	_, password := m.conf()
	plain, err := Decrypt(data, password)
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
	return os.WriteFile(filepath.Join(m.Folder(), restoreMarker), []byte(name+"\n"), 0o600)
}

// PendingRestore vráti názov zálohy naplánovanej na obnovu ("" = žiadna).
func (m *Manager) PendingRestore() string {
	dir := m.Folder()
	if dir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dir, restoreMarker))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// CancelRestore zruší naplánovanú obnovu.
func (m *Manager) CancelRestore() error {
	err := os.Remove(filepath.Join(m.Folder(), restoreMarker))
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
