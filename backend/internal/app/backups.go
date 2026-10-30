package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
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
	// Settings je nastavenie priečinka a hesla v programe (nil = iba parametrami).
	Settings *BackupSettings `json:"settings,omitempty"`
	// Remote je kópia záloh v cloude (tajný údaj sa nevracia).
	Remote BackupRemote `json:"remote"`
}

// BackupRemote je nastavenie a stav kópie záloh v cloude pre formulár.
type BackupRemote struct {
	Kind      string             `json:"kind"`
	URL       string             `json:"url"`
	User      string             `json:"user"`
	Bucket    string             `json:"bucket"`
	Region    string             `json:"region"`
	Prefix    string             `json:"prefix"`
	SecretSet bool               `json:"secretSet"`
	State     backup.RemoteState `json:"state"`
}

// BackupRemoteRequest je zmena cloudu; prázdny Secret = ponechať, prázdny Kind = vypnúť.
type BackupRemoteRequest struct {
	Kind   string `json:"kind"`
	URL    string `json:"url"`
	User   string `json:"user"`
	Secret string `json:"secret"`
	Bucket string `json:"bucket"`
	Region string `json:"region"`
	Prefix string `json:"prefix"`
}

// BackupSetup opisuje, odkiaľ pochádza nastavenie záloh (nastavuje main).
type BackupSetup struct {
	ConfigPath    string // súbor s nastavením vedľa databázy
	DefaultDir    string // predvolený priečinok
	DirFixed      bool   // priečinok je určený parametrom -backup-dir
	PasswordFixed bool   // heslo je v premennej FISKUS_BACKUP_PASSWORD
	Home          string // domovský priečinok pre návrh cloudových priečinkov (desktop)
}

// BackupSettings je nastavenie záloh pre formulár (heslo sa nikdy nevracia).
type BackupSettings struct {
	Dir           string   `json:"dir"`
	DefaultDir    string   `json:"defaultDir"`
	DirFixed      bool     `json:"dirFixed"`
	PasswordSet   bool     `json:"passwordSet"`
	PasswordFixed bool     `json:"passwordFixed"`
	Suggestions   []string `json:"suggestions"` // priečinky synchronizované do cloudu
}

// BackupSettingsRequest je zmena nastavenia; prázdne heslo = ponechať.
type BackupSettingsRequest struct {
	Dir      string `json:"dir"`
	Password string `json:"password"`
}

// MinBackupPassword je najmenšia dĺžka hesla záloh.
const MinBackupPassword = 8

// ErrCloud — cloud so zálohami odpovedal chybou (správa neobsahuje prístupové údaje).
var ErrCloud = errors.New("chyba cloudu")

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
	st.Enabled, st.Dir, st.Policy, st.Reason, st.Pending = m.Ready(), m.Folder(), m.Policy, "", m.PendingRestore()
	if !st.Enabled {
		st.Reason = "Zálohy sa zatiaľ nevytvárajú — nastavte heslo záloh."
	}
	if a.BackupSetup != nil {
		st.Settings = a.backupSettings()
	}
	rc := m.RemoteConfig()
	st.Remote = BackupRemote{Kind: rc.Kind, URL: rc.URL, User: rc.User, Bucket: rc.Bucket, Region: rc.Region,
		Prefix: rc.Prefix, SecretSet: rc.Secret != "", State: m.RemoteState()}
	list, err := m.List()
	if err != nil {
		return st, err
	}
	for _, b := range list {
		st.Backups = append(st.Backups, item(b))
	}
	return st, nil
}

func (a *App) backupSettings() *BackupSettings {
	set, m := a.BackupSetup, a.Backups
	sugg := backup.CloudFolders(set.Home)
	if sugg == nil {
		sugg = []string{}
	}
	return &BackupSettings{
		Dir: m.Folder(), DefaultDir: set.DefaultDir, DirFixed: set.DirFixed,
		PasswordSet: m.Ready(), PasswordFixed: set.PasswordFixed, Suggestions: sugg,
	}
}

