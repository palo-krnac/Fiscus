package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
)

// Skladová karta produktu a jeho obrázky.

func (s *Server) itemCard(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.ItemCard(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) addItemImage(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, app.MaxItemImageSize+1))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Obrázok je príliš veľký (najviac 2 MB)", nil)
		return
	}
	out, err := s.App.AddItemImage(r.Context(), cid, id, r.URL.Query().Get("name"), data)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) itemImage(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	item, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	id, ok := pathID(w, r, "img")
	if !ok {
		return
	}
	im, err := s.App.ItemImage(r.Context(), cid, item, id)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", im.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(im.Data)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(im.Data)
}

func (s *Server) deleteItemImage(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	item, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	id, ok := pathID(w, r, "img")
	if !ok {
		return
	}
	if err := s.App.DeleteItemImage(r.Context(), cid, item, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) orderItemImages(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.OrderItemImages(r.Context(), cid, id, in.IDs)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setItemStock(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in app.ItemStock
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.SetItemStock(r.Context(), cid, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
