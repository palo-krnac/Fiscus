package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Mobil v sieti (desktop): Fiškus počúva aj na adrese počítača v domácej
// sieti, ale pustí dnu iba spárované telefóny. Spárovanie: na počítači sa
// ukáže QR kód s jednorazovým kódom (platí 5 minút), telefón ním dostane
// trvalý prístupový token (v cookie; uložený je iba jeho odtlačok).

// DefaultMobilePort je port pre telefóny v sieti.
const DefaultMobilePort = 48081

const (
	mobileKey       = "mobile"
	pairValidity    = 5 * time.Minute
	pairAlphabet    = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	pairCodeLen     = 6
	maxPairFailures = 20 // nesprávnych pokusov za 10 minút — potom sa párovanie zablokuje
	maxDevices      = 20
)

// Chyby párovania.
var (
	ErrPairCode    = errors.New("kód na spárovanie neplatí — na počítači otvorte Nastavenia → Mobil v sieti a naskenujte nový QR kód")
	ErrPairBlocked = errors.New("priveľa nesprávnych pokusov o spárovanie — skúste to o niekoľko minút")
	ErrNotPaired   = errors.New("telefón nie je spárovaný s týmto počítačom — naskenujte QR kód v Nastaveniach → Mobil v sieti")
	ErrDesktopOnly = errors.New("mobil v sieti je iba v desktopovej verzii — serverová verzia je dostupná cez https")
)

// MobileDevice je spárovaný telefón.
type MobileDevice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Created  string `json:"created"`
	LastSeen string `json:"lastSeen"`
	Hash     string `json:"hash,omitempty"` // iba v úložisku
}

type mobileConfig struct {
	Enabled bool           `json:"enabled"`
	Port    int            `json:"port"`
	Devices []MobileDevice `json:"devices"`
}

// MobileStatus je stav pre Nastavenia (bez odtlačkov tokenov).
type MobileStatus struct {
	Enabled bool           `json:"enabled"`
	Port    int            `json:"port"`
	URLs    []string       `json:"urls"` // adresy počítača v sieti
	Devices []MobileDevice `json:"devices"`
	Error   string         `json:"error,omitempty"` // počúvanie sa nepodarilo
}

// MobilePairing je jednorazový kód na spárovanie.
type MobilePairing struct {
	Code    string    `json:"code"`
	Expires time.Time `json:"expires"`
	URLs    []string  `json:"urls"` // adresy na otvorenie v telefóne (s kódom)
}

type mobileState struct {
	mu       sync.Mutex
	codes    map[string]time.Time
	failures []time.Time
	seen     map[string]time.Time // zariadenie → posledný zápis LastSeen
	listenEr string
}

func (a *App) loadMobile(ctx context.Context) (mobileConfig, error) {
	c := mobileConfig{Port: DefaultMobilePort, Devices: []MobileDevice{}}
	raw, err := a.Store.GetSetting(ctx, mobileKey)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return c, nil
	case err != nil:
		return c, err
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, err
	}
	if c.Port == 0 {
		c.Port = DefaultMobilePort
	}
	if c.Devices == nil {
		c.Devices = []MobileDevice{}
	}
	return c, nil
}

func (a *App) saveMobile(ctx context.Context, c mobileConfig) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, mobileKey, string(b))
}

func (a *App) requireMobileAdmin(ctx context.Context) error {
	if a.Caps.Mode != edition.Desktop {
		return ErrDesktopOnly
	}
	act := auth.ActorFrom(ctx)
	if !act.SuperAdmin || act.Mobile {
		return ErrForbidden // spárovaný telefón nespravuje párovanie
	}
	return nil
}

// LANAddresses vráti IPv4 adresy počítača v súkromných sieťach.
func LANAddresses() []string {
	var out []string
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, ad := range addrs {
			if n, ok := ad.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				out = append(out, n.IP.String())
			}
		}
	}
	return out
}

func (a *App) lanAddresses() []string {
	if a.LANAddresses != nil {
		return a.LANAddresses()
	}
	return LANAddresses()
}

func (a *App) mobileStatus(c mobileConfig) MobileStatus {
	st := MobileStatus{Enabled: c.Enabled, Port: c.Port, URLs: []string{}, Devices: []MobileDevice{}}
	for _, ip := range a.lanAddresses() {
		st.URLs = append(st.URLs, fmt.Sprintf("http://%s:%d", ip, c.Port))
	}
	for _, d := range c.Devices {
		d.Hash = ""
		st.Devices = append(st.Devices, d)
	}
	a.mobile.mu.Lock()
	st.Error = a.mobile.listenEr
	a.mobile.mu.Unlock()
	return st
}

