package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestDeleteDraftAndCancelCreditNote(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	orig, _ := a.Issue(ctx, c.ID, d.ID) // 492 €

	// koncept dobropisu sa dá zmazať
	cn, err := a.CreateCreditNote(ctx, c.ID, orig.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteInvoice(ctx, c.ID, cn.ID); err != nil {
		t.Fatalf("zmazanie konceptu dobropisu: %v", err)
	}
	if _, err := a.Store.GetInvoice(ctx, c.ID, cn.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("koncept ostal")
	}
	// vystavený doklad sa nemaže
	if err := a.DeleteInvoice(ctx, c.ID, orig.ID); !fieldErr(err, "status") {
		t.Fatalf("vystavená faktúra: %v", err)
	}

	// vystavený dobropis so zápočtom sa stornuje a faktúre sa vráti zostatok
	cn, _ = a.CreateCreditNote(ctx, c.ID, orig.ID)
	cn.Lines[0].Quantity = m.MustParse("-4")
	cn.Correction.Reason = "Zľava"
	cn, _ = a.SaveDraft(ctx, c.ID, cn)
	issued, err := a.Issue(ctx, c.ID, cn.ID)
	if err != nil || issued.Status != invoice.StatusPaid {
		t.Fatalf("dobropis: %v %s", err, issued.Status)
	}
	if o, _ := a.Store.GetInvoice(ctx, c.ID, orig.ID); o.Remaining().String() != "295.20" {
		t.Fatalf("pred stornom: %s", o.Remaining())
	}
	cancelled, err := a.Cancel(ctx, c.ID, issued.ID)
	if err != nil || cancelled.Status != invoice.StatusCancelled {
		t.Fatalf("storno dobropisu: %v %s", err, cancelled.Status)
	}
	o, _ := a.Store.GetInvoice(ctx, c.ID, orig.ID)
	if o.Status != invoice.StatusIssued || o.Remaining().String() != "492.00" || len(o.Payments) != 0 {
		t.Fatalf("faktúra po storne dobropisu: %s %s %+v", o.Status, o.Remaining(), o.Payments)
	}
	if _, err := a.Cancel(ctx, c.ID, issued.ID); err == nil {
		t.Fatal("druhé storno")
	}
	// po storne sa dá vystaviť nový dobropis na celú sumu
	cn3, _ := a.CreateCreditNote(ctx, c.ID, orig.ID)
	cn3.Correction.Reason = "Vrátenie"
	cn3, _ = a.SaveDraft(ctx, c.ID, cn3)
	if _, err := a.Issue(ctx, c.ID, cn3.ID); err != nil {
		t.Fatalf("nový dobropis po storne: %v", err)
	}
}
