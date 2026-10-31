package api

import (
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Server) updateCompany(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	var c store.Company
	if !decode(w, r, &c) {
		return
	}
	c.ID = cid
	c, err := s.App.UpdateCompany(r.Context(), c)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) listContacts(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	list, err := s.App.Store.ListContacts(r.Context(), cid, r.URL.Query().Get("q"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getContact(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	c, err := s.App.Store.GetContact(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) saveContact(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var c store.Contact
	if !decode(w, r, &c) {
		return
	}
	status := http.StatusCreated
	if r.Method == http.MethodPut {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		c.ID, status = id, http.StatusOK
	} else {
		c.ID = 0
	}
	saved, err := s.App.SaveContact(r.Context(), cid, c)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, status, saved)
}

func (s *Server) deleteContact(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteContact(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteCompany: DELETE /api/companies/{cid} {"name": "…"} — zmazanie firmy
// so všetkými dátami (iba správca inštalácie, po zálohe).
func (s *Server) deleteCompany(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.DeleteCompany(r.Context(), cid, in.Name)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
