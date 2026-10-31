package api

import (
	"io"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/app"
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

// importItems: POST …/items/import — telo je súbor CSV alebo XLSX;
// ?apply=1 import vykoná, inak vráti náhľad.
func (s *Server) importItems(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, app.MaxImportBytes))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 5 MB)", nil)
		return
	}
	res, err := s.App.ImportItems(r.Context(), cid, data, r.URL.Query().Get("apply") == "1")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// importPohoda: POST …/import/pohoda — telo je XML export z POHODY;
// ?apply=1 prenos vykoná, inak vráti náhľad.
func (s *Server) importPohoda(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, app.MaxPohodaBytes))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 50 MB)", nil)
		return
	}
	res, err := s.App.ImportPohoda(r.Context(), cid, data, r.URL.Query().Get("apply") == "1")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
