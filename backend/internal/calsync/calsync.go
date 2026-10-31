// Package calsync prenáša termíny Fiškusa do Google Kalendára a do
// kalendára Outlook (Microsoft 365, Outlook.com) cez ich API. Funguje aj
// v desktopovej verzii: Fiškus termíny posiela sám, kalendár sa k počítaču
// nepripája. Prihlásenie je OAuth 2.0 s PKCE; Fiškus vytvorí v účte
// samostatný kalendár „Fiškus – firma“ a spravuje iba udalosti v ňom.
package calsync

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Provider je služba kalendára.
type Provider string

const (
	Google    Provider = "google"
	Microsoft Provider = "microsoft"
)

// Label vráti názov služby na zobrazenie.
func (p Provider) Label() string {
	switch p {
	case Google:
		return "Google Kalendár"
	case Microsoft:
		return "Outlook (Microsoft 365)"
	}
	return string(p)
}

// Config sú prihlasovacie údaje aplikácie Fiškus u poskytovateľov
// (OAuth klient). Tajomstvo klienta Google sa nikdy nevracia cez API.
type Config struct {
	GoogleClientID     string `json:"googleClientId"`
	GoogleClientSecret string `json:"googleClientSecret,omitempty"`
	MicrosoftClientID  string `json:"microsoftClientId"`
}

// Has vráti true, ak je poskytovateľ nastavený.
func (c Config) Has(p Provider) bool {
	switch p {
	case Google:
		return c.GoogleClientID != ""
	case Microsoft:
		return c.MicrosoftClientID != ""
	}
	return false
}

func (c Config) clientID(p Provider) string {
	if p == Google {
		return c.GoogleClientID
	}
	return c.MicrosoftClientID
}

// Endpoints sú adresy poskytovateľa (v testoch sa nahradia).
type Endpoints struct {
	Auth, Token, API string
}

// DefaultEndpoints sú adresy služieb.
var DefaultEndpoints = map[Provider]Endpoints{
	Google: {Auth: "https://accounts.google.com/o/oauth2/v2/auth", Token: "https://oauth2.googleapis.com/token",
		API: "https://www.googleapis.com/calendar/v3"},
	Microsoft: {Auth: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		Token: "https://login.microsoftonline.com/common/oauth2/v2.0/token", API: "https://graph.microsoft.com/v1.0"},
}

// scopes: Google iba kalendáre vytvorené Fiškusom, Microsoft kalendáre
// používateľa; e-mail účtu na zobrazenie, offline prístup na obnovu.
var scopes = map[Provider]string{
	Google:    "openid email https://www.googleapis.com/auth/calendar.app.created",
	Microsoft: "openid email offline_access User.Read Calendars.ReadWrite",
}

// Verifier vráti náhodný kód PKCE a jeho výzvu (S256).
func Verifier() (verifier, challenge string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthURL vráti adresu prihlásenia u poskytovateľa.
func AuthURL(ep Endpoints, p Provider, cfg Config, redirect, state, challenge string) string {
	q := url.Values{
		"client_id": {cfg.clientID(p)}, "redirect_uri": {redirect}, "response_type": {"code"},
		"scope": {scopes[p]}, "state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	if p == Google {
		q.Set("access_type", "offline")
		q.Set("prompt", "consent") // obnovovací token aj pri opakovanom prepojení
	} else {
		q.Set("prompt", "select_account")
	}
	return ep.Auth + "?" + q.Encode()
}

// Token sú prístupové údaje účtu.
type Token struct {
	Access  string
	Refresh string
	Expiry  time.Time
	Email   string // z id_token (iba pri prihlásení)
}

// ErrRevoked — poskytovateľ prístup odmietol (zrušený, vypršaný); treba
// sa znova prihlásiť.
var ErrRevoked = errors.New("prístup ku kalendáru bol zrušený — prepojte kalendár znova")

// Exchange vymení autorizačný kód za tokeny.
func Exchange(ctx context.Context, hc *http.Client, ep Endpoints, p Provider, cfg Config, code, verifier, redirect string) (Token, error) {
	return token(ctx, hc, ep, p, cfg, url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"code_verifier": {verifier}, "redirect_uri": {redirect}})
}

