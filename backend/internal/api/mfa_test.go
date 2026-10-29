package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestMFAFlow(t *testing.T) {
	admin := setup(t, edition.Server)
	clock := time.Unix(1_800_000_000, 0)
	admin.app.Now = func() time.Time { return clock }
	code := func(secret string) string {
		c, _ := auth.TOTPCode(secret, clock)
		return c
	}
	var st struct {
		Enabled      bool
		RecoveryLeft int
	}
	admin.do("GET", "/api/me/mfa", nil, 200, &st)
	if st.Enabled {
		t.Fatal("na začiatku vypnuté")
	}
	admin.do("POST", "/api/me/mfa/setup", map[string]string{"password": "zle"}, 422, nil)
	var setupOut struct{ Secret, URI string }
	admin.do("POST", "/api/me/mfa/setup", map[string]string{"password": adminPass}, 200, &setupOut)
	admin.do("POST", "/api/me/mfa/enable", map[string]string{"code": "000000"}, 422, nil)
	var en struct{ RecoveryCodes []string }
	admin.do("POST", "/api/me/mfa/enable", map[string]string{"code": code(setupOut.Secret)}, 200, &en)
	if len(en.RecoveryCodes) != auth.RecoveryCount {
		t.Fatalf("záložné kódy %v", en.RecoveryCodes)
	}

	// prihlásenie: heslo → lístok (bez relácie) → kód → relácia
	b := admin.anonymous()
	var step1 struct {
		MFA    bool
		Ticket string
	}
	b.do("POST", "/api/auth/login", map[string]string{"email": adminEmail, "password": adminPass}, 200, &step1)
	if !step1.MFA || step1.Ticket == "" {
		t.Fatalf("%+v", step1)
	}
	b.do("GET", "/api/me", nil, 401, nil)
	b.do("POST", "/api/auth/mfa", map[string]string{"ticket": step1.Ticket, "code": "000000"}, 401, nil)
	clock = clock.Add(30 * time.Second)
	b.do("POST", "/api/auth/mfa", map[string]string{"ticket": step1.Ticket, "code": code(setupOut.Secret)}, 200, nil)
	b.do("GET", "/api/me", nil, 200, nil)

	// záložným kódom
	c := admin.anonymous()
	c.do("POST", "/api/auth/login", map[string]string{"email": adminEmail, "password": adminPass}, 200, &step1)
	c.do("POST", "/api/auth/mfa", map[string]string{"ticket": step1.Ticket, "code": en.RecoveryCodes[2]}, 200, nil)
	c.do("GET", "/api/me/mfa", nil, 200, &st)
	if !st.Enabled || st.RecoveryLeft != auth.RecoveryCount-1 {
		t.Fatalf("%+v", st)
	}

	// nové kódy a vypnutie
	var rc struct{ RecoveryCodes []string }
	c.do("POST", "/api/me/mfa/recovery", map[string]string{"password": adminPass}, 200, &rc)
	c.do("POST", "/api/me/mfa/disable", map[string]string{"password": adminPass, "code": "000000"}, 422, nil)
	c.do("POST", "/api/me/mfa/disable", map[string]string{"password": adminPass, "code": rc.RecoveryCodes[0]}, 204, nil)
	admin.anonymous().login(adminEmail, adminPass, 200)
}

func TestMFAAttemptsLimited(t *testing.T) {
	admin := setup(t, edition.Server)
	e := admin.anonymous()
	for range loginAttempts {
		e.do("POST", "/api/auth/mfa", map[string]string{"ticket": "x", "code": "000000"}, 401, nil)
	}
	e.do("POST", "/api/auth/mfa", map[string]string{"ticket": "x", "code": "000000"}, 429, nil)
}

func TestMFAResetByInstallAdmin(t *testing.T) {
	admin := setup(t, edition.Server)
	users := fmt.Sprintf("/api/companies/%d/users", admin.cid)
	var m struct {
		User struct{ ID int64 }
	}
	admin.do("POST", users, map[string]string{"email": "jana@firma.sk", "name": "Jana", "password": "jana-heslo-123", "role": "accountant"}, 201, &m)
	j := admin.anonymous()
	j.login("jana@firma.sk", "jana-heslo-123", 200)
	var s struct{ Secret string }
	j.do("POST", "/api/me/mfa/setup", map[string]string{"password": "jana-heslo-123"}, 200, &s)
	c, _ := auth.TOTPCode(s.Secret, time.Now())
	j.do("POST", "/api/me/mfa/enable", map[string]string{"code": c}, 200, nil)
	// Jana nemôže zrušiť 2FA sebe ani iným cez reset
	j.do("POST", fmt.Sprintf("/api/users/%d/mfa-reset", m.User.ID), nil, 403, nil)
	admin.do("POST", fmt.Sprintf("/api/users/%d/mfa-reset", m.User.ID), nil, 204, nil)
	// reset zrušil aj jej relácie; prihlási sa iba heslom
	j.do("GET", "/api/me", nil, 401, nil)
	admin.anonymous().login("jana@firma.sk", "jana-heslo-123", 200)
	var list []struct {
		User struct{ TwoFactor bool }
	}
	admin.do("GET", users, nil, 200, &list)
	for _, x := range list {
		if x.User.TwoFactor {
			t.Fatal("po resete")
		}
	}
}