// SetBackupSettings zmení priečinok alebo heslo záloh, hneď vytvorí zálohu na
// overenie a nastavenie uloží. Pri chybe ostane pôvodné nastavenie.
func (a *App) SetBackupSettings(ctx context.Context, req BackupSettingsRequest) (BackupStatus, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return BackupStatus{}, err
	}
	set, m := a.BackupSetup, a.Backups
	if set == nil || m == nil {
		return BackupStatus{}, ErrBackupsOff
	}
	oldDir, oldPw := m.Settings()
	var fields []invoice.FieldError
	bad := func(f, msg string) { fields = append(fields, invoice.FieldError{Field: f, Message: msg}) }

	dir := strings.TrimSpace(req.Dir)
	if dir == "" {
		dir = set.DefaultDir
	}
	if set.DirFixed {
		if req.Dir != "" && filepath.Clean(dir) != filepath.Clean(oldDir) {
			bad("dir", "Priečinok je určený parametrom -backup-dir pri spustení programu")
		}
		dir = oldDir
	}
	dir = filepath.Clean(dir)
	switch {
	case set.DirFixed:
	case dir != filepath.Clean(oldDir) || oldDir == "":
		if err := backup.CheckDir(dir); err != nil {
			bad("dir", strings.ToUpper(err.Error()[:1])+err.Error()[1:])
		}
	}
	pw := req.Password
	switch {
	case pw != "" && set.PasswordFixed:
		bad("password", "Heslo je určené premennou FISKUS_BACKUP_PASSWORD")
	case pw == "" && oldPw == "":
		bad("password", "Zadajte heslo záloh")
	case pw != "" && len([]rune(pw)) < MinBackupPassword:
		bad("password", "Heslo musí mať aspoň 8 znakov")
	case pw != "" && strings.TrimSpace(pw) != pw:
		bad("password", "Heslo nesmie začínať ani končiť medzerou")
	}
	if len(fields) > 0 {
		return BackupStatus{}, &invoice.ValidationError{Fields: fields}
	}
	if pw == "" {
		pw = oldPw
	}

	m.Configure(dir, pw)
	b, err := m.Create(ctx, time.Now())
	if err != nil && !errors.Is(err, backup.ErrRemoteCopy) { // chyba cloudu je v stave záloh
		m.Configure(oldDir, oldPw)
		return BackupStatus{}, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "dir", Message: "Záloha sa nepodarila: " + err.Error()}}}
	}
	cfg := backup.Config{Remote: m.RemoteConfig()}
	if !set.DirFixed && dir != filepath.Clean(set.DefaultDir) {
		cfg.Dir = dir
	}
	if !set.PasswordFixed {
		cfg.Password = pw
	}
	if err := backup.SaveConfig(set.ConfigPath, cfg); err != nil {
		return BackupStatus{}, err
	}
	what := "priečinok " + dir
	if req.Password != "" {
		what += ", nové heslo"
	}
	a.record(ctx, 0, audit.CatBackups, "settings", "backup", 0, "Nastavenie záloh: "+what+"; overovacia záloha "+filepath.Base(b.Path), nil, nil)
	return a.BackupStatus(ctx)
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
	if errors.Is(err, backup.ErrRemoteCopy) {
		it := item(b)
		a.record(ctx, 0, audit.CatBackups, "backup", "backup", 0, "Ručná záloha "+it.Name+"; "+err.Error(), nil, it)
		return it, nil
	}
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

// SetBackupRemote nastaví kópiu záloh v cloude. Spojenie overí skúšobným
// súborom (zápis, čítanie, zmazanie), nastavenie uloží a hneď skopíruje
// najnovšiu zálohu. Tajný údaj sa nikdy nevracia ani nezapisuje do auditu.
func (a *App) SetBackupRemote(ctx context.Context, req BackupRemoteRequest) (BackupStatus, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return BackupStatus{}, err
	}
	set, m := a.BackupSetup, a.Backups
	if set == nil || m == nil {
		return BackupStatus{}, ErrBackupsOff
	}
	old := m.RemoteConfig()
	c := backup.RemoteConfig{
		Kind: strings.TrimSpace(req.Kind), URL: strings.TrimSpace(req.URL), User: strings.TrimSpace(req.User),
		Secret: req.Secret, Bucket: strings.TrimSpace(req.Bucket), Region: strings.TrimSpace(req.Region),
		Prefix: strings.Trim(strings.TrimSpace(req.Prefix), "/"),
	}
	what := "vypnutá"
	if c.Kind != "" {
		if c.Secret == "" && old.Kind == c.Kind && old.User == c.User {
			c.Secret = old.Secret
		}
		if fe := remoteFieldErrors(c); len(fe) > 0 {
			return BackupStatus{}, &invoice.ValidationError{Fields: fe}
		}
		r, err := backup.NewRemote(c, m.Client)
		if err == nil {
			err = backup.TestRemote(ctx, r)
		}
		if err != nil {
			return BackupStatus{}, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "url", Message: "Spojenie s cloudom zlyhalo: " + err.Error()}}}
		}
		what = remoteLabel(c)
	}
	cfg, err := backup.LoadConfig(set.ConfigPath)
	if err != nil {
		return BackupStatus{}, err
	}
	cfg.Remote = c
	if err := backup.SaveConfig(set.ConfigPath, cfg); err != nil {
		return BackupStatus{}, err
	}
	m.SetRemote(c)
	a.record(ctx, 0, audit.CatBackups, "remote_settings", "backup", 0, "Kópia záloh v cloude: "+what, nil, nil)
	if c.Kind != "" && m.Ready() {
		if list, err := m.List(); err == nil && len(list) > 0 {
			_ = m.UploadRemote(ctx, list[0]) // výsledok je v stave záloh
		}
	}
	return a.BackupStatus(ctx)
}

