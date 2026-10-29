package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestAPIKeys(t *testing.T) {
	o := newOffice(t)
	jana := as(o.jana)

	token, k, err := o.a.CreateAPIKey(jana, "Claude", false)
	if err != nil || !strings.HasPrefix(token, KeyPrefix) || k.Prefix != token[:10] || k.UserID != o.jana.UserID || k.Hash == token {
		t.Fatalf("vytvorenie: %q %+v %v", token, k, err)
	}
	roToken, _, _ := o.a.CreateAPIKey(jana, "Len čítanie", true)
	if _, _, err := o.a.CreateAPIKey(jana, " ", false); err == nil {
		t.Fatal("prázdny názov")
	}

	act, err := o.a.KeyActor(ctx, token, auth.ChannelMCP, "10.0.0.1")
	if err != nil || act.UserID != o.jana.UserID || act.Channel != auth.ChannelMCP || act.KeyID != k.ID || !strings.Contains(act.Name, "Claude") {
		t.Fatalf("actor: %+v %v", act, err)
	}
	// kľúč má práva používateľa
	if err := o.a.Require(as(act), o.alfa.ID, auth.PermInvoicesWrite); err != nil {
		t.Fatal(err)
	}
	if err := o.a.Require(as(act), o.beta.ID, auth.PermInvoicesRead); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cudzia firma: %v", err)
	}
	// kľúč len na čítanie
	ro, _ := o.a.KeyActor(ctx, roToken, auth.ChannelMCP, "")
	if err := o.a.Require(as(ro), o.alfa.ID, auth.PermInvoicesRead); err != nil {
		t.Fatal(err)
	}
	if err := o.a.Require(as(ro), o.alfa.ID, auth.PermInvoicesWrite); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("zápis kľúčom na čítanie: %v", err)
	}
	// kľúč nemôže vytvárať ďalšie kľúče
	if _, _, err := o.a.CreateAPIKey(as(act), "ďalší", false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("kľúč vytvoril kľúč: %v", err)
	}

	for _, bad := range []string{"", "fk_neexistuje", "bez-predpony", token + "x"} {
		if _, err := o.a.KeyActor(ctx, bad, auth.ChannelMCP, ""); !errors.Is(err, ErrBadKey) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	// zrušenie: iba vlastný kľúč
	if err := o.a.RevokeAPIKey(as(o.admin), k.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cudzí kľúč: %v", err)
	}
	if err := o.a.RevokeAPIKey(jana, k.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := o.a.KeyActor(ctx, token, auth.ChannelMCP, ""); !errors.Is(err, ErrBadKey) {
		t.Fatal("zrušený kľúč stále platí")
	}
	keys, _ := o.a.ListAPIKeys(jana)
	if len(keys) != 2 || !keys[1].Revoked {
		t.Fatalf("zoznam: %+v", keys)
	}
	list, _, _ := o.a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatAI})
	if len(list) != 3 {
		t.Fatalf("audit kľúčov: %d", len(list))
	}
}

func TestDesktopKeyOnlyInDesktop(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	token, _, err := a.CreateAPIKey(as(auth.Local), "Claude", false)
	if err != nil {
		t.Fatal(err)
	}
	act, err := a.KeyActor(ctx, token, auth.ChannelMCP, "")
	if err != nil || !act.SuperAdmin || act.UserID != 0 {
		t.Fatalf("desktop: %+v %v", act, err)
	}
	// ten istý záznam v serverovej verzii neplatí (kľúč bez používateľa)
	srv, _ := edition.For(edition.Server)
	a.Caps = srv
	if _, err := a.KeyActor(ctx, token, auth.ChannelMCP, ""); !errors.Is(err, ErrBadKey) {
		t.Fatalf("server: %v", err)
	}
}
