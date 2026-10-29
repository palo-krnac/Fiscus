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

func (s *Server) getStatements(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	out, err := s.App.StatementsFor(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setStatementInputs(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
		return
	}
	var in journal.AssetInputs
	if !decode(w, r, &in) {
		return
	}
	if err := s.App.SetStatementInputs(r.Context(), cid, year, in); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addClosing(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var body struct {
		Year  int            `json:"year"`
		Text  string         `json:"text"`
		Lines []journal.Line `json:"lines"`
	}
	if !decode(w, r, &body) {
		return
	}
	key, err := s.App.AddClosing(r.Context(), cid, body.Year, body.Text, body.Lines)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"key": key})
}
