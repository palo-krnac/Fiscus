package app

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

const claudeCallback = "https://claude.ai/api/mcp/auth_callback"

func pkce(v string) string {
	s := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(s[:])
}

func isOAuth(err error, code string) bool {
	var oe *OAuthError
	return errors.As(err, &oe) && oe.Code == code
}

func TestOAuthRegister(t *testing.T) {
	o := newOffice(t)
	tests := []struct {
		uris []string
		ok   bool
	}{
		{[]string{claudeCallback}, true},
		{[]string{"http://localhost:6274/oauth/callback", "http://127.0.0.1:33418/cb"}, true},
		{[]string{"http://zly.example.sk/cb"}, false},
		{[]string{"https://x.sk/cb#frag"}, false},
		{[]string{"javascript:alert(1)"}, false},
		{nil, false},
		{[]string{"https://a", "https://b", "https://c", "https://d", "https://e", "https://f"}, false},
	}
	for _, tt := range tests {
		id, c, err := o.a.RegisterOAuthClient(ctx, "Claude", tt.uris)
		if (err == nil) != tt.ok {
			t.Errorf("%v: %v", tt.uris, err)
			continue
		}
		if tt.ok {
			got, err := o.a.OAuthClientByID(ctx, id)
			if err != nil || got.Name != "Claude" || len(got.RedirectURIs) != len(c.RedirectURIs) {
				t.Errorf("client_id: %+v %v", got, err)
			}
		}
	}
	id, _, _ := o.a.RegisterOAuthClient(ctx, "Claude", []string{claudeCallback})
	if _, err := o.a.OAuthClientByID(ctx, id[:len(id)-2]+"xx"); !isOAuth(err, "invalid_client") {
		t.Fatal("podvrhnutý client_id")
	}
	// desktop OAuth nemá
	caps, _ := edition.For(edition.Desktop)
	if _, _, err := New(memstore.New(), caps).RegisterOAuthClient(ctx, "x", []string{claudeCallback}); !errors.Is(err, ErrOAuthOff) {
		t.Fatal("desktop")
	}
}

