package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/app"
)

// Zabezpečenie: PIN desktopu a povinné 2FA na serveri.

func (s *Server) unlockPIN(w http.ResponseWriter, r *http.Request) {
	if s.authRequired() {
		writeErr(w, http.StatusBadRequest, "Server používa prihlásenie heslom", nil)
		return
	}
	var body struct {
		PIN string `json:"pin"`
	}
	if !decode(w, r, &body) {
		return
	}
	key := clientIP(r) + "|pin"
	now := time.Now()
	if !s.limit.allow(key, now) {
		writeErr(w, http.StatusTooManyRequests, "Priveľa nesprávnych PIN-ov. Skúste to o 15 minút.", nil)
		return
	}
	token, err := s.App.UnlockPIN(r.Context(), body.PIN)
	if errors.Is(err, app.ErrBadPIN) {
		s.limit.fail(key, now)
		writeErr(w, http.StatusUnauthorized, err.Error(), nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	s.limit.reset(key)
	// bez MaxAge: cookie zanikne so zatvorením prehliadača
	http.SetCookie(w, &http.Cookie{Name: pinCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) lockPIN(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(pinCookie); err == nil {
		s.App.LockPIN(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: pinCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getSecurity(w http.ResponseWriter, r *http.Request) {
	st, err := s.App.Security(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) setPIN(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		PIN     string `json:"pin"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.App.SetPIN(r.Context(), body.Current, body.PIN); err != nil {
		if errors.Is(err, app.ErrPINDesktopOnly) {
			writeErr(w, http.StatusBadRequest, err.Error(), nil)
			return
		}
		fail(w, err)
		return
	}
	// kto PIN nastavil, ostane odomknutý
	if strings.TrimSpace(body.PIN) != "" {
		if token, err := s.App.UnlockPIN(r.Context(), body.PIN); err == nil {
			http.SetCookie(w, &http.Cookie{Name: pinCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setRequireMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.App.SetRequireMFA(r.Context(), body.On); err != nil {
		if errors.Is(err, app.ErrMFAServerOnly) {
			writeErr(w, http.StatusBadRequest, err.Error(), nil)
			return
		}
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
