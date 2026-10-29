package app

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

func TestRecurringDraftsAndNotice(t *testing.T) {
	a, c, f := mailApp(t) // dnes 29. 10. 2026
	// august a september sa dobehnú ako koncepty, október sa vystaví
	if _, err := a.SaveRecurring(ctx, c.ID, validRecurring("2026-08-05")); err != nil {
		t.Fatal(err)
	}
	manual := validRecurring("2026-10-10")
	manual.AutoIssue = false
	manual.Customer.Name = "Ručná kontrola s.r.o."
	if _, err := a.SaveRecurring(ctx, c.ID, manual); err != nil {
		t.Fatal(err)
	}
	// bez adresy pre upozornenia sa nič neposiela
	if _, err := a.SetMailSettings(ctx, c.ID, smtpOK, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunRecurring(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 0 {
		t.Fatalf("poslané bez adresy: %d", len(f.sent))
	}
	drafts, err := a.RecurringDrafts(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 3 || drafts[0].Period != "august 2026" || !strings.Contains(drafts[0].Reason, "minulého mesiaca") ||
		drafts[2].Customer != "Ručná kontrola s.r.o." || !strings.Contains(drafts[2].Reason, "koncepty") || drafts[2].Total.String() != "123.00" {
		t.Fatalf("koncepty: %+v", drafts)
	}
	// vystavený koncept zo zoznamu zmizne
	if _, err := a.Issue(ctx, c.ID, drafts[0].InvoiceID); err != nil {
		t.Fatal(err)
	}
	if drafts, _ = a.RecurringDrafts(ctx, c.ID); len(drafts) != 2 {
		t.Fatalf("po vystavení: %d", len(drafts))
	}

	// s adresou príde jedno súhrnné upozornenie na nové koncepty z behu
	s := smtpOK
	s.NotifyDrafts = "uctovnik@firma.sk"
	if _, err := a.SetMailSettings(ctx, c.ID, s, false); err != nil {
		t.Fatal(err)
	}
	a.Today = func() invoice.Date { return invoice.MustDate("2026-11-12") }
	if _, err := a.RunRecurring(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("upozornenia: %d", len(f.sent))
	}
	msg := f.last()
	if msg.To[0] != "uctovnik@firma.sk" || msg.Subject != "Fiškus: 1 koncept faktúry čaká na kontrolu" ||
		!strings.Contains(msg.Body, "Ručná kontrola s.r.o. — Servis, november 2026, 123,00 €") {
		t.Fatalf("%+v\n%s", msg, msg.Body)
	}
	all, _ := a.Emails(ctx, c.ID, 2026)
	if all[0].DocKind != MailNotice {
		t.Fatalf("záznam: %+v", all[0])
	}
}

func TestDraftNoticeSubject(t *testing.T) {
	for n, want := range map[int]string{1: "1 koncept faktúry čaká", 3: "3 koncepty faktúr čakajú", 5: "5 konceptov faktúr čaká"} {
		if got := draftNoticeSubject(n); !strings.Contains(got, want) {
			t.Errorf("%d: %s", n, got)
		}
	}
}
