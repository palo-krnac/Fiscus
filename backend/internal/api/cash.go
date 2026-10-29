package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/cash"
)

func failCash(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cash.ErrCancelled), errors.Is(err, cash.ErrNegative):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	default:
		fail(w, err)
	}
}

// cashBook: GET /api/companies/{cid}/cash?year=2026
func (s *Server) cashBook(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	if year == 0 {
		year = s.App.Today().Year()
	}
	book, err := s.App.CashBook(r.Context(), cid, year)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (s *Server) getCash(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	d, err := s.App.Store.GetCashDocument(r.Context(), cid, id)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) cashCandidates(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.CashCandidates(r.Context(), cid, cash.Kind(r.URL.Query().Get("kind")))
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createCash(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var d cash.Document
	if !decodeLoose(w, r, &d) {
		return
	}
	d, err := s.App.CreateCash(r.Context(), cid, d)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) updateCash(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Partner string `json:"partner"`
		Purpose string `json:"purpose"`
		Note    string `json:"note"`
	}
	if !decodeLoose(w, r, &body) {
		return
	}
	d, err := s.App.UpdateCash(r.Context(), cid, id, body.Partner, body.Purpose, body.Note)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) cancelCash(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	d, err := s.App.CancelCash(r.Context(), cid, id)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
