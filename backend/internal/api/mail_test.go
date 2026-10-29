package api

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/mail/mailtest"
)

func TestMailAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	srv := mailtest.Start(t, func(s *mailtest.Server) { s.User, s.Password = "faktury@firma.sk", "tajne" })
	e.app.Mailer = mail.SMTP{RootCAs: srv.RootCAs, Timeout: 5 * time.Second}
	ms := fmt.Sprintf("/api/companies/%d/mail-settings", e.cid)

	var set struct {
		Host        string
		PasswordSet bool `json:"passwordSet"`
		Password    string
		Templates   map[string]struct{ Subject string }
	}
	e.do("GET", ms, nil, 200, &set)
	if set.Host != "" || set.Templates["invoice"].Subject == "" {
		t.Fatalf("prázdne: %+v", set)
	}
	e.do("PUT", ms, `{"host":"","fromEmail":"x"}`, 422, nil)
	body := fmt.Sprintf(`{"host":"%s","port":%d,"security":"starttls","username":"faktury@firma.sk","password":"tajne","fromEmail":"faktury@firma.sk"}`, srv.Addr, srv.Port)
	e.do("PUT", ms, body, 200, &set)
	if !set.PasswordSet || set.Password != "" {
		t.Fatalf("uložené: %+v", set)
	}
	e.do("POST", ms+"/test", `{"to":"ja@firma.sk"}`, 200, nil)
	e.do("POST", ms+"/test", `{"to":"zlá"}`, 422, nil)

	inv := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	var a struct{ ID int64 }
	e.do("POST", inv, `{"customer":{"name":"Odberateľ s.r.o.","email":"odberatel@firma.sk","country":"SK"},"issueDate":"2026-10-01","deliveryDate":"2026-10-01",`+
		`"dueDate":"2026-10-15","lines":[{"description":"Práca","quantity":"10","unit":"hod","unitPrice":"40","vatRate":23}]}`, 201, &a)
	path := fmt.Sprintf("%s/%d/email", inv, a.ID)
	e.do("POST", path, `{"to":"odberatel@firma.sk"}`, 409, nil) // koncept
	e.do("POST", fmt.Sprintf("%s/%d/issue", inv, a.ID), nil, 200, nil)

	var d struct {
		To, Subject, Body, File string
		Configured, Sendable    bool
		History                 []struct{ To string }
	}
	e.do("GET", path, nil, 200, &d)
	if d.To != "odberatel@firma.sk" || !d.Configured || !d.Sendable || !strings.HasSuffix(d.File, ".pdf") {
		t.Fatalf("návrh: %+v", d)
	}
	e.do("POST", path, map[string]string{"to": d.To, "subject": d.Subject, "body": d.Body}, 201, nil)
	e.do("POST", path, `{"to":""}`, 422, nil)

	got := srv.Mails()
	if len(got) != 2 || !got[1].TLS || !strings.Contains(got[1].Data, "application/pdf") || got[1].To[0] != "odberatel@firma.sk" {
		t.Fatalf("server prijal: %d", len(got))
	}

	// zlé heslo: 502 a záznam s chybou v zozname
	e.do("PUT", ms, strings.Replace(body, `"tajne"`, `"zle"`, 1), 200, nil)
	e.do("POST", path, `{"to":"odberatel@firma.sk"}`, 502, nil)
	var list []struct{ DocKind, Error string }
	e.do("GET", fmt.Sprintf("/api/companies/%d/emails?year=%d", e.cid, time.Now().Year()), nil, 200, &list)
	if len(list) != 3 || list[0].Error == "" || list[1].DocKind != "invoice" || list[2].DocKind != "test" {
		t.Fatalf("zoznam: %+v", list)
	}
	e.do("GET", fmt.Sprintf("/api/companies/%d/emails?year=x", e.cid), nil, 400, nil)
}
