package api

import "net/http"

// creditNote vytvorí koncept dobropisu k vystavenej faktúre.
func (s *Server) creditNote(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cn, err := s.App.CreateCreditNote(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cn)
}
