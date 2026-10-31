package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/calendar"
	"github.com/palo-krnac/fiscus/backend/internal/calsync"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Prepojenie kalendára termínov s Google Kalendárom alebo Outlookom:
// Fiškus sám posiela termíny do samostatného kalendára v účte používateľa
// (funguje aj v desktopovej verzii, ktorej odber cez adresu Google
// nestiahne). Obnovovací token je tajný: ukladá sa iba v nastaveniach
// a nikdy sa nevracia cez API ani nezapisuje do denníka.

// CalSyncCallbackPath je adresa návratu po prihlásení u poskytovateľa.
const CalSyncCallbackPath = "/api/calendar-sync/callback"

// calSyncWindow: termíny mesiac dozadu a rok dopredu.
const (
	calSyncBack    = 31
	calSyncForward = 365
)

// calSyncConn je prepojenie firmy (nastavenie calsync.<cid>).
type calSyncConn struct {
	Provider   calsync.Provider        `json:"provider"`
	Account    string                  `json:"account"`
	CalendarID string                  `json:"calendarId"`
	Refresh    string                  `json:"refresh"` // tajné
	UserID     int64                   `json:"userId"`  // kto prepojil (0 = lokálny používateľ desktopu)
	Created    int64                   `json:"created"`
	LastSync   int64                   `json:"lastSync,omitempty"`
	LastError  string                  `json:"lastError,omitempty"`
	Items      map[string]calsync.Item `json:"items,omitempty"`
}

func calSyncKey(companyID int64) string { return fmt.Sprintf("calsync.%d", companyID) }

const calSyncConfigKey = "calsync.config"

// CalSyncStatus je stav prepojenia (bez tajomstiev).
type CalSyncStatus struct {
	Google    bool             `json:"google"`    // poskytovateľ je nastavený
	Microsoft bool             `json:"microsoft"` // (prihlasovacie údaje aplikácie)
	Connected bool             `json:"connected"`
	Provider  calsync.Provider `json:"provider,omitempty"`
	Label     string           `json:"label,omitempty"`
	Account   string           `json:"account,omitempty"`
	LastSync  int64            `json:"lastSync,omitempty"`
	LastError string           `json:"lastError,omitempty"`
	Events    int              `json:"events"`
	// Config: prihlasovacie údaje aplikácie pre správcu (bez tajomstva).
	Config *CalSyncConfigView `json:"config,omitempty"`
}

// CalSyncConfigView sú prihlasovacie údaje aplikácie na zobrazenie.
type CalSyncConfigView struct {
	GoogleClientID    string `json:"googleClientId"`
	GoogleSecretSet   bool   `json:"googleSecretSet"`
	MicrosoftClientID string `json:"microsoftClientId"`
	// Builtin: údaje sú zabudované v programe alebo zadané pri spustení.
	Builtin bool `json:"builtin"`
}

// calSyncState sú rozpracované prihlásenia (state → overovací kód PKCE).
type calSyncState struct {
	mu      sync.Mutex
	pending map[string]calSyncPending
	tokens  map[int64]calsync.Token // prístupové tokeny v pamäti
	run     sync.Mutex              // jedna synchronizácia naraz
}

type calSyncPending struct {
	companyID int64
	userID    int64
	provider  calsync.Provider
	verifier  string
	redirect  string
	expires   time.Time
}

