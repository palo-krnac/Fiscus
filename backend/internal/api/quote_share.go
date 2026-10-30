package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
)

func (s *Server) shareQuote(r *http.Request, cid, id int64) (quote.Quote, error) {
	return s.App.ShareQuote(r.Context(), cid, id)
}

func (s *Server) unshareQuote(r *http.Request, cid, id int64) (quote.Quote, error) {
	return s.App.UnshareQuote(r.Context(), cid, id)
}

// publicGuard obmedzí hádanie tokenov: po 5 neplatných odkazoch z IP 15 minút nič.
func (s *Server) publicGuard(w http.ResponseWriter, r *http.Request) bool {
	if !s.shareLimit.allow(clientIP(r), time.Now()) {
		writeErr(w, http.StatusTooManyRequests, "Priveľa neplatných odkazov — skúste to neskôr", nil)
		return false
	}
	return true
}

func (s *Server) failPublic(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrShareNotFound):
		s.shareLimit.fail(clientIP(r), time.Now())
		writeErr(w, http.StatusNotFound, err.Error(), nil)
	case errors.Is(err, quote.ErrExpired), errors.Is(err, quote.ErrAnswered), errors.Is(err, quote.ErrBadTransition):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	default:
		fail(w, err)
	}
}

// publicQuote: GET /api/public/quotes/{token} — ponuka pre zákazníka.
func (s *Server) publicQuote(w http.ResponseWriter, r *http.Request) {
	if !s.publicGuard(w, r) {
		return
	}
	v, err := s.App.QuoteForCustomer(r.Context(), r.PathValue("token"))
	if err != nil {
		s.failPublic(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	writeJSON(w, http.StatusOK, v)
}

// publicQuotePDF: GET /api/public/quotes/{token}/pdf — PDF ponuky.
func (s *Server) publicQuotePDF(w http.ResponseWriter, r *http.Request) {
	if !s.publicGuard(w, r) {
		return
	}
	doc, err := s.App.QuotePDFForCustomer(r.Context(), r.PathValue("token"))
	if err != nil {
		s.failPublic(w, r, err)
		return
	}
	disp := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disp = "inline"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", disp+`; filename="`+doc.Name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(doc.Data)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(doc.Data)
}

// publicQuoteRespond: POST /api/public/quotes/{token}/respond — prijatie alebo odmietnutie.
func (s *Server) publicQuoteRespond(w http.ResponseWriter, r *http.Request) {
	if !s.publicGuard(w, r) {
		return
	}
	var body quote.Response
	if !decode(w, r, &body) {
		return
	}
	v, err := s.App.RespondQuote(r.Context(), r.PathValue("token"), body)
	if err != nil {
		s.failPublic(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
