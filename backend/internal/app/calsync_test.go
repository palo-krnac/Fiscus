package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/calendar"
	"github.com/palo-krnac/fiscus/backend/internal/calsync"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// fakeCalendar je zjednodušený Google Kalendár (OAuth aj API).
type fakeCalendar struct {
	mu     sync.Mutex
	cals   map[string]string            // id → názov
	events map[string]map[string]string // kalendár → udalosť → názov
	n      int
}

func newFakeCalendar(t *testing.T) (*fakeCalendar, *httptest.Server) {
	f := &fakeCalendar{cals: map[string]string{}, events: map[string]map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			if r.PostForm.Get("code") != "" && r.PostForm.Get("code_verifier") == "" {
				w.WriteHeader(400)
				return
			}
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"palo@example.sk"}`))
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "AT", "refresh_token": "RT-tajny", "expires_in": 3600, "id_token": "h." + claims + ".s"})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.Method == "GET" && len(parts) == 2:
			if _, ok := f.cals[parts[1]]; !ok {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": parts[1]})
		case r.Method == "POST" && len(parts) == 1:
			f.n++
			id := fmt.Sprintf("cal%d", f.n)
			f.cals[id], f.events[id] = body["summary"].(string), map[string]string{}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
		case r.Method == "DELETE" && len(parts) == 2:
			delete(f.cals, parts[1])
			delete(f.events, parts[1])
			w.WriteHeader(204)
		case r.Method == "POST" && len(parts) == 3:
			f.n++
			id := fmt.Sprintf("ev%d", f.n)
			f.events[parts[1]][id] = body["summary"].(string)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
		case r.Method == "PUT":
			f.events[parts[1]][parts[3]] = body["summary"].(string)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": parts[3]})
		case r.Method == "DELETE":
			delete(f.events[parts[1]], parts[3])
			w.WriteHeader(204)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func TestCalendarSync(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	f, srv := newFakeCalendar(t)
	a.HTTP = srv.Client()
	a.CalSyncEndpoints = map[calsync.Provider]calsync.Endpoints{calsync.Google: {Auth: srv.URL + "/auth", Token: srv.URL + "/token", API: srv.URL}}

	if _, err := a.StartCalendarSync(ctx, c.ID, calsync.Google, "http://127.0.0.1:8765"); !fieldErr(err, "provider") {
		t.Fatalf("bez nastavenia: %v", err)
	}
	if err := a.SetCalSyncConfig(ctx, calsync.Config{GoogleClientID: "x"}); err != ErrForbidden {
		t.Errorf("iba hlavný správca: %v", err)
	}
	ctx := auth.WithActor(ctx, auth.Local) // desktop: lokálny používateľ je hlavný správca
	if err := a.SetCalSyncConfig(ctx, calsync.Config{GoogleClientID: "gid", GoogleClientSecret: "gsec"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetCalSyncConfig(ctx, calsync.Config{GoogleClientID: "gid"}); err != nil { // tajomstvo ostane
		t.Fatal(err)
	}
	st, _ := a.CalendarSyncStatus(ctx, c.ID)
	if !st.Google || st.Microsoft || st.Connected || st.Config == nil || !st.Config.GoogleSecretSet {
		t.Fatalf("stav: %+v %+v", st, st.Config)
	}
	if _, err := a.SaveOwnEvent(ctx, c.ID, calendar.Own{Title: "STK", Date: invoice.MustDate("2026-12-03")}); err != nil {
		t.Fatal(err)
	}

	link, err := a.StartCalendarSync(ctx, c.ID, calsync.Google, "http://127.0.0.1:8765")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	if u.Query().Get("redirect_uri") != "http://127.0.0.1:8765"+CalSyncCallbackPath {
		t.Errorf("návrat: %s", link)
	}
	if _, err := a.FinishCalendarSync(ctx, "cudzi", "kod"); err != ErrCalSyncState {
		t.Errorf("cudzí state: %v", err)
	}
	cid, err := a.FinishCalendarSync(ctx, u.Query().Get("state"), "kod")
	if err != nil || cid != c.ID {
		t.Fatalf("prepojenie: %d %v", cid, err)
	}
	if _, err := a.FinishCalendarSync(ctx, u.Query().Get("state"), "kod"); err != ErrCalSyncState {
		t.Errorf("state platí iba raz: %v", err)
	}
	st, _ = a.CalendarSyncStatus(ctx, c.ID)
	if !st.Connected || st.Account != "palo@example.sk" || st.Events == 0 || st.LastError != "" {
		t.Fatalf("po prepojení: %+v", st)
	}
	raw, _ := json.Marshal(st)
	if strings.Contains(string(raw), "RT-tajny") || strings.Contains(string(raw), "gsec") {
		t.Fatal("stav prezradil tajomstvo")
	}
	if len(f.cals) != 1 || len(f.events["cal1"]) != st.Events || f.cals["cal1"] != "Fiškus – "+c.Name {
		t.Fatalf("kalendár v účte: %v %v", f.cals, f.events)
	}
	found := false
	for _, title := range f.events["cal1"] {
		found = found || title == "STK"
	}
	if !found {
		t.Errorf("vlastný termín chýba: %v", f.events["cal1"])
	}

	// nový termín sa prenesie, nezmenené nie
	if _, err := a.SaveOwnEvent(ctx, c.ID, calendar.Own{Title: "Poistka auta", Date: invoice.MustDate("2027-01-15")}); err != nil {
		t.Fatal(err)
	}
	res, err := a.SyncCalendar(ctx, c.ID)
	if err != nil || res.Created != 1 || res.Updated != 0 || res.Deleted != 0 || res.Error != "" {
		t.Fatalf("prenos: %+v %v", res, err)
	}
	// kalendár zmazaný v účte → vytvorí sa nový
	delete(f.cals, "cal1")
	delete(f.events, "cal1")
	if res, err = a.SyncCalendar(ctx, c.ID); err != nil || res.Error != "" || len(f.cals) != 1 || res.Created < 2 {
		t.Fatalf("obnova kalendára: %+v %v %v", res, err, f.cals)
	}
	// zrušenie prepojenia aj s kalendárom
	if err := a.DisconnectCalendarSync(ctx, c.ID, true); err != nil {
		t.Fatal(err)
	}
	if st, _ = a.CalendarSyncStatus(ctx, c.ID); st.Connected || len(f.cals) != 0 {
		t.Errorf("po zrušení: %+v %v", st, f.cals)
	}
	a.SyncAllCalendars(ctx) // bez prepojení nič nerobí
}
