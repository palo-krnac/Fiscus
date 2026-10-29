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

// issueMissingReceipts: POST /invoices/{id}/receipts — dodatočné daňové
// doklady k platbám zálohy (dodanie do 15 dní sa nestihlo).
func (s *Server) issueMissingReceipts(w http.ResponseWriter, r *http.Request) {
	s.action(w, r, func(cid, id int64) (invoice.Invoice, error) {
		return s.App.IssueMissingReceipts(r.Context(), cid, id)
	})
}

// advanceCandidates: GET /invoices/{id}/advances — ďalšie zálohy odberateľa do vyúčtovania.
func (s *Server) advanceCandidates(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.App.AdvanceCandidates(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// addAdvance: POST /invoices/{id}/advances/{pid} — pridá zálohu do konceptu vyúčtovania.
func (s *Server) addAdvance(w http.ResponseWriter, r *http.Request) {
	pid, ok := pathID(w, r, "pid")
	if !ok {
		return
	}
	s.action(w, r, func(cid, id int64) (invoice.Invoice, error) {
		return s.App.AddAdvanceToFinal(r.Context(), cid, id, pid)
	})
}

// removeAdvance: DELETE /invoices/{id}/advances/{pid} — odoberie pridanú zálohu.
func (s *Server) removeAdvance(w http.ResponseWriter, r *http.Request) {
	pid, ok := pathID(w, r, "pid")
	if !ok {
		return
	}
	s.action(w, r, func(cid, id int64) (invoice.Invoice, error) {
		return s.App.RemoveAdvanceFromFinal(r.Context(), cid, id, pid)
	})
}
