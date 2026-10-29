package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/postman"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/ubl"
)

// failEInvoice: nastavenie a odoslanie e-faktúr.
func failEInvoice(w http.ResponseWriter, err error) {
	var se *postman.SettingsError
	switch {
	case errors.As(err, &se):
		fields := make([]invoice.FieldError, len(se.Problems))
		for i, p := range se.Problems {
			fields[i] = invoice.FieldError{Field: p.Field, Message: p.Message}
		}
		writeErr(w, http.StatusUnprocessableEntity, "Nastavenie e-faktúr obsahuje chyby", fields)
	case errors.Is(err, postman.ErrOff):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	case errors.As(err, new(*ubl.IncompleteError)):
		failUBL(w, err)
	case errors.Is(err, ubl.ErrNotUBL):
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
	case statusOf(err) == http.StatusInternalServerError:
		// chyba poštára (priečinok, sieť) — vysvetlenie pre používateľa
		writeErr(w, http.StatusBadGateway, err.Error(), nil)
	default:
		fail(w, err)
	}
}

// statusOf vráti stav, ktorý by zvolil fail (500 = neznáma chyba).
func statusOf(err error) int {
	rec := &statusRecorder{}
	fail(rec, err)
	return rec.code
}

type statusRecorder struct {
	code int
	h    http.Header
}

func (r *statusRecorder) Header() http.Header {
	if r.h == nil {
		r.h = http.Header{}
	}
	return r.h
}
func (r *statusRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *statusRecorder) WriteHeader(c int)           { r.code = c }

func (s *Server) getEInvoiceSettings(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	set, err := s.App.EInvoiceSettings(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) putEInvoiceSettings(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var b postman.Settings
	if !decodeLoose(w, r, &b) {
		return
	}
	set, err := s.App.SetEInvoiceSettings(r.Context(), cid, b)
	if err != nil {
		failEInvoice(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) sendEInvoice(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	e, err := s.App.SendEInvoice(r.Context(), cid, id)
	if err != nil {
		if e.ID != 0 { // poštár odmietol — záznam s chybou je v zozname
			writeErr(w, http.StatusBadGateway, "E-faktúra sa neodoslala: "+err.Error(), nil)
			return
		}
		failEInvoice(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) listEInvoices(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	q := r.URL.Query()
	docID, _ := strconv.ParseInt(q.Get("docId"), 10, 64)
	list, err := s.App.EInvoices(r.Context(), cid, store.EInvoiceFilter{Direction: q.Get("direction"), DocID: docID})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) einvoiceInbox(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.EInvoiceInbox(r.Context(), cid)
	if err != nil {
		failEInvoice(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) takeEInvoice(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	d, err := s.App.TakeEInvoice(r.Context(), cid, r.PathValue("msg"))
	if err != nil {
		failEInvoice(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) reopenEInvoice(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	d, err := s.App.ReopenEInvoice(r.Context(), cid, id)
	if err != nil {
		failEInvoice(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