// remoteLabel opíše cieľ bez prístupových údajov.
func remoteLabel(c backup.RemoteConfig) string {
	if c.Kind == backup.RemoteS3 {
		l := "S3 " + c.URL + " bucket " + c.Bucket
		if c.Prefix != "" {
			l += "/" + c.Prefix
		}
		return l
	}
	return "WebDAV " + c.URL
}

func remoteFieldErrors(c backup.RemoteConfig) []invoice.FieldError {
	var fe []invoice.FieldError
	bad := func(f, msg string) { fe = append(fe, invoice.FieldError{Field: f, Message: msg}) }
	if c.Kind != backup.RemoteWebDAV && c.Kind != backup.RemoteS3 {
		bad("kind", "Vyberte WebDAV alebo S3")
		return fe
	}
	if err := (backup.RemoteConfig{Kind: c.Kind, URL: c.URL, User: "x", Secret: "x", Bucket: "x", Region: "x"}).Validate(); err != nil {
		bad("url", strings.TrimPrefix(err.Error(), backup.ErrRemoteConfig.Error()+": "))
	}
	user, secret := "Zadajte používateľa", "Zadajte heslo"
	if c.Kind == backup.RemoteS3 {
		user, secret = "Zadajte prístupový kľúč (access key)", "Zadajte tajný kľúč (secret key)"
		if c.Bucket == "" {
			bad("bucket", "Zadajte názov bucketu")
		}
		if c.Region == "" {
			bad("region", "Zadajte región, napr. eu-central-1")
		}
	}
	if c.User == "" {
		bad("user", user)
	}
	if c.Secret == "" {
		bad("secret", secret)
	}
	return fe
}

// RemoteBackups vráti zálohy uložené v cloude.
func (a *App) RemoteBackups(ctx context.Context) ([]BackupItem, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return nil, err
	}
	if a.Backups == nil {
		return nil, ErrBackupsOff
	}
	r, err := a.Backups.Remote()
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, backup.ErrNoRemote
	}
	list, err := r.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCloud, err)
	}
	out := make([]BackupItem, 0, len(list))
	for _, b := range list {
		out = append(out, BackupItem{Name: b.Path, Time: b.Time, Size: b.Size})
	}
	return out, nil
}

// FetchRemoteBackup stiahne zálohu z cloudu do priečinka záloh.
func (a *App) FetchRemoteBackup(ctx context.Context, name string) (BackupItem, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return BackupItem{}, err
	}
	if a.Backups == nil {
		return BackupItem{}, ErrBackupsOff
	}
	b, err := a.Backups.FetchRemote(ctx, name)
	if err != nil {
		if errors.Is(err, backup.ErrNoBackup) || errors.Is(err, backup.ErrNoRemote) || errors.Is(err, backup.ErrNotReady) {
			return BackupItem{}, err
		}
		return BackupItem{}, fmt.Errorf("%w: %v", ErrCloud, err)
	}
	it := item(b)
	a.record(ctx, 0, audit.CatBackups, "remote_fetch", "backup", 0, "Stiahnutá záloha z cloudu "+it.Name, nil, nil)
	return it, nil
}
