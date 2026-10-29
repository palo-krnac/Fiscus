package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func validQuote() quote.Quote {
	return quote.Quote{
		Customer: invoice.Party{Name: "Odberateľ s.r.o.", Country: "SK"},
		Lines: []invoice.Line{{Description: "Elektroinštalácia", Quantity: m.MustParse("10"),
			Unit: "hod", UnitPrice: m.MustParse("40"), Rate: vat.Rate23}},
	}
}

func TestQuoteFlow(t *testing.T) {
	a, c := newApp(t, edition.Desktop)

	// koncept: predvolené dátumy, sumy, neúplný sa uloží
	empty, err := a.SaveQuote(ctx, c.ID, quote.Quote{})
	if err != nil || empty.Status != quote.StatusDraft || empty.ValidUntil.String() != "2026-11-28" {
		t.Fatalf("prázdny koncept: %+v %v", empty, err)
	}
	if _, err := a.IssueQuote(ctx, c.ID, empty.ID); !fieldErr(err, "customer.name") || !fieldErr(err, "lines") {
		t.Fatalf("neúplná ponuka sa vystavila: %v", err)
	}
	q, err := a.SaveQuote(ctx, c.ID, validQuote())
	if err != nil || q.Summary.Total.String() != "492.00" || q.NonVATPayer {
		t.Fatalf("koncept: %s %v", q.Summary.Total, err)
	}
	// klient nevie podstrčiť číslo ani stav
	q.Number, q.Status, q.InvoiceID = "X1", quote.StatusInvoiced, 99
	q.Note = "Cena platí pri objednávke do 30 dní."
	if q, err = a.SaveQuote(ctx, c.ID, q); err != nil || q.Number != "" || q.Status != quote.StatusDraft || q.InvoiceID != 0 {
		t.Fatalf("úprava konceptu: %+v %v", q, err)
	}

	// vystavenie: číslo z vlastného radu, nezávislého od faktúr; neúplná ponuka nezhltla číslo
	q, err = a.IssueQuote(ctx, c.ID, q.ID)
	if err != nil || q.Number != "CP20260001" || q.Status != quote.StatusIssued {
		t.Fatalf("vystavenie: %+v %v", q, err)
	}
	if _, err := a.SaveQuote(ctx, c.ID, q); !errors.Is(err, quote.ErrLocked) {
		t.Fatalf("úprava vystavenej: %v", err)
	}
	if _, err := a.IssueQuote(ctx, c.ID, q.ID); !errors.Is(err, quote.ErrBadTransition) {
		t.Fatalf("druhé vystavenie: %v", err)
	}

	// ponuka nie je daňový doklad: nie je v zozname faktúr ani v DPH
	if invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{}); len(invs) != 0 {
		t.Fatalf("ponuka medzi faktúrami: %d", len(invs))
	}

	if q, err = a.RejectQuote(ctx, c.ID, q.ID); err != nil || q.Status != quote.StatusRejected {
		t.Fatalf("odmietnutie: %v", err)
	}
	if _, err := a.InvoiceQuote(ctx, c.ID, q.ID); !errors.Is(err, quote.ErrBadTransition) {
		t.Fatalf("faktúra z odmietnutej: %v", err)
	}
	if q, err = a.AcceptQuote(ctx, c.ID, q.ID); err != nil || q.Status != quote.StatusAccepted {
		t.Fatalf("prijatie: %v", err)
	}

	// faktúra z ponuky: koncept s položkami, dnešným dátumom a splatnosťou firmy
	inv, err := a.InvoiceQuote(ctx, c.ID, q.ID)
	if err != nil || inv.Status != invoice.StatusDraft || inv.ID == 0 {
		t.Fatalf("faktúra z ponuky: %+v %v", inv, err)
	}
	if inv.IssueDate.String() != "2026-10-29" || inv.DueDate.String() != "2026-11-12" || !inv.Summary.Total.Equal(q.Summary.Total) {
		t.Fatalf("faktúra: %s %s %s", inv.IssueDate, inv.DueDate, inv.Summary.Total)
	}
	if inv.Note != "Podľa cenovej ponuky CP20260001.\nCena platí pri objednávke do 30 dní." {
		t.Fatalf("poznámka: %q", inv.Note)
	}
	if _, err := a.InvoiceQuote(ctx, c.ID, q.ID); !errors.Is(err, quote.ErrBadTransition) {
		t.Fatalf("druhá faktúra z tej istej ponuky: %v", err)
	}
	got, _ := a.Quote(ctx, c.ID, q.ID)
	if got.Status != quote.StatusInvoiced || got.InvoiceID != inv.ID || got.InvoiceNumber != "" {
		t.Fatalf("ponuka po fakturácii: %+v", got)
	}
	// po vystavení faktúry ponuka ukazuje jej číslo
	issued, err := a.Issue(ctx, c.ID, inv.ID)
	if err != nil || issued.Number != "FA20260001" {
		t.Fatalf("vystavenie faktúry: %v", err)
	}
	if list, _ := a.Quotes(ctx, c.ID, 2026); len(list) != 2 || list[0].InvoiceNumber+list[1].InvoiceNumber != "FA20260001" {
		t.Fatalf("číslo faktúry v zozname ponúk: %+v", list)
	}

	// kópia: nový koncept, rovnaká dĺžka platnosti
	cp, err := a.CopyQuote(ctx, c.ID, q.ID)
	if err != nil || cp.ID == q.ID || cp.Status != quote.StatusDraft || cp.Number != "" || cp.InvoiceID != 0 ||
		cp.ValidUntil.String() != "2026-11-28" || len(cp.Lines) != 1 {
		t.Fatalf("kópia: %+v %v", cp, err)
	}
	if cp, _ = a.IssueQuote(ctx, c.ID, cp.ID); cp.Number != "CP20260002" {
		t.Fatalf("číslo kópie: %s", cp.Number)
	}

	list, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatQuotes})
	// 3 koncepty (prázdny, ponuka, kópia), úprava, 2 vystavenia, odmietnutie, prijatie, fakturácia
	if len(list) != 9 {
		t.Fatalf("audit ponúk: %d", len(list))
	}
}

