package app

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

func TestMain(m *testing.M) {
	auth.HashIterations = 1000 // rýchle testy
	os.Exit(m.Run())
}

// office pripraví serverovú inštaláciu: správca kancelárie, dve firmy, účtovníčka a klient.
type office struct {
	a                   *App
	admin, jana, klient auth.Actor
	alfa, beta          store.Company
}

func newOffice(t *testing.T) office {
	t.Helper()
	caps, _ := edition.For(edition.Server)
	a := New(memstore.New(), caps)
	a.Today = func() invoice.Date { return invoice.MustDate("2026-10-29") }
	unlock(t, a)
	if ok, err := a.EnsureAdmin(ctx, "sef@kancelaria.sk", "Šéf", "silne-heslo-123"); !ok || err != nil {
		t.Fatal(err)
	}
	sef, _ := a.Store.GetUserByEmail(ctx, "sef@kancelaria.sk")
	o := office{a: a, admin: actorOf(sef, auth.Actor{Channel: auth.ChannelWeb})}
	actx := auth.WithActor(ctx, o.admin)
	o.alfa, _ = a.CreateCompany(actx, store.Company{Name: "Alfa", VATPayer: true, ICDPH: "SK2020372640"})
	o.beta, _ = a.CreateCompany(actx, store.Company{Name: "Beta"})
	j, err := a.AddMember(actx, o.alfa.ID, NewMember{Email: "Jana@Kancelaria.sk", Name: "Jana", Password: "jana-heslo-123", Role: auth.RoleAccountant})
	if err != nil {
		t.Fatal(err)
	}
	k, _ := a.AddMember(actx, o.beta.ID, NewMember{Email: "klient@beta.sk", Name: "Klient", Password: "klient-heslo-1", Role: auth.RoleClient})
	ju, _ := a.Store.GetUser(ctx, j.User.ID)
	ku, _ := a.Store.GetUser(ctx, k.User.ID)
	o.jana = actorOf(ju, auth.Actor{Channel: auth.ChannelWeb})
	o.klient = actorOf(ku, auth.Actor{Channel: auth.ChannelMCP})
	return o
}

func as(a auth.Actor) context.Context { return auth.WithActor(ctx, a) }

func TestLogin(t *testing.T) {
	o := newOffice(t)
	if _, _, err := o.a.Login(ctx, "jana@kancelaria.sk", "zle heslo"); !errors.Is(err, ErrBadLogin) {
		t.Fatal("zlé heslo")
	}
	if _, _, err := o.a.Login(ctx, "nikto@nikde.sk", "x"); !errors.Is(err, ErrBadLogin) {
		t.Fatal("neexistujúci používateľ")
	}
	token, u, err := o.a.Login(ctx, " JANA@kancelaria.sk ", "jana-heslo-123")
	if err != nil || u.Name != "Jana" {
		t.Fatal(err)
	}
	act, err := o.a.SessionActor(ctx, token, auth.ChannelWeb, "10.0.0.5")
	if err != nil || act.UserID != u.ID || act.IP != "10.0.0.5" {
		t.Fatalf("relácia: %+v %v", act, err)
	}
	o.a.Logout(as(act), token)
	if _, err := o.a.SessionActor(ctx, token, auth.ChannelWeb, ""); !errors.Is(err, ErrNoSession) {
		t.Fatal("po odhlásení")
	}
	// prihlásenia sú v audite (mimo firmy)
	list, _, _ := o.a.AuditFor(as(o.admin), 0, audit.Filter{Category: audit.CatAuth})
	actions := map[string]int{}
	for _, e := range list {
		actions[e.Action]++
	}
	if actions["login_failed"] != 2 || actions["login"] != 1 || actions["logout"] != 1 {
		t.Fatalf("audit prihlásení: %v", actions)
	}
	// neaktívny používateľ sa neprihlási
	ju, _ := o.a.Store.GetUser(ctx, o.jana.UserID)
	ju.Active = false
	_ = o.a.Store.UpdateUser(ctx, ju)
	if _, _, err := o.a.Login(ctx, "jana@kancelaria.sk", "jana-heslo-123"); !errors.Is(err, ErrBadLogin) {
		t.Fatal("neaktívny")
	}
}

