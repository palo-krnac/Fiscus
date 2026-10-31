package api

import (
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/stock"
)

// Sklad: karty, doklady, nastavenie skladov a skladových položiek.

func (s *Server) getStock(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	v, err := s.App.Stock(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) saveStockConfig(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var c app.StockConfig
	if !decode(w, r, &c) {
		return
	}
	out, err := s.App.SaveStockConfig(r.Context(), cid, c)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveStockDoc(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var d stock.Doc
	if !decode(w, r, &d) {
		return
	}
	status := http.StatusCreated
	d.ID = 0
	if id := r.PathValue("id"); id != "" {
		d.ID, _ = strconv.ParseInt(id, 10, 64)
		status = http.StatusOK
	}
	out, err := s.App.SaveStockDoc(r.Context(), cid, d)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteStockDoc(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.App.DeleteStockDoc(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) stockCard(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	item, _ := strconv.ParseInt(r.PathValue("item"), 10, 64)
	moves, err := s.App.StockCardMoves(r.Context(), cid, r.PathValue("warehouse"), item)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, moves)
}
