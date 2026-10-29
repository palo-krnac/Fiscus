package api

import (
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
)

// OAuth 2.1 pre vzdialených klientov MCP (Claude na webe a v mobile).
// Iba serverová verzia; desktop sa pripája mostom fiskus-mcp s API kľúčom.
//
//	/.well-known/oauth-protected-resource   (RFC 9728) — kto vydáva tokeny pre /mcp
//	/.well-known/oauth-authorization-server (RFC 8414) — adresy autorizačného servera
//	POST /oauth/register  (RFC 7591) — registrácia aplikácie
//	GET  /oauth/authorize — presmeruje na súhlas v rozhraní (/oauth/povolit)
//	POST /oauth/token     — kód + PKCE alebo obnovovací token → tokeny

func (s *Server) routeOAuth(mux *http.ServeMux, open func(string, http.HandlerFunc)) {
	if !s.authRequired() {
		return
	}
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.protectedResource)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", s.protectedResource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.authServerMeta)
	mux.HandleFunc("POST /oauth/register", s.oauthRegister)
	mux.HandleFunc("GET /oauth/authorize", s.oauthAuthorize)
	mux.HandleFunc("POST /oauth/token", s.oauthToken)
	open("GET /api/oauth/request", s.oauthRequestInfo)
	open("POST /api/oauth/decision", s.oauthDecision)
}

// baseURL je verejná adresa servera: -public-url, inak z požiadavky.
func (s *Server) baseURL(r *http.Request) string {
	if s.PublicURL != "" {
		return strings.TrimRight(s.PublicURL, "/")
	}
	scheme := "http"
	if secure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) protectedResource(w http.ResponseWriter, r *http.Request) {
	b := s.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 b + "/mcp",
		"authorization_servers":    []string{b},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "Fiškus",
	})
}

func (s *Server) authServerMeta(w http.ResponseWriter, r *http.Request) {
	b := s.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                b,
		"authorization_endpoint":                b + "/oauth/authorize",
		"token_endpoint":                        b + "/oauth/token",
		"registration_endpoint":                 b + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mcp"},
	})
}

// oauthJSONErr zapíše chybu vo formáte RFC 6749.
func oauthJSONErr(w http.ResponseWriter, err error) {
	var oe *app.OAuthError
	if !errors.As(err, &oe) {
		if errors.Is(err, app.ErrOAuthOff) {
			oe = &app.OAuthError{Code: "invalid_request", Description: err.Error()}
		} else {
			fail(w, err)
			return
		}
	}
	status := http.StatusBadRequest
	if oe.Code == "invalid_client" {
		status = http.StatusUnauthorized
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]string{"error": oe.Code, "error_description": oe.Description})
}

func (s *Server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	// klienti posielajú aj ďalšie polia (grant_types, logo_uri…) — neodmietať
	if err := decodeMeta(r, &body); err != nil {
		oauthJSONErr(w, &app.OAuthError{Code: "invalid_client_metadata", Description: err.Error()})
		return
	}
	id, c, err := s.App.RegisterOAuthClient(r.Context(), body.ClientName, body.RedirectURIs)
	if err != nil {
		oauthJSONErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": id, "client_id_issued_at": c.IssuedAt, "client_name": c.Name, "redirect_uris": c.RedirectURIs,
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// oauthAuthorize overí aplikáciu a adresu návratu a pošle používateľa na
// súhlas v rozhraní (tam sa prihlási, ak ešte nie je).
func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	req := app.AuthRequestFrom(r.URL.Query())
	if _, err := s.App.CheckAuthRequest(r.Context(), req); err != nil {
		// na neoverenú adresu návratu nepresmerovať — chyba ostane tu
		msg := err.Error()
		var oe *app.OAuthError
		if errors.As(err, &oe) {
			msg = oe.Description
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Fiškus</title>` +
			`<p style="font:16px system-ui;margin:2rem">Žiadosť o prístup k Fiškusu je neplatná: ` + html.EscapeString(msg) + `</p>`))
		return
	}
	http.Redirect(w, r, "/oauth/povolit?"+r.URL.RawQuery, http.StatusFound)
}

// oauthRequestInfo opíše žiadosť pre obrazovku súhlasu.
func (s *Server) oauthRequestInfo(w http.ResponseWriter, r *http.Request) {
	req := app.AuthRequestFrom(r.URL.Query())
	c, err := s.App.CheckAuthRequest(r.Context(), req)
	if err != nil {
		writeOAuthAPIErr(w, err)
		return
	}
	u, _ := url.Parse(req.RedirectURI)
	act := auth.ActorFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"client": c.Name, "redirectHost": u.Host, "user": act.Name})
}

// decodeMeta načíta metadáta klienta; neznáme polia ignoruje.
func decodeMeta(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v)
}

func writeOAuthAPIErr(w http.ResponseWriter, err error) {
	var oe *app.OAuthError
	if errors.As(err, &oe) {
		writeErr(w, http.StatusBadRequest, oe.Description, nil)
		return
	}
	fail(w, err)
}

// oauthDecision: používateľ povolil alebo zamietol prístup; vráti adresu návratu.
func (s *Server) oauthDecision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query    string `json:"query"` // pôvodné parametre /oauth/authorize
		Allow    bool   `json:"allow"`
		ReadOnly bool   `json:"readOnly"`
	}
	if !decode(w, r, &body) {
		return
	}
	q, err := url.ParseQuery(strings.TrimPrefix(body.Query, "?"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatná žiadosť", nil)
		return
	}
	req := app.AuthRequestFrom(q)
	var back string
	if body.Allow {
		back, err = s.App.ApproveOAuth(r.Context(), req, body.ReadOnly)
	} else {
		back, err = s.App.DenyOAuth(r.Context(), req)
	}
	if err != nil {
		writeOAuthAPIErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirect": back})
}

func (s *Server) oauthToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		oauthJSONErr(w, &app.OAuthError{Code: "invalid_request", Description: "očakáva sa application/x-www-form-urlencoded"})
		return
	}
	// bez obmedzenia podľa IP: všetci používatelia Claude chodia z rovnakých
	// adries Anthropicu; kódy a tokeny majú 256 bitov, hádať sa nedajú
	f := r.PostForm
	var (
		tok app.TokenResponse
		err error
	)
	switch f.Get("grant_type") {
	case "authorization_code":
		tok, err = s.App.ExchangeCode(r.Context(), f.Get("client_id"), f.Get("code"), f.Get("redirect_uri"), f.Get("code_verifier"))
	case "refresh_token":
		tok, err = s.App.RefreshOAuth(r.Context(), f.Get("client_id"), f.Get("refresh_token"))
	default:
		err = &app.OAuthError{Code: "unsupported_grant_type", Description: "podporované sú authorization_code a refresh_token"}
	}
	if err != nil {
		oauthJSONErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, tok)
}
