package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
)

// failMail preloží chyby e-mailov. Chybu servera SMTP (správa už je v zozname
// s chybou) obslúžia volajúci ako 502.
func failMail(w http.ResponseWriter, err error) {
	var ve *mail.ValidationError
	switch {
	case errors.As(err, &ve):
		fields := make([]invoice.FieldError, len(ve.Fields))
		for i, f := range ve.Fields {
			fields[i] = invoice.FieldError{Field: f.Field, Message: f.Message}
		}
		writeErr(w, http.StatusUnprocessableEntity, "Skontrolujte nastavenie", fields)
	case errors.Is(err, app.ErrMailNotConfigured), errors.Is(err, app.ErrBadRecipient):
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
	case errors.Is(err, app.ErrNotSendable):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	default:
		fail(w, err)
	}
}

// mailSettingsBody je nastavenie z formulára.
type mailSettingsBody struct {
	mail.Settings
	ClearPassword bool `json:"clearPassword"`
}

func (s *Server) getMailSettings(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	set, err := s.App.MailSettings(r.Context(), cid)
	if err != nil {
		failMail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) putMailSettings(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var b mailSettingsBody
	if !decodeLoose(w, r, &b) {
		return
	}
	set, err := s.App.SetMailSettings(r.Context(), cid, b.Settings, b.ClearPassword)
	if err != nil {
		failMail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) testMail(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var b struct {
		To string `json:"to"`
	}
	if !decodeLoose(w, r, &b) {
		return
	}
	e, err := s.App.TestMail(r.Context(), cid, b.To)
	if err != nil {
		if e.ID != 0 { // spojenie zlyhalo — správa je v zozname s chybou
			writeErr(w, http.StatusBadGateway, "E-mail sa neodoslal: "+err.Error(), nil)
			return
		}
		failMail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) listEmails(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	year := 0
	if y := r.URL.Query().Get("year"); y != "" {
		var err error
		if year, err = strconv.Atoi(y); err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
			return
		}
	}
	list, err := s.App.Emails(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// mailDoc obslúži GET (návrh správy) a POST (odoslanie) pre druh dokladu.
func (s *Server) mailDoc(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, _ := pathID(w, r, "cid")
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		if r.Method == http.MethodGet {
			d, err := s.App.MailDraft(r.Context(), cid, kind, id)
			if err != nil {
				failMail(w, err)
				return
			}
			writeJSON(w, http.StatusOK, d)
			return
		}
		var req app.MailRequest
		if !decodeLoose(w, r, &req) {
			return
		}
		e, err := s.App.SendMail(r.Context(), cid, kind, id, req)
		if err != nil {
			if e.ID != 0 { // spojenie zlyhalo — správa je v zozname s chybou
				writeErr(w, http.StatusBadGateway, "E-mail sa neodoslal: "+err.Error(), nil)
				return
			}
			failMail(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, e)
	}
}
