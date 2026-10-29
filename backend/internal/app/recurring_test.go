package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func validRecurring(next string) recurring.Template {
	return recurring.Template{
		Name: "Servis", Active: true, Interval: recurring.Monthly, NextDate: invoice.MustDate(next), AutoIssue: true,
		Customer: invoice.Party{Name: "Odberateľ s.r.o.", Country: "SK"},
		Lines: []invoice.Line{{Description: "Servisný paušál {obdobie}", Quantity: m.MustParse("1"),
			Unit: "mes", UnitPrice: m.MustParse("100"), Rate: vat.Rate23}},
	}
}

func TestRecurringRun(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026

	// neúplná šablóna sa neuloží — beží bez dozoru
	if _, err := a.SaveRecurring(ctx, c.ID, recurring.Template{Interval: recurring.Monthly}); !fieldErr(err, "nextDate") || !fieldErr(err, "customer.name") {
		t.Fatalf("neúplná šablóna: %v", err)
	}
	// program nebežal od augusta: august a september sa dobehnú ako koncepty, október sa vystaví
	tp, err := a.SaveRecurring(ctx, c.ID, validRecurring("2026-08-05"))
	if err != nil || tp.NextDate.String() != "2026-08-05" || tp.Summary.Total.String() != "123.00" {
		t.Fatalf("uloženie: %+v %v", tp, err)
	}
	// klient nevie podstrčiť históriu
	tp.History, tp.Generated = []recurring.Run{{InvoiceID: 999}}, 50
	if tp, err = a.SaveRecurring(ctx, c.ID, tp); err != nil || len(tp.History) != 0 || tp.Generated != 0 {
		t.Fatalf("história z klienta: %+v %v", tp.History, err)
	}
	paused := validRecurring("2026-09-01")
	paused.Active = false
	if _, err := a.SaveRecurring(ctx, c.ID, paused); err != nil {
		t.Fatal(err)
	}

	res, err := a.RunRecurring(ctx)
	if err != nil || res.Created != 3 || res.Issued != 1 {
		t.Fatalf("beh: %+v %v", res, err)
	}
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
	byDate := map[string]invoice.Invoice{}
	for _, inv := range invs {
		byDate[inv.IssueDate.String()] = inv
	}
	oct := byDate["2026-10-05"]
	if oct.Status != invoice.StatusIssued || oct.Number != "FA20260001" || oct.Lines[0].Description != "Servisný paušál október 2026" ||
		oct.DueDate.String() != "2026-10-19" {
		t.Fatalf("október: %s %s %q %s", oct.Status, oct.Number, oct.Lines[0].Description, oct.DueDate)
	}
	if aug := byDate["2026-08-05"]; aug.Status != invoice.StatusDraft || aug.Lines[0].Description != "Servisný paušál august 2026" {
		t.Fatalf("august: %s %q", aug.Status, aug.Lines[0].Description)
	}
	got, _ := a.RecurringTemplate(ctx, c.ID, tp.ID)
	if got.Generated != 3 || got.NextDate.String() != "2026-11-05" || len(got.History) != 3 ||
		got.History[0].Number != "FA20260001" || !got.History[0].Issued || !strings.Contains(got.History[2].Error, "Dobehnuté") {
		t.Fatalf("šablóna po behu: %+v", got)
	}
	// druhý beh v ten istý deň nič nevytvorí
	if res, _ := a.RunRecurring(ctx); res.Created != 0 {
		t.Fatalf("opakovaný beh: %+v", res)
	}

	// faktúra hneď: novembrové obdobie s dnešným dátumom
	now, err := a.GenerateRecurringNow(ctx, c.ID, tp.ID)
	if err != nil || now.Status != invoice.StatusIssued || now.IssueDate.String() != "2026-10-29" ||
		now.Lines[0].Description != "Servisný paušál november 2026" {
		t.Fatalf("hneď: %+v %v", now, err)
	}
	if got, _ := a.RecurringTemplate(ctx, c.ID, tp.ID); got.NextDate.String() != "2026-12-05" || !got.History[0].Manual {
		t.Fatalf("po faktúre hneď: %s", got.NextDate)
	}

	// pozastavenie a obnovenie
	if p, err := a.SetRecurringActive(ctx, c.ID, tp.ID, false); err != nil || p.Active {
		t.Fatalf("pozastavenie: %v", err)
	}
	a.Today = func() invoice.Date { return invoice.MustDate("2027-02-10") }
	if res, _ := a.RunRecurring(ctx); res.Created != 0 {
		t.Fatal("pozastavená šablóna fakturuje")
	}
	if r, err := a.SetRecurringActive(ctx, c.ID, tp.ID, true); err != nil || r.NextDate.String() != "2027-03-05" {
		t.Fatalf("obnovenie (december–február sa preskočí): %s %v", r.NextDate, err)
	}

	// zmazanie: faktúry zostanú
	if err := a.DeleteRecurring(ctx, c.ID, tp.ID); err != nil {
		t.Fatal(err)
	}
	if after, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{}); len(after) != 4 {
		t.Fatalf("faktúry po zmazaní šablóny: %d", len(after))
	}
	entries, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatInvoices})
	var sys int
	for _, e := range entries {
		if e.Channel == auth.ChannelSystem {
			sys++
		}
	}
	if sys < 4 { // 3 koncepty + vystavenie z automatického behu
		t.Fatalf("audit systému: %d", sys)
	}
}

