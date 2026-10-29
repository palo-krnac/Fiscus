package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestDesktopPINLock(t *testing.T) {
	e := setup(t, edition.Desktop)
	e.do("GET", "/api/companies", nil, 200, nil)
	e.do("PUT", "/api/security/pin", map[string]string{"pin": "12"}, 422, nil)
	e.do("PUT", "/api/security/pin", map[string]string{"pin": "2580"}, 204, nil)
	var caps struct{ PIN bool }
	e.do("GET", "/api/capabilities", nil, 200, &caps)
	if !caps.PIN {
		t.Fatal("capabilities")
	}
	// kto PIN nastavil, ostal odomknutý; iné okno nie
	e.do("GET", "/api/companies", nil, 200, nil)
	other := e.anonymous()
	other.do("GET", "/api/companies", nil, 401, nil)
	other.do("GET", "/api/capabilities", nil, 200, nil) // verejné
	other.do("POST", "/api/auth/pin", map[string]string{"pin": "0000"}, 401, nil)
	other.do("POST", "/api/auth/pin", map[string]string{"pin": "2580"}, 204, nil)
	other.do("GET", "/api/companies", nil, 200, nil)
	other.do("POST", "/api/auth/lock", nil, 204, nil)
	other.do("GET", "/api/companies", nil, 401, nil)
	// limit pokusov
	third := e.anonymous()
	for range loginAttempts {
		third.do("POST", "/api/auth/pin", map[string]string{"pin": "9999"}, 401, nil)
	}
	third.do("POST", "/api/auth/pin", map[string]string{"pin": "2580"}, 429, nil)
	// zrušenie PIN-u
	e.do("PUT", "/api/security/pin", map[string]string{"current": "2580", "pin": ""}, 204, nil)
	other.do("GET", "/api/companies", nil, 200, nil)
}

func TestRequireMFAGate(t *testing.T) {
	admin := setup(t, edition.Server)
	clock := time.Unix(1_800_000_000, 0)
	admin.app.Now = func() time.Time { return clock }
	users := fmt.Sprintf("/api/companies/%d/users", admin.cid)
	admin.do("POST", users, map[string]string{"email": "jana@firma.sk", "name": "Jana", "password": "jana-heslo-123", "role": "accountant"}, 201, nil)

	admin.do("PUT", "/api/security/require-mfa", map[string]bool{"on": true}, 422, nil) // správca bez 2FA
	var s struct{ Secret string }
	admin.do("POST", "/api/me/mfa/setup", map[string]string{"password": adminPass}, 200, &s)
	c, _ := auth.TOTPCode(s.Secret, clock)
	admin.do("POST", "/api/me/mfa/enable", map[string]string{"code": c}, 200, nil)
	admin.do("PUT", "/api/security/require-mfa", map[string]bool{"on": true}, 204, nil)

	j := admin.anonymous()
	j.login("jana@firma.sk", "jana-heslo-123", 200)
	var me struct{ MfaRequired bool }
	j.do("GET", "/api/me", nil, 200, &me)
	if !me.MfaRequired {
		t.Fatal("me.mfaRequired")
	}
	j.do("GET", "/api/companies", nil, 200, nil)
	j.do("GET", fmt.Sprintf("/api/companies/%d/invoices", admin.cid), nil, 403, nil)
	j.do("PUT", "/api/security/require-mfa", map[string]bool{"on": false}, 403, nil)
	var js struct{ Secret string }
	j.do("POST", "/api/me/mfa/setup", map[string]string{"password": "jana-heslo-123"}, 200, &js)
	jc, _ := auth.TOTPCode(js.Secret, clock)
	j.do("POST", "/api/me/mfa/enable", map[string]string{"code": jc}, 200, nil)
	j.do("GET", fmt.Sprintf("/api/companies/%d/invoices", admin.cid), nil, 200, nil)

	var st struct{ RequireMfa bool }
	admin.do("GET", "/api/security", nil, 200, &st)
	if !st.RequireMfa {
		t.Fatal("stav")
	}
	j.do("GET", "/api/security", nil, 403, nil) // iba správca inštalácie
}