func TestQuoteNonVATPayer(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	co, _ := a.Store.GetCompany(ctx, c.ID)
	co.VATPayer, co.ICDPH = false, ""
	if _, err := a.UpdateCompany(ctx, co); err != nil {
		t.Fatal(err)
	}
	q := validQuote()
	q.ReverseCharge = true
	q, err := a.SaveQuote(ctx, c.ID, q)
	if err != nil || !q.NonVATPayer || q.ReverseCharge || q.Summary.Total.String() != "400.00" {
		t.Fatalf("neplatiteľ: %+v %v", q.Summary, err)
	}
}

func TestQuotePermissions(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	member := func(email string, role auth.Role) auth.Actor {
		mb, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: email, Name: email, Password: "heslo-1234567", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
		return actorOf(u, auth.Actor{Channel: auth.ChannelWeb})
	}
	tests := []struct {
		name  string
		actor auth.Actor
		write error
	}{
		{"fakturant", member("f@alfa.sk", auth.RoleInvoicer), nil},
		{"len čítanie", member("v@alfa.sk", auth.RoleViewer), ErrForbidden},
	}
	for _, tt := range tests {
		if _, err := o.a.SaveQuote(as(tt.actor), o.alfa.ID, validQuote()); !errors.Is(err, tt.write) {
			t.Errorf("%s: %v", tt.name, err)
		}
		if _, err := o.a.Quotes(as(tt.actor), o.alfa.ID, 0); err != nil {
			t.Errorf("%s čítanie: %v", tt.name, err)
		}
	}
	// cudzia firma sa tvári ako neexistujúca
	stranger := member("x@alfa.sk", auth.RoleInvoicer)
	if _, err := o.a.Quotes(as(stranger), o.beta.ID, 0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("cudzia firma: %v", err)
	}
}
