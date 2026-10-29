package api

import (
	"net/http"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// excessParam prečíta nadmerný odpočet z minulého obdobia (?excess=123,45).
func excessParam(w http.ResponseWriter, r *http.Request) (money.Dec, bool) {
	v := strings.TrimSpace(r.URL.Query().Get("excess"))
	if v == "" {
		return money.Zero, true
	}
	d, err := money.Parse(v)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatná suma nadmerného odpočtu", nil)
		return d, false
	}
	return d, true
}

// vatReturn: GET /api/companies/{cid}/vat?period=2026-10&excess=0
func (s *Server) vatReturn(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	excess, ok := excessParam(w, r)
	if !ok {
		return
	}
	ret, _, err := s.App.VATReturn(r.Context(), cid, r.URL.Query().Get("period"), excess)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ret)
}

// vatExport: GET /api/companies/{cid}/vat/dph.xml | kv.xml ?period=…
func (s *Server) vatExport(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	kind := strings.TrimSuffix(r.PathValue("kind"), ".xml")
	if kind != "dph" && kind != "kv" {
		writeErr(w, http.StatusNotFound, "Neznámy výstup (dph.xml alebo kv.xml)", nil)
		return
	}
	excess, ok := excessParam(w, r)
	if !ok {
		return
	}
	out, name, err := s.App.VATExport(r.Context(), cid, kind, r.URL.Query().Get("period"), excess)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(out)
}
