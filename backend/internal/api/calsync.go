package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/calsync"
)

// Prepojenie kalendára termínov s Google Kalendárom a Outlookom.

func (s *Server) calSyncStatus(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	st, err := s.App.CalendarSyncStatus(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) calSyncConnect(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var body struct {
		Provider calsync.Provider `json:"provider"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := s.App.StartCalendarSync(r.Context(), cid, body.Provider, s.baseURL(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
}

func (s *Server) calSyncRun(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	res, err := s.App.SyncCalendar(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) calSyncDisconnect(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	if err := s.App.DisconnectCalendarSync(r.Context(), cid, r.URL.Query().Get("remove") == "1"); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) calSyncConfig(w http.ResponseWriter, r *http.Request) {
	var c calsync.Config
	if !decode(w, r, &c) {
		return
	}
	if err := s.App.SetCalSyncConfig(r.Context(), c); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// calSyncCallback: návrat od Google alebo Microsoftu po prihlásení. Chráni
// ho jednorazový „state“; výsledok sa ukáže v kalendári.
func (s *Server) calSyncCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	back := func(cid int64, msg string) {
		u := "/kalendar?sync=ok"
		if msg != "" {
			u = "/kalendar?sync=" + url.QueryEscape(msg)
		}
		if cid != 0 {
			u += "&company=" + strconv.FormatInt(cid, 10)
		}
		http.Redirect(w, r, u, http.StatusSeeOther)
	}
	if e := q.Get("error"); e != "" {
		msg := "Prihlásenie bolo zrušené"
		if e != "access_denied" {
			msg = "Prihlásenie sa nepodarilo (" + e + ")"
		}
		back(0, msg)
		return
	}
	cid, err := s.App.FinishCalendarSync(r.Context(), q.Get("state"), q.Get("code"))
	switch {
	case err == nil:
		back(cid, "")
	case errors.Is(err, app.ErrCalSyncState):
		back(0, err.Error())
	default:
		back(cid, "Kalendár sa neprepojil: "+err.Error())
	}
}
