package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
)

func TestMFA(t *testing.T) {
	o := newOffice(t)
	clock := time.Unix(1_800_000_000, 0)
	o.a.Now = func() time.Time { return clock }
	jana := as(o.jana)
	code := func(secret string) string {
		c, _ := auth.TOTPCode(secret, clock)
		return c
	}

	// nastavenie: heslo, potom kód; bez kódu ešte nie je zapnuté
	if _, err := o.a.SetupMFA(jana, "zle"); err == nil {
		t.Fatal("nastavenie bez hesla")
	}
	setup, err := o.a.SetupMFA(jana, "jana-heslo-123")
	if err != nil || setup.Secret == "" || setup.URI == "" {
		t.Fatal(err)
	}
	if _, _, err := o.a.Login(ctx, "jana@kancelaria.sk", "jana-heslo-123"); err != nil {
		t.Fatalf("pred potvrdením kódu sa prihlasuje iba heslom: %v", err)
	}
	if _, err := o.a.EnableMFA(jana, "000000"); err == nil {
		t.Fatal("zlý prvý kód")
	}
	codes, err := o.a.EnableMFA(jana, code(setup.Secret))
	if err != nil || len(codes) != auth.RecoveryCount {
		t.Fatal(err)
	}
	if st, _ := o.a.MyMFA(jana); !st.Enabled || st.RecoveryLeft != auth.RecoveryCount {
		t.Fatalf("stav %+v", st)
	}
	if _, err := o.a.SetupMFA(jana, "jana-heslo-123"); err == nil {
		t.Fatal("zapnuté 2FA sa neprepíše novým kľúčom")
	}

	login := func() string {
		t.Helper()
		_, _, err := o.a.Login(ctx, "jana@kancelaria.sk", "jana-heslo-123")
		var need *MFARequired
		if !errors.As(err, &need) || need.Ticket == "" {
			t.Fatalf("heslo nestačí: %v", err)
		}
		return need.Ticket
	}

	// kód z aplikácie; ten istý kód druhýkrát neplatí
	clock = clock.Add(30 * time.Second)
	ticket := login()
	if _, _, err := o.a.LoginMFA(ctx, ticket, "123456"); !errors.Is(err, ErrMFACode) {
		t.Fatalf("zlý kód: %v", err)
	}
	c := code(setup.Secret)
	token, u, err := o.a.LoginMFA(ctx, ticket, c)
	if err != nil || u.ID != o.jana.UserID {
		t.Fatal(err)
	}
	if act, err := o.a.SessionActor(ctx, token, auth.ChannelWeb, ""); err != nil || act.UserID != o.jana.UserID {
		t.Fatal("relácia po 2FA")
	}
	if _, _, err := o.a.LoginMFA(ctx, ticket, c); !errors.Is(err, ErrMFATicket) {
		t.Fatal("lístok platí iba raz")
	}
	if _, _, err := o.a.LoginMFA(ctx, login(), c); !errors.Is(err, ErrMFACode) {
		t.Fatal("ten istý kód druhýkrát")
	}

	// záložný kód platí raz
	ticket = login()
	if _, _, err := o.a.LoginMFA(ctx, ticket, codes[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.a.LoginMFA(ctx, login(), codes[0]); !errors.Is(err, ErrMFACode) {
		t.Fatal("použitý záložný kód")
	}
	if st, _ := o.a.MyMFA(jana); st.RecoveryLeft != auth.RecoveryCount-1 {
		t.Fatalf("zostáva %d", st.RecoveryLeft)
	}

	// priveľa pokusov a vypršaný lístok
	ticket = login()
	for range mfaTicketTries {
		_, _, _ = o.a.LoginMFA(ctx, ticket, "000000")
	}
	clock = clock.Add(30 * time.Second)
	if _, _, err := o.a.LoginMFA(ctx, ticket, code(setup.Secret)); !errors.Is(err, ErrMFATicket) {
		t.Fatal("po 5 pokusoch lístok neplatí")
	}
	ticket = login()
	clock = clock.Add(mfaTicketTTL + time.Second)
	if _, _, err := o.a.LoginMFA(ctx, ticket, code(setup.Secret)); !errors.Is(err, ErrMFATicket) {
		t.Fatal("vypršaný lístok")
	}

	// nové záložné kódy: staré prestanú platiť
	fresh, err := o.a.NewRecoveryCodes(jana, "jana-heslo-123")
	if err != nil || len(fresh) != auth.RecoveryCount {
		t.Fatal(err)
	}
	if _, _, err := o.a.LoginMFA(ctx, login(), codes[1]); !errors.Is(err, ErrMFACode) {
		t.Fatal("starý záložný kód")
	}

	// vypnutie: heslo aj kód
	clock = clock.Add(30 * time.Second)
	if err := o.a.DisableMFA(jana, "jana-heslo-123", "000000"); err == nil {
		t.Fatal("vypnutie bez kódu")
	}
	if err := o.a.DisableMFA(jana, "jana-heslo-123", fresh[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.a.Login(ctx, "jana@kancelaria.sk", "jana-heslo-123"); err != nil {
		t.Fatal("po vypnutí stačí heslo")
	}

	// správca inštalácie zruší 2FA (stratený telefón); iný používateľ nie
	setup, _ = o.a.SetupMFA(jana, "jana-heslo-123")
	clock = clock.Add(30 * time.Second)
	_, _ = o.a.EnableMFA(jana, code(setup.Secret))
	if err := o.a.ResetMFA(as(o.klient), o.jana.UserID); !errors.Is(err, ErrForbidden) {
		t.Fatal("reset smie iba správca inštalácie")
	}
	if err := o.a.ResetMFA(as(o.admin), o.jana.UserID); err != nil {
		t.Fatal(err)
	}
	if st, _ := o.a.MyMFA(jana); st.Enabled {
		t.Fatal("po resete")
	}

	// audit bez tajomstiev
	list, _, _ := o.a.AuditFor(as(o.admin), 0, audit.Filter{Category: audit.CatAuth})
	acts := map[string]int{}
	for _, e := range list {
		acts[e.Action]++
		for _, s := range append(append([]string{setup.Secret}, codes...), fresh...) {
			if strings.Contains(e.Summary, s) || strings.Contains(string(e.Before), s) || strings.Contains(string(e.After), s) {
				t.Fatalf("tajomstvo v audite: %s", e.Summary)
			}
		}
	}
	for _, a := range []string{"mfa_on", "mfa_off", "mfa_reset", "mfa_recovery_used", "mfa_recovery_new"} {
		if acts[a] == 0 {
			t.Errorf("audit bez %s: %v", a, acts)
		}
	}
}
