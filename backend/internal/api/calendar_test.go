package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestCalendarAPI(t *testing.T) {
	e := setup(t, edition.Server)
	base := fmt.Sprintf("/api/companies/%d/calendar", e.cid)
	var v struct {
		Events []struct {
			ID, Date, Kind string
			Shifted        string
		}
		Holidays []struct{ Date, Title string }
		CanEdit  bool
	}
	e.do("GET", base+"?from=2026-11-01&to=2026-11-30", nil, 200, &v)
	if !v.CanEdit || len(v.Holidays) != 1 {
		t.Fatalf("november: %+v", v)
	}
	e.do("GET", base+"?from=2026-11", nil, 400, nil)

	var own struct{ ID int64 }
	e.do("POST", base+"/own", map[string]any{"title": "Odvody SP", "date": "2026-01-08", "repeat": "monthly", "workday": true}, 201, &own)
	e.do("POST", base+"/own", map[string]any{"title": ""}, 422, nil)
	e.do("PUT", fmt.Sprintf("%s/own/%d", base, own.ID), map[string]any{"title": "Odvody do SP", "date": "2026-01-08", "repeat": "monthly", "workday": true}, 200, nil)
	e.do("POST", fmt.Sprintf("%s/own/%d/done", base, own.ID), map[string]any{"date": "2026-11-08", "done": true}, 200, nil)
	e.do("GET", base+"?from=2026-11-01&to=2026-11-30", nil, 200, &v)
	found := false
	for _, ev := range v.Events {
		if ev.ID == fmt.Sprintf("own-%d-2026-11-08", own.ID) && ev.Date == "2026-11-09" && ev.Shifted == "2026-11-08" {
			found = true
		}
	}
	if !found {
		t.Errorf("vlastný termín: %+v", v.Events)
	}

	// súbor .ics
	resp, err := e.client.Get(e.srv.URL + base + ".ics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/calendar") || !strings.Contains(string(body), "Odvody do SP") {
		t.Fatalf("ics: %d %s", resp.StatusCode, body)
	}

	// odber cez tajný odkaz bez prihlásenia
	var feed struct{ Token string }
	e.do("POST", "/api/me/calendar-feed", nil, 201, &feed)
	var st struct{ Active bool }
	e.do("GET", "/api/me/calendar-feed", nil, 200, &st)
	if !st.Active || !strings.HasPrefix(feed.Token, "fkc_") {
		t.Fatalf("odber: %+v %+v", st, feed)
	}
	get := func(path string) (int, string) {
		resp, err := http.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	pub := fmt.Sprintf("/api/public/calendar/%d/%s/fiskus.ics", e.cid, feed.Token)
	if code, b := get(pub); code != 200 || !strings.Contains(b, "Odvody do SP") || !strings.Contains(b, "Priznanie k DPH") {
		t.Fatalf("odber: %d %s", code, b)
	}
	// tokenom sa nedá volať API
	req, _ := http.NewRequest("GET", e.srv.URL+base+"?from=2026-11-01&to=2026-11-30", nil)
	req.Header.Set("Authorization", "Bearer "+feed.Token)
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 401 {
		t.Errorf("token ako API kľúč: %d", resp.StatusCode)
	}
	e.do("DELETE", "/api/me/calendar-feed", nil, 204, nil)
	if code, _ := get(pub); code != 404 {
		t.Errorf("zrušený odkaz: %d", code)
	}
	if code, _ := get(fmt.Sprintf("/api/public/calendar/x/%s/fiskus.ics", feed.Token)); code != 404 {
		t.Errorf("zlá firma: %d", code)
	}
	e.do("DELETE", fmt.Sprintf("%s/own/%d", base, own.ID), nil, 204, nil)
}
