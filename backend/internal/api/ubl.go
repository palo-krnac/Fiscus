package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/ubl"
)

// failUBL: chýbajúce údaje e-faktúry → 422 s poľami (zobrazia sa pri faktúre).
func failUBL(w http.ResponseWriter, err error) {
	var ie *ubl.IncompleteError
	if errors.As(err, &ie) {
		fields := make([]invoice.FieldError, len(ie.Problems))
		for i, p := range ie.Problems {
			fields[i] = invoice.FieldError{Field: p.Field, Message: p.Message}
		}
		writeErr(w, http.StatusUnprocessableEntity, "E-faktúru nemožno vytvoriť", fields)
		return
	}
	fail(w, err)
}

// importUBL: POST …/purchases/import-ubl?name=… — telo je XML e-faktúry.
func (s *Server) importUBL(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 5 MB)", nil)
		return
	}
	d, err := s.App.ImportUBL(r.Context(), cid, r.URL.Query().Get("name"), data)
	if errors.Is(err, ubl.ErrNotUBL) {
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
