package api

import (
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/asset"
)

func (s *Server) listAssets(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	out, err := s.App.Assets(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveAsset(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var x asset.Asset
	if !decode(w, r, &x) {
		return
	}
	x.ID = 0
	status := http.StatusCreated
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		x.ID, status = id, http.StatusOK
	}
	out, err := s.App.SaveAsset(r.Context(), cid, x)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteAsset(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postDepreciation(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
		return
	}
	sum, err := s.App.PostDepreciation(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posted": sum})
}
