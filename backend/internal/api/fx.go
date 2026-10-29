package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
)

func (s *Server) listRates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := fx.Filter{Currency: q.Get("currency")}
	if y := q.Get("year"); y != "" {
		var err error
		if f.Year, err = strconv.Atoi(y); err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
			return
		}
	}
	list, err := s.App.Rates(r.Context(), f)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) setRate(w http.ResponseWriter, r *http.Request) {
	var x fx.Rate
	if !decodeLoose(w, r, &x) {
		return
	}
	saved, err := s.App.SetRate(r.Context(), x)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) updateRates(w http.ResponseWriter, r *http.Request) {
	res, err := s.App.UpdateRates(r.Context())
	switch {
	case errors.Is(err, app.ErrRatesOff):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, app.ErrForbidden), errors.Is(err, app.ErrReadOnly):
		fail(w, err)
	case err != nil:
		writeErr(w, http.StatusBadGateway, "Kurzy sa nepodarilo stiahnuť: "+err.Error(), nil)
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

func (s *Server) deleteRate(w http.ResponseWriter, r *http.Request) {
	if err := s.App.DeleteRate(r.Context(), r.PathValue("currency"), r.PathValue("date")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
