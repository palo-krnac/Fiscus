package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
)

func failQuote(w http.ResponseWriter, err error) {
	if errors.Is(err, quote.ErrLocked) || errors.Is(err, quote.ErrBadTransition) {
		writeErr(w, http.StatusConflict, err.Error(), nil)
		return
	}
	fail(w, err)
}

// quoteRow je ponuka so stavom platnosti k dnešku.
type quoteRow struct {
	quote.Quote
	Expired bool `json:"expired"`
}

func (s *Server) quoteRow(q quote.Quote) quoteRow {
	return quoteRow{Quote: q, Expired: q.Expired(s.App.Today())}
}

// listQuotes: GET /api/companies/{cid}/quotes?year=2026 (bez roku = všetky)
func (s *Server) listQuotes(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	year := 0
	if y := r.URL.Query().Get("year"); y != "" {
		var err error
		if year, err = strconv.Atoi(y); err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
			return
		}
	}
	list, err := s.App.Quotes(r.Context(), cid, year)
	if err != nil {
		failQuote(w, err)
		return
	}
	rows := make([]quoteRow, len(list))
	for i, q := range list {
		rows[i] = s.quoteRow(q)
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getQuote(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q, err := s.App.Quote(r.Context(), cid, id)
	if err != nil {
		failQuote(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.quoteRow(q))
}

// saveQuote: POST (nová) alebo PUT /{id} (úprava konceptu).
func (s *Server) saveQuote(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var q quote.Quote
	if !decodeLoose(w, r, &q) {
		return
	}
	q.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		q.ID = id
	}
	out, err := s.App.SaveQuote(r.Context(), cid, q)
	if err != nil {
		failQuote(w, err)
		return
	}
	code := http.StatusOK
	if q.ID == 0 {
		code = http.StatusCreated
	}
	writeJSON(w, code, s.quoteRow(out))
}

func (s *Server) previewQuote(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var q quote.Quote
	if !decodeLoose(w, r, &q) {
		return
	}
	out, err := s.App.PreviewQuote(r.Context(), cid, q)
	if err != nil {
		failQuote(w, err)
		return
	}
	resp := struct {
		quote.Quote
		Errors []invoice.FieldError `json:"errors"`
	}{Quote: out, Errors: []invoice.FieldError{}}
	var ve *invoice.ValidationError
	if errors.As(out.Validate(), &ve) {
		resp.Errors = ve.Fields
	}
	writeJSON(w, http.StatusOK, resp)
}

// quoteAction spustí akciu nad ponukou (vystavenie, prijatie, ...).
func (s *Server) quoteAction(fn func(*Server, *http.Request, int64, int64) (quote.Quote, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, _ := pathID(w, r, "cid")
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		q, err := fn(s, r, cid, id)
		if err != nil {
			failQuote(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s.quoteRow(q))
	}
}

func (s *Server) issueQuote(r *http.Request, cid, id int64) (quote.Quote, error) {
	return s.App.IssueQuote(r.Context(), cid, id)
}
func (s *Server) acceptQuote(r *http.Request, cid, id int64) (quote.Quote, error) {
	return s.App.AcceptQuote(r.Context(), cid, id)
}
func (s *Server) rejectQuote(r *http.Request, cid, id int64) (quote.Quote, error) {
	return s.App.RejectQuote(r.Context(), cid, id)
}
func (s *Server) copyQuote(r *http.Request, cid, id int64) (quote.Quote, error) {
	return s.App.CopyQuote(r.Context(), cid, id)
}

// invoiceQuote: POST /quotes/{id}/invoice → koncept faktúry.
func (s *Server) invoiceQuote(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	inv, err := s.App.InvoiceQuote(r.Context(), cid, id)
	if err != nil {
		failQuote(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}
