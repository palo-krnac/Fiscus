package quote

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

func TestToken(t *testing.T) {
	a, err := NewToken()
	b, _ := NewToken()
	if err != nil || a == b || !ValidToken(a) {
		t.Fatalf("%q %q %v", a, b, err)
	}
	for _, bad := range []string{"", "abc", strings.Repeat("!", 43), a + "x"} {
		if ValidToken(bad) {
			t.Errorf("platný: %q", bad)
		}
	}
}

func TestRespond(t *testing.T) {
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	today := invoice.MustDate("2026-10-29")
	issued := func() Quote {
		return Quote{Status: StatusIssued, ValidUntil: invoice.MustDate("2026-11-15"), Share: &Share{Token: "t"}}
	}
	tests := []struct {
		name   string
		mod    func(*Quote)
		r      Response
		day    string
		status Status
		err    error
		field  string
	}{
		{"prijatie", nil, Response{Accepted: true, Name: "  Ján   Novák ", Order: "OBJ-15\n"}, "", StatusAccepted, nil, ""},
		{"odmietnutie", nil, Response{Name: "Ján", Note: "Drahé"}, "", StatusRejected, nil, ""},
		{"bez mena", nil, Response{Accepted: true}, "", StatusIssued, nil, "name"},
		{"dlhá poznámka", nil, Response{Name: "Ján", Note: strings.Repeat("x", 1001)}, "", StatusIssued, nil, "note"},
		{"po platnosti", nil, Response{Accepted: true, Name: "Ján"}, "2026-11-16", StatusIssued, ErrExpired, ""},
		{"odmietnuť po platnosti možno", nil, Response{Name: "Ján"}, "2026-11-16", StatusRejected, nil, ""},
		{"druhá odpoveď", func(q *Quote) { q.Share.Response = &Response{} }, Response{Name: "Ján"}, "", StatusIssued, ErrAnswered, ""},
		{"už prijatá dodávateľom", func(q *Quote) { q.Status = StatusAccepted }, Response{Name: "Ján"}, "", StatusAccepted, ErrAnswered, ""},
		{"nezdieľaná", func(q *Quote) { q.Share = nil }, Response{Name: "Ján"}, "", StatusIssued, ErrBadTransition, ""},
	}
	for _, tt := range tests {
		q := issued()
		if tt.mod != nil {
			tt.mod(&q)
		}
		day := today
		if tt.day != "" {
			day = invoice.MustDate(tt.day)
		}
		err := q.Respond(tt.r, day, now)
		var ve *invoice.ValidationError
		switch {
		case tt.field != "":
			if !errors.As(err, &ve) || ve.Fields[0].Field != tt.field {
				t.Errorf("%s: %v", tt.name, err)
			}
		case tt.err != nil:
			if !errors.Is(err, tt.err) {
				t.Errorf("%s: %v", tt.name, err)
			}
		case err != nil:
			t.Errorf("%s: %v", tt.name, err)
		}
		if q.Status != tt.status {
			t.Errorf("%s: stav %s", tt.name, q.Status)
		}
	}
	q := issued()
	_ = q.Respond(Response{Accepted: true, Name: "  Ján   Novák ", Order: "OBJ-15\n"}, today, now)
	if r := q.Share.Response; r.Name != "Ján Novák" || r.Order != "OBJ-15" || !r.At.Equal(now) {
		t.Fatalf("odpoveď: %+v", r)
	}
	q.Number = "CP1"
	if inv := q.ToInvoice(today, 14); !strings.Contains(inv.Note, "objednávky OBJ-15") {
		t.Fatalf("faktúra bez objednávky: %q", inv.Note)
	}
	c := q.Copy(today, 30)
	if c.Share != nil {
		t.Fatal("kópia prevzala odkaz")
	}
}
