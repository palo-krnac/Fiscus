package invoice

import (
	"errors"
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func issuedSample(t *testing.T) Invoice {
	t.Helper()
	inv := sample()
	inv.ID = 7
	if err := inv.Issue("FA20260007"); err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestNewCreditNote(t *testing.T) {
	orig := issuedSample(t)
	orig.Subject = "Dodávka svietidiel"
	cn, err := NewCreditNote(orig, MustDate("2026-10-29"))
	if err != nil {
		t.Fatal(err)
	}
	if cn.Kind != KindCreditNote || cn.Status != StatusDraft || cn.Correction.OriginalNumber != "FA20260007" {
		t.Fatalf("%+v", cn)
	}
	if cn.Lines[0].Quantity.String() != "-8.00" || cn.Summary.Total.String() != "-448.93" {
		t.Fatalf("položky %s, spolu %s", cn.Lines[0].Quantity, cn.Summary.Total)
	}
	if cn.Subject != orig.Subject {
		t.Fatalf("predmet sa má prevziať: %q", cn.Subject)
	}
	if cn.Customer != orig.Customer {
		t.Fatal("odberateľ sa má prevziať")
	}
	// bez dôvodu sa nevystaví
	if !fields(cn.Validate())["correction.reason"] {
		t.Fatal("chýbajúci dôvod opravy")
	}
	cn.Correction.Reason = "Vrátenie tovaru"
	if err := cn.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCreditNoteRules(t *testing.T) {
	orig := issuedSample(t)
	draft := sample()
	if _, err := NewCreditNote(draft, MustDate("2026-10-29")); !errors.Is(err, ErrNotCreditable) {
		t.Fatal("ku konceptu nemožno dobropis")
	}
	cancelled := orig
	cancelled.Status = StatusCancelled
	if _, err := NewCreditNote(cancelled, MustDate("2026-10-29")); err == nil {
		t.Fatal("k stornovanej faktúre nemožno dobropis")
	}
	cn, _ := NewCreditNote(orig, MustDate("2026-10-29"))
	cn.Correction.Reason = "Zľava"
	if _, err := NewCreditNote(cn, MustDate("2026-10-29")); err == nil {
		t.Fatal("k dobropisu nemožno ďalší dobropis")
	}
	// kladné množstvá v dobropise sú chyba
	cn.Lines[0].Quantity = m.MustParse("8")
	cn.Lines[1].Quantity = m.MustParse("2.5")
	_ = cn.Recalculate()
	if !fields(cn.Validate())["lines"] {
		t.Fatal("dobropis musí znižovať sumu")
	}
	// bežná faktúra nesmie mať údaje o oprave
	bad := sample()
	bad.Correction = &Correction{OriginalID: 1}
	if !fields(bad.Validate())["correction"] {
		t.Fatal("correction pri faktúre")
	}
	// dobropis po splatnosti nie je pohľadávka
	cn.Status = StatusIssued
	if cn.Overdue(MustDate("2030-01-01")) {
		t.Fatal("dobropis nemá byť po splatnosti")
	}
}

func TestCreditNoteUsesOriginalVATRate(t *testing.T) {
	orig := sample()
	orig.ID = 3
	orig.IssueDate, orig.DeliveryDate, orig.DueDate = MustDate("2024-12-20"), MustDate("2024-12-20"), MustDate("2025-01-03")
	orig.Lines = []Line{{Description: "Práca", Quantity: m.MustParse("1"), UnitPrice: m.MustParse("100"), Rate: vat.Rate20}}
	if err := orig.Issue("FA20240099"); err != nil {
		t.Fatal(err)
	}
	cn, _ := NewCreditNote(orig, MustDate("2025-02-10")) // dobropis už v roku 2025
	cn.Correction.Reason = "Reklamácia"
	if err := cn.Validate(); err != nil {
		t.Fatalf("dobropis k plneniu z 2024 má mať 20 %%: %v", err)
	}
	if cn.Summary.VAT.String() != "-20.00" {
		t.Fatalf("DPH %s", cn.Summary.VAT)
	}
}

func TestCreditLimit(t *testing.T) {
	orig := issuedSample(t) // 448.93
	half := func(id int64, total string, st Status) Invoice {
		return Invoice{ID: id, Status: st, Summary: vat.Summary{Total: m.MustParse(total)}}
	}
	issued := []Invoice{half(10, "-300.00", StatusIssued), half(11, "-500.00", StatusCancelled)}
	ok := half(0, "-148.93", StatusDraft)
	if err := CheckCreditLimit(orig, issued, ok); err != nil {
		t.Fatalf("presne do limitu: %v", err)
	}
	over := half(0, "-148.94", StatusDraft)
	if !fields(CheckCreditLimit(orig, issued, over))["lines"] {
		t.Fatal("prekročenie sumy faktúry")
	}
	// úprava už existujúceho dobropisu sa nezapočíta dvakrát
	if err := CheckCreditLimit(orig, issued, half(10, "-448.93", StatusDraft)); err != nil {
		t.Fatalf("úprava vlastného dobropisu: %v", err)
	}
}
