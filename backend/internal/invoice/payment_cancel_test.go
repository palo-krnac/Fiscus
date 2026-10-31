package invoice

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestCancelCreditNote(t *testing.T) {
	mk := func() (Invoice, Invoice) {
		orig := Invoice{ID: 1, Kind: KindInvoice, Status: StatusIssued, Summary: vat.Summary{Total: money.MustParse("100")}}
		cn := Invoice{ID: 2, Kind: KindCreditNote, Status: StatusIssued, Summary: vat.Summary{Total: money.MustParse("-30")},
			Correction: &Correction{OriginalID: 1}}
		return orig, cn
	}
	tests := []struct {
		name       string
		prepare    func(o, c *Invoice)
		err        error
		origStatus Status
		origPaid   string
	}{
		{"bez zápočtu", func(o, c *Invoice) {}, nil, StatusIssued, "0.00"},
		{"so zápočtom", func(o, c *Invoice) {
			if _, err := OffsetCreditNote(o, c); err != nil {
				t.Fatal(err)
			}
		}, nil, StatusIssued, "0.00"},
		{"zápočet vyrovnal faktúru", func(o, c *Invoice) {
			o.Summary.Total = money.MustParse("30")
			if _, err := OffsetCreditNote(o, c); err != nil || o.Status != StatusPaid {
				t.Fatalf("zápočet: %v %s", err, o.Status)
			}
		}, nil, StatusIssued, "0.00"},
		{"vrátený odberateľovi", func(o, c *Invoice) {
			c.Payments = []Payment{{Amount: money.MustParse("-30"), Source: PaymentManual}}
			c.Status = StatusPaid
		}, ErrCreditRefunded, StatusIssued, "0.00"},
		{"koncept", func(o, c *Invoice) { c.Status = StatusDraft }, ErrBadTransition, StatusIssued, "0.00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, c := mk()
			tc.prepare(&o, &c)
			err := CancelCreditNote(&c, &o)
			if !errors.Is(err, tc.err) {
				t.Fatalf("chyba %v, chcem %v", err, tc.err)
			}
			if err != nil {
				return
			}
			if c.Status != StatusCancelled || len(c.Payments) != 0 || o.Status != tc.origStatus || o.Paid().String() != tc.origPaid {
				t.Errorf("dobropis %s %v, faktúra %s %s", c.Status, c.Payments, o.Status, o.Paid())
			}
		})
	}
}

func TestCanDelete(t *testing.T) {
	for _, tc := range []struct {
		inv Invoice
		ok  bool
	}{
		{Invoice{Status: StatusDraft}, true},
		{Invoice{Status: StatusDraft, Number: "DB20260001"}, false},
		{Invoice{Status: StatusIssued, Number: "FA1"}, false},
		{Invoice{Status: StatusCancelled, Number: "FA1"}, false},
	} {
		if err := tc.inv.CanDelete(); (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc.inv, err)
		}
	}
}
