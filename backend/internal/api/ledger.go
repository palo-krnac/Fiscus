package api

import (
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/ledger"
)

// Účtovný denník a hlavná kniha (podvojné účtovníctvo).

func (s *Server) getLedger(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	book, err := s.App.Ledger(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func pathYear(r *http.Request) int {
	y, _ := strconv.Atoi(r.PathValue("year"))
	return y
}

func (s *Server) setPosting(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var body struct {
		Template string        `json:"template"`
		Lines    []ledger.Line `json:"lines"`
		Note     string        `json:"note"`
		ledger.Dims
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.App.SetPosting(r.Context(), cid, pathYear(r), r.PathValue("key"), body.Template, body.Lines, body.Note, body.Dims); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setLedgerOpening(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var body struct {
		Lines []ledger.Line `json:"lines"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.App.SetLedgerOpening(r.Context(), cid, pathYear(r), body.Lines); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveInternal(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var doc app.InternalDoc
	if !decode(w, r, &doc) {
		return
	}
	status := http.StatusCreated
	if id := r.PathValue("id"); id != "" {
		doc.ID, _ = strconv.ParseInt(id, 10, 64)
		status = http.StatusOK
	} else {
		doc.ID = 0
	}
	out, err := s.App.SaveInternal(r.Context(), cid, pathYear(r), doc)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteInternal(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.App.DeleteInternal(r.Context(), cid, pathYear(r), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ledgerStatements(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	st, err := s.App.LedgerStatementsFor(r.Context(), cid, pathYear(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) openingProposal(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	lines, err := s.App.OpeningProposal(r.Context(), cid, pathYear(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) setMoneyAccounts(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var body map[string]string
	if !decode(w, r, &body) {
		return
	}
	if err := s.App.SetMoneyAccounts(r.Context(), cid, body); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
