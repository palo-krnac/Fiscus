package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// issuedOn vystaví faktúru (492,00 €) s danou splatnosťou pre odberateľa.
func issuedOn(t *testing.T, a *App, cid int64, name, ico, due string) invoice.Invoice {
	t.Helper()
	d := validDraft()
	d.Customer = invoice.Party{Name: name, ICO: ico, Country: "SK"}
	d.IssueDate = invoice.MustDate(due).AddDays(-14)
	d.DeliveryDate = d.IssueDate
	d.DueDate = invoice.MustDate(due)
	dr, err := a.SaveDraft(ctx, cid, d)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := a.Issue(ctx, cid, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestReminders(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	f1 := issuedOn(t, a, c.ID, "Dlžník s.r.o.", "31322832", "2026-09-30")
	f2 := issuedOn(t, a, c.ID, "Dlžník s.r.o.", "31322832", "2026-10-14")
	f3 := issuedOn(t, a, c.ID, "Ján Novák", "", "2026-10-20")
	notDue := issuedOn(t, a, c.ID, "Dlžník s.r.o.", "31322832", "2026-11-10")
	paid := issuedOn(t, a, c.ID, "Dlžník s.r.o.", "31322832", "2026-09-01")
	if _, err := a.MarkPaid(ctx, c.ID, paid.ID, invoice.MustDate("2026-10-01")); err != nil {
		t.Fatal(err)
	}

	groups, err := a.Overdue(ctx, c.ID)
	if err != nil || len(groups) != 2 {
		t.Fatalf("skupiny: %+v %v", groups, err)
	}
	g := groups[0] // väčší dlh prvý
	if g.Customer.Name != "Dlžník s.r.o." || len(g.Invoices) != 2 || g.Invoices[0].ID != f1.ID || g.Total.String() != "984.00" ||
		g.MaxDays != 29 || !g.Business || g.NextLevel != 1 {
		t.Fatalf("dlžník: %+v", g)
	}
	if groups[1].Business || groups[1].Invoices[0].ID != f3.ID {
		t.Fatalf("spotrebiteľ: %+v", groups[1])
	}

	req := ReminderRequest{InvoiceIDs: []int64{f1.ID, f2.ID}, Options: reminder.Options{ChargeFee: true}}
	pv, err := a.PreviewReminder(ctx, c.ID, req)
	if err != nil || pv.Number != "" || pv.PayBy.String() != "2026-11-05" || pv.Interest != reminder.InterestStatutory {
		t.Fatalf("náhľad: %+v %v", pv, err)
	}
	r1, err := a.CreateReminder(ctx, c.ID, req)
	if err != nil || r1.Number != "UP20260001" || r1.Level != 1 || r1.Fees.String() != "80.00" || r1.InterestTotal.Sign() <= 0 {
		t.Fatalf("upomienka: %+v %v", r1, err)
	}
	// druhá upomienka na f2 je o stupeň vyššia
	r2, err := a.CreateReminder(ctx, c.ID, ReminderRequest{InvoiceIDs: []int64{f2.ID, f2.ID}})
	if err != nil || r2.Level != 2 || len(r2.Items) != 1 || r2.Number != "UP20260002" {
		t.Fatalf("2. upomienka: %+v %v", r2, err)
	}
	groups, _ = a.Overdue(ctx, c.ID)
	if groups[0].NextLevel != 3 || groups[0].Invoices[1].Reminded != 2 || groups[0].Invoices[1].LastRemind.String() != "2026-10-29" {
		t.Fatalf("po upomienkach: %+v", groups[0])
	}

	// chyby: rôzni odberatelia, ešte nie po splatnosti, cudzia faktúra
	if _, err := a.CreateReminder(ctx, c.ID, ReminderRequest{InvoiceIDs: []int64{f1.ID, f3.ID}}); !errors.Is(err, reminder.ErrMixedCustomer) {
		t.Fatalf("dvaja odberatelia: %v", err)
	}
	if _, err := a.CreateReminder(ctx, c.ID, ReminderRequest{InvoiceIDs: []int64{notDue.ID}}); err == nil {
		t.Fatal("faktúra pred splatnosťou")
	}
	if _, err := a.CreateReminder(ctx, c.ID, ReminderRequest{InvoiceIDs: []int64{999}}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("neexistujúca faktúra: %v", err)
	}

	list, _ := a.Reminders(ctx, c.ID, 2026)
	if len(list) != 2 {
		t.Fatalf("zoznam: %d", len(list))
	}
	if err := a.DeleteReminder(ctx, c.ID, r2.ID); err != nil {
		t.Fatal(err)
	}
	if groups, _ = a.Overdue(ctx, c.ID); groups[0].NextLevel != 2 {
		t.Fatalf("po zmazaní 2. upomienky: %d", groups[0].NextLevel)
	}
	entries, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatInvoices})
	var reminds int
	for _, e := range entries {
		if e.EntityType == "reminder" {
			reminds++
		}
	}
	if reminds != 3 {
		t.Fatalf("audit upomienok: %d", reminds)
	}
}

func TestReminderPermissions(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	mb, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: "v@alfa.sk", Name: "V", Password: "heslo-1234567", Role: auth.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
	viewer := as(actorOf(u, auth.Actor{Channel: auth.ChannelWeb}))
	if _, err := o.a.Overdue(viewer, o.alfa.ID); err != nil {
		t.Fatalf("čítanie: %v", err)
	}
	if _, err := o.a.CreateReminder(viewer, o.alfa.ID, ReminderRequest{InvoiceIDs: []int64{1}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("zápis: %v", err)
	}
	if _, err := o.a.Overdue(viewer, o.beta.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cudzia firma: %v", err)
	}
}
