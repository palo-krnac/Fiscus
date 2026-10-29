package app

import (
	"errors"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestDesktopPIN(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	clock := time.Unix(1_800_000_000, 0)
	a.Now = func() time.Time { return clock }
	local := auth.WithActor(ctx, auth.Local)
	if a.PINEnabled(ctx) {
		t.Fatal("na začiatku bez PIN-u")
	}
	tests := []struct {
		name, current, pin, field string
	}{
		{"krátky", "", "123", "pin"},
		{"dlhý", "", "123456789012345678901234567890123", "pin"},
		{"nastavenie", "", "2580", ""},
		{"zmena bez súčasného", "", "1111", "current"},
		{"zmena so zlým súčasným", "0000", "1111", "current"},
		{"zmena", "2580", "1470", ""},
	}
	for _, tt := range tests {
		err := a.SetPIN(local, tt.current, tt.pin)
		if tt.field == "" && err != nil || tt.field != "" && !fieldErr(err, tt.field) {
			t.Fatalf("%s: %v", tt.name, err)
		}
	}
	if !a.PINEnabled(ctx) {
		t.Fatal("PIN je nastavený")
	}
	if _, err := a.UnlockPIN(ctx, "2580"); !errors.Is(err, ErrBadPIN) {
		t.Fatal("starý PIN")
	}
	tok, err := a.UnlockPIN(ctx, " 1470 ")
	if err != nil || !a.PINSession(tok) || a.PINSession("iny") || a.PINSession("") {
		t.Fatalf("odomknutie: %v", err)
	}
	// nečinnosť zamkne, používanie predlžuje
	clock = clock.Add(PINIdle - time.Minute)
	if !a.PINSession(tok) {
		t.Fatal("ešte platí")
	}
	clock = clock.Add(PINIdle - time.Minute)
	if !a.PINSession(tok) {
		t.Fatal("použitie predĺžilo platnosť")
	}
	clock = clock.Add(PINIdle + time.Second)
	if a.PINSession(tok) {
		t.Fatal("po nečinnosti zamknuté")
	}
	tok, _ = a.UnlockPIN(ctx, "1470")
	a.LockPIN(tok)
	if a.PINSession(tok) {
		t.Fatal("zamknúť")
	}
	// zmena PIN-u zamkne všetky okná; zrušenie vyžaduje súčasný PIN
	tok, _ = a.UnlockPIN(ctx, "1470")
	if err := a.SetPIN(local, "1470", ""); err != nil || a.PINEnabled(ctx) || a.PINSession(tok) {
		t.Fatalf("zrušenie: %v", err)
	}
	// server PIN nemá
	o := newOffice(t)
	if err := o.a.SetPIN(as(o.admin), "", "1234"); !errors.Is(err, ErrPINDesktopOnly) || o.a.PINEnabled(ctx) {
		t.Fatal("server")
	}
}

func TestRequireMFA(t *testing.T) {
	o := newOffice(t)
	clock := time.Unix(1_800_000_000, 0)
	o.a.Now = func() time.Time { return clock }
	admin := as(o.admin)
	if err := o.a.SetRequireMFA(as(o.jana), true); !errors.Is(err, ErrForbidden) {
		t.Fatal("iba správca inštalácie")
	}
	if err := o.a.SetRequireMFA(admin, true); !fieldErr(err, "requireMfa") {
		t.Fatalf("správca bez 2FA by sa zamkol: %v", err)
	}
	setup, _ := o.a.SetupMFA(admin, "silne-heslo-123")
	code, _ := auth.TOTPCode(setup.Secret, clock)
	if _, err := o.a.EnableMFA(admin, code); err != nil {
		t.Fatal(err)
	}
	if err := o.a.SetRequireMFA(admin, true); err != nil || !o.a.RequireMFA(ctx) {
		t.Fatal(err)
	}
	// Jana bez 2FA: relácia je obmedzená na nastavenie
	token, _, err := o.a.Login(ctx, "jana@kancelaria.sk", "jana-heslo-123")
	if err != nil {
		t.Fatal(err)
	}
	act, _ := o.a.SessionActor(ctx, token, auth.ChannelWeb, "")
	if !act.MFASetup {
		t.Fatal("Jana si musí zapnúť 2FA")
	}
	js, _ := o.a.SetupMFA(as(act), "jana-heslo-123")
	jc, _ := auth.TOTPCode(js.Secret, clock)
	if _, err := o.a.EnableMFA(as(act), jc); err != nil {
		t.Fatal(err)
	}
	if act, _ = o.a.SessionActor(ctx, token, auth.ChannelWeb, ""); act.MFASetup {
		t.Fatal("po zapnutí už bez obmedzenia")
	}
	if err := o.a.SetRequireMFA(admin, false); err != nil || o.a.RequireMFA(ctx) {
		t.Fatal(err)
	}
	// desktop
	d, _ := newApp(t, edition.Desktop)
	if err := d.SetRequireMFA(auth.WithActor(ctx, auth.Local), true); !errors.Is(err, ErrMFAServerOnly) {
		t.Fatal("desktop")
	}
}
