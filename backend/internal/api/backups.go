package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
)

func failBackup(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, backup.ErrNoBackup):
		writeErr(w, http.StatusNotFound, "Záloha neexistuje", nil)
	case errors.Is(err, app.ErrBackupsOff):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	default:
		fail(w, err)
	}
}

func (s *Server) backupStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.App.BackupStatus(r.Context())
	if err != nil {
		failBackup(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	it, err := s.App.CreateBackup(r.Context())
	if err != nil {
		failBackup(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

// downloadBackup pošle šifrovaný súbor zálohy (.fbk).
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	f, it, err := s.App.OpenBackup(r.Context(), r.PathValue("name"))
	if err != nil {
		failBackup(w, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+it.Name+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(it.Size, 10))
	_, _ = io.Copy(w, f)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.App.RequestRestore(r.Context(), r.PathValue("name")); err != nil {
		failBackup(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cancelRestore(w http.ResponseWriter, r *http.Request) {
	if err := s.App.CancelRestore(r.Context()); err != nil {
		failBackup(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
