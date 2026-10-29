package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/extract"
)

func (s *Server) getAISettings(w http.ResponseWriter, r *http.Request) {
	set, err := s.App.AISettings(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) putAISettings(w http.ResponseWriter, r *http.Request) {
	var b struct {
		extract.Settings
		ClearKey bool `json:"clearKey"`
	}
	if !decodeLoose(w, r, &b) {
		return
	}
	set, err := s.App.SetAISettings(r.Context(), b.Settings, b.ClearKey)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// extractPurchase: POST …/purchases/extract?name=… — telo je PDF alebo obrázok.
func (s *Server) extractPurchase(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, extract.MaxBytes))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 10 MB)", nil)
		return
	}
	d, err := s.App.ExtractPurchase(r.Context(), cid, r.URL.Query().Get("name"), data)
	switch {
	case errors.Is(err, extract.ErrOff):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, extract.ErrMedia):
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
	case errors.Is(err, app.ErrForbidden), errors.Is(err, app.ErrReadOnly), errors.Is(err, app.ErrNoSession):
		fail(w, err)
	case err != nil:
		writeErr(w, http.StatusBadGateway, err.Error(), nil)
	default:
		writeJSON(w, http.StatusOK, d)
	}
}
