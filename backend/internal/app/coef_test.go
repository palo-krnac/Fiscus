package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

func ptr(n int) *int { return &n }

func TestVATCoefficient(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026, mesačný platiteľ

	issue := func(date string, rate vat.Rate, price string) {
		d := validDraft()
		d.IssueDate, d.DeliveryDate = invoice.MustDate(date), invoice.MustDate(date)
		d.Lines[0].Rate, d.Lines[0].Quantity, d.Lines[0].UnitPrice = rate, m.MustParse("1"), m.MustParse(price)
		if rate == 0 {
			d.Note = "Oslobodené od dane podľa § 38 ods. 3 zákona o DPH (nájom bytu)."
		}
		d, err := a.SaveDraft(ctx, c.ID, d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Issue(ctx, c.ID, d.ID); err != nil {
			t.Fatal(err)
		}
	}
	issue("2026-10-05", 23, "400") // s nárokom na odpočet
	issue("2026-10-06", 0, "200")  // oslobodený nájom bez nároku

	// bez koeficientu predchádzajúceho roka sa spoločný nákup neuloží
	mixed := validPurchase()
	mixed.Mixed = true
	if _, _, err := a.SavePurchase(ctx, c.ID, mixed); !fieldErr(err, "mixed") {
		t.Fatalf("bez koeficientu: %v", err)
	}
	v, err := a.SetVATCoefficient(ctx, c.ID, 2026, CoefYear{Provisional: ptr(80)})
	if err != nil {
		t.Fatal(err)
	}
	if v.Provisional != 80 || v.ProvisionalSource != "zadaný" || v.Computed != 67 || v.Final != 67 || v.LastPeriod != "2026-12" {
		t.Fatalf("koeficient: %+v", v) // 400 / 600 = 0,6667 → 0,67
	}
	p, _, err := a.SavePurchase(ctx, c.ID, mixed)
	if err != nil {
		t.Fatal(err)
	}
	if p.Coef != 80 || p.Deducted.String() != "14.96" { // 18,70 × 0,80
		t.Fatalf("predbežný odpočet: coef %d, odpočet %s", p.Coef, p.Deducted)
	}
	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil {
		t.Fatal(err)
	}
	if ret.Rows["r19"].String() != "14.96" || ret.KV.B2[0].Deducted.String() != "14.96" || ret.Coef != nil {
		t.Fatalf("október: r19 %s, B.2 %+v", ret.Rows["r19"], ret.KV.B2)
	}
	if ret.Turnover.WithRight.String() != "400.00" || ret.Turnover.Exempt.String() != "200.00" {
		t.Fatalf("plnenia: %+v", ret.Turnover)
	}

	// zmena predbežného koeficientu prepočíta doklady v nepodaných obdobiach
	if _, err := a.SetVATCoefficient(ctx, c.ID, 2026, CoefYear{Provisional: ptr(70)}); err != nil {
		t.Fatal(err)
	}
	p, _ = a.Store.GetPurchase(ctx, c.ID, p.ID)
	if p.Coef != 70 || p.Deducted.String() != "13.09" {
		t.Fatalf("prepočet: %d %s", p.Coef, p.Deducted)
	}
	if _, err := a.RecordVATFiling(ctx, c.ID, "2026-10", m.Zero, vatreturn.Options{}, invoice.MustDate("2026-10-29")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetVATCoefficient(ctx, c.ID, 2026, CoefYear{Provisional: ptr(75)}); err != nil {
		t.Fatal(err)
	}
	if p, _ = a.Store.GetPurchase(ctx, c.ID, p.ID); p.Coef != 70 {
		t.Fatalf("podané obdobie sa nemení: %d", p.Coef)
	}

	// december: vysporiadanie ročným koeficientom 67 % (18,70 × 0,67 = 12,53; odpočítané 13,09)
	a.Today = func() invoice.Date { return invoice.MustDate("2027-01-10") }
	ret, _, err = a.VATReturn(ctx, c.ID, "2026-12", m.Zero)
	if err != nil {
		t.Fatal(err)
	}
	if ret.Coef == nil || ret.Coef.Final != 67 || ret.Coef.Provisional != 70 || ret.Coef.Total.String() != "-0.56" || ret.Rows["r19"].String() != "-0.56" {
		t.Fatalf("vysporiadanie: %+v r19 %s", ret.Coef, ret.Rows["r19"])
	}
	// ručný ročný koeficient (napr. po vylúčení predaja majetku)
	if _, err := a.SetVATCoefficient(ctx, c.ID, 2026, CoefYear{Provisional: ptr(75), Final: ptr(72)}); err != nil {
		t.Fatal(err)
	}
	ret, _, _ = a.VATReturn(ctx, c.ID, "2026-12", m.Zero)
	if ret.Coef == nil || ret.Coef.Final != 72 || ret.Rows["r19"].String() != "0.37" { // 13,46 − 13,09
		t.Fatalf("ručný koeficient: %+v r19 %s", ret.Coef, ret.Rows["r19"])
	}
	if ret, _, _ := a.VATReturn(ctx, c.ID, "2026-11", m.Zero); ret.Coef != nil {
		t.Error("vysporiadanie patrí iba do posledného obdobia roka")
	}
	// nasledujúci rok: predbežný koeficient = ročný za 2026
	v, err = a.VATCoefficient(ctx, c.ID, 2027)
	if err != nil || !v.ProvisionalOK || v.Provisional != 72 {
		t.Fatalf("2027: %+v %v", v, err)
	}

	if _, err := a.SetVATCoefficient(ctx, c.ID, 2026, CoefYear{Provisional: ptr(120)}); !fieldErr(err, "provisional") {
		t.Errorf("neplatný koeficient: %v", err)
	}
	a2, c2 := newApp(t, edition.Desktop)
	c2.VATPayer = false
	_ = a2.Store.UpdateCompany(ctx, c2)
	if _, err := a2.VATCoefficient(ctx, c2.ID, 2026); !errors.Is(err, ErrNotVATPayer) {
		t.Errorf("neplatiteľ: %v", err)
	}
}