func TestPermissions(t *testing.T) {
	o := newOffice(t)
	cases := []struct {
		who  auth.Actor
		cid  int64
		perm auth.Perm
		want error
	}{
		{o.admin, o.beta.ID, auth.PermUsersManage, nil},
		{o.jana, o.alfa.ID, auth.PermInvoicesWrite, nil},
		{o.jana, o.alfa.ID, auth.PermUsersManage, ErrForbidden},
		{o.jana, o.beta.ID, auth.PermInvoicesRead, store.ErrNotFound}, // cudzia firma
		{o.klient, o.beta.ID, auth.PermInvoicesRead, nil},
		{o.klient, o.beta.ID, auth.PermInvoicesWrite, ErrForbidden},
	}
	for _, c := range cases {
		if err := o.a.Require(as(c.who), c.cid, c.perm); !errors.Is(err, c.want) && !(err == nil && c.want == nil) {
			t.Errorf("%s %d %s: %v, chcem %v", c.who.Name, c.cid, c.perm, err, c.want)
		}
	}
	vis, _ := o.a.VisibleCompanies(as(o.jana))
	if len(vis) != 1 || vis[0].ID != o.alfa.ID {
		t.Fatalf("Jana vidí iba Alfu: %+v", vis)
	}
	all, _ := o.a.VisibleCompanies(as(o.admin))
	if len(all) != 2 {
		t.Fatal("správca vidí všetky")
	}
}

func TestMembers(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	// existujúci e-mail → iba pridanie roly, heslo netreba
	if _, err := o.a.AddMember(actx, o.beta.ID, NewMember{Email: "jana@kancelaria.sk", Role: auth.RoleInvoicer}); err != nil {
		t.Fatal(err)
	}
	if r, _ := o.a.RoleIn(as(o.jana), o.beta.ID); r != auth.RoleInvoicer {
		t.Fatalf("Jana v Bete: %s", r)
	}
	// nový používateľ potrebuje meno a silné heslo
	_, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: "novy@x.sk", Password: "kratke", Role: auth.RoleViewer})
	var ve *invoice.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) != 2 {
		t.Fatalf("validácia: %v", err)
	}
	// posledný správca: šéf je správca Alfy (zakladateľ)
	sefID := o.admin.UserID
	if err := o.a.SetRole(actx, o.alfa.ID, sefID, auth.RoleViewer); !errors.Is(err, ErrSelfDemote) {
		t.Fatalf("vlastná rola: %v", err)
	}
	if err := o.a.RemoveMember(as(o.jana), o.alfa.ID, sefID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("posledný správca: %v", err)
	}
	if err := o.a.SetRole(actx, o.alfa.ID, o.jana.UserID, auth.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := o.a.RemoveMember(as(o.jana), o.alfa.ID, sefID); err != nil {
		t.Fatalf("druhý správca existuje: %v", err)
	}
	users, _ := o.a.CompanyUsers(actx, o.alfa.ID)
	if len(users) != 1 || users[0].Role != auth.RoleAdmin {
		t.Fatalf("%+v", users)
	}
	// heslo
	if err := o.a.ChangePassword(as(o.jana), "zle", "nove-heslo-1234"); err == nil {
		t.Fatal("zlé súčasné heslo")
	}
	if err := o.a.ChangePassword(as(o.jana), "jana-heslo-123", "nove-heslo-1234"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.a.Login(ctx, "jana@kancelaria.sk", "nove-heslo-1234"); err != nil {
		t.Fatal(err)
	}
}

func TestAuditRecordsChanges(t *testing.T) {
	o := newOffice(t)
	jctx := as(o.jana)
	d, _ := o.a.SaveDraft(jctx, o.alfa.ID, validDraft())
	d.Note = "zmena"
	d, _ = o.a.SaveDraft(jctx, o.alfa.ID, d)
	inv, _ := o.a.Issue(jctx, o.alfa.ID, d.ID)
	_, _ = o.a.MarkPaid(jctx, o.alfa.ID, inv.ID, invoice.Date{})
	_, _, _ = o.a.SavePurchase(as(o.admin), o.alfa.ID, validPurchase())

	list, total, err := o.a.AuditFor(jctx, o.alfa.ID, audit.Filter{Category: audit.CatInvoices})
	if err != nil || total != 4 {
		t.Fatalf("faktúry v audite: %d %v", total, err)
	}
	if list[1].Action != "issue" || list[1].Summary != "Vystavená faktúra FA20260001 pre Odberateľ s.r.o., 492,00 €" || list[1].UserName != "Jana" {
		t.Fatalf("%+v", list[1])
	}
	if list[2].Before == nil || list[2].After == nil {
		t.Fatal("úprava má mať stav pred aj po")
	}
	// Jana nevidí audit Bety ani celej inštalácie; klient audit nevidí vôbec
	if _, _, err := o.a.AuditFor(jctx, o.beta.ID, audit.Filter{}); err == nil {
		t.Fatal("cudzia firma")
	}
	if _, _, err := o.a.AuditFor(jctx, 0, audit.Filter{}); !errors.Is(err, ErrForbidden) {
		t.Fatal("celá inštalácia")
	}
	if _, _, err := o.a.AuditFor(as(o.klient), o.beta.ID, audit.Filter{}); !errors.Is(err, ErrForbidden) {
		t.Fatal("klient")
	}
	n, err := o.a.VerifyAudit(as(o.admin))
	if err != nil || n < 10 {
		t.Fatalf("reťazec %d: %v", n, err)
	}
}
