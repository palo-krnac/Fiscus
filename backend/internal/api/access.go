package api

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/license"
)

const (
	sessionCookie = "fiskus_session"
	pinCookie     = "fiskus_pin" // odomknuté okno desktopu s PIN-om
)

// CSRFHeader musí poslať frontend pri každej zmene (POST/PUT/DELETE).
// Cudzia stránka ho v prehliadači poslať nevie bez povolenia CORS.
const CSRFHeader = "X-Fiskus"

func (s *Server) authRequired() bool { return s.App.Caps.Mode == edition.Server }

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// csrf odmietne zmeny bez hlavičky X-Fiskus.
func (s *Server) csrf(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(CSRFHeader) == "" {
			writeErr(w, http.StatusForbidden, "Chýba hlavička "+CSRFHeader+" (ochrana proti CSRF)", nil)
			return
		}
		h(w, r)
	}
}

// authed zistí, kto volá: v desktopovom režime lokálny používateľ,
// v serverovom režime používateľ z cookie relácie.
func (s *Server) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var actor auth.Actor
		if !s.authRequired() {
			// desktop s PIN-om: okno musí byť odomknuté
			if s.App.PINEnabled(r.Context()) {
				c, err := r.Cookie(pinCookie)
				if err != nil || !s.App.PINSession(c.Value) {
					writeErr(w, http.StatusUnauthorized, app.ErrPINLocked.Error(), nil)
					return
				}
			}
			actor = auth.Local
			actor.IP = clientIP(r)
		} else {
			c, err := r.Cookie(sessionCookie)
			if err != nil {
				fail(w, app.ErrNoSession)
				return
			}
			actor, err = s.App.SessionActor(r.Context(), c.Value, auth.ChannelWeb, clientIP(r))
			if err != nil {
				clearCookie(w, r)
				fail(w, err)
				return
			}
			if actor.MFASetup && !mfaSetupAllowed(r) {
				writeErr(w, http.StatusForbidden, app.ErrMFASetup.Error(), nil)
				return
			}
		}
		h(w, r.WithContext(auth.WithActor(r.Context(), actor)))
	}
}

// mfaSetupAllowed: čo smie používateľ, kým si nezapne povinné 2FA.
func mfaSetupAllowed(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case p == "/api/me", p == "/api/auth/logout", p == "/api/auth/password",
		strings.HasPrefix(p, "/api/me/mfa"):
		return true
	case r.Method == http.MethodGet && p == "/api/companies":
		return true // rozhranie potrebuje zoznam firiem na načítanie
	}
	return false
}

// need overí právo vo firme z cesty {cid}.
func (s *Server) need(p auth.Perm, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, ok := pathID(w, r, "cid")
		if !ok {
			return
		}
		if err := s.App.Require(r.Context(), cid, p); err != nil {
			fail(w, err)
			return
		}
		h(w, r)
	}
}

func secure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure(r)})
}

// ---- obmedzenie pokusov o prihlásenie ----

type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

const (
	loginAttempts = 5
	loginWindow   = 15 * time.Minute
)

// allow vráti false, ak bolo z tejto IP na tento e-mail priveľa neúspešných pokusov.
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < loginWindow {
			recent = append(recent, t)
		}
	}
	l.hits[key] = recent
	return len(recent) < loginAttempts
}

func (l *limiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	l.hits[key] = append(l.hits[key], now)
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

// ---- prihlásenie ----

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.authRequired() {
		writeErr(w, http.StatusBadRequest, "Desktopová verzia nepoužíva prihlasovanie", nil)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	key := clientIP(r) + "|" + strings.ToLower(strings.TrimSpace(body.Email))
	now := time.Now()
	if !s.limit.allow(key, now) {
		writeErr(w, http.StatusTooManyRequests, "Priveľa neúspešných pokusov. Skúste to o 15 minút.", nil)
		return
	}
	ctx := auth.WithActor(r.Context(), auth.Actor{Name: strings.TrimSpace(body.Email), Channel: auth.ChannelWeb, IP: clientIP(r)})
	token, u, err := s.App.Login(ctx, body.Email, body.Password)
	var need *app.MFARequired
	switch {
	case errors.As(err, &need):
		// heslo sedí, relácia vznikne až po kóde z aplikácie
		writeJSON(w, http.StatusOK, map[string]any{"mfa": true, "ticket": need.Ticket})
		return
	case errors.Is(err, app.ErrBadLogin):
		s.limit.fail(key, now)
		writeErr(w, http.StatusUnauthorized, err.Error(), nil)
		return
	case err != nil:
		fail(w, err)
		return
	}
	s.limit.reset(key)
	s.setSession(w, r, token, u)
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, token string, u auth.User) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/",
		MaxAge: int(app.SessionDuration.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure(r)})
	writeJSON(w, http.StatusOK, map[string]any{"id": u.ID, "name": u.Name, "email": u.Email})
}

