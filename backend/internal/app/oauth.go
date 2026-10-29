package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// OAuth 2.1 pre vzdialených klientov MCP (Claude na webe a v mobile).
// Fiškus je zároveň autorizačný server aj chránený zdroj (/mcp).
//
//   - Registrácia klienta (RFC 7591) je bez stavu: client_id je podpísaný
//     záznam s názvom a povolenými adresami návratu, nič sa neukladá.
//   - Autorizačný kód žije v pamäti 1 minútu a platí raz; povinné PKCE S256.
//   - Povolenie (grant) je API kľúč používateľa s názvom aplikácie — vidno ho
//     a dá sa zrušiť v časti Prístup pre AI. Jeho odtlačok je odtlačok
//     obnovovacieho tokenu, ktorý sa pri každom použití vymení; opakované
//     použitia starého tokenu (krádež) povolenie zruší.
//   - Prístupový token je podpísaný, platí hodinu a pri každom použití sa
//     overí, že povolenie nie je zrušené.

const (
	OAuthAccessPrefix  = "fo_"
	OAuthClientPrefix  = "fc_"
	OAuthRefreshPrefix = "fr_"
	OAuthAccessTTL     = time.Hour
	oauthCodeTTL       = time.Minute
	oauthSecretSetting = "oauth.secret"
	oauthGrantPrefix   = "OAuth" // APIKey.Prefix povolenia (zobrazí sa v zozname kľúčov)
)

// OAuthError je chyba podľa RFC 6749 (kód ide klientovi).
type OAuthError struct {
	Code        string // invalid_request, invalid_client, invalid_grant, unsupported_grant_type…
	Description string
}

func (e *OAuthError) Error() string { return e.Code + ": " + e.Description }

func oauthErr(code, desc string) error { return &OAuthError{code, desc} }

// ErrOAuthOff: OAuth je iba v serverovej verzii (desktop používa most fiskus-mcp).
var ErrOAuthOff = errors.New("Pripojenie aplikácií cez OAuth je iba v serverovej verzii")

// OAuthClient je zaregistrovaná aplikácia (obsah podpísaného client_id).
type OAuthClient struct {
	Name         string   `json:"n"`
	RedirectURIs []string `json:"r"`
	IssuedAt     int64    `json:"t"`
}

type oauthCode struct {
	clientID, redirectURI, challenge, resource string
	userID                                     int64
	readOnly                                   bool
	exp                                        time.Time
}

type oauthState struct {
	mu    sync.Mutex
	key   []byte
	codes map[string]*oauthCode
}

func (a *App) oauthOn() error {
	if a.Caps.Mode != edition.Server {
		return ErrOAuthOff
	}
	return nil
}

