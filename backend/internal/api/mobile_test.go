package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestLANHandler(t *testing.T) {
	e := setup(t, edition.Desktop)
	e.app.LANAddresses = func() []string { return []string{"192.168.1.20"} }
	e.app.OnMobile = func(bool, int) error { return nil }
	var st struct{ Enabled bool }
	e.do("PUT", "/api/mobile", map[string]any{"enabled": true}, 200, &st)
	var p struct{ Code string }
	e.do("POST", "/api/mobile/pairing", nil, 201, &p)

	srv := &Server{App: e.app}
	lan := httptest.NewServer(srv.LANHandler(srv.Handler()))
	defer lan.Close()
	call := func(method, path, body, cookie string) (int, string, *http.Response) {
		t.Helper()
		req, _ := http.NewRequest(method, lan.URL+path, strings.NewReader(body))
		req.Header.Set(CSRFHeader, "1")
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: deviceCookie, Value: cookie})
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res.StatusCode, string(b), res
	}
	// bez spárovania: dáta nie
	if code, body, _ := call("GET", "/api/companies", "", ""); code != 403 || !strings.Contains(body, "nie je spárovaný") {
		t.Fatalf("nespárovaný: %d %s", code, body)
	}
	// spárovanie → trvalá cookie
	code, body, res := call("POST", "/api/mobile/pair", `{"code":"`+p.Code+`","name":"Telefón"}`, "")
	if code != 200 {
		t.Fatalf("pair %d %s", code, body)
	}
	var token string
	for _, c := range res.Cookies() {
		if c.Name == deviceCookie && c.HttpOnly && c.MaxAge > 3600*24*365 && c.SameSite == http.SameSiteStrictMode {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatalf("cookie %+v", res.Cookies())
	}
	if code, _, _ := call("GET", "/api/companies", "", token); code != 200 {
		t.Errorf("spárovaný: %d", code)
	}
	if code, _, _ := call("GET", fmt.Sprintf("/api/companies/%d/purchases", e.cid), "", token); code != 200 {
		t.Errorf("prijaté: %d", code)
	}
	// správa programu z telefónu nie
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/mobile"}, {"POST", "/api/mobile/pairing"}, {"POST", "/api/backups"}, {"POST", "/api/desktop/quit"},
		{"PUT", "/api/security/pin"}, {"DELETE", fmt.Sprintf("/api/companies/%d", e.cid)}, {"POST", "/api/auth/lock"}, {"GET", "/api/license"},
	} {
		if code, _, _ := call(c.method, c.path, "{}", token); code != 403 {
			t.Errorf("%s %s: %d", c.method, c.path, code)
		}
	}
	// na počítači (bez LAN) ostáva všetko ako doteraz
	e.do("GET", "/api/mobile", nil, 200, nil)
}
