package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestOAuthHTTPFlow(t *testing.T) {
	admin := setup(t, edition.Server)
	base := admin.srv.URL
	raw := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// /mcp bez tokenu povie, kde je autorizačný server
	res, _ := http.Post(base+"/mcp", "application/json", strings.NewReader(`{}`))
	if res.StatusCode != 401 || !strings.Contains(res.Header.Get("WWW-Authenticate"), `resource_metadata="`+base+`/.well-known/oauth-protected-resource"`) {
		t.Fatalf("401: %d %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}
	var pr, as map[string]any
	getJSON(t, base+"/.well-known/oauth-protected-resource", &pr)
	getJSON(t, base+"/.well-known/oauth-authorization-server", &as)
	if pr["resource"] != base+"/mcp" || as["token_endpoint"] != base+"/oauth/token" || as["registration_endpoint"] != base+"/oauth/register" {
		t.Fatalf("metadáta: %v %v", pr, as)
	}

	// registrácia (neznáme polia sa ignorujú)
	reg := `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"],"grant_types":["authorization_code","refresh_token"],"token_endpoint_auth_method":"none"}`
	res, _ = http.Post(base+"/oauth/register", "application/json", strings.NewReader(reg))
	var client struct {
		ClientID string `json:"client_id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&client)
	if res.StatusCode != 201 || client.ClientID == "" {
		t.Fatalf("registrácia %d", res.StatusCode)
	}
	res, _ = http.Post(base+"/oauth/register", "application/json", strings.NewReader(`{"redirect_uris":["http://zle.sk/cb"]}`))
	if res.StatusCode != 400 {
		t.Fatalf("zlá adresa návratu %d", res.StatusCode)
	}

	verifier := strings.Repeat("a1", 30)
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {client.ClientID}, "redirect_uri": {"https://claude.ai/api/mcp/auth_callback"},
		"state": {"s1"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}}
	res, _ = raw.Get(base + "/oauth/authorize?" + q.Encode())
	if res.StatusCode != 302 || !strings.HasPrefix(res.Header.Get("Location"), "/oauth/povolit?") {
		t.Fatalf("authorize: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	bad := url.Values{"client_id": {client.ClientID}, "redirect_uri": {"https://utocnik.sk/cb"}}
	if res, _ = raw.Get(base + "/oauth/authorize?" + bad.Encode()); res.StatusCode != 400 {
		t.Fatal("neoverená adresa návratu nesmie presmerovať")
	}

	// súhlas iba prihláseného
	admin.anonymous().do("GET", "/api/oauth/request?"+q.Encode(), nil, 401, nil)
	var info struct{ Client, RedirectHost string }
	admin.do("GET", "/api/oauth/request?"+q.Encode(), nil, 200, &info)
	if info.Client != "Claude" || info.RedirectHost != "claude.ai" {
		t.Fatalf("%+v", info)
	}
	var dec struct{ Redirect string }
	admin.do("POST", "/api/oauth/decision", map[string]any{"query": q.Encode(), "allow": true}, 200, &dec)
	back, _ := url.Parse(dec.Redirect)
	code := back.Query().Get("code")
	if back.Host != "claude.ai" || back.Query().Get("state") != "s1" || code == "" {
		t.Fatalf("návrat %s", dec.Redirect)
	}

	// token
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {client.ClientID}, "code": {code},
		"redirect_uri": {"https://claude.ai/api/mcp/auth_callback"}, "code_verifier": {verifier}}
	res, _ = http.PostForm(base+"/oauth/token", form)
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
	}
	_ = json.NewDecoder(res.Body).Decode(&tok)
	if res.StatusCode != 200 || tok.TokenType != "Bearer" || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("token %d %+v", res.StatusCode, tok)
	}
	res, _ = http.PostForm(base+"/oauth/token", form) // druhýkrát
	var e struct{ Error string }
	_ = json.NewDecoder(res.Body).Decode(&e)
	if res.StatusCode != 400 || e.Error != "invalid_grant" {
		t.Fatalf("kód druhýkrát %d %v", res.StatusCode, e)
	}

	// MCP s prístupovým tokenom
	mcpCall := func(token string) int {
		req, _ := http.NewRequest("POST", base+"/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode == 200 && !strings.Contains(string(b), "create_purchase") {
			t.Fatalf("zoznam nástrojov: %s", b)
		}
		return res.StatusCode
	}
	if s := mcpCall(tok.AccessToken); s != 200 {
		t.Fatalf("mcp %d", s)
	}
	// obnovenie
	res, _ = http.PostForm(base+"/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {client.ClientID}, "refresh_token": {tok.RefreshToken}})
	if res.StatusCode != 200 {
		t.Fatalf("obnovenie %d", res.StatusCode)
	}
	res, _ = http.PostForm(base+"/oauth/token", url.Values{"grant_type": {"password"}})
	if res.StatusCode != 400 {
		t.Fatal("nepodporovaný grant")
	}
	// povolenie je medzi kľúčmi a dá sa zrušiť
	var keys []struct {
		ID     int64
		Prefix string
	}
	admin.do("GET", "/api/me/keys", nil, 200, &keys)
	if len(keys) != 1 || keys[0].Prefix != "OAuth" {
		t.Fatalf("%+v", keys)
	}
	admin.do("DELETE", "/api/me/keys/"+itoa64(keys[0].ID), nil, 204, nil)
	if s := mcpCall(tok.AccessToken); s != 401 {
		t.Fatalf("po zrušení %d", s)
	}
}

func TestOAuthOffOnDesktop(t *testing.T) {
	d := setup(t, edition.Desktop)
	res, _ := http.Get(d.srv.URL + "/.well-known/oauth-authorization-server")
	if ct := res.Header.Get("Content-Type"); res.StatusCode == 200 && strings.Contains(ct, "json") {
		t.Fatal("desktop nemá OAuth")
	}
}

func getJSON(t *testing.T, u string, out any) {
	t.Helper()
	res, err := http.Get(u)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("%s: %v", u, err)
	}
	_ = json.NewDecoder(res.Body).Decode(out)
}

func itoa64(n int64) string { b, _ := json.Marshal(n); return string(b) }
