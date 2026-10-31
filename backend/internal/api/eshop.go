package api

import (
	"errors"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/app"
)

// Napojenie na e-shopy: nastavenie obchodov, načítanie objednávok, zásoby.

func failEshop(w http.ResponseWriter, err error) {
	var se *app.EshopError
	if errors.As(err, &se) {
		writeErr(w, http.StatusBadGateway, se.Error(), nil)
		return
	}
	fail(w, err)
}

func (s *Server) getEshops(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.Eshops(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveEshops(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var in struct {
		Conns []app.EshopConn `json:"conns"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.SaveEshops(r.Context(), cid, in.Conns)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) testEshop(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	name, err := s.App.TestEshop(r.Context(), cid, r.PathValue("id"))
	if err != nil {
		failEshop(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func (s *Server) syncEshopOrders(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.SyncEshopOrders(r.Context(), cid, r.PathValue("id"))
	if err != nil {
		failEshop(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) pushEshopStock(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.PushEshopStock(r.Context(), cid, r.PathValue("id"))
	if err != nil {
		failEshop(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) importEshopCatalog(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.ImportEshopCatalog(r.Context(), cid, r.PathValue("id"), r.URL.Query().Get("apply") == "1")
	if err != nil {
		failEshop(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) pushEshopCatalog(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.PushEshopCatalog(r.Context(), cid, r.PathValue("id"), r.URL.Query().Get("all") == "1")
	if err != nil {
		failEshop(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
