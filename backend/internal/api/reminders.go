package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
)

func failReminder(w http.ResponseWriter, err error) {
	if errors.Is(err, reminder.ErrNoInvoices) || errors.Is(err, reminder.ErrMixedCustomer) || errors.Is(err, reminder.ErrNotRemindable) {
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	fail(w, err)
}

// overdue: GET /api/companies/{cid}/overdue — faktúry po splatnosti podľa odberateľov.
func (s *Server) overdue(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	groups, err := s.App.Overdue(r.Context(), cid)
	if err != nil {
		failReminder(w, err)
		return
	}
	law := s.App.Law()
	writeJSON(w, http.StatusOK, struct {
		Groups         []app.OverdueGroup `json:"groups"`
		ECBUpdated     invoice.Date       `json:"ecbUpdated"`
		FlatFee        money.Dec          `json:"flatFee"`
		BusinessMargin money.Dec          `json:"businessMargin"`
		ConsumerMargin money.Dec          `json:"consumerMargin"`
	}{groups, law.ECB.Updated(), law.FlatFee, law.BusinessMargin, law.ConsumerMargin})
}

func (s *Server) listReminders(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	year := 0
	if y := r.URL.Query().Get("year"); y != "" {
		var err error
		if year, err = strconv.Atoi(y); err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
			return
		}
	}
	list, err := s.App.Reminders(r.Context(), cid, year)
	if err != nil {
		failReminder(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getReminder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	x, err := s.App.Reminder(r.Context(), cid, id)
	if err != nil {
		failReminder(w, err)
		return
	}
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) reminderRequest(create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, _ := pathID(w, r, "cid")
		var req app.ReminderRequest
		if !decodeLoose(w, r, &req) {
			return
		}
		fn, code := s.App.PreviewReminder, http.StatusOK
		if create {
			fn, code = s.App.CreateReminder, http.StatusCreated
		}
		x, err := fn(r.Context(), cid, req)
		if err != nil {
			failReminder(w, err)
			return
		}
		writeJSON(w, code, x)
	}
}

func (s *Server) deleteReminder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteReminder(r.Context(), cid, id); err != nil {
		failReminder(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
