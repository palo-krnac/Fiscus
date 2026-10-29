package api

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
)

const maxMCPBody = 4 << 20

// mcpHandler: POST /mcp — Model Context Protocol pre AI asistentov.
// Prihlásenie hlavičkou „Authorization: Bearer fk_…“ (API kľúč používateľa).
func (s *Server) mcpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeErr(w, http.StatusMethodNotAllowed, "MCP server prijíma iba POST", nil)
		return
	}
	if s.Desktop != nil {
		s.Desktop.Ping() // kým pracuje AI asistent, aplikácia sa neukončí
	}
	// ochrana proti DNS rebindingu: požiadavka z prehliadača musí byť z rovnakého pôvodu
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || u.Host != r.Host {
			writeErr(w, http.StatusForbidden, "Nepovolený pôvod požiadavky", nil)
			return
		}
	}
	// na serveri klient z 401 zistí, kde sa prihlásiť cez OAuth (RFC 9728)
	challenge := `Bearer realm="fiskus"`
	if s.authRequired() {
		challenge += `, resource_metadata="` + s.baseURL(r) + `/.well-known/oauth-protected-resource"`
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		w.Header().Set("WWW-Authenticate", challenge)
		writeErr(w, http.StatusUnauthorized, "Chýba API kľúč (Authorization: Bearer fk_…)", nil)
		return
	}
	token = strings.TrimSpace(token)
	var actor auth.Actor
	var err error
	if strings.HasPrefix(token, app.OAuthAccessPrefix) {
		actor, err = s.App.OAuthActor(r.Context(), token, auth.ChannelMCP, clientIP(r))
	} else {
		actor, err = s.App.KeyActor(r.Context(), token, auth.ChannelMCP, clientIP(r))
	}
	if err != nil {
		w.Header().Set("WWW-Authenticate", challenge+`, error="invalid_token"`)
		writeErr(w, http.StatusUnauthorized, err.Error(), nil)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMCPBody))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Požiadavka je príliš veľká", nil)
		return
	}
	out := s.MCP.Handle(auth.WithActor(r.Context(), actor), body)
	if out == nil {
		w.WriteHeader(http.StatusAccepted) // iba notifikácie
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// ---- API kľúče prihláseného používateľa ----

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	list, err := s.App.ListAPIKeys(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		ReadOnly bool   `json:"readOnly"`
	}
	if !decode(w, r, &body) {
		return
	}
	token, k, err := s.App.CreateAPIKey(r.Context(), body.Name, body.ReadOnly)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "key": k})
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.RevokeAPIKey(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
