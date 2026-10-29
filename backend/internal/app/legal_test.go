package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/legal"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestLegal(t *testing.T) {
	t.Cleanup(func() { vat.SetPeriods(vat.DefaultPeriods) })
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	admin := auth.WithActor(ctx, auth.Local)

	// nová inštalácia: predvyplnenie a zápis do databázy
	if err := a.LoadLegal(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := a.Store.GetSetting(ctx, legal.SettingKey)
	if err != nil || raw == "" {
		t.Fatalf("predvyplnenie sa neuložilo: %v", err)
	}
	if err := a.LoadLegal(ctx); err != nil { // druhý štart nič nemení
		t.Fatal(err)
	}

	// správca doplní novú sadzbu ECB a zmení prirážku a náhradu
	p := a.Legal(admin)
	p.ECB = append(p.ECB, reminder.Rate{From: invoice.MustDate("2026-10-15"), Rate: m.MustParse("3.15")})
	p.BusinessMargin, p.FlatFee = m.MustParse("8"), m.MustParse("45")
	p.Seeded = nil // klient nevie zmeniť
	saved, err := a.SetLegal(admin, p)
	if err != nil || len(saved.Seeded) == 0 {
		t.Fatalf("uloženie: %v", err)
	}

	// upomienka použije nové hodnoty: omeškanie od 21. 10. → ECB 3,15 + 8
	f := issuedOn(t, a, c.ID, "Dlžník s.r.o.", "31322832", "2026-10-20")
	r, err := a.PreviewReminder(ctx, c.ID, ReminderRequest{InvoiceIDs: []int64{f.ID}, Options: reminder.Options{ChargeFee: true}})
	if err != nil || r.Items[0].Rate.String() != "11.15" || r.Fees.String() != "45.00" {
		t.Fatalf("upomienka: %+v %v", r.Items, err)
	}

	// po reštarte (nová App nad tou istou databázou) platí uložené
	b := New(a.Store, a.Caps)
	if err := b.LoadLegal(ctx); err != nil || b.Law().FlatFee.String() != "45.00" {
		t.Fatalf("po reštarte: %v %s", err, b.Law().FlatFee)
	}

	// nová sadzba DPH platí pre faktúry
	p = a.Legal(admin)
	p.VATPeriods[len(p.VATPeriods)-1].To = "2026-12-31"
	p.VATPeriods = append(p.VATPeriods, vat.Period{From: "2027-01-01", Rates: []vat.Rate{21, 19, 5, 0}})
	if _, err := a.SetLegal(admin, p); err != nil {
		t.Fatal(err)
	}
	if !vat.Rate(21).ValidOn("2027-02-01") || vat.Rate(23).ValidOn("2027-02-01") {
		t.Fatal("sadzby DPH sa neuplatnili")
	}

	// neplatné hodnoty sa neuložia
	bad := a.Legal(admin)
	bad.ConsumerMargin = m.MustParse("-1")
	if _, err := a.SetLegal(admin, bad); !fieldErr(err, "consumerMargin") {
		t.Fatalf("neplatné: %v", err)
	}
	// iba správca
	user := auth.WithActor(ctx, auth.Actor{UserID: 5, Name: "Jana", Channel: auth.ChannelWeb})
	if _, err := a.SetLegal(user, a.Legal(user)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bez práv: %v", err)
	}
	if _, err := a.ResetLegal(user); !errors.Is(err, ErrForbidden) {
		t.Fatalf("obnovenie bez práv: %v", err)
	}

	// obnovenie predvolených
	if p, err := a.ResetLegal(admin); err != nil || p.FlatFee.String() != "40.00" || !vat.Rate(23).ValidOn("2027-02-01") {
		t.Fatalf("obnovenie: %v", err)
	}
	entries, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatSettings})
	var n int
	for _, e := range entries {
		if e.EntityType == "legal" {
			n++
		}
	}
	if n != 4 { // predvyplnenie, 2× úprava, obnovenie
		t.Fatalf("audit legislatívy: %d", n)
	}
}
