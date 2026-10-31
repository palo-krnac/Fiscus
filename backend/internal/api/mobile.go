package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
)

// Mobil v sieti (desktop): telefóny sa pripájajú na adresu počítača v sieti.
// LANHandler pustí dnu iba spárované telefóny (cookie s tokenom) a zakáže
// im správu programu (párovanie, zálohy, licencia, PIN, ukončenie).

const deviceCookie = "fiskus_device"

type lanKey struct{}

// fromLAN: požiadavka prišla z telefónu v sieti (nie z okna na počítači).
func fromLAN(ctx context.Context) (auth.Actor, bool) {
	a, ok := ctx.Value(lanKey{}).(auth.Actor)
	return a, ok
}

// lanDenied sú cesty, ktoré telefón v sieti nesmie volať.
func lanDenied(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case p == "/api/mobile/pair":
		return false
	case strings.HasPrefix(p, "/api/mobile"), strings.HasPrefix(p, "/api/backups"), strings.HasPrefix(p, "/api/license"),
		strings.HasPrefix(p, "/api/desktop"), strings.HasPrefix(p, "/api/auth"), strings.HasPrefix(p, "/api/security"),
		strings.HasPrefix(p, "/api/legal"), strings.HasPrefix(p, "/mcp"), strings.HasPrefix(p, "/oauth"):
		return true
	case r.Method == http.MethodDelete && strings.Count(strings.Trim(p, "/"), "/") == 2 && strings.HasPrefix(p, "/api/companies/"):
		return true // zmazanie firmy
	case r.Method == http.MethodPost && p == "/api/companies":
		return true
	}
	return false
}

// LANHandler obalí rozhranie pre telefóny v sieti.
func (s *Server) LANHandler(base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		if lanDenied(r) {
			writeErr(w, http.StatusForbidden, "Z telefónu sa toto robí na počítači", nil)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/mobile/pair" {
			// stránky programu (bez dát) a spárovanie
			base.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(deviceCookie)
		token := ""
		if err == nil {
			token = c.Value
		}
		actor, err := s.App.MobileActor(r.Context(), token, clientIP(r))
		if err != nil {
			// 403, nie 401: rozhranie nemá ukázať prihlásenie, ale výzvu na spárovanie
			writeErr(w, http.StatusForbidden, err.Error(), nil)
			return
		}
		base.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), lanKey{}, actor)))
	})
}

func (s *Server) mobileStatus(w http.ResponseWriter, r *http.Request) {
	out, err := s.App.MobileStatus(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setMobile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
		Port    int  `json:"port"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.SetMobile(r.Context(), in.Enabled, in.Port)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) newPairing(w http.ResponseWriter, r *http.Request) {
	out, err := s.App.NewMobilePairing(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) removeDevice(w http.ResponseWriter, r *http.Request) {
	out, err := s.App.RemoveMobileDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// pairMobile: POST /api/mobile/pair {code, name} — telefón dostane trvalú cookie.
func (s *Server) pairMobile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	token, err := s.App.PairMobile(r.Context(), in.Code, in.Name)
	switch err {
	case nil:
	case app.ErrPairBlocked:
		writeErr(w, http.StatusTooManyRequests, err.Error(), nil)
		return
	case app.ErrPairCode, app.ErrDesktopOnly:
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	default:
		fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: deviceCookie, Value: token, Path: "/", MaxAge: 10 * 365 * 24 * 3600,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: secure(r)})
	writeJSON(w, http.StatusOK, map[string]bool{"paired": true})
}