func TestOAuthFlow(t *testing.T) {
	o := newOffice(t)
	clock := time.Unix(1_800_000_000, 0)
	o.a.Now = func() time.Time { return clock }
	clientID, _, _ := o.a.RegisterOAuthClient(ctx, "Claude", []string{claudeCallback})
	verifier := strings.Repeat("v", 50)
	req := AuthRequest{ResponseType: "code", ClientID: clientID, RedirectURI: claudeCallback, State: "xyz",
		CodeChallenge: pkce(verifier), Method: "S256", Resource: "https://fiskus.example.sk/mcp"}
	jana := as(o.jana)

	// zlá adresa návratu sa neposiela nikam
	bad := req
	bad.RedirectURI = "https://utocnik.example/cb"
	if _, err := o.a.ApproveOAuth(jana, bad, false); err == nil {
		t.Fatal("nezaregistrovaná adresa návratu")
	}
	// bez PKCE: chyba ide späť aplikácii
	plain := req
	plain.Method = "plain"
	back, err := o.a.ApproveOAuth(jana, plain, false)
	if err != nil || !strings.Contains(back, "error=invalid_request") || !strings.Contains(back, "state=xyz") {
		t.Fatalf("bez PKCE: %s %v", back, err)
	}
	// povoliť smie iba človek, nie API kľúč
	keyed := o.jana
	keyed.KeyID = 5
	if _, err := o.a.ApproveOAuth(as(keyed), req, false); !errors.Is(err, ErrForbidden) {
		t.Fatal("povolenie API kľúčom")
	}
	// zamietnutie
	if d, _ := o.a.DenyOAuth(jana, req); !strings.Contains(d, "error=access_denied") {
		t.Fatal(d)
	}

	code := func(readOnly bool) string {
		t.Helper()
		back, err := o.a.ApproveOAuth(jana, req, readOnly)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(back)
		if !strings.HasPrefix(back, claudeCallback+"?") || u.Query().Get("state") != "xyz" || u.Query().Get("code") == "" {
			t.Fatalf("návrat: %s", back)
		}
		return u.Query().Get("code")
	}

	// zlý verifier: kód sa spotrebuje
	c := code(true)
	if _, err := o.a.ExchangeCode(ctx, clientID, c, claudeCallback, strings.Repeat("x", 50)); !isOAuth(err, "invalid_grant") {
		t.Fatal("zlý verifier")
	}
	if _, err := o.a.ExchangeCode(ctx, clientID, c, claudeCallback, verifier); !isOAuth(err, "invalid_grant") {
		t.Fatal("kód platí raz")
	}
	// vypršaný kód
	c = code(true)
	clock = clock.Add(2 * time.Minute)
	if _, err := o.a.ExchangeCode(ctx, clientID, c, claudeCallback, verifier); !isOAuth(err, "invalid_grant") {
		t.Fatal("vypršaný kód")
	}

	tok, err := o.a.ExchangeCode(ctx, clientID, code(true), claudeCallback, verifier)
	if err != nil || !strings.HasPrefix(tok.AccessToken, OAuthAccessPrefix) || !strings.HasPrefix(tok.RefreshToken, OAuthRefreshPrefix) || tok.ExpiresIn != 3600 {
		t.Fatalf("%+v %v", tok, err)
	}
	act, err := o.a.OAuthActor(ctx, tok.AccessToken, auth.ChannelMCP, "1.2.3.4")
	if err != nil || act.UserID != o.jana.UserID || !act.ReadOnly || act.KeyID == 0 || !strings.Contains(act.Name, "Claude") {
		t.Fatalf("actor: %+v %v", act, err)
	}
	// tokeny sa nedajú zameniť s API kľúčom
	if _, err := o.a.KeyActor(ctx, tok.AccessToken, auth.ChannelMCP, ""); err == nil {
		t.Fatal("prístupový token ako API kľúč")
	}
	if _, err := o.a.OAuthActor(ctx, tok.RefreshToken, auth.ChannelMCP, ""); err == nil {
		t.Fatal("obnovovací token ako prístupový")
	}
	// povolenie je v zozname kľúčov používateľa
	keys, _ := o.a.ListAPIKeys(jana)
	if len(keys) != 1 || keys[0].Prefix != "OAuth" || keys[0].Name != "Claude" {
		t.Fatalf("kľúče: %+v", keys)
	}

	// po hodine prístupový token neplatí, obnovovací áno
	clock = clock.Add(OAuthAccessTTL + time.Second)
	if _, err := o.a.OAuthActor(ctx, tok.AccessToken, auth.ChannelMCP, ""); err == nil {
		t.Fatal("vypršaný prístupový token")
	}
	tok2, err := o.a.RefreshOAuth(ctx, clientID, tok.RefreshToken)
	if err != nil || tok2.RefreshToken == tok.RefreshToken {
		t.Fatalf("obnovenie: %v", err)
	}
	if _, err := o.a.OAuthActor(ctx, tok2.AccessToken, auth.ChannelMCP, ""); err != nil {
		t.Fatal(err)
	}
	// vymyslené tokeny (bez podpisu Fiškusa) povolenie nezrušia
	grant := strings.SplitN(strings.TrimPrefix(tok2.RefreshToken, OAuthRefreshPrefix), ".", 2)[0]
	for _, fake := range []string{OAuthRefreshPrefix + grant + ".x", OAuthRefreshPrefix + grant + ".x.y", OAuthRefreshPrefix + grant + ".x.y.z"} {
		if _, err := o.a.RefreshOAuth(ctx, clientID, fake); !isOAuth(err, "invalid_grant") {
			t.Fatalf("vymyslený token %q: %v", fake, err)
		}
	}
	if _, err := o.a.OAuthActor(ctx, tok2.AccessToken, auth.ChannelMCP, ""); err != nil {
		t.Fatal("vymyslený token zrušil povolenie")
	}
	// starý obnovovací token znova = krádež: povolenie sa zruší
	if _, err := o.a.RefreshOAuth(ctx, clientID, tok.RefreshToken); !isOAuth(err, "invalid_grant") {
		t.Fatal("starý obnovovací token")
	}
	if _, err := o.a.OAuthActor(ctx, tok2.AccessToken, auth.ChannelMCP, ""); err == nil {
		t.Fatal("po opakovanom použití má byť povolenie zrušené")
	}
	if _, err := o.a.RefreshOAuth(ctx, clientID, tok2.RefreshToken); err == nil {
		t.Fatal("zrušené povolenie sa neobnoví")
	}

	// zrušenie v Prístupe pre AI
	tok3, _ := o.a.ExchangeCode(ctx, clientID, code(false), claudeCallback, verifier)
	act3, err := o.a.OAuthActor(ctx, tok3.AccessToken, auth.ChannelMCP, "")
	if err != nil || act3.ReadOnly {
		t.Fatalf("plné práva: %+v %v", act3, err)
	}
	if err := o.a.RevokeAPIKey(jana, act3.KeyID); err != nil {
		t.Fatal(err)
	}
	if _, err := o.a.OAuthActor(ctx, tok3.AccessToken, auth.ChannelMCP, ""); err == nil {
		t.Fatal("zrušené povolenie")
	}
	// neaktívny používateľ
	tok4, _ := o.a.ExchangeCode(ctx, clientID, code(false), claudeCallback, verifier)
	u, _ := o.a.Store.GetUser(ctx, o.jana.UserID)
	u.Active = false
	_ = o.a.Store.UpdateUser(ctx, u)
	if _, err := o.a.OAuthActor(ctx, tok4.AccessToken, auth.ChannelMCP, ""); err == nil {
		t.Fatal("neaktívny používateľ")
	}
	if _, err := o.a.RefreshOAuth(ctx, clientID, tok4.RefreshToken); err == nil {
		t.Fatal("neaktívny používateľ obnovuje")
	}
}
