package api

import (
	"net/http"
	"strconv"
)

// Dvojfázové overenie prihláseného používateľa (iba serverový režim).

func (s *Server) mfaOnly(w http.ResponseWriter) bool {
	if !s.authRequired() {
		writeErr(w, http.StatusBadRequest, "Desktopová verzia nepoužíva prihlasovanie", nil)
		return false
	}
	return true
}

func (s *Server) myMFA(w http.ResponseWriter, r *http.Request) {
	if !s.mfaOnly(w) {
		return
	}
	st, err := s.App.MyMFA(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) setupMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !s.mfaOnly(w) || !decode(w, r, &body) {
		return
	}
	out, err := s.App.SetupMFA(r.Context(), body.Password)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) enableMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !s.mfaOnly(w) || !decode(w, r, &body) {
		return
	}
	codes, err := s.App.EnableMFA(r.Context(), body.Code)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func (s *Server) disableMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !s.mfaOnly(w) || !decode(w, r, &body) {
		return
	}
	if err := s.App.DisableMFA(r.Context(), body.Password, body.Code); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) recoveryMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !s.mfaOnly(w) || !decode(w, r, &body) {
		return
	}
	codes, err := s.App.NewRecoveryCodes(r.Context(), body.Password)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func (s *Server) resetMFA(w http.ResponseWriter, r *http.Request) {
	if !s.mfaOnly(w) {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatný používateľ", nil)
		return
	}
	if err := s.App.ResetMFA(r.Context(), uid); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
