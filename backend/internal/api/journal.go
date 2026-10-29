package api

import (
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"

	"github.com/palo-krnac/fiscus/backend/internal/journal"
)

// Peňažný denník (jednoduché účtovníctvo).

func (s *Server) getJournal(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	book, err := s.App.Journal(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (s *Server) classifyJournal(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var body struct {
		Lines []journal.Line `json:"lines"`
		Note  string         `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Lines == nil {
		body.Lines = []journal.Line{}
	}
	if err := s.App.ClassifyJournal(r.Context(), cid, r.PathValue("key"), body.Lines, body.Note); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resetJournal(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	if err := s.App.ClassifyJournal(r.Context(), cid, r.PathValue("key"), nil, ""); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setJournalOpening(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var body struct {
		Year int                  `json:"year"`
		Bank map[string]money.Dec `json:"bank"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.App.SetJournalOpening(r.Context(), cid, body.Year, body.Bank); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) openItems(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var at invoice.Date
	if d := r.URL.Query().Get("date"); d != "" {
		var err error
		if at, err = invoice.ParseDate(d); err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatný dátum", nil)
			return
		}
	}
	out, err := s.App.OpenItemsAt(r.Context(), cid, at)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