// oauthKey vráti tajný kľúč na podpisy (vytvorí ho pri prvom použití).
func (a *App) oauthKey(ctx context.Context) ([]byte, error) {
	a.oauth.mu.Lock()
	defer a.oauth.mu.Unlock()
	if a.oauth.key != nil {
		return a.oauth.key, nil
	}
	v, err := a.Store.GetSetting(ctx, oauthSecretSetting)
	if errors.Is(err, store.ErrNotFound) {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		v = base64.RawStdEncoding.EncodeToString(b)
		if err := a.Store.SetSetting(ctx, oauthSecretSetting, v); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	key, err := base64.RawStdEncoding.DecodeString(v)
	if err != nil || len(key) < 32 {
		return nil, errors.New("poškodený kľúč OAuth v nastaveniach")
	}
	a.oauth.key = key
	return key, nil
}

var b64 = base64.RawURLEncoding

// sign vytvorí token prefix + payload + "." + HMAC.
func (a *App) sign(ctx context.Context, prefix string, payload any) (string, error) {
	key, err := a.oauthKey(ctx)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	msg := prefix + b64.EncodeToString(body)
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return msg + "." + b64.EncodeToString(m.Sum(nil)), nil
}

// verify overí podpis a načíta obsah tokenu.
func (a *App) verify(ctx context.Context, token, prefix string, into any) bool {
	if !strings.HasPrefix(token, prefix) || len(token) > 4096 {
		return false
	}
	msg, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	key, err := a.oauthKey(ctx)
	if err != nil {
		return false
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	got, err := b64.DecodeString(sig)
	if err != nil || !hmac.Equal(got, m.Sum(nil)) {
		return false
	}
	body, err := b64.DecodeString(strings.TrimPrefix(msg, prefix))
	return err == nil && json.Unmarshal(body, into) == nil
}

// validRedirect: https s hostom, alebo http iba na vlastný počítač (desktopoví klienti).
func validRedirect(u string) bool {
	if len(u) > 500 {
		return false
	}
	p, err := url.Parse(u)
	if err != nil || p.Host == "" || p.Fragment != "" || p.User != nil {
		return false
	}
	switch p.Scheme {
	case "https":
		return true
	case "http":
		h := p.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}

// RegisterOAuthClient zaregistruje aplikáciu (dynamická registrácia klienta).
func (a *App) RegisterOAuthClient(ctx context.Context, name string, redirectURIs []string) (string, OAuthClient, error) {
	if err := a.oauthOn(); err != nil {
		return "", OAuthClient{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Aplikácia"
	}
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	if len(redirectURIs) == 0 || len(redirectURIs) > 5 {
		return "", OAuthClient{}, oauthErr("invalid_redirect_uri", "zadajte 1 až 5 adries redirect_uris")
	}
	for _, u := range redirectURIs {
		if !validRedirect(u) {
			return "", OAuthClient{}, oauthErr("invalid_redirect_uri", "adresa návratu musí byť https (http iba localhost): "+u)
		}
	}
	c := OAuthClient{Name: name, RedirectURIs: redirectURIs, IssuedAt: a.now().Unix()}
	id, err := a.sign(ctx, OAuthClientPrefix, c)
	return id, c, err
}

// OAuthClientByID overí client_id a vráti aplikáciu.
func (a *App) OAuthClientByID(ctx context.Context, id string) (OAuthClient, error) {
	var c OAuthClient
	if !a.verify(ctx, id, OAuthClientPrefix, &c) {
		return c, oauthErr("invalid_client", "neznáma aplikácia (client_id)")
	}
	return c, nil
}

// AuthRequest sú parametre žiadosti o povolenie (/oauth/authorize).
type AuthRequest struct {
	ResponseType  string `json:"response_type"`
	ClientID      string `json:"client_id"`
	RedirectURI   string `json:"redirect_uri"`
	State         string `json:"state"`
	CodeChallenge string `json:"code_challenge"`
	Method        string `json:"code_challenge_method"`
	Scope         string `json:"scope"`
	Resource      string `json:"resource"`
}

// AuthRequestFrom načíta žiadosť z parametrov adresy.
func AuthRequestFrom(q url.Values) AuthRequest {
	return AuthRequest{q.Get("response_type"), q.Get("client_id"), q.Get("redirect_uri"), q.Get("state"),
		q.Get("code_challenge"), q.Get("code_challenge_method"), q.Get("scope"), q.Get("resource")}
}

// CheckAuthRequest overí žiadosť. Chyba v client_id alebo redirect_uri sa
// nesmie poslať na redirect_uri (mohla by to byť cudzia adresa).
func (a *App) CheckAuthRequest(ctx context.Context, r AuthRequest) (OAuthClient, error) {
	if err := a.oauthOn(); err != nil {
		return OAuthClient{}, err
	}
	c, err := a.OAuthClientByID(ctx, r.ClientID)
	if err != nil {
		return c, err
	}
	ok := false
	for _, u := range c.RedirectURIs {
		if u == r.RedirectURI {
			ok = true
		}
	}
	if !ok {
		return c, oauthErr("invalid_request", "adresa návratu nie je zaregistrovaná pre túto aplikáciu")
	}
	return c, nil
}

// redirectParams overí zvyšok žiadosti (chyby už smú ísť na redirect_uri).
func redirectParams(r AuthRequest) error {
	switch {
	case r.ResponseType != "code":
		return oauthErr("unsupported_response_type", "podporované je iba response_type=code")
	case r.Method != "S256":
		return oauthErr("invalid_request", "povinné je PKCE s code_challenge_method=S256")
	case len(r.CodeChallenge) < 43 || len(r.CodeChallenge) > 128:
		return oauthErr("invalid_request", "chýba alebo je neplatný code_challenge")
	}
	return nil
}

// withQuery pridá parametre k adrese návratu.
func withQuery(base string, v url.Values) string {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + v.Encode()
}

// ApproveOAuth: prihlásený používateľ povolil aplikácii prístup. Vráti adresu
// návratu s kódom (alebo s chybou žiadosti).
func (a *App) ApproveOAuth(ctx context.Context, r AuthRequest, readOnly bool) (string, error) {
	c, err := a.CheckAuthRequest(ctx, r)
	if err != nil {
		return "", err
	}
	act := auth.ActorFrom(ctx)
	if act.UserID == 0 || act.KeyID != 0 {
		return "", ErrForbidden // povoliť smie iba človek prihlásený heslom, nie kľúč
	}
	back := url.Values{}
	if r.State != "" {
		back.Set("state", r.State)
	}
	if err := redirectParams(r); err != nil {
		var oe *OAuthError
		errors.As(err, &oe)
		back.Set("error", oe.Code)
		back.Set("error_description", oe.Description)
		return withQuery(r.RedirectURI, back), nil
	}
	raw, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	a.oauth.mu.Lock()
	if a.oauth.codes == nil {
		a.oauth.codes = map[string]*oauthCode{}
	}
	now := a.now()
	for k, v := range a.oauth.codes {
		if now.After(v.exp) {
			delete(a.oauth.codes, k)
		}
	}
	a.oauth.codes[hash] = &oauthCode{clientID: r.ClientID, redirectURI: r.RedirectURI, challenge: r.CodeChallenge,
		resource: r.Resource, userID: act.UserID, readOnly: readOnly, exp: now.Add(oauthCodeTTL)}
	a.oauth.mu.Unlock()
	a.record(ctx, 0, audit.CatAI, "oauth_approve", "oauth", 0, "Povolený prístup aplikácii „"+c.Name+"“ ("+accessText(readOnly)+")", nil, nil)
	back.Set("code", raw)
	return withQuery(r.RedirectURI, back), nil
}

// DenyOAuth: používateľ prístup zamietol.
func (a *App) DenyOAuth(ctx context.Context, r AuthRequest) (string, error) {
	if _, err := a.CheckAuthRequest(ctx, r); err != nil {
		return "", err
	}
	back := url.Values{"error": {"access_denied"}, "error_description": {"Používateľ prístup nepovolil"}}
	if r.State != "" {
		back.Set("state", r.State)
	}
	return withQuery(r.RedirectURI, back), nil
}

func accessText(readOnly bool) string {
	if readOnly {
		return "iba na čítanie"
	}
	return "s právami používateľa"
}

// TokenResponse je odpoveď tokenového koncového bodu.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope,omitempty"`
}

type accessClaims struct {
	Grant int64 `json:"g"`
	Exp   int64 `json:"e"`
}

// tokens vydá prístupový token a nový obnovovací token pre povolenie.
func (a *App) tokens(ctx context.Context, grantID int64) (TokenResponse, string, error) {
	access, err := a.sign(ctx, OAuthAccessPrefix, accessClaims{grantID, a.now().Add(OAuthAccessTTL).Unix()})
	if err != nil {
		return TokenResponse{}, "", err
	}
	raw, _, err := auth.NewToken()
	if err != nil {
		return TokenResponse{}, "", err
	}
	refresh := OAuthRefreshPrefix + strconv.FormatInt(grantID, 10) + "." + raw
	return TokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int(OAuthAccessTTL.Seconds()), RefreshToken: refresh, Scope: "mcp"},
		auth.TokenHash(refresh), nil
}

// ExchangeCode vymení autorizačný kód za tokeny (overí PKCE).
func (a *App) ExchangeCode(ctx context.Context, clientID, code, redirectURI, verifier string) (TokenResponse, error) {
	if err := a.oauthOn(); err != nil {
		return TokenResponse{}, err
	}
	c, err := a.OAuthClientByID(ctx, clientID)
	if err != nil {
		return TokenResponse{}, err
	}
	h := auth.TokenHash(code)
	a.oauth.mu.Lock()
	oc := a.oauth.codes[h]
	delete(a.oauth.codes, h) // kód platí raz, aj pri chybe
	a.oauth.mu.Unlock()
	if oc == nil || a.now().After(oc.exp) || oc.clientID != clientID || oc.redirectURI != redirectURI {
		return TokenResponse{}, oauthErr("invalid_grant", "neplatný alebo použitý autorizačný kód")
	}
	sum := sha256.Sum256([]byte(verifier))
	if len(verifier) < 43 || subtle.ConstantTimeCompare([]byte(b64.EncodeToString(sum[:])), []byte(oc.challenge)) != 1 {
		return TokenResponse{}, oauthErr("invalid_grant", "code_verifier nesedí (PKCE)")
	}
	u, err := a.Store.GetUser(ctx, oc.userID)
	if err != nil || !u.Active {
		return TokenResponse{}, oauthErr("invalid_grant", "používateľ nie je aktívny")
	}
	k := auth.APIKey{UserID: u.ID, Name: c.Name, Prefix: oauthGrantPrefix, ReadOnly: oc.readOnly, CreatedAt: a.now().Unix()}
	// odtlačok zatiaľ náhodný; po vytvorení (ID je v tokene) sa vymení za skutočný
	_, tmp, err := auth.NewToken()
	if err != nil {
		return TokenResponse{}, err
	}
	k.Hash = tmp
	if err := a.Store.CreateAPIKey(ctx, &k); err != nil {
		return TokenResponse{}, err
	}
	resp, hash, err := a.tokens(ctx, k.ID)
	if err != nil {
		return TokenResponse{}, err
	}
	if err := a.Store.RotateAPIKey(ctx, k.ID, tmp, hash); err != nil {
		return TokenResponse{}, err
	}
	uctx := auth.WithActor(ctx, actorOf(u, auth.ActorFrom(ctx)))
	a.record(uctx, 0, audit.CatAI, "oauth_grant", "api_key", k.ID,
		fmt.Sprintf("Pripojená aplikácia „%s“ cez OAuth (%s)", c.Name, accessText(k.ReadOnly)), nil, k)
	return resp, nil
}

// RefreshOAuth vydá nové tokeny za obnovovací token (ten sa vymení).
func (a *App) RefreshOAuth(ctx context.Context, clientID, refresh string) (TokenResponse, error) {
	if err := a.oauthOn(); err != nil {
		return TokenResponse{}, err
	}
	if _, err := a.OAuthClientByID(ctx, clientID); err != nil {
		return TokenResponse{}, err
	}
	bad := oauthErr("invalid_grant", "neplatný alebo zrušený obnovovací token")
	rest, ok := strings.CutPrefix(refresh, OAuthRefreshPrefix)
	idText, _, ok2 := strings.Cut(rest, ".")
	id, err := strconv.ParseInt(idText, 10, 64)
	if !ok || !ok2 || err != nil {
		return TokenResponse{}, bad
	}
	k, err := a.Store.GetAPIKey(ctx, id)
	if err != nil || k.Revoked || k.Prefix != oauthGrantPrefix {
		return TokenResponse{}, bad
	}
	if u, err := a.Store.GetUser(ctx, k.UserID); err != nil || !u.Active {
		return TokenResponse{}, bad
	}
	old := auth.TokenHash(refresh)
	resp, hash, err := a.tokens(ctx, k.ID)
	if err != nil {
		return TokenResponse{}, err
	}
	if subtle.ConstantTimeCompare([]byte(old), []byte(k.Hash)) != 1 || a.Store.RotateAPIKey(ctx, k.ID, old, hash) != nil {
		// starý (už vymenený) token: niekto ho mohol ukradnúť — povolenie zrušíme
		_ = a.Store.RevokeAPIKey(ctx, k.UserID, k.ID)
		a.record(ctx, 0, audit.CatAI, "oauth_reuse", "api_key", k.ID,
			"Zrušené povolenie „"+k.Name+"“: opakovane použitý obnovovací token", nil, nil)
		return TokenResponse{}, bad
	}
	return resp, nil
}

// OAuthActor overí prístupový token a vráti actora s právami používateľa.
func (a *App) OAuthActor(ctx context.Context, token string, channel auth.Channel, ip string) (auth.Actor, error) {
	if a.oauthOn() != nil {
		return auth.Actor{}, ErrBadKey
	}
	var c accessClaims
	if !a.verify(ctx, token, OAuthAccessPrefix, &c) || a.now().Unix() > c.Exp {
		return auth.Actor{}, ErrBadKey
	}
	k, err := a.Store.GetAPIKey(ctx, c.Grant)
	if err != nil || k.Revoked || k.Prefix != oauthGrantPrefix {
		return auth.Actor{}, ErrBadKey
	}
	return a.keyActor(ctx, k, channel, ip)
}