func TestRecurringIssueFailureKeepsDraft(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	tp, err := a.SaveRecurring(ctx, c.ID, validRecurring("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	// po uložení zmizne odberateľ z faktúry (napr. zmena sadzieb) — simulujeme cez úložisko
	tp.Customer.Name = ""
	_ = a.Store.SaveRecurring(ctx, c.ID, &tp)
	res, err := a.RunRecurring(ctx)
	if err != nil || res.Created != 1 || res.Issued != 0 {
		t.Fatalf("beh: %+v %v", res, err)
	}
	got, _ := a.RecurringTemplate(ctx, c.ID, tp.ID)
	if !strings.HasPrefix(got.History[0].Error, "Nevystavená: Zadajte odberateľa") || got.NextDate.String() != "2026-11-01" {
		t.Fatalf("chyba vystavenia: %+v", got.History[0])
	}
}

func TestRecurringFinished(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	r := validRecurring("2026-10-01")
	r.EndDate = invoice.MustDate("2026-10-31")
	tp, _ := a.SaveRecurring(ctx, c.ID, r)
	if res, _ := a.RunRecurring(ctx); res.Created != 1 {
		t.Fatalf("beh: %+v", res)
	}
	got, _ := a.RecurringTemplate(ctx, c.ID, tp.ID)
	if got.Active || !got.NextDate.IsZero() {
		t.Fatalf("po poslednej: %+v", got)
	}
	if _, err := a.GenerateRecurringNow(ctx, c.ID, tp.ID); !errors.Is(err, recurring.ErrFinished) {
		t.Fatalf("hneď po skončení: %v", err)
	}
	if _, err := a.SetRecurringActive(ctx, c.ID, tp.ID, true); !errors.Is(err, recurring.ErrFinished) {
		t.Fatalf("obnovenie skončenej: %v", err)
	}
}

func TestRecurringPreviewAndPermissions(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	r := validRecurring("2026-12-01")
	r.Interval = recurring.Quarterly
	r.Note = "Za {minule_obdobie}"
	out, smp, err := o.a.PreviewRecurring(actx, o.alfa.ID, r)
	if err != nil || smp.Period != "4. štvrťrok 2026" || smp.Lines[0] != "Servisný paušál 4. štvrťrok 2026" ||
		smp.Note != "Za 3. štvrťrok 2026" || out.Summary.Total.String() != "123.00" {
		t.Fatalf("ukážka: %+v %+v %v", smp, out.Summary, err)
	}
	mb, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: "v@alfa.sk", Name: "V", Password: "heslo-1234567", Role: auth.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
	viewer := as(actorOf(u, auth.Actor{Channel: auth.ChannelWeb}))
	if _, err := o.a.SaveRecurring(viewer, o.alfa.ID, r); !errors.Is(err, ErrForbidden) {
		t.Fatalf("len čítanie: %v", err)
	}
	if _, err := o.a.Recurring(viewer, o.alfa.ID); err != nil {
		t.Fatalf("čítanie: %v", err)
	}
	if _, err := o.a.Recurring(viewer, o.beta.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cudzia firma: %v", err)
	}
}
