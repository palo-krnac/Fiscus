package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/registry"
)

// lookupICO vráti údaje firmy z registra (RPO) podľa IČO.
func (s *Server) lookupICO(w http.ResponseWriter, r *http.Request) {
	if s.Registry == nil {
		writeErr(w, http.StatusNotImplemented, "Vyhľadávanie v registri je vypnuté", nil)
		return
	}
	c, err := s.Registry.ByICO(r.Context(), r.PathValue("ico"))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, c)
	case errors.Is(err, registry.ErrInvalidICO):
		writeErr(w, http.StatusBadRequest, "Neplatné IČO",
			[]invoice.FieldError{{Field: "ico", Message: "Neplatné IČO"}})
	case errors.Is(err, registry.ErrNotFound):
		writeErr(w, http.StatusNotFound, registry.ErrNotFound.Error(), nil)
	default:
		log.Printf("register: %v", err)
		writeErr(w, http.StatusServiceUnavailable, registry.ErrUnavailable.Error(), nil)
	}
}

// checkVAT overí IČ DPH vo VIES (platiteľ DPH v EÚ, názov a adresa).
func (s *Server) checkVAT(w http.ResponseWriter, r *http.Request) {
	if s.VAT == nil {
		writeErr(w, http.StatusNotImplemented, "Overenie IČ DPH vo VIES je vypnuté", nil)
		return
	}
	c, err := s.VAT.Check(r.Context(), r.PathValue("vatid"))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, c)
	case errors.Is(err, registry.ErrBadVATID):
		writeErr(w, http.StatusBadRequest, err.Error(), []invoice.FieldError{{Field: "icdph", Message: err.Error()}})
	default:
		log.Printf("VIES: %v", err)
		writeErr(w, http.StatusServiceUnavailable, "Systém VIES neodpovedá (alebo daňový úrad štátu je nedostupný) — skúste neskôr", nil)
	}
}