// Refresh obnoví prístupový token (Microsoft vráti aj nový obnovovací).
func Refresh(ctx context.Context, hc *http.Client, ep Endpoints, p Provider, cfg Config, refresh string) (Token, error) {
	f := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}
	if p == Microsoft {
		f.Set("scope", scopes[p])
	}
	t, err := token(ctx, hc, ep, p, cfg, f)
	if err == nil && t.Refresh == "" {
		t.Refresh = refresh
	}
	return t, err
}

func token(ctx context.Context, hc *http.Client, ep Endpoints, p Provider, cfg Config, f url.Values) (Token, error) {
	f.Set("client_id", cfg.clientID(p))
	if p == Google && cfg.GoogleClientSecret != "" {
		f.Set("client_secret", cfg.GoogleClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Token, strings.NewReader(f.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := hc.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer res.Body.Close()
	var body struct {
		Access   string `json:"access_token"`
		Refresh  string `json:"refresh_token"`
		Expires  int    `json:"expires_in"`
		IDToken  string `json:"id_token"`
		Error    string `json:"error"`
		ErrorMsg string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return Token{}, fmt.Errorf("odpoveď %s: %d", p.Label(), res.StatusCode)
	}
	if body.Error == "invalid_grant" {
		return Token{}, ErrRevoked
	}
	if res.StatusCode != http.StatusOK || body.Access == "" {
		return Token{}, fmt.Errorf("prihlásenie do %s: %s %s", p.Label(), body.Error, firstLine(body.ErrorMsg))
	}
	return Token{Access: body.Access, Refresh: body.Refresh, Expiry: time.Now().Add(time.Duration(body.Expires) * time.Second),
		Email: emailOf(body.IDToken)}, nil
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return strings.TrimSpace(s)
}

// emailOf prečíta e-mail z id_token (prišiel priamo od poskytovateľa cez
// TLS, podpis sa neoveruje — slúži iba na zobrazenie účtu).
func emailOf(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var c struct {
		Email string `json:"email"`
		Pref  string `json:"preferred_username"`
	}
	_ = json.Unmarshal(raw, &c)
	if c.Email != "" {
		return c.Email
	}
	return c.Pref
}

// Event je termín na prenos: celodenná udalosť.
type Event struct {
	Date        string `json:"date"` // YYYY-MM-DD
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	// Remind: pripomienka 3 dni a 1 deň vopred o 9:00 (Outlook iba deň vopred).
	Remind bool `json:"remind,omitempty"`
}

// Hash je odtlačok obsahu udalosti (zmena → aktualizácia).
func (e Event) Hash() string {
	b, _ := json.Marshal(e)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// Client volá API kalendára s prístupovým tokenom.
type Client struct {
	HTTP   *http.Client
	EP     Endpoints
	P      Provider
	Access string
}

// ErrNotFound — kalendár alebo udalosť už neexistuje (zmazaná v účte).
var ErrNotFound = errors.New("neexistuje")

func (c Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.EP.API+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Access)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return ErrNotFound
	case res.StatusCode == http.StatusUnauthorized:
		return ErrRevoked
	case res.StatusCode >= 300:
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("%s %d: %s", c.P.Label(), res.StatusCode, firstLine(e.Error.Message))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// CreateCalendar vytvorí kalendár a vráti jeho identifikátor.
func (c Client) CreateCalendar(ctx context.Context, name string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	var err error
	if c.P == Google {
		err = c.do(ctx, http.MethodPost, "/calendars", map[string]string{"summary": name, "timeZone": "Europe/Bratislava"}, &out)
	} else {
		err = c.do(ctx, http.MethodPost, "/me/calendars", map[string]string{"name": name}, &out)
	}
	if err == nil && out.ID == "" {
		err = fmt.Errorf("%s nevrátil kalendár", c.P.Label())
	}
	return out.ID, err
}

// CalendarExists overí, že kalendár v účte ešte existuje.
func (c Client) CalendarExists(ctx context.Context, calID string) (bool, error) {
	path := "/calendars/" + url.PathEscape(calID)
	if c.P == Microsoft {
		path = "/me" + path
	}
	err := c.do(ctx, http.MethodGet, path, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// DeleteCalendar zmaže kalendár (aj so všetkými udalosťami).
func (c Client) DeleteCalendar(ctx context.Context, calID string) error {
	path := "/calendars/" + url.PathEscape(calID)
	if c.P == Microsoft {
		path = "/me" + path
	}
	err := c.do(ctx, http.MethodDelete, path, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (c Client) body(e Event) any {
	next := e.Date
	if t, err := time.Parse("2006-01-02", e.Date); err == nil {
		next = t.AddDate(0, 0, 1).Format("2006-01-02")
	}
	if c.P == Google {
		rem := map[string]any{"useDefault": false, "overrides": []any{}}
		if e.Remind {
			// celodenná udalosť začína o polnoci: 3 dni a 1 deň vopred o 9:00
			rem["overrides"] = []any{map[string]any{"method": "popup", "minutes": 3*1440 - 540}, map[string]any{"method": "popup", "minutes": 1440 - 540}}
		}
		return map[string]any{"summary": e.Title, "description": e.Description, "start": map[string]string{"date": e.Date},
			"end": map[string]string{"date": next}, "transparency": "transparent", "reminders": rem}
	}
	return map[string]any{"subject": e.Title, "body": map[string]string{"contentType": "text", "content": e.Description},
		"start":    map[string]string{"dateTime": e.Date + "T00:00:00", "timeZone": "Europe/Bratislava"},
		"end":      map[string]string{"dateTime": next + "T00:00:00", "timeZone": "Europe/Bratislava"},
		"isAllDay": true, "showAs": "free", "isReminderOn": e.Remind, "reminderMinutesBeforeStart": 1440 - 540}
}

// Upsert vytvorí udalosť (remote = "") alebo ju prepíše; vráti jej identifikátor.
func (c Client) Upsert(ctx context.Context, calID, remote string, e Event) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	cal := "/calendars/" + url.PathEscape(calID) + "/events"
	if c.P == Microsoft {
		cal = "/me" + cal
	}
	if remote != "" {
		path := cal + "/" + url.PathEscape(remote)
		method := http.MethodPut
		if c.P == Microsoft {
			path, method = "/me/events/"+url.PathEscape(remote), http.MethodPatch
		}
		err := c.do(ctx, method, path, c.body(e), &out)
		if !errors.Is(err, ErrNotFound) {
			if out.ID == "" {
				out.ID = remote
			}
			return out.ID, err
		}
		// udalosť niekto v účte zmazal → vytvoriť znova
	}
	if err := c.do(ctx, http.MethodPost, cal, c.body(e), &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// Delete zmaže udalosť (už zmazaná nevadí).
func (c Client) Delete(ctx context.Context, calID, remote string) error {
	path := "/calendars/" + url.PathEscape(calID) + "/events/" + url.PathEscape(remote)
	if c.P == Microsoft {
		path = "/me/events/" + url.PathEscape(remote)
	}
	err := c.do(ctx, http.MethodDelete, path, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Item je prenesená udalosť: identifikátor v účte a odtlačok obsahu.
type Item struct {
	Remote string `json:"r"`
	Hash   string `json:"h"`
}

// Result je výsledok synchronizácie.
type Result struct {
	Created, Updated, Deleted, Failed int
	// CalendarGone: kalendár Fiškus v účte niekto zmazal.
	CalendarGone bool
	Err          error // prvá chyba (ostatné udalosti sa skúsili)
}

// Sync prenesie želané udalosti (kľúč = identifikátor termínu vo Fiškuse):
// nové vytvorí, zmenené prepíše, zaniknuté zmaže. Stav sa upraví na mieste.
func Sync(ctx context.Context, c Client, calID string, want map[string]Event, state map[string]Item) Result {
	var res Result
	fail := func(err error) bool {
		res.Failed++
		if res.Err == nil {
			res.Err = err
		}
		if errors.Is(err, ErrNotFound) {
			res.CalendarGone = true
		}
		return errors.Is(err, ErrRevoked) || res.CalendarGone || ctx.Err() != nil
	}
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := want[k]
		h := e.Hash()
		it, ok := state[k]
		if ok && it.Hash == h {
			continue
		}
		id, err := c.Upsert(ctx, calID, it.Remote, e)
		if err != nil {
			if fail(err) {
				return res
			}
			continue
		}
		if ok {
			res.Updated++
		} else {
			res.Created++
		}
		state[k] = Item{Remote: id, Hash: h}
	}
	var gone []string
	for k := range state {
		if _, ok := want[k]; !ok {
			gone = append(gone, k)
		}
	}
	sort.Strings(gone)
	for _, k := range gone {
		if err := c.Delete(ctx, calID, state[k].Remote); err != nil {
			if fail(err) {
				return res
			}
			continue
		}
		delete(state, k)
		res.Deleted++
	}
	return res
}
