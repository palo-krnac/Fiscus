package api

import (
	"errors"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
)

func failRecurring(w http.ResponseWriter, err error) {
	if errors.Is(err, recurring.ErrFinished) {
		writeErr(w, http.StatusConflict, err.Error(), nil)
		return
	}
	fail(w, err)
}

func (s *Server) listRecurring(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.Recurring(r.Context(), cid)
	if err != nil {
		failRecurring(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getRecurring(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	t, err := s.App.RecurringTemplate(r.Context(), cid, id)
	if err != nil {
		failRecurring(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) saveRecurring(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var t recurring.Template
	if !decodeLoose(w, r, &t) {
		return
	}
	t.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		t.ID = id
	}
	isNew := t.ID == 0
	out, err := s.App.SaveRecurring(r.Context(), cid, t)
	if err != nil {
		failRecurring(w, err)
		return
	}
	code := http.StatusOK
	if isNew {
		code = http.StatusCreated
	}
	writeJSON(w, code, out)
}

func (s *Server) previewRecurring(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var t recurring.Template
	if !decodeLoose(w, r, &t) {
		return
	}
	out, smp, err := s.App.PreviewRecurring(r.Context(), cid, t)
	if err != nil {
		failRecurring(w, err)
		return
	}
	resp := struct {
		recurring.Template
		Sample app.RecurringSample  `json:"sample"`
		Errors []invoice.FieldError `json:"errors"`
	}{Template: out, Sample: smp, Errors: []invoice.FieldError{}}
	var ve *invoice.ValidationError
	if errors.As(out.Validate(), &ve) {
		resp.Errors = ve.Fields
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) setRecurringActive(active bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, _ := pathID(w, r, "cid")
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		t, err := s.App.SetRecurringActive(r.Context(), cid, id, active)
		if err != nil {
			failRecurring(w, err)
			return
		}
		writeJSON(w, http.StatusOK, t)
	}
}

func (s *Server) generateRecurring(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	inv, err := s.App.GenerateRecurringNow(r.Context(), cid, id)
	if err != nil {
		failRecurring(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

func (s *Server) deleteRecurring(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteRecurring(r.Context(), cid, id); err != nil {
		failRecurring(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