// MobileStatus vráti nastavenie mobilu v sieti.
func (a *App) MobileStatus(ctx context.Context) (MobileStatus, error) {
	if err := a.requireMobileAdmin(ctx); err != nil {
		return MobileStatus{}, err
	}
	c, err := a.loadMobile(ctx)
	if err != nil {
		return MobileStatus{}, err
	}
	return a.mobileStatus(c), nil
}

// MobileListening zapne alebo vypne počúvanie v sieti (volá program pri
// štarte a po zmene). Chyba (napr. obsadený port) sa ukáže v Nastaveniach.
func (a *App) mobileApply(c mobileConfig) {
	if a.OnMobile == nil {
		return
	}
	err := a.OnMobile(c.Enabled, c.Port)
	a.mobile.mu.Lock()
	a.mobile.listenEr = ""
	if err != nil {
		a.mobile.listenEr = err.Error()
	}
	a.mobile.mu.Unlock()
}

// StartMobile spustí počúvanie v sieti podľa uloženého nastavenia (pri štarte).
func (a *App) StartMobile(ctx context.Context) {
	if a.Caps.Mode != edition.Desktop {
		return
	}
	if c, err := a.loadMobile(ctx); err == nil && c.Enabled {
		a.mobileApply(c)
	}
}

// SetMobile zapne alebo vypne mobil v sieti a nastaví port.
func (a *App) SetMobile(ctx context.Context, enabled bool, port int) (MobileStatus, error) {
	if err := a.requireMobileAdmin(ctx); err != nil {
		return MobileStatus{}, err
	}
	if port == 0 {
		port = DefaultMobilePort
	}
	if port < 1024 || port > 65535 {
		return MobileStatus{}, mfaFieldErr("port", "Port je číslo od 1024 do 65535")
	}
	c, err := a.loadMobile(ctx)
	if err != nil {
		return MobileStatus{}, err
	}
	c.Enabled, c.Port = enabled, port
	if err := a.saveMobile(ctx, c); err != nil {
		return MobileStatus{}, err
	}
	a.mobileApply(c)
	state := "vypnutý"
	if enabled {
		state = fmt.Sprintf("zapnutý (port %d)", port)
	}
	a.record(ctx, 0, audit.CatSettings, "update", "mobile", 0, "Mobil v sieti "+state, nil, nil)
	return a.mobileStatus(c), nil
}

// NewMobilePairing vytvorí jednorazový kód na spárovanie telefónu.
func (a *App) NewMobilePairing(ctx context.Context) (MobilePairing, error) {
	if err := a.requireMobileAdmin(ctx); err != nil {
		return MobilePairing{}, err
	}
	c, err := a.loadMobile(ctx)
	if err != nil {
		return MobilePairing{}, err
	}
	if !c.Enabled {
		return MobilePairing{}, mfaFieldErr("enabled", "Najprv zapnite mobil v sieti")
	}
	if len(c.Devices) >= maxDevices {
		return MobilePairing{}, mfaFieldErr("devices", fmt.Sprintf("Spárovať sa dá najviac %d zariadení — odoberte nepoužívané", maxDevices))
	}
	b := make([]byte, pairCodeLen)
	if _, err := rand.Read(b); err != nil {
		return MobilePairing{}, err
	}
	code := make([]byte, pairCodeLen)
	for i := range b {
		code[i] = pairAlphabet[int(b[i])%len(pairAlphabet)]
	}
	p := MobilePairing{Code: string(code), Expires: a.now().Add(pairValidity), URLs: []string{}}
	a.mobile.mu.Lock()
	if a.mobile.codes == nil {
		a.mobile.codes = map[string]time.Time{}
	}
	for k, exp := range a.mobile.codes {
		if a.now().After(exp) {
			delete(a.mobile.codes, k)
		}
	}
	a.mobile.codes[p.Code] = p.Expires
	a.mobile.mu.Unlock()
	for _, u := range a.mobileStatus(c).URLs {
		p.URLs = append(p.URLs, u+"/parovanie?kod="+p.Code)
	}
	return p, nil
}

