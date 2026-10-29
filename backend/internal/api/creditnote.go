package api

import (
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// creditNote vytvorí koncept dobropisu k vystavenej faktúre.
func (s *Server) creditNote(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cn, err := s.App.CreateCreditNote(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cn)
}

// settleAdvance: POST /invoices/{id}/settle — koncept vyúčtovacej faktúry k uhradenej zálohe.
func (s *Server) settleAdvance(w http.ResponseWriter, r *http.Request) {
	s.action(w, r, func(cid, id int64) (invoice.Invoice, error) {
		return s.App.SettleAdvance(r.Context(), cid, id)
	})
}
