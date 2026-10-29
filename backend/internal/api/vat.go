package api

import (
	"net/http"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

// kindOptions prečíta druh priznania z dotazu:
// ?druh=riadne|opravne|dodatocne&zistene=2026-11-03&posledne=-60&r37=760
func kindOptions(w http.ResponseWriter, r *http.Request) (vatreturn.Options, bool) {
	q := r.URL.Query()
	o := vatreturn.Options{Kind: vatreturn.Kind(q.Get("druh"))}
	bad := func(msg string) (vatreturn.Options, bool) {
		writeErr(w, http.StatusBadRequest, msg, nil)
		return o, false
	}
	if v := q.Get("zistene"); v != "" {
		d, err := invoice.ParseDate(v)
		if err != nil {
			return bad("Neplatný dátum zistenia")
		}
		o.Discovered = d
	}
	for key, dst := range map[string]**money.Dec{"posledne": &o.LastKnown, "r37": &o.R37} {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			d, err := money.Parse(strings.ReplaceAll(v, ",", "."))
			if err != nil {
				return bad("Neplatná suma " + key)
			}
			*dst = &d
		}
	}
	return o, true
}

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
	o, ok := kindOptions(w, r)
	if !ok {
		return
	}
	ret, _, err := s.App.VATReturnWith(r.Context(), cid, r.URL.Query().Get("period"), excess, o)
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
	if kind != "dph" && kind != "kv" && kind != "sv" {
		writeErr(w, http.StatusNotFound, "Neznámy výstup (dph.xml, kv.xml alebo sv.xml)", nil)
		return
	}
	excess, ok := excessParam(w, r)
	if !ok {
		return
	}
	o, ok := kindOptions(w, r)
	if !ok {
		return
	}
	out, name, err := s.App.VATExportWith(r.Context(), cid, kind, r.URL.Query().Get("period"), excess, o)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(out)
}

// vatFilings: GET …/vat/filings?period= — zaznamenané podania za obdobie.
func (s *Server) vatFilings(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.VATFilings(r.Context(), cid, r.URL.Query().Get("period"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// recordVATFiling: POST …/vat/filings?period=…&druh=…&podane=2026-10-20 — zápis podania.
func (s *Server) recordVATFiling(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	excess, ok := excessParam(w, r)
	if !ok {
		return
	}
	o, ok := kindOptions(w, r)
	if !ok {
		return
	}
	var filed invoice.Date
	if v := r.URL.Query().Get("podane"); v != "" {
		d, err := invoice.ParseDate(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatný dátum podania", nil)
			return
		}
		filed = d
	}
	list, err := s.App.RecordVATFiling(r.Context(), cid, r.URL.Query().Get("period"), excess, o, filed)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
