package api

import (
	"errors"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/chart"
)

func (s *Server) chartView(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	v, err := s.App.Chart(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// saveAccount: POST …/chart/accounts (nový) alebo PUT …/chart/accounts/{number}.
func (s *Server) saveAccount(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var a chart.Account
	if !decode(w, r, &a) {
		return
	}
	old := r.PathValue("number")
	out, err := s.App.SaveAccount(r.Context(), cid, old, a)
	if err != nil {
		fail(w, err)
		return
	}
	status := http.StatusOK
	if old == "" {
		status = http.StatusCreated
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	if err := s.App.DeleteAccount(r.Context(), cid, r.PathValue("number")); err != nil {
		failChart(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveCenter(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var c chart.Center
	if !decode(w, r, &c) {
		return
	}
	old := r.PathValue("code")
	out, err := s.App.SaveCenter(r.Context(), cid, old, c)
	if err != nil {
		fail(w, err)
		return
	}
	status := http.StatusOK
	if old == "" {
		status = http.StatusCreated
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteCenter(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	if err := s.App.DeleteCenter(r.Context(), cid, r.PathValue("code")); err != nil {
		failChart(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveJob(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var j chart.Job
	if !decode(w, r, &j) {
		return
	}
	old := r.PathValue("code")
	out, err := s.App.SaveJob(r.Context(), cid, old, j)
	if err != nil {
		fail(w, err)
		return
	}
	status := http.StatusOK
	if old == "" {
		status = http.StatusCreated
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	if err := s.App.DeleteJob(r.Context(), cid, r.PathValue("code")); err != nil {
		failChart(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func failChart(w http.ResponseWriter, err error) {
	if errors.Is(err, chart.ErrInUse) {
		writeErr(w, http.StatusConflict, err.Error(), nil)
		return
	}
	fail(w, err)
}
