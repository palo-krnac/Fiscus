package api

import (
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/legal"
)

// getLegal: GET /api/legal — zákonné parametre spoločné pre všetky firmy.
func (s *Server) getLegal(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.App.Legal(r.Context()))
}

// setLegal: PUT /api/legal (iba správca).
func (s *Server) setLegal(w http.ResponseWriter, r *http.Request) {
	var p legal.Params
	if !decodeLoose(w, r, &p) {
		return
	}
	out, err := s.App.SetLegal(r.Context(), p)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// resetLegal: POST /api/legal/reset — hodnoty z programu (iba správca).
func (s *Server) resetLegal(w http.ResponseWriter, r *http.Request) {
	out, err := s.App.ResetLegal(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
