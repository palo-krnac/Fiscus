package calsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fake je zjednodušené API kalendára (Google aj Microsoft Graph).
type fake struct {
	mu     sync.Mutex
	events map[string]map[string]any // id → telo
	cals   map[string]bool
	n      int
	calls  []string
	form   url.Values
}

func newFake(t *testing.T) (*fake, *httptest.Server) {
	f := &fake{events: map[string]map[string]any{}, cals: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			f.form = r.PostForm
			if r.PostForm.Get("refresh_token") == "zly" {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
				return
			}
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"palo@example.sk"}`))
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "AT", "refresh_token": "RT", "expires_in": 3600, "id_token": "h." + claims + ".s"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer AT" {
			w.WriteHeader(401)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		path := strings.TrimPrefix(r.URL.Path, "/me")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		switch {
		case r.Method == "POST" && path == "/calendars":
			f.n++
			id := fmt.Sprintf("cal%d", f.n)
			f.cals[id] = true
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
		case r.Method == "DELETE" && len(parts) == 2 && parts[0] == "calendars":
			delete(f.cals, parts[1])
			w.WriteHeader(204)
		case r.Method == "POST" && len(parts) == 3 && parts[2] == "events":
			if !f.cals[parts[1]] {
				w.WriteHeader(404)
				return
			}
			f.n++
			id := fmt.Sprintf("ev%d", f.n)
			f.events[id] = body
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
		case r.Method == "PUT" || r.Method == "PATCH":
			id := parts[len(parts)-1]
			if _, ok := f.events[id]; !ok {
				w.WriteHeader(404)
				return
			}
			f.events[id] = body
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
		case r.Method == "DELETE":
			id := parts[len(parts)-1]
			if _, ok := f.events[id]; !ok {
				w.WriteHeader(404)
				return
			}
			delete(f.events, id)
			w.WriteHeader(204)
		default:
			w.WriteHeader(400)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func TestOAuth(t *testing.T) {
	f, srv := newFake(t)
	ep := Endpoints{Auth: srv.URL + "/auth", Token: srv.URL + "/token", API: srv.URL}
	cfg := Config{GoogleClientID: "gid", GoogleClientSecret: "gsec", MicrosoftClientID: "mid"}
	v, ch := Verifier()
	u, _ := url.Parse(AuthURL(ep, Google, cfg, "http://127.0.0.1:8080/api/calendar-sync/callback", "st", ch))
	q := u.Query()
	if q.Get("client_id") != "gid" || q.Get("code_challenge") != ch || q.Get("access_type") != "offline" || !strings.Contains(q.Get("scope"), "calendar.app.created") {
		t.Fatalf("adresa prihlásenia: %s", u)
	}
	tok, err := Exchange(context.Background(), srv.Client(), ep, Google, cfg, "kod", v, "http://127.0.0.1:8080/cb")
	if err != nil || tok.Access != "AT" || tok.Refresh != "RT" || tok.Email != "palo@example.sk" {
		t.Fatalf("výmena kódu: %+v %v", tok, err)
	}
	if f.form.Get("code_verifier") != v || f.form.Get("client_secret") != "gsec" {
		t.Errorf("formulár: %v", f.form)
	}
	if _, err := Refresh(context.Background(), srv.Client(), ep, Microsoft, cfg, "zly"); err != ErrRevoked {
		t.Errorf("zrušený prístup: %v", err)
	}
	if f.form.Get("client_secret") != "" || f.form.Get("client_id") != "mid" {
		t.Errorf("Microsoft bez tajomstva: %v", f.form)
	}
}

func TestSync(t *testing.T) {
	for _, p := range []Provider{Google, Microsoft} {
		t.Run(string(p), func(t *testing.T) {
			f, srv := newFake(t)
			ctx := context.Background()
			c := Client{HTTP: srv.Client(), EP: Endpoints{API: srv.URL}, P: p, Access: "AT"}
			cal, err := c.CreateCalendar(ctx, "Fiškus – Demo")
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]Event{
				"tax-dph-2026-11": {Date: "2026-11-25", Title: "DPH za október", Remind: true},
				"inv-12":          {Date: "2026-11-10", Title: "Splatnosť FA20260012", Description: "Suma: 120,00 EUR"},
			}
			state := map[string]Item{}
			if r := Sync(ctx, c, cal, want, state); r.Created != 2 || r.Failed != 0 || len(f.events) != 2 {
				t.Fatalf("prvý prenos: %+v", r)
			}
			ev := f.events[state["tax-dph-2026-11"].Remote]
			if p == Google {
				if ev["start"].(map[string]any)["date"] != "2026-11-25" || ev["end"].(map[string]any)["date"] != "2026-11-26" ||
					len(ev["reminders"].(map[string]any)["overrides"].([]any)) != 2 {
					t.Errorf("udalosť Google: %v", ev)
				}
			} else if ev["isAllDay"] != true || ev["isReminderOn"] != true || ev["subject"] != "DPH za október" {
				t.Errorf("udalosť Graph: %v", ev)
			}
			// bez zmeny sa nič nevolá
			n := len(f.calls)
			if r := Sync(ctx, c, cal, want, state); r.Created+r.Updated+r.Deleted != 0 || len(f.calls) != n {
				t.Errorf("bez zmeny: %+v", r)
			}
			// zmena, zánik, udalosť zmazaná v účte
			want["inv-12"] = Event{Date: "2026-11-10", Title: "✓ Splatnosť FA20260012"}
			delete(want, "tax-dph-2026-11")
			want["own-1"] = Event{Date: "2026-12-03", Title: "STK"}
			delete(f.events, state["inv-12"].Remote)
			if r := Sync(ctx, c, cal, want, state); r.Created != 1 || r.Updated != 1 || r.Deleted != 1 || r.Failed != 0 || len(f.events) != 2 {
				t.Fatalf("zmeny: %+v %v", r, f.events)
			}
			// zmazaný kalendár
			_ = c.DeleteCalendar(ctx, cal)
			want["own-2"] = Event{Date: "2026-12-04", Title: "Poistka"}
			if r := Sync(ctx, c, cal, want, state); !r.CalendarGone {
				t.Errorf("zmazaný kalendár: %+v", r)
			}
			// neplatný token
			c.Access = "zly"
			if r := Sync(ctx, c, cal, map[string]Event{"x": {Date: "2026-12-01", Title: "x"}}, map[string]Item{}); r.Err != ErrRevoked {
				t.Errorf("neplatný token: %+v", r)
			}
		})
	}
}
