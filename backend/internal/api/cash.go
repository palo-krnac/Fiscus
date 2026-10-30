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
	reg, _ := strconv.ParseInt(r.URL.Query().Get("register"), 10, 64)
	book, err := s.App.CashBookOf(r.Context(), cid, year, reg)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, book)
}

// cashRegisters: GET /api/companies/{cid}/cash-registers — pokladne so zostatkami.
func (s *Server) cashRegisters(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.CashRegisters(r.Context(), cid)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// saveCashRegisters: PUT /api/companies/{cid}/cash-registers — celý zoznam (nové so záporným ID).
func (s *Server) saveCashRegisters(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var list []cash.Register
	if !decode(w, r, &list) {
		return
	}
	out, err := s.App.SaveCashRegisters(r.Context(), cid, list)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
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
	reg, _ := strconv.ParseInt(r.URL.Query().Get("register"), 10, 64)
	list, err := s.App.CashCandidates(r.Context(), cid, cash.Kind(r.URL.Query().Get("kind")), reg)
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

// previewCash: POST …/cash/preview — prepočet DPH dokladu bez uloženia.
func (s *Server) previewCash(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var d cash.Document
	if !decodeLoose(w, r, &d) {
		return
	}
	pv, err := s.App.PreviewCash(r.Context(), cid, d)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}
