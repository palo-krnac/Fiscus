package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/calendar"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// calendarView: GET /api/companies/{cid}/calendar?from=2026-10-01&to=2026-10-31
func (s *Server) calendarView(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	from, err1 := invoice.ParseDate(r.URL.Query().Get("from"))
	to, err2 := invoice.ParseDate(r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil {
		writeErr(w, http.StatusBadRequest, "Zadajte rozsah from a to (RRRR-MM-DD)", nil)
		return
	}
	v, err := s.App.Calendar(r.Context(), cid, from, to)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// calendarICS: GET /api/companies/{cid}/calendar.ics — súbor na import.
func (s *Server) calendarICS(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	doc, err := s.App.CalendarDownload(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+doc.Name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(doc.Data)))
	_, _ = w.Write(doc.Data)
}

// publicCalendar: GET /api/public/calendar/{cid}/{token}/fiskus.ics — odber
// v Google Kalendári, Outlooku či v telefóne (bez prihlásenia, tajný odkaz).
func (s *Server) publicCalendar(w http.ResponseWriter, r *http.Request) {
	if !s.publicGuard(w, r) {
		return
	}
	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusNotFound, app.ErrBadFeed.Error(), nil)
		return
	}
	b, err := s.App.CalendarICS(r.Context(), cid, r.PathValue("token"), clientIP(r))
	if err != nil {
		if errors.Is(err, app.ErrBadFeed) {
			s.shareLimit.fail(clientIP(r), time.Now())
			writeErr(w, http.StatusNotFound, err.Error(), nil)
			return
		}
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write(b)
}

func (s *Server) ownEvents(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	list, err := s.App.OwnEvents(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) saveOwnEvent(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var o calendar.Own
	if !decode(w, r, &o) {
		return
	}
	o.ID = 0
	status := http.StatusCreated
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		o.ID, status = id, http.StatusOK
	}
	out, err := s.App.SaveOwnEvent(r.Context(), cid, o)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteOwnEvent(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteOwnEvent(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// markOwnEvent: POST …/calendar/own/{id}/done {"date": "2026-11-08", "done": true}
func (s *Server) markOwnEvent(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Date invoice.Date `json:"date"`
		Done bool         `json:"done"`
	}
	if !decode(w, r, &req) {
		return
	}
	out, err := s.App.MarkOwnEvent(r.Context(), cid, id, req.Date, req.Done)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) calendarFeed(w http.ResponseWriter, r *http.Request) {
	st, err := s.App.CalendarFeed(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// createCalendarFeed vráti tajný token (iba raz); adresu zloží rozhranie.
func (s *Server) createCalendarFeed(w http.ResponseWriter, r *http.Request) {
	token, err := s.App.CreateCalendarFeed(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

func (s *Server) revokeCalendarFeed(w http.ResponseWriter, r *http.Request) {
	if err := s.App.RevokeCalendarFeed(r.Context()); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
