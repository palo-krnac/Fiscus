package api

import (
	"encoding/csv"
	"errors"
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

func (s *Server) createStockFeed(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	token, err := s.App.CreateStockFeed(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

func (s *Server) revokeStockFeed(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	if err := s.App.RevokeStockFeed(r.Context(), cid); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// publicStockFeed: GET /api/public/stock/{cid}/{token}/zasoby.{json,csv} —
// zásoby pre e-shop cez tajný odkaz (bez prihlásenia).
func (s *Server) publicStockFeed(w http.ResponseWriter, r *http.Request) {
	cid, _ := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	items, err := s.App.StockFeed(r.Context(), cid, r.PathValue("token"))
	if err != nil {
		http.Error(w, "Neplatný odkaz", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.PathValue("format") == "zasoby.csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		cw := csv.NewWriter(w)
		cw.Comma = ';'
		_ = cw.Write([]string{"kod", "nazov", "mj", "mnozstvo", "cena_bez_dph", "cena_s_dph", "sadzba_dph"})
		for _, it := range items {
			_ = cw.Write([]string{it.Code, it.Name, it.Unit, it.Quantity.String(), it.Price.String(), it.PriceGross.String(), strconv.Itoa(it.VATRate)})
		}
		cw.Flush()
		return
	}
	if r.PathValue("format") != "zasoby.json" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// publicShopOrder: POST /api/public/stock/{cid}/{token}/objednavky —
// objednávka z e-shopu ako výdajka (iba ak je zapnutá v nastavení skladu).
func (s *Server) publicShopOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	var o app.ShopOrder
	if !decode(w, r, &o) {
		return
	}
	d, err := s.App.ShopOrderIssue(r.Context(), cid, r.PathValue("token"), o)
	switch {
	case errors.Is(err, app.ErrBadStockFeed):
		writeErr(w, http.StatusNotFound, "Neplatný odkaz", nil)
	case errors.Is(err, app.ErrOrdersOff):
		writeErr(w, http.StatusForbidden, err.Error(), nil)
	case err != nil:
		fail(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"number": d.Number, "lines": len(d.Lines)})
	}
}
