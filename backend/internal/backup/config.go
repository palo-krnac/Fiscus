package backup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config je nastavenie záloh zadané v programe. Ukladá sa do súboru vedľa
// databázy (práva 0600), nie do databázy: heslo chráni zálohy skopírované
// inam (USB, cloud) a databáza sama je na disku nešifrovaná.
type Config struct {
	Dir      string `json:"dir,omitempty"`
	Password string `json:"password,omitempty"`
}

// ConfigPath vráti cestu k súboru nastavenia pre databázu: fiskus.db → fiskus-zalohy.json.
func ConfigPath(dbPath string) string {
	return strings.TrimSuffix(dbPath, filepath.Ext(dbPath)) + "-zalohy.json"
}

// LoadConfig načíta nastavenie; chýbajúci súbor = prázdne nastavenie.
func LoadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

// SaveConfig zapíše nastavenie atomicky s právami iba pre vlastníka.
func SaveConfig(path string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CheckDir overí, že priečinok záloh sa dá vytvoriť a zapisovať doň.
func CheckDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("zadajte úplnú cestu k priečinku")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("priečinok sa nedá vytvoriť: " + err.Error())
	}
	f, err := os.CreateTemp(dir, ".fiskus-test-")
	if err != nil {
		return errors.New("do priečinka sa nedá zapisovať: " + err.Error())
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// CloudFolders vráti priečinky synchronizované do cloudu (OneDrive, Dropbox,
// Google Disk, iCloud), ktoré v domovskom priečinku existujú — záloha v nich
// sa sama skopíruje mimo počítača.
func CloudFolders(home string) []string {
	if home == "" {
		return nil
	}
	var out []string
	add := func(p string) {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			out = append(out, filepath.Join(p, "Fiškus zálohy"))
		}
	}
	add(filepath.Join(home, "OneDrive"))
	if more, _ := filepath.Glob(filepath.Join(home, "OneDrive - *")); len(more) > 0 {
		for _, p := range more {
			add(p)
		}
	}
	add(filepath.Join(home, "Dropbox"))
	if _, err := os.Stat(filepath.Join(home, "Google Drive", "My Drive")); err == nil {
		add(filepath.Join(home, "Google Drive", "My Drive"))
	} else {
		add(filepath.Join(home, "Google Drive"))
	}
	add(filepath.Join(home, "iCloudDrive"))
	add(filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs"))
	return out
}
