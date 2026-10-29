package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestCSRFHeaderRequired(t *testing.T) {
	for _, mode := range []edition.Mode{edition.Desktop, edition.Server} {
		e := setup(t, mode)
		bad := *e
		bad.noCSRF = true
		path := fmt.Sprintf("/api/companies/%d/contacts", e.cid)
		bad.do("POST", path, `{"name":"Bez hlavičky"}`, 403, nil)
		// čítanie hlavičku nepotrebuje
		bad.do("GET", path, nil, 200, nil)
	}
}

func TestServerNeedsLogin(t *testing.T) {
	e := setup(t, edition.Server)
	anon := e.anonymous()
	anon.do("GET", "/api/companies", nil, 401, nil)
	anon.do("GET", fmt.Sprintf("/api/companies/%d/invoices", e.cid), nil, 401, nil)
	anon.do("GET", "/api/me", nil, 401, nil)
	// verejné
	anon.do("GET", "/api/capabilities", nil, 200, nil)
	anon.do("GET", "/api/codes", nil, 200, nil)

	anon.login(adminEmail, "zle-heslo-123", 401)
	anon.login(adminEmail, adminPass, 200)
	anon.do("GET", "/api/companies", nil, 200, nil)
	anon.do("POST", "/api/auth/logout", nil, 204, nil)
	anon.do("GET", "/api/companies", nil, 401, nil)
}

func TestDesktopIsLocalUser(t *testing.T) {
	e := setup(t, edition.Desktop)
	var me struct {
		User struct {
			SuperAdmin bool `json:"superAdmin"`
		} `json:"user"`
		AuthRequired bool `json:"authRequired"`
		Access       []struct {
			Role string `json:"role"`
		} `json:"access"`
	}
	e.do("GET", "/api/me", nil, 200, &me)
	if me.AuthRequired || !me.User.SuperAdmin || len(me.Access) != 1 || me.Access[0].Role != "admin" {
		t.Fatalf("desktop /api/me: %+v", me)
	}
	e.login(adminEmail, adminPass, 400)
}

func TestLoginRateLimit(t *testing.T) {
	e := setup(t, edition.Server).anonymous()
	for i := 0; i < loginAttempts; i++ {
		e.login(adminEmail, "zle-heslo-123", 401)
	}
	// aj so správnym heslom je teraz zablokované
	e.login(adminEmail, adminPass, 429)
}

func TestRolesInAPI(t *testing.T) {
	admin := setup(t, edition.Server)
	users := fmt.Sprintf("/api/companies/%d/users", admin.cid)
	admin.do("POST", users, map[string]string{
		"email": "fakturant@firma.sk", "name": "Fero Fakturant", "password": "fakturant-heslo", "role": "invoicer",
	}, 201, nil)
	// druhá firma, kam fakturant nemá prístup
	var other struct{ ID int64 }
	admin.do("POST", "/api/companies", `{"name":"Cudzia firma"}`, 201, &other)

	f := admin.anonymous()
	f.login("fakturant@firma.sk", "fakturant-heslo", 200)

	inv := fmt.Sprintf("/api/companies/%d/invoices", admin.cid)
	f.do("POST", inv, draftJSON, 201, nil)
	f.do("GET", fmt.Sprintf("/api/companies/%d/purchases", admin.cid), nil, 403, nil)
	f.do("GET", users, nil, 403, nil)
	f.do("GET", fmt.Sprintf("/api/companies/%d/audit", admin.cid), nil, 403, nil)
	f.do("PUT", fmt.Sprintf("/api/companies/%d", admin.cid), `{"name":"X"}`, 403, nil)
	f.do("GET", "/api/audit", nil, 403, nil)
	f.do("POST", "/api/companies", `{"name":"Moja"}`, 403, nil)
	// cudzia firma sa tvári ako neexistujúca
	f.do("GET", fmt.Sprintf("/api/companies/%d/invoices", other.ID), nil, 404, nil)

	var list []struct{ ID int64 }
	f.do("GET", "/api/companies", nil, 200, &list)
	if len(list) != 1 || list[0].ID != admin.cid {
		t.Fatalf("fakturant vidí firmy %+v", list)
	}

	// audit firmy podľa kategórie
	var page struct {
		Entries []struct {
			Category string `json:"category"`
			UserName string `json:"userName"`
			Summary  string `json:"summary"`
		} `json:"entries"`
		Total int `json:"total"`
	}
	admin.do("GET", fmt.Sprintf("/api/companies/%d/audit?category=invoices", admin.cid), nil, 200, &page)
	if page.Total != 1 || page.Entries[0].UserName != "Fero Fakturant" {
		t.Fatalf("audit faktúr: %+v", page)
	}
	admin.do("GET", fmt.Sprintf("/api/companies/%d/audit?category=users", admin.cid), nil, 200, &page)
	if page.Total < 2 {
		t.Fatalf("audit používateľov: %+v", page)
	}
	admin.do("GET", "/api/audit?category=auth", nil, 200, &page)
	if page.Total < 2 {
		t.Fatalf("audit prihlásení: %+v", page)
	}
	admin.do("GET", "/api/audit?from=nezmysel", nil, 400, nil)
	// rozdelenie podľa firiem: company=0 sú iba záznamy mimo firmy
	admin.do("GET", "/api/audit?company=0", nil, 200, &page)
	for _, x := range page.Entries {
		if x.Category != "auth" && x.Category != "users" {
			t.Errorf("mimo firmy je %+v", x)
		}
	}
	all := page.Total
	admin.do("GET", fmt.Sprintf("/api/audit?company=%d", other.ID), nil, 200, &page)
	if page.Total != 1 || all == 0 {
		t.Fatalf("audit cudzej firmy: %+v", page)
	}

	var v struct {
		OK      bool `json:"ok"`
		Entries int  `json:"entries"`
	}
	admin.do("GET", "/api/audit/verify", nil, 200, &v)
	if !v.OK || v.Entries == 0 {
		t.Fatalf("overenie auditu: %+v", v)
	}

	// po zmene roly na „len čítanie“ už fakturant nesmie vytvárať doklady
	var members []struct {
		User struct{ ID int64 } `json:"user"`
		Role string             `json:"role"`
	}
	admin.do("GET", users, nil, 200, &members)
	for _, m := range members {
		if m.Role == "invoicer" {
			admin.do("PUT", fmt.Sprintf("%s/%d", users, m.User.ID), `{"role":"viewer"}`, 204, nil)
		}
	}
	f.do("POST", inv, draftJSON, 403, nil)
}