// loginMFA dokončí prihlásenie kódom 2FA.
func (s *Server) loginMFA(w http.ResponseWriter, r *http.Request) {
	if !s.authRequired() {
		writeErr(w, http.StatusBadRequest, "Desktopová verzia nepoužíva prihlasovanie", nil)
		return
	}
	var body struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	// podľa lístka, nie iba IP: v kancelárii za jednou IP by inak zlé kódy
	// jedného človeka zablokovali prihlásenie všetkým
	key := clientIP(r) + "|2fa|" + auth.TokenHash(body.Ticket)
	now := time.Now()
	if !s.limit.allow(key, now) {
		writeErr(w, http.StatusTooManyRequests, "Priveľa neúspešných pokusov. Skúste to o 15 minút.", nil)
		return
	}
	ctx := auth.WithActor(r.Context(), auth.Actor{Channel: auth.ChannelWeb, IP: clientIP(r)})
	token, u, err := s.App.LoginMFA(ctx, body.Ticket, body.Code)
	switch {
	case errors.Is(err, app.ErrMFACode), errors.Is(err, app.ErrMFATicket):
		s.limit.fail(key, now)
		writeErr(w, http.StatusUnauthorized, err.Error(), nil)
		return
	case err != nil:
		fail(w, err)
		return
	}
	s.setSession(w, r, token, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.App.Logout(r.Context(), c.Value)
	}
	clearCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	if !s.authRequired() {
		writeErr(w, http.StatusBadRequest, "Desktopová verzia nepoužíva heslá", nil)
		return
	}
	var body struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.App.ChangePassword(r.Context(), body.Current, body.Password); err != nil {
		fail(w, err)
		return
	}
	clearCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// me vráti prihláseného používateľa a jeho roly a práva vo firmách.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	act := auth.ActorFrom(r.Context())
	companies, err := s.App.VisibleCompanies(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	type access struct {
		CompanyID int64       `json:"companyId"`
		Role      auth.Role   `json:"role"`
		Perms     []auth.Perm `json:"perms"`
	}
	out := []access{}
	for _, c := range companies {
		role, _ := s.App.RoleIn(r.Context(), c.ID)
		out = append(out, access{c.ID, role, role.Perms()})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         map[string]any{"id": act.UserID, "name": act.Name, "superAdmin": act.SuperAdmin},
		"authRequired": s.authRequired(),
		"mfaRequired":  act.MFASetup, // musí si najprv zapnúť 2FA
		"access":       out,
	})
}

// ---- používatelia firmy ----

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.CompanyUsers(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	if !s.authRequired() {
		writeErr(w, http.StatusBadRequest, "Používateľov spravuje serverová verzia", nil)
		return
	}
	cid, _ := pathID(w, r, "cid")
	var nm app.NewMember
	if !decode(w, r, &nm) {
		return
	}
	m, err := s.App.AddMember(r.Context(), cid, nm)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) setRole(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	var body struct {
		Role auth.Role `json:"role"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.App.SetRole(r.Context(), cid, uid, body.Role); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	if err := s.App.RemoveMember(r.Context(), cid, uid); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- audit ----

func auditFilter(r *http.Request) (audit.Filter, error) {
	q := r.URL.Query()
	f := audit.Filter{Category: audit.Category(q.Get("category")), Search: q.Get("q")}
	var err error
	if v := q.Get("user"); v != "" {
		if f.UserID, err = strconv.ParseInt(v, 10, 64); err != nil {
			return f, err
		}
	}
	if v := q.Get("from"); v != "" {
		if f.From, err = time.ParseInLocation("2006-01-02", v, bratislava()); err != nil {
			return f, err
		}
	}
	if v := q.Get("to"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, bratislava())
		if err != nil {
			return f, err
		}
		f.To = t.AddDate(0, 0, 1) // vrátane celého dňa
	}
	if v, ok := q["company"]; ok && v[0] != "" {
		id, err := strconv.ParseInt(v[0], 10, 64)
		if err != nil {
			return f, err
		}
		f.CompanyIDs = []int64{id}
	}
	if v := q.Get("limit"); v != "" {
		f.Limit, _ = strconv.Atoi(v)
	}
	if v := q.Get("offset"); v != "" {
		f.Offset, _ = strconv.Atoi(v)
	}
	return f, nil
}

func bratislava() *time.Location {
	if l, err := time.LoadLocation("Europe/Bratislava"); err == nil {
		return l
	}
	return time.UTC
}

func (s *Server) writeAudit(w http.ResponseWriter, r *http.Request, companyID int64) {
	f, err := auditFilter(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatný filter auditu", nil)
		return
	}
	list, total, err := s.App.AuditFor(r.Context(), companyID, f)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": list, "total": total})
}

func (s *Server) companyAudit(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	s.writeAudit(w, r, cid)
}

// allAudit je audit celej inštalácie; ?company=ID zúži na firmu (0 = prihlásenia a systém).
func (s *Server) allAudit(w http.ResponseWriter, r *http.Request) {
	s.writeAudit(w, r, 0)
}

func (s *Server) verifyAudit(w http.ResponseWriter, r *http.Request) {
	n, err := s.App.VerifyAudit(r.Context())
	if errors.Is(err, app.ErrForbidden) {
		fail(w, err)
		return
	}
	res := map[string]any{"entries": n, "ok": err == nil}
	if err != nil {
		res["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

// ---- licencia ----

func (s *Server) getLicense(w http.ResponseWriter, r *http.Request) {
	if !auth.ActorFrom(r.Context()).SuperAdmin {
		fail(w, app.ErrForbidden)
		return
	}
	writeJSON(w, http.StatusOK, s.App.Capabilities())
}

func (s *Server) setLicense(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &body) {
		return
	}
	c, err := s.App.SetLicense(r.Context(), body.Key)
	switch {
	case errors.Is(err, license.ErrFormat), errors.Is(err, license.ErrSignature), errors.Is(err, license.ErrExpired),
		errors.Is(err, app.ErrLicenseDesktop):
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), []invoice.FieldError{{Field: "key", Message: err.Error()}})
		return
	case err != nil:
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}
