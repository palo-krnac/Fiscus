package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Remote je vzdialené úložisko záloh (druhá kópia mimo počítača). Zálohy
// sú už zašifrované heslom záloh — poskytovateľ cloudu ich neprečíta.
type Remote interface {
	Put(ctx context.Context, name string, data []byte) error
	List(ctx context.Context) ([]Info, error) // Info.Path = názov súboru
	Get(ctx context.Context, name string) ([]byte, error)
	Delete(ctx context.Context, name string) error
}

// Druhy vzdialeného úložiska.
const (
	RemoteWebDAV = "webdav"
	RemoteS3     = "s3"
)

// RemoteConfig je nastavenie cloudu v súbore vedľa databázy (0600).
// Secret (heslo WebDAV alebo tajný kľúč S3) sa nikdy nevracia cez API.
type RemoteConfig struct {
	Kind   string `json:"kind,omitempty"`
	URL    string `json:"url,omitempty"`    // WebDAV priečinok alebo S3 endpoint (https://s3.eu-central-1.amazonaws.com)
	User   string `json:"user,omitempty"`   // WebDAV používateľ alebo S3 access key
	Secret string `json:"secret,omitempty"` // WebDAV heslo alebo S3 secret key
	Bucket string `json:"bucket,omitempty"` // S3
	Region string `json:"region,omitempty"` // S3, napr. eu-central-1
	Prefix string `json:"prefix,omitempty"` // S3 priečinok v buckete
}

// Enabled vráti true, ak je cloud nastavený.
func (c RemoteConfig) Enabled() bool { return c.Kind != "" }

// ErrRemoteConfig — neúplné nastavenie cloudu.
var ErrRemoteConfig = errors.New("neúplné nastavenie cloudu")

// Validate skontroluje nastavenie.
func (c RemoteConfig) Validate() error {
	u, err := url.Parse(c.URL)
	switch {
	case c.Kind != RemoteWebDAV && c.Kind != RemoteS3:
		return fmt.Errorf("%w: vyberte WebDAV alebo S3", ErrRemoteConfig)
	case err != nil || u.User != nil:
		return fmt.Errorf("%w: adresa nesmie obsahovať meno ani heslo — zadajte ich do polí nižšie", ErrRemoteConfig)
	case (u.Scheme != "https" && !(u.Scheme == "http" && isLocalHost(u.Hostname()))) || u.Host == "":
		return fmt.Errorf("%w: adresa musí začínať https:// (http iba pre localhost)", ErrRemoteConfig)
	case c.User == "" || c.Secret == "":
		return fmt.Errorf("%w: zadajte prihlasovacie údaje", ErrRemoteConfig)
	case c.Kind == RemoteS3 && (c.Bucket == "" || c.Region == ""):
		return fmt.Errorf("%w: pri S3 zadajte bucket a región", ErrRemoteConfig)
	}
	return nil
}

func isLocalHost(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

// NewRemote vytvorí klienta podľa nastavenia.
func NewRemote(c RemoteConfig, client *http.Client) (Remote, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	if c.Kind == RemoteWebDAV {
		u, _ := url.Parse(strings.TrimRight(c.URL, "/") + "/") // Validate už adresu overila
		return &WebDAV{Base: u.String(), User: c.User, Password: c.Secret, Client: client}, nil
	}
	return &S3{Endpoint: strings.TrimRight(c.URL, "/"), Bucket: c.Bucket, Region: c.Region, Prefix: strings.Trim(c.Prefix, "/"),
		AccessKey: c.User, SecretKey: c.Secret, Client: client, Now: time.Now}, nil
}

// remoteBackups vyberie zo zoznamu súborov zálohy Fiškusa podľa názvu.
func remoteBackups(names map[string]int64) []Info {
	var out []Info
	for n, size := range names {
		if !strings.HasPrefix(n, filePrefix) || !strings.HasSuffix(n, fileExt) {
			continue
		}
		t, err := time.Parse(timeLayout, strings.TrimSuffix(strings.TrimPrefix(n, filePrefix), fileExt))
		if err != nil {
			continue
		}
		out = append(out, Info{Path: n, Time: t, Size: size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

// httpError vráti chybu odpovede bez tela (môže obsahovať citlivé údaje).
func httpError(op string, resp *http.Response) error {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%s: prístup zamietnutý (%d) — skontrolujte prihlasovacie údaje a práva", op, resp.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("%s: priečinok alebo bucket neexistuje (404)", op)
	}
	return fmt.Errorf("%s: cloud odpovedal %d", op, resp.StatusCode)
}

// SyncRemote nahrá zálohu do cloudu a zmaže v cloude tie, ktoré politika
// neponecháva. Vráti počet záloh v cloude.
func SyncRemote(ctx context.Context, r Remote, policy Policy, name string, data []byte) (int, error) {
	if err := r.Put(ctx, name, data); err != nil {
		return 0, err
	}
	list, err := r.List(ctx)
	if err != nil {
		return 0, err
	}
	times := make([]time.Time, len(list))
	for i, b := range list {
		times[i] = b.Time
	}
	keep := policy.Keep(times)
	n := 0
	for _, b := range list {
		if keep[b.Time] {
			n++
			continue
		}
		if err := r.Delete(ctx, b.Path); err != nil {
			return n, err
		}
	}
	return n, nil
}

// TestRemote overí zápis, čítanie aj mazanie skúšobným súborom.
func TestRemote(ctx context.Context, r Remote) error {
	name := "fiskus-test-" + time.Now().UTC().Format("20060102150405") + ".txt"
	data := []byte("Fiškus – skúška zápisu záloh")
	if err := r.Put(ctx, name, data); err != nil {
		return err
	}
	got, err := r.Get(ctx, name)
	if err != nil {
		return err
	}
	if string(got) != string(data) {
		return errors.New("skúšobný súbor sa v cloude zmenil")
	}
	return r.Delete(ctx, name)
}

// RemoteState je výsledok poslednej kópie do cloudu.
type RemoteState struct {
	LastOK    time.Time `json:"lastOk,omitempty"`
	LastName  string    `json:"lastName,omitempty"`
	LastError string    `json:"lastError,omitempty"`
	ErrorAt   time.Time `json:"errorAt,omitempty"`
	Count     int       `json:"count"` // záloh v cloude po poslednej synchronizácii
}
