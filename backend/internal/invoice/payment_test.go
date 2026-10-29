package invoice

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func issued(t *testing.T, mod func(*Invoice)) Invoice {
	t.Helper()
	inv := sample() // 8 × 45 + 2,5 × 1,99 = 364,98 + 23 % = 448,93
	if mod != nil {
		mod(&inv)
	}
	if err := inv.Issue("FA1"); err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestPayments(t *testing.T) {
	d := MustDate
	inv := issued(t, nil)
	total := inv.Summary.Total
	if total.String() != "448.93" {
		t.Fatalf("suma: %s", total)
	}
	steps := []struct {
		name      string
		p         Payment
		err       error
		status    Status
		remaining string
	}{
		{"čiastočná", Payment{Date: d("2026-10-10"), Amount: money.MustParse("200"), Source: PaymentBank, Ref: 1}, nil, StatusIssued, "248.93"},
		{"nulová", Payment{Date: d("2026-10-11"), Amount: money.Zero}, ErrBadPayment, StatusIssued, "248.93"},
		{"záporná", Payment{Date: d("2026-10-11"), Amount: money.MustParse("-5")}, ErrBadPayment, StatusIssued, "248.93"},
		{"zvyšok", Payment{Date: d("2026-10-15"), Amount: money.MustParse("248.93"), Source: PaymentCash, Ref: 7}, nil, StatusPaid, "0.00"},
		{"po úhrade", Payment{Date: d("2026-10-16"), Amount: money.MustParse("1")}, ErrBadTransition, StatusPaid, "0.00"},
	}
	for _, s := range steps {
		err := inv.AddPayment(s.p)
		if !errors.Is(err, s.err) || inv.Status != s.status || inv.Remaining().String() != money.MustParse(s.remaining).String() {
			t.Fatalf("%s: %v %s %s", s.name, err, inv.Status, inv.Remaining())
		}
	}
	if inv.PaidDate.String() != "2026-10-15" || inv.PaidAmount != total || len(inv.Payments) != 2 || !inv.Balance.IsZero() || !inv.BalanceEUR.IsZero() {
		t.Fatalf("uhradená: %s %s %d", inv.PaidDate, inv.PaidAmount, len(inv.Payments))
	}
	// zrušené spárovanie pohybu z banky vráti faktúru medzi neuhradené
	if !inv.RemovePayment(PaymentBank, 1) || inv.Status != StatusIssued || !inv.PaidDate.IsZero() || inv.Remaining().String() != "200.00" || !inv.PartlyPaid() {
		t.Fatalf("zrušenie: %s %s", inv.Status, inv.Remaining())
	}
	if inv.RemovePayment(PaymentBank, 1) {
		t.Fatal("druhé zrušenie")
	}
	// ručné označenie doplatí zvyšok
	if err := inv.MarkPaid(d("2026-10-20")); err != nil || inv.Status != StatusPaid || len(inv.Payments) != 2 || inv.Payments[1].Amount.String() != "200.00" {
		t.Fatalf("doplatenie: %v %+v", err, inv.Payments)
	}
	if err := inv.Unpay(); err != nil || inv.Status != StatusIssued || inv.Payments != nil || !inv.PaidAmount.IsZero() {
		t.Fatalf("zrušenie úhrad: %v", err)
	}
	if err := inv.Unpay(); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("zrušenie bez úhrad: %v", err)
	}
}

func TestPaymentsSpecial(t *testing.T) {
	// preplatok: uhradená, zostatok záporný
	inv := issued(t, nil)
	if err := inv.AddPayment(Payment{Date: MustDate("2026-10-10"), Amount: money.MustParse("500")}); err != nil || inv.Status != StatusPaid || inv.Remaining().Sign() >= 0 {
		t.Fatalf("preplatok: %v %s", err, inv.Remaining())
	}
	// dobropis: vrátená suma je záporná
	cn := issued(t, func(i *Invoice) {
		i.Kind = KindCreditNote
		i.Correction = &Correction{OriginalID: 9, OriginalNumber: "FA0", OriginalIssueDate: MustDate("2026-09-01"), OriginalDeliveryDate: MustDate("2026-09-01"), Reason: "vrátenie"}
		for k := range i.Lines {
			i.Lines[k].Quantity = i.Lines[k].Quantity.Neg()
		}
	})
	if err := cn.AddPayment(Payment{Date: MustDate("2026-10-10"), Amount: money.MustParse("100")}); !errors.Is(err, ErrBadPayment) {
		t.Fatalf("dobropis kladná: %v", err)
	}
	if err := cn.AddPayment(Payment{Date: MustDate("2026-10-10"), Amount: money.MustParse("-100")}); err != nil || cn.Status != StatusIssued || !cn.PartlyPaid() {
		t.Fatalf("dobropis čiastočne: %v %s", err, cn.Status)
	}
	if err := cn.MarkPaid(MustDate("2026-10-11")); err != nil || cn.Status != StatusPaid {
		t.Fatalf("dobropis doplatenie: %v", err)
	}
	// záloha aj čiastočne (k platbe vznikne daňový doklad)
	pf := issued(t, func(i *Invoice) { i.Kind = KindProforma })
	if err := pf.AddPayment(Payment{Date: MustDate("2026-10-10"), Amount: money.MustParse("100")}); err != nil || pf.Status != StatusIssued || !pf.PartlyPaid() {
		t.Fatalf("záloha čiastočne: %v", err)
	}
	// jediná úhrada zrušená → neuhradená
	one := issued(t, nil)
	_ = one.AddPayment(Payment{Date: MustDate("2026-10-10"), Amount: one.Summary.Total, Source: PaymentBank, Ref: 5})
	if !one.RemovePayment(PaymentBank, 5) || one.Status != StatusIssued || !one.Remaining().Equal(one.Summary.Total) {
		t.Fatalf("jediná úhrada: %s %s", one.Status, one.Remaining())
	}
	// stará uhradená faktúra bez zoznamu úhrad
	old := issued(t, nil)
	old.Status = StatusPaid
	if !old.Paid().Equal(old.Summary.Total) || !old.Remaining().IsZero() {
		t.Fatal("stará uhradená")
	}
}

func TestBalanceEUR(t *testing.T) {
	inv := issued(t, func(i *Invoice) { i.Currency, i.FXRate = "CZK", money.MustParse("25") })
	_ = inv.AddPayment(Payment{Date: MustDate("2026-10-10"), Amount: money.MustParse("100")})
	if inv.Balance.String() != "348.93" || inv.BalanceEUR.String() != "13.96" {
		t.Fatalf("zostatok: %s %s", inv.Balance, inv.BalanceEUR)
	}
}
