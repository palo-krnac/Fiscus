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

func TestOffsetCreditNote(t *testing.T) {
	// faktúra 448.93; dobropis celý (-448.93) alebo 3 hodiny (-166.05)
	full := func(o Invoice) Invoice {
		cn, _ := NewCreditNote(o, MustDate("2026-10-20"))
		cn.ID, cn.Correction.Reason = 50, "Vrátenie"
		_ = cn.Issue("DB20260001")
		return cn
	}
	part := func(o Invoice) Invoice {
		cn := full(o)
		cn.Lines = cn.Lines[:1]
		cn.Lines[0].Quantity = m.MustParse("-3")
		_ = cn.Recalculate()
		return cn
	}
	pay := func(amount string) func(*Invoice) {
		return func(o *Invoice) {
			_ = o.AddPayment(Payment{Date: MustDate("2026-10-10"), Amount: m.MustParse(amount), Source: PaymentBank, Ref: 1})
		}
	}
	tests := []struct {
		name                 string
		prep                 func(*Invoice)
		cn                   func(Invoice) Invoice
		offset               string
		origStatus, cnStatus Status
		origLeft, cnLeft     string
	}{
		{"celý dobropis k neuhradenej", nil, full, "448.93", StatusPaid, StatusPaid, "0.00", "0.00"},
		{"časť k neuhradenej", nil, part, "166.05", StatusIssued, StatusPaid, "282.88", "0.00"},
		{"celý k čiastočne uhradenej", pay("100"), full, "348.93", StatusPaid, StatusIssued, "0.00", "-100.00"},
		{"časť k čiastočne uhradenej, zostatok menší", pay("400"), part, "48.93", StatusPaid, StatusIssued, "0.00", "-117.12"},
		{"k uhradenej sa nezapočíta", func(o *Invoice) { _ = o.MarkPaid(MustDate("2026-10-10")) }, full, "0.00", StatusPaid, StatusIssued, "0.00", "-448.93"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := issuedSample(t)
			if tt.prep != nil {
				tt.prep(&orig)
			}
			cn := tt.cn(orig)
			got, err := OffsetCreditNote(&orig, &cn)
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != tt.offset || orig.Status != tt.origStatus || cn.Status != tt.cnStatus ||
				orig.Remaining().String() != tt.origLeft || cn.Remaining().String() != tt.cnLeft {
				t.Fatalf("zápočet %s, faktúra %s zostáva %s, dobropis %s zostáva %s", got, orig.Status, orig.Remaining(), cn.Status, cn.Remaining())
			}
			if tt.origStatus == StatusPaid && got.Sign() > 0 && orig.PaidDate != cn.IssueDate && tt.prep == nil {
				t.Errorf("dátum vyrovnania %s", orig.PaidDate)
			}
		})
	}
	// koncept dobropisu ani cudzí dobropis sa nezapočíta
	orig := issuedSample(t)
	draft, _ := NewCreditNote(orig, MustDate("2026-10-20"))
	if _, err := OffsetCreditNote(&orig, &draft); err == nil {
		t.Fatal("koncept")
	}
}

func TestUnpayKeepsCreditOffset(t *testing.T) {
	orig := issuedSample(t)
	_ = orig.AddPayment(Payment{Date: MustDate("2026-10-10"), Amount: m.MustParse("100"), Source: PaymentBank, Ref: 1})
	cn, _ := NewCreditNote(orig, MustDate("2026-10-20"))
	cn.ID, cn.Correction.Reason = 50, "x"
	cn.Lines = cn.Lines[:1]
	cn.Lines[0].Quantity = m.MustParse("-3")
	_ = cn.Recalculate()
	_ = cn.Issue("DB20260001")
	_, _ = OffsetCreditNote(&orig, &cn)
	if err := orig.Unpay(); err != nil {
		t.Fatal(err)
	}
	if len(orig.Payments) != 1 || orig.Payments[0].Source != PaymentCredit || orig.Status != StatusIssued || orig.Remaining().String() != "282.88" {
		t.Fatalf("po zrušení úhrad: %s %v", orig.Remaining(), orig.Payments)
	}
	if err := orig.Unpay(); err == nil {
		t.Fatal("zápočet sa ručne nezruší")
	}
	// faktúra vyrovnaná iba dobropisom
	o2 := issuedSample(t)
	c2, _ := NewCreditNote(o2, MustDate("2026-10-20"))
	c2.ID, c2.Correction.Reason = 51, "x"
	_ = c2.Issue("DB20260002")
	_, _ = OffsetCreditNote(&o2, &c2)
	if err := o2.Unpay(); err == nil || o2.Status != StatusPaid {
		t.Fatal("vyrovnaná dobropisom ostáva vyrovnaná")
	}
}
