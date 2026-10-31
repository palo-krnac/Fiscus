package app

import (
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestCompanySetupFields(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if c.SetupDone {
		t.Fatal("nová firma má mať úvodné nastavenie pred sebou")
	}
	tests := []struct {
		name string
		book string
		ok   bool
	}{
		{"jednoduché", store.BookkeepingSimple, true},
		{"podvojné", store.BookkeepingDouble, true},
		{"nezvolené", "", true},
		{"neznáme", "danova-evidencia", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := c
			x.Bookkeeping, x.SetupDone = tt.book, true
			x.MigratedFrom, x.MigratedAt = "podvrh", "2020-01-01T00:00:00Z" // klient ich nemení
			got, err := a.UpdateCompany(ctx, x)
			if (err == nil) != tt.ok {
				t.Fatalf("chyba %v", err)
			}
			if tt.ok && (got.Bookkeeping != tt.book || !got.SetupDone || got.MigratedFrom != "" || got.MigratedAt != "") {
				t.Errorf("firma %+v", got)
			}
		})
	}
}

func TestMigrationOnce(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	inv := func(n string) migration {
		mi := migInvoice{Number: n, Ref: n, Summary: validDraft().Lines}
		mi.Inv = validDraft()
		mi.Inv.IssueDate, mi.Inv.DeliveryDate, mi.Inv.DueDate = invoice.MustDate("2026-09-01"), invoice.MustDate("2026-09-01"), invoice.MustDate("2026-09-15")
		return migration{Source: "POHODA", Invoices: []migInvoice{mi}}
	}
	if _, err := a.migrate(ctx, c.ID, inv("FA1"), MigrateOptions{}, true); err != nil {
		t.Fatal(err)
	}
	co, _ := a.Store.GetCompany(ctx, c.ID)
	if co.MigratedFrom != "POHODA" || co.MigratedAt == "" {
		t.Fatalf("prenos sa nezapísal: %+v", co)
	}
	// ten istý program v ten istý deň: doplniť ďalší súbor sa dá
	now = now.Add(3 * time.Hour)
	if _, err := a.migrate(ctx, c.ID, inv("FA2"), MigrateOptions{}, true); err != nil {
		t.Fatalf("doplnenie: %v", err)
	}
	// iný program alebo neskôr: nie (ani náhľad)
	other := inv("FA3")
	other.Source = "Money S3"
	if _, err := a.migrate(ctx, c.ID, other, MigrateOptions{}, false); !fieldErr(err, "file") || !strings.Contains(err.Error(), "") {
		t.Errorf("iný program: %v", err)
	}
	now = now.Add(24 * time.Hour)
	if _, err := a.migrate(ctx, c.ID, inv("FA4"), MigrateOptions{}, false); !fieldErr(err, "file") {
		t.Errorf("neskôr: %v", err)
	}
	// adresár bez dokladov ide vždy
	if _, err := a.migrate(ctx, c.ID, migration{Source: "vCard", Contacts: []migContact{{Party: invoice.Party{Name: "Nový kontakt"}}}}, MigrateOptions{}, true); err != nil {
		t.Errorf("adresár: %v", err)
	}
	// prvý zápis ostane
	co, _ = a.Store.GetCompany(ctx, c.ID)
	if co.MigratedFrom != "POHODA" {
		t.Errorf("prepísaný prenos %+v", co)
	}
}

func TestDeleteCompany(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if _, err := a.SaveDraft(ctx, c.ID, validDraft()); err != nil {
		t.Fatal(err)
	}
	admin := auth.WithActor(ctx, auth.Local) // desktop: správca inštalácie
	if _, err := a.DeleteCompany(admin, c.ID, "Iná firma"); !fieldErr(err, "name") {
		t.Fatalf("zlý názov: %v", err)
	}
	user := auth.WithActor(ctx, auth.Actor{UserID: 5, Name: "Účtovník"})
	if _, err := a.DeleteCompany(user, c.ID, c.Name); err != ErrForbidden {
		t.Fatalf("bez práv správcu: %v", err)
	}
	if _, err := a.DeleteCompany(admin, c.ID, " "+c.Name+" "); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.Store.CountCompanies(ctx); n != 0 {
		t.Fatalf("firiem %d", n)
	}
	// desktop: po zmazaní sa dá založiť nová firma a začína sprievodcom
	nc, err := a.CreateCompany(ctx, store.Company{Name: "Pavol Krnáč – elektroservis"})
	if err != nil || nc.SetupDone {
		t.Fatalf("nová firma %+v %v", nc, err)
	}
}
