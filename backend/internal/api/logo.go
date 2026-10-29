package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/app"
)

func failLogo(w http.ResponseWriter, err error) {
	if errors.Is(err, app.ErrBadLogo) {
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	fail(w, err)
}

// getLogo: GET /api/companies/{cid}/logo — obrázok pre tlač dokladov.
func (s *Server) getLogo(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	l, err := s.App.Logo(r.Context(), cid)
	if err != nil {
		failLogo(w, err)
		return
	}
	w.Header().Set("Content-Type", l.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "logo", l.Updated, bytesReader(l.Data))
}

// putLogo: PUT /api/companies/{cid}/logo — telo je samotný obrázok.
func (s *Server) putLogo(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, app.MaxLogoBytes+1))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Logo je príliš veľké (najviac 512 kB)", nil)
		return
	}
	if _, err := s.App.SetLogo(r.Context(), cid, data); err != nil {
		failLogo(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteLogo(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	if err := s.App.DeleteLogo(r.Context(), cid); err != nil {
		failLogo(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
