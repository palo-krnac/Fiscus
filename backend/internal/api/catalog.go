package api

import (
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
)

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.Items(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// saveItem: POST (nová) alebo PUT /{id} (úprava) položky cenníka.
func (s *Server) saveItem(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var it catalog.Item
	if !decodeLoose(w, r, &it) {
		return
	}
	it.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		it.ID = id
	}
	isNew := it.ID == 0
	out, err := s.App.SaveItem(r.Context(), cid, it)
	if err != nil {
		fail(w, err)
		return
	}
	code := http.StatusOK
	if isNew {
		code = http.StatusCreated
	}
	writeJSON(w, code, out)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteItem(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