func tokenHash(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// PairMobile spáruje telefón kódom a vráti trvalý prístupový token.
func (a *App) PairMobile(ctx context.Context, code, name string) (string, error) {
	if a.Caps.Mode != edition.Desktop {
		return "", ErrDesktopOnly
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	now := a.now()
	a.mobile.mu.Lock()
	recent := a.mobile.failures[:0]
	for _, t := range a.mobile.failures {
		if now.Sub(t) < 10*time.Minute {
			recent = append(recent, t)
		}
	}
	a.mobile.failures = recent
	if len(recent) >= maxPairFailures {
		a.mobile.mu.Unlock()
		return "", ErrPairBlocked
	}
	exp, ok := a.mobile.codes[code]
	if !ok || now.After(exp) {
		a.mobile.failures = append(a.mobile.failures, now)
		a.mobile.mu.Unlock()
		return "", ErrPairCode
	}
	delete(a.mobile.codes, code) // kód je jednorazový
	a.mobile.mu.Unlock()

	c, err := a.loadMobile(ctx)
	if err != nil {
		return "", err
	}
	if !c.Enabled {
		return "", ErrPairCode
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := "fmd_" + base64.RawURLEncoding.EncodeToString(raw)
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 40 {
		name = string(r[:40])
	}
	if name == "" {
		name = "Telefón"
	}
	id := make([]byte, 6)
	_, _ = rand.Read(id)
	d := MobileDevice{ID: hex.EncodeToString(id), Name: name, Created: now.UTC().Format(time.RFC3339), LastSeen: now.UTC().Format(time.RFC3339), Hash: tokenHash(token)}
	c.Devices = append(c.Devices, d)
	if err := a.saveMobile(ctx, c); err != nil {
		return "", err
	}
	a.record(auth.WithActor(ctx, auth.System), 0, audit.CatSettings, "create", "mobile_device", 0, "Spárovaný telefón "+name, nil, nil)
	return token, nil
}

// MobileActor overí token telefónu a vráti, kto volá (správca na telefóne).
func (a *App) MobileActor(ctx context.Context, token, ip string) (auth.Actor, error) {
	if token == "" || a.Caps.Mode != edition.Desktop {
		return auth.Actor{}, ErrNotPaired
	}
	c, err := a.loadMobile(ctx)
	if err != nil {
		return auth.Actor{}, err
	}
	if !c.Enabled {
		return auth.Actor{}, ErrNotPaired
	}
	h := tokenHash(token)
	for i, d := range c.Devices {
		if subtle.ConstantTimeCompare([]byte(d.Hash), []byte(h)) != 1 {
			continue
		}
		// posledné použitie zapísať najviac raz za hodinu
		now := a.now()
		a.mobile.mu.Lock()
		if a.mobile.seen == nil {
			a.mobile.seen = map[string]time.Time{}
		}
		stale := now.Sub(a.mobile.seen[d.ID]) > time.Hour
		if stale {
			a.mobile.seen[d.ID] = now
		}
		a.mobile.mu.Unlock()
		if stale {
			c.Devices[i].LastSeen = now.UTC().Format(time.RFC3339)
			_ = a.saveMobile(ctx, c)
		}
		act := auth.Local
		act.Name, act.IP, act.Mobile = "Telefón: "+d.Name, ip, true
		return act, nil
	}
	return auth.Actor{}, ErrNotPaired
}

// RemoveMobileDevice odoberie spárovaný telefón (jeho token prestane platiť).
func (a *App) RemoveMobileDevice(ctx context.Context, id string) (MobileStatus, error) {
	if err := a.requireMobileAdmin(ctx); err != nil {
		return MobileStatus{}, err
	}
	c, err := a.loadMobile(ctx)
	if err != nil {
		return MobileStatus{}, err
	}
	out := c.Devices[:0]
	removed := ""
	for _, d := range c.Devices {
		if d.ID == id {
			removed = d.Name
			continue
		}
		out = append(out, d)
	}
	if removed == "" {
		return MobileStatus{}, store.ErrNotFound
	}
	c.Devices = out
	if err := a.saveMobile(ctx, c); err != nil {
		return MobileStatus{}, err
	}
	a.record(ctx, 0, audit.CatSettings, "delete", "mobile_device", 0, "Odobraný telefón "+removed, nil, nil)
	return a.mobileStatus(c), nil
}
