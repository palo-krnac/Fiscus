package journal

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func inv(id int64, kind invoice.Kind, status invoice.Status, issue, due, total string, pays ...invoice.Payment) invoice.Invoice {
	return invoice.Invoice{ID: id, Kind: kind, Status: status, Number: "FA" + issue, IssueDate: invoice.MustDate(issue), DueDate: invoice.MustDate(due),
		Customer: invoice.Party{Name: "Odberateľ"}, Summary: vat.Summary{Total: m.MustParse(total)}, Payments: pays}
}

func pay(date, amount string) invoice.Payment {
	return invoice.Payment{Date: invoice.MustDate(date), Amount: m.MustParse(amount), Source: invoice.PaymentBank}
}

func TestReceivables(t *testing.T) {
	invs := []invoice.Invoice{
		inv(1, invoice.KindInvoice, invoice.StatusIssued, "2026-12-01", "2026-12-15", "492"),                                                                      // celá
		inv(2, invoice.KindInvoice, invoice.StatusPaid, "2026-12-10", "2026-12-24", "100", pay("2027-01-05", "100")),                                              // uhradená až po 31. 12.
		inv(3, invoice.KindInvoice, invoice.StatusPaid, "2026-11-10", "2026-11-24", "100", pay("2026-11-20", "100")),                                              // uhradená
		inv(4, invoice.KindInvoice, invoice.StatusIssued, "2026-12-20", "2027-01-03", "300", pay("2026-12-28", "120")),                                            // čiastočne
		inv(5, invoice.KindInvoice, invoice.StatusIssued, "2027-01-02", "2027-01-16", "50"),                                                                       // vystavená neskôr
		inv(6, invoice.KindInvoice, invoice.StatusDraft, "2026-12-01", "2026-12-15", "10"),                                                                        // koncept
		inv(7, invoice.KindInvoice, invoice.StatusCancelled, "2026-12-01", "2026-12-15", "10"),                                                                    // storno
		inv(8, invoice.KindAdvanceReceipt, invoice.StatusPaid, "2026-12-01", "2026-12-01", "60"),                                                                  // daňový doklad k platbe
		inv(9, invoice.KindCreditNote, invoice.StatusIssued, "2026-12-30", "2027-01-13", "-40"),                                                                   // vrátiť odberateľovi
		{ID: 10, Kind: invoice.KindInvoice, Status: invoice.StatusPaid, IssueDate: invoice.MustDate("2026-06-01"), Summary: vat.Summary{Total: m.MustParse("5")}}, // starý formát úhrady
	}
	got := Receivables(invs, invoice.MustDate("2026-12-31"))
	want := map[int64]string{1: "492.00", 2: "100.00", 4: "180.00", 9: "-40.00"}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for _, it := range got {
		if it.Remaining.String() != want[it.ID] {
			t.Errorf("%d: %s", it.ID, it.Remaining)
		}
	}
	if got[0].DaysLate != 16 || SumEUR(got).String() != "732.00" {
		t.Errorf("omeškanie %d, spolu %s", got[0].DaysLate, SumEUR(got))
	}
	// k inému dňu: faktúra 2 je už uhradená
	if l := Receivables(invs, invoice.MustDate("2027-01-10")); len(l) != 4 {
		t.Errorf("k 10. 1.: %d", len(l))
	}
}

func TestPayables(t *testing.T) {
	p := func(id int64, received, due, total string, paid string) purchase.Purchase {
		x := purchase.Purchase{ID: id, Number: "PF", ReceivedDate: invoice.MustDate(received), DueDate: invoice.MustDate(due),
			Status: purchase.StatusBooked, Summary: purchase.Summary{Total: m.MustParse(total)}}
		if paid != "" {
			x.Status, x.PaidDate = purchase.StatusPaid, invoice.MustDate(paid)
		}
		return x
	}
	purs := []purchase.Purchase{
		p(1, "2026-12-05", "2026-12-19", "123", ""),
		p(2, "2026-12-05", "2026-12-19", "50", "2026-12-18"),
		p(3, "2026-12-20", "2027-01-05", "80", "2027-01-04"),
		p(4, "2027-01-02", "2027-01-16", "99", ""),
	}
	got := Payables(purs, invoice.MustDate("2026-12-31"))
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 || got[0].DaysLate != 12 || SumEUR(got).String() != "203.00" {
		t.Fatalf("%+v", got)
	}
}