func (a *App) calSyncConfig(ctx context.Context) (calsync.Config, bool, error) {
	if a.CalSync.Has(calsync.Google) || a.CalSync.Has(calsync.Microsoft) {
		return a.CalSync, true, nil
	}
	var c calsync.Config
	raw, err := a.Store.GetSetting(ctx, calSyncConfigKey)
	if errors.Is(err, store.ErrNotFound) {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	return c, false, json.Unmarshal([]byte(raw), &c)
}

// SetCalSyncConfig uloží prihlasovacie údaje aplikácie (iba hlavný správca;
// prázdne tajomstvo ponechá uložené).
func (a *App) SetCalSyncConfig(ctx context.Context, c calsync.Config) error {
	if !auth.ActorFrom(ctx).SuperAdmin {
		return ErrForbidden
	}
	old, builtin, err := a.calSyncConfig(ctx)
	if err != nil {
		return err
	}
	if builtin {
		return mfaFieldErr("googleClientId", "Prihlasovacie údaje sú zadané pri spustení programu")
	}
	c.GoogleClientID, c.MicrosoftClientID = strings.TrimSpace(c.GoogleClientID), strings.TrimSpace(c.MicrosoftClientID)
	c.GoogleClientSecret = strings.TrimSpace(c.GoogleClientSecret)
	if c.GoogleClientSecret == "" && c.GoogleClientID == old.GoogleClientID {
		c.GoogleClientSecret = old.GoogleClientSecret
	}
	if c.GoogleClientID == "" {
		c.GoogleClientSecret = ""
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := a.Store.SetSetting(ctx, calSyncConfigKey, string(b)); err != nil {
		return err
	}
	a.record(ctx, 0, audit.CatSettings, "update", "calendar_sync", 0, "Prihlasovacie údaje pre Google Kalendár a Outlook", nil, nil)
	return nil
}

func (a *App) loadCalSync(ctx context.Context, companyID int64) (*calSyncConn, error) {
	raw, err := a.Store.GetSetting(ctx, calSyncKey(companyID))
	if errors.Is(err, store.ErrNotFound) || raw == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c calSyncConn
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (a *App) saveCalSync(ctx context.Context, companyID int64, c *calSyncConn) error {
	if c == nil {
		return a.Store.SetSetting(ctx, calSyncKey(companyID), "")
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, calSyncKey(companyID), string(b))
}

// CalendarSyncStatus vráti stav prepojenia firmy.
func (a *App) CalendarSyncStatus(ctx context.Context, companyID int64) (CalSyncStatus, error) {
	if err := a.Require(ctx, companyID, ""); err != nil {
		return CalSyncStatus{}, err
	}
	cfg, builtin, err := a.calSyncConfig(ctx)
	if err != nil {
		return CalSyncStatus{}, err
	}
	st := CalSyncStatus{Google: cfg.Has(calsync.Google), Microsoft: cfg.Has(calsync.Microsoft)}
	if auth.ActorFrom(ctx).SuperAdmin {
		st.Config = &CalSyncConfigView{GoogleClientID: cfg.GoogleClientID, GoogleSecretSet: cfg.GoogleClientSecret != "",
			MicrosoftClientID: cfg.MicrosoftClientID, Builtin: builtin}
	}
	c, err := a.loadCalSync(ctx, companyID)
	if err != nil || c == nil {
		return st, err
	}
	st.Connected, st.Provider, st.Label, st.Account = true, c.Provider, c.Provider.Label(), c.Account
	st.LastSync, st.LastError, st.Events = c.LastSync, c.LastError, len(c.Items)
	return st, nil
}

func (a *App) httpClient() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (a *App) calEndpoints(p calsync.Provider) calsync.Endpoints {
	if ep, ok := a.CalSyncEndpoints[p]; ok {
		return ep
	}
	return calsync.DefaultEndpoints[p]
}

// StartCalendarSync začne prepojenie: vráti adresu prihlásenia u
// poskytovateľa. origin je adresa Fiškusa v prehliadači (napr.
// http://127.0.0.1:8765), kam sa prihlásenie vráti.
func (a *App) StartCalendarSync(ctx context.Context, companyID int64, p calsync.Provider, origin string) (string, error) {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return "", err
	}
	cfg, _, err := a.calSyncConfig(ctx)
	if err != nil {
		return "", err
	}
	if !cfg.Has(p) {
		return "", mfaFieldErr("provider", p.Label()+" nie je v programe nastavený — správca musí zadať prihlasovacie údaje aplikácie")
	}
	origin = strings.TrimRight(origin, "/")
	if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
		return "", mfaFieldErr("origin", "Neplatná adresa programu")
	}
	redirect := origin + CalSyncCallbackPath
	if p == calsync.Microsoft {
		// Microsoft povoľuje pri lokálnej adrese ľubovoľný port, ale iba „localhost“
		redirect = strings.Replace(redirect, "://127.0.0.1:", "://localhost:", 1)
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	state := base64.RawURLEncoding.EncodeToString(b)
	verifier, challenge := calsync.Verifier()
	a.calSync.mu.Lock()
	if a.calSync.pending == nil {
		a.calSync.pending = map[string]calSyncPending{}
	}
	now := time.Now()
	for k, v := range a.calSync.pending {
		if now.After(v.expires) {
			delete(a.calSync.pending, k)
		}
	}
	a.calSync.pending[state] = calSyncPending{companyID: companyID, userID: auth.ActorFrom(ctx).UserID, provider: p,
		verifier: verifier, redirect: redirect, expires: now.Add(10 * time.Minute)}
	a.calSync.mu.Unlock()
	return calsync.AuthURL(a.calEndpoints(p), p, cfg, redirect, state, challenge), nil
}

// ErrCalSyncState — prihlásenie vypršalo alebo neprišlo z tohto programu.
var ErrCalSyncState = errors.New("prihlásenie vypršalo — skúste prepojiť kalendár znova")

// FinishCalendarSync dokončí prepojenie po návrate od poskytovateľa:
// vymení kód za tokeny, vytvorí kalendár a prenesie termíny. Vráti firmu.
func (a *App) FinishCalendarSync(ctx context.Context, state, code string) (int64, error) {
	a.calSync.mu.Lock()
	pend, ok := a.calSync.pending[state]
	delete(a.calSync.pending, state)
	a.calSync.mu.Unlock()
	if !ok || time.Now().After(pend.expires) || code == "" {
		return 0, ErrCalSyncState
	}
	ctx, err := a.calSyncActor(ctx, pend.userID)
	if err != nil {
		return 0, err
	}
	if err := a.Require(ctx, pend.companyID, auth.PermSettingsWrite); err != nil {
		return 0, err
	}
	cfg, _, err := a.calSyncConfig(ctx)
	if err != nil {
		return 0, err
	}
	ep := a.calEndpoints(pend.provider)
	tok, err := calsync.Exchange(ctx, a.httpClient(), ep, pend.provider, cfg, code, pend.verifier, pend.redirect)
	if err != nil {
		return 0, err
	}
	if tok.Refresh == "" {
		return 0, fmt.Errorf("%s nevrátil trvalý prístup — v účte zrušte prístup pre Fiškus a prepojte znova", pend.provider.Label())
	}
	co, err := a.Store.GetCompany(ctx, pend.companyID)
	if err != nil {
		return 0, err
	}
	cl := calsync.Client{HTTP: a.httpClient(), EP: ep, P: pend.provider, Access: tok.Access}
	calID, err := cl.CreateCalendar(ctx, "Fiškus – "+co.Name)
	if err != nil {
		return 0, err
	}
	old, _ := a.loadCalSync(ctx, pend.companyID)
	if old != nil && old.Provider == pend.provider {
		_ = a.disconnectRemote(ctx, old) // predchádzajúci kalendár Fiškus v účte
	}
	conn := &calSyncConn{Provider: pend.provider, Account: tok.Email, CalendarID: calID, Refresh: tok.Refresh,
		UserID: pend.userID, Created: time.Now().Unix(), Items: map[string]calsync.Item{}}
	if err := a.saveCalSync(ctx, pend.companyID, conn); err != nil {
		return 0, err
	}
	a.calSync.mu.Lock()
	if a.calSync.tokens == nil {
		a.calSync.tokens = map[int64]calsync.Token{}
	}
	a.calSync.tokens[pend.companyID] = tok
	a.calSync.mu.Unlock()
	a.record(ctx, pend.companyID, audit.CatSettings, "create", "calendar_sync", 0,
		"Prepojený kalendár termínov: "+pend.provider.Label()+" "+tok.Email, nil, nil)
	_, _ = a.SyncCalendar(ctx, pend.companyID)
	return pend.companyID, nil
}

// calSyncActor vráti kontext s používateľom, ktorý kalendár prepojil.
func (a *App) calSyncActor(ctx context.Context, userID int64) (context.Context, error) {
	if userID == 0 {
		if a.Caps.Mode != edition.Desktop {
			return ctx, ErrForbidden
		}
		return auth.WithActor(ctx, auth.Local), nil
	}
	u, err := a.Store.GetUser(ctx, userID)
	if err != nil || !u.Active {
		return ctx, ErrForbidden
	}
	act := actorOf(u, auth.Actor{})
	act.Name += " · kalendár"
	return auth.WithActor(ctx, act), nil
}

// accessToken vráti platný prístupový token (obnoví ho podľa potreby).
func (a *App) calAccess(ctx context.Context, companyID int64, c *calSyncConn, cfg calsync.Config) (string, error) {
	a.calSync.mu.Lock()
	tok, ok := a.calSync.tokens[companyID]
	a.calSync.mu.Unlock()
	if ok && time.Until(tok.Expiry) > time.Minute {
		return tok.Access, nil
	}
	tok, err := calsync.Refresh(ctx, a.httpClient(), a.calEndpoints(c.Provider), c.Provider, cfg, c.Refresh)
	if err != nil {
		return "", err
	}
	if tok.Refresh != c.Refresh {
		c.Refresh = tok.Refresh // Microsoft vymieňa obnovovací token
	}
	a.calSync.mu.Lock()
	if a.calSync.tokens == nil {
		a.calSync.tokens = map[int64]calsync.Token{}
	}
	a.calSync.tokens[companyID] = tok
	a.calSync.mu.Unlock()
	return tok.Access, nil
}

// CalSyncResult je výsledok prenosu termínov.
type CalSyncResult struct {
	Created int    `json:"created"`
	Updated int    `json:"updated"`
	Deleted int    `json:"deleted"`
	Failed  int    `json:"failed"`
	Error   string `json:"error,omitempty"`
}

// SyncCalendar prenesie termíny firmy do prepojeného kalendára.
func (a *App) SyncCalendar(ctx context.Context, companyID int64) (CalSyncResult, error) {
	// zapisuje do cudzieho kalendára — iba s právom meniť nastavenia firmy
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return CalSyncResult{}, err
	}
	a.calSync.run.Lock()
	defer a.calSync.run.Unlock()
	c, err := a.loadCalSync(ctx, companyID)
	if err != nil {
		return CalSyncResult{}, err
	}
	if c == nil {
		return CalSyncResult{}, store.ErrNotFound
	}
	cfg, _, err := a.calSyncConfig(ctx)
	if err != nil {
		return CalSyncResult{}, err
	}
	res, serr := a.syncCalendar(ctx, companyID, c, cfg)
	c.LastError = ""
	if serr != nil {
		c.LastError = serr.Error()
		res.Error = c.LastError
	} else {
		c.LastSync = a.clock().Unix()
	}
	if err := a.saveCalSync(ctx, companyID, c); err != nil {
		return res, err
	}
	return res, nil
}

func (a *App) syncCalendar(ctx context.Context, companyID int64, c *calSyncConn, cfg calsync.Config) (CalSyncResult, error) {
	var res CalSyncResult
	if !cfg.Has(c.Provider) {
		return res, errors.New(c.Provider.Label() + " nie je v programe nastavený")
	}
	actx, err := a.calSyncActor(ctx, c.UserID)
	if err != nil {
		return res, errors.New("používateľ, ktorý kalendár prepojil, už nemá prístup — prepojte kalendár znova")
	}
	today := a.Today()
	v, err := a.Calendar(actx, companyID, today.AddDays(-calSyncBack), today.AddDays(calSyncForward))
	if err != nil {
		return res, err
	}
	want := map[string]calsync.Event{}
	for _, e := range append(v.Overdue, v.Events...) {
		title, desc, remind, ok := calendar.Summary(e)
		if !ok {
			continue
		}
		want[e.ID] = calsync.Event{Date: e.Date.String(), Title: title, Description: desc, Remind: remind}
	}
	access, err := a.calAccess(ctx, companyID, c, cfg)
	if err != nil {
		return res, err
	}
	cl := calsync.Client{HTTP: a.httpClient(), EP: a.calEndpoints(c.Provider), P: c.Provider, Access: access}
	if c.Items == nil {
		c.Items = map[string]calsync.Item{}
	}
	exists, err := cl.CalendarExists(ctx, c.CalendarID)
	if err != nil {
		return res, err
	}
	r := calsync.Result{CalendarGone: !exists}
	if exists {
		r = calsync.Sync(ctx, cl, c.CalendarID, want, c.Items)
	}
	if r.CalendarGone {
		// kalendár Fiškus v účte niekto zmazal → vytvoriť nový a preniesť všetko
		co, err := a.Store.GetCompany(ctx, companyID)
		if err != nil {
			return res, err
		}
		id, err := cl.CreateCalendar(ctx, "Fiškus – "+co.Name)
		if err != nil {
			return res, err
		}
		c.CalendarID, c.Items = id, map[string]calsync.Item{}
		r = calsync.Sync(ctx, cl, c.CalendarID, want, c.Items)
	}
	res = CalSyncResult{Created: r.Created, Updated: r.Updated, Deleted: r.Deleted, Failed: r.Failed}
	return res, r.Err
}

// disconnectRemote zmaže kalendár Fiškus v účte (iba ak je prístup platný).
func (a *App) disconnectRemote(ctx context.Context, c *calSyncConn) error {
	cfg, _, err := a.calSyncConfig(ctx)
	if err != nil || !cfg.Has(c.Provider) {
		return err
	}
	tok, err := calsync.Refresh(ctx, a.httpClient(), a.calEndpoints(c.Provider), c.Provider, cfg, c.Refresh)
	if err != nil {
		return err
	}
	cl := calsync.Client{HTTP: a.httpClient(), EP: a.calEndpoints(c.Provider), P: c.Provider, Access: tok.Access}
	return cl.DeleteCalendar(ctx, c.CalendarID)
}

// DisconnectCalendarSync zruší prepojenie; removeCalendar zmaže aj kalendár
// Fiškus v účte používateľa.
func (a *App) DisconnectCalendarSync(ctx context.Context, companyID int64, removeCalendar bool) error {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return err
	}
	c, err := a.loadCalSync(ctx, companyID)
	if err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	if removeCalendar {
		if err := a.disconnectRemote(ctx, c); err != nil {
			log.Printf("kalendár %s sa nezmazal: %v", c.Provider.Label(), err)
		}
	}
	a.calSync.mu.Lock()
	delete(a.calSync.tokens, companyID)
	a.calSync.mu.Unlock()
	if err := a.saveCalSync(ctx, companyID, nil); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatSettings, "delete", "calendar_sync", 0, "Zrušené prepojenie kalendára "+c.Provider.Label(), nil, nil)
	return nil
}

// SyncAllCalendars prenesie termíny všetkých prepojených firiem (beží na
// pozadí; chyba sa zapíše k prepojeniu a do denníka bez tajomstiev).
func (a *App) SyncAllCalendars(ctx context.Context) {
	companies, err := a.Store.ListCompanies(ctx)
	if err != nil {
		return
	}
	for _, co := range companies {
		c, err := a.loadCalSync(ctx, co.ID)
		if err != nil || c == nil {
			continue
		}
		actx, err := a.calSyncActor(ctx, c.UserID)
		if err != nil {
			actx = auth.WithActor(ctx, auth.System)
		}
		if res, err := a.SyncCalendar(actx, co.ID); err != nil {
			log.Printf("kalendár firmy %d: %v", co.ID, err)
		} else if res.Error != "" {
			log.Printf("kalendár firmy %d: %s", co.ID, res.Error)
		}
	}
}
