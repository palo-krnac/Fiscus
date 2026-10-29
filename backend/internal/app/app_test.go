package app

import (
	"context"
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

var ctx = context.Background()

func newApp(t *testing.T, mode edition.Mode) (*App, store.Company) {
	t.Helper()
	caps, _ := edition.For(mode)
	a := New(memstore.New(), caps)
	a.Today = func() invoice.Date { return invoice.MustDate("2026-10-29") }
	c, err := a.CreateCompany(ctx, store.Company{Name: "Moja firma", ICO: "31322832"})
	if err != nil {
		t.Fatal(err)
	}
	return a, c
}

func validDraft() invoice.Invoice {
	return invoice.Invoice{
		Customer: invoice.Party{Name: "Odberateľ s.r.o."},
		Lines: []invoice.Line{{Description: "Práca", Quantity: m.MustParse("10"),
			Unit: "hod", UnitPrice: m.MustParse("40"), Rate: vat.Rate23}},
	}
}

func TestDesktopAllowsOnlyOneCompany(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	if _, err := a.CreateCompany(ctx, store.Company{Name: "Druhá"}); !errors.Is(err, ErrCompanyLimit) {
		t.Fatalf("druhá firma v desktope: %v", err)
	}
}

func TestServerAllowsManyCompanies(t *testing.T) {
	a, _ := newApp(t, edition.Server)
	for i := 0; i < 3; i++ {
		if _, err := a.CreateCompany(ctx, store.Company{Name: "Ďalšia"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompanyValidation(t *testing.T) {
	a, _ := newApp(t, edition.Server)
	_, err := a.CreateCompany(ctx, store.Company{Name: "X", ICO: "11111111"})
	var ve *invoice.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("zlé IČO malo zlyhať: %v", err)
	}
}

func TestDraftDefaults(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	inv, err := a.SaveDraft(ctx, c.ID, validDraft())
	if err != nil {
		t.Fatal(err)
	}
	if inv.IssueDate.String() != "2026-10-29" || inv.DeliveryDate.String() != "2026-10-29" ||
		inv.DueDate.String() != "2026-11-12" || inv.Status != invoice.StatusDraft {
		t.Fatalf("predvolené hodnoty: %+v", inv)
	}
	if inv.Summary.Total.String() != "492.00" {
		t.Fatalf("spolu %s", inv.Summary.Total)
	}
}

func TestIncompleteDraftCanBeSavedButNotIssued(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	inv, err := a.SaveDraft(ctx, c.ID, invoice.Invoice{})
	if err != nil {
		t.Fatalf("neúplný koncept sa má dať uložiť: %v", err)
	}
	if _, err := a.Issue(ctx, c.ID, inv.ID); err == nil {
		t.Fatal("neúplný koncept sa nesmie vystaviť")
	}
	// nepodarené vystavenie nesmie spotrebovať číslo z radu
	good, _ := a.SaveDraft(ctx, c.ID, validDraft())
	issued, err := a.Issue(ctx, c.ID, good.ID)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Number != "FA20260001" {
		t.Fatalf("v číselnom rade vznikla diera: %s", issued.Number)
	}
}

func TestIssueNumbersSequentially(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	var nums []string
	for i := 0; i < 3; i++ {
		d, _ := a.SaveDraft(ctx, c.ID, validDraft())
		inv, err := a.Issue(ctx, c.ID, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		nums = append(nums, inv.Number)
	}
	want := []string{"FA20260001", "FA20260002", "FA20260003"}
	for i := range want {
		if nums[i] != want[i] {
			t.Fatalf("čísla %v, chcem %v", nums, want)
		}
	}
}

func TestIssuedInvoiceIsLocked(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, _ := a.Issue(ctx, c.ID, d.ID)
	inv.Customer.Name = "Zmenené"
	if _, err := a.SaveDraft(ctx, c.ID, inv); !errors.Is(err, invoice.ErrLocked) {
		t.Fatalf("vystavená faktúra sa nesmie meniť: %v", err)
	}
	stored, _ := a.Store.GetInvoice(ctx, c.ID, inv.ID)
	if stored.Customer.Name != "Odberateľ s.r.o." {
		t.Fatal("vystavená faktúra bola prepísaná")
	}
}

func TestPayAndCancel(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, _ := a.Issue(ctx, c.ID, d.ID)
	paid, err := a.MarkPaid(ctx, c.ID, inv.ID, invoice.Date{})
	if err != nil || paid.Status != invoice.StatusPaid || paid.PaidDate.String() != "2026-10-29" {
		t.Fatalf("úhrada: %v %+v", err, paid)
	}
	if _, err := a.Cancel(ctx, c.ID, inv.ID); !errors.Is(err, invoice.ErrBadTransition) {
		t.Fatal("uhradenú nemožno stornovať")
	}
}

func TestOtherCompanyCannotIssue(t *testing.T) {
	a, c := newApp(t, edition.Server)
	other, _ := a.CreateCompany(ctx, store.Company{Name: "Cudzia"})
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	if _, err := a.Issue(ctx, other.ID, d.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cudzia firma vystavila faktúru: %v", err)
	}
}
