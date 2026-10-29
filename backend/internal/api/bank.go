package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

const maxStatement = 20 << 20 // 20 MB

func failBank(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bank.ErrFormat):
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
	case errors.Is(err, app.ErrAlreadyMatched), errors.Is(err, app.ErrDocumentPaid), errors.Is(err, app.ErrWrongDirection):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	default:
		fail(w, err)
	}
}

// listBank: GET /api/companies/{cid}/bank?year=2026&status=open
func (s *Server) listBank(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	f := store.BankFilter{Status: bank.Status(r.URL.Query().Get("status"))}
	if y := r.URL.Query().Get("year"); y != "" {
		f.Year, _ = strconv.Atoi(y)
	}
	list, err := s.App.ListBank(r.Context(), cid, f)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// importBank: POST /api/companies/{cid}/bank/import?name=vypis.xml (telo = súbor)
func (s *Server) importBank(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxStatement))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 20 MB)", nil)
		return
	}
	res, err := s.App.ImportStatement(r.Context(), cid, r.URL.Query().Get("name"), data)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) bankSuggestions(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	sug, err := s.App.Suggestions(r.Context(), cid, id)
	if err != nil {
		failBank(w, err)
		return
	}
	docs, err := s.App.OpenDocuments(r.Context(), cid, id)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": sug, "documents": docs})
}

func (s *Server) matchBank(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Kind string `json:"kind"`
		ID   int64  `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	t, err := s.App.MatchTransaction(r.Context(), cid, id, body.Kind, body.ID)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) unmatchBank(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	t, err := s.App.UnmatchTransaction(r.Context(), cid, id)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) ignoreBank(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Ignore bool   `json:"ignore"`
		Note   string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	t, err := s.App.IgnoreTransaction(r.Context(), cid, id, body.Ignore, body.Note)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}
