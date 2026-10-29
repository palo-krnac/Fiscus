package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// fakeMailer zapamätá odoslané správy; Fail vráti chybu servera.
type fakeMailer struct {
	mu   sync.Mutex
	sent []mail.Message
	Fail error
}

func (f *fakeMailer) Send(_ context.Context, s mail.Settings, m mail.Message) error {
	if f.Fail != nil {
		return f.Fail
	}
	if _, err := m.Bytes(time.Now()); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMailer) last() mail.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[len(f.sent)-1]
}

var smtpOK = mail.Settings{Host: "smtp.firma.sk", Username: "faktury@firma.sk", Password: "tajne-heslo", FromEmail: "faktury@firma.sk", BccSelf: true}

func mailApp(t *testing.T) (*App, store.Company, *fakeMailer) {
	t.Helper()
	a, c := newApp(t, edition.Desktop)
	f := &fakeMailer{}
	a.Mailer = f
	c.IBAN = "SK3112000000198742637541"
	if err := a.Store.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	return a, c, f
}

func TestMailSettings(t *testing.T) {
	a, c, _ := mailApp(t)
	s, err := a.MailSettings(ctx, c.ID)
	if err != nil || s.Configured() || s.Templates[mail.KindInvoice].Subject == "" {
		t.Fatalf("prázdne nastavenie: %+v %v", s, err)
	}
	if _, err := a.SetMailSettings(ctx, c.ID, mail.Settings{FromEmail: "x"}, false); err == nil {
		t.Fatal("neplatné nastavenie sa uložilo")
	}
	saved, err := a.SetMailSettings(ctx, c.ID, smtpOK, false)
	if err != nil || saved.Password != "" || !saved.PasswordSet || saved.Port != 587 {
		t.Fatalf("uloženie: %+v %v", saved, err)
	}
	// prázdne heslo z formulára ponechá uložené
	saved.FromName = "Moja firma – faktúry"
	if _, err := a.SetMailSettings(ctx, c.ID, saved, false); err != nil {
		t.Fatal(err)
	}
	if raw, _ := a.loadMail(ctx, c.ID); raw.Password != "tajne-heslo" || raw.FromName != "Moja firma – faktúry" {
		t.Fatalf("heslo sa stratilo: %+v", raw)
	}
	if s, _ := a.SetMailSettings(ctx, c.ID, saved, true); s.PasswordSet {
		t.Fatal("heslo sa nezmazalo")
	}
	// heslo nie je v audite
	entries, _, _ := a.Store.ListAudit(ctx, audit.Filter{CompanyIDs: []int64{c.ID}})
	for _, e := range entries {
		if strings.Contains(string(e.Before)+string(e.After), "tajne-heslo") {
			t.Fatal("heslo v audite")
		}
	}
}

func TestSendInvoiceMail(t *testing.T) {
	a, c, f := mailApp(t)
	d := validDraft()
	d.Customer.Email = "odberatel@firma.sk"
	dr, err := a.SaveDraft(ctx, c.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	// koncept sa neposiela
	if md, _ := a.MailDraft(ctx, c.ID, MailInvoice, dr.ID); md.Sendable || md.Configured {
		t.Fatalf("koncept: %+v", md)
	}
	if _, err := a.SendMail(ctx, c.ID, MailInvoice, dr.ID, MailRequest{To: "odberatel@firma.sk"}); !errors.Is(err, ErrNotSendable) {
		t.Fatalf("koncept: %v", err)
	}
	inv, err := a.Issue(ctx, c.ID, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SendMail(ctx, c.ID, MailInvoice, inv.ID, MailRequest{To: "odberatel@firma.sk"}); !errors.Is(err, ErrMailNotConfigured) {
		t.Fatalf("bez nastavenia: %v", err)
	}
	if _, err := a.SetMailSettings(ctx, c.ID, smtpOK, false); err != nil {
		t.Fatal(err)
	}
	md, err := a.MailDraft(ctx, c.ID, MailInvoice, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if md.To != "odberatel@firma.sk" || md.Subject != "Faktúra "+inv.Number+" – Moja firma" || !md.Sendable ||
		!strings.Contains(md.Body, "faktúru "+inv.Number+" na sumu 492,00 €") ||
		!strings.Contains(md.Body, "na účet SK3112000000198742637541, variabilný symbol "+inv.VariableSymbol) ||
		md.File != "Faktura-"+inv.Number+".pdf" {
		t.Fatalf("návrh: %+v", md)
	}

	tests := []struct {
		name string
		req  MailRequest
		err  error
	}{
		{"bez príjemcu", MailRequest{To: " "}, ErrBadRecipient},
		{"zlá adresa", MailRequest{To: "odberatel"}, ErrBadRecipient},
		{"zlá kópia", MailRequest{To: "odberatel@firma.sk", Cc: "x"}, ErrBadRecipient},
		{"odoslanie", MailRequest{To: "odberatel@firma.sk; uctaren@firma.sk", Cc: "sef@firma.sk", Subject: md.Subject + "\r\nBcc: zly@x.sk", Body: md.Body}, nil},
	}
	for _, tt := range tests {
		if _, err := a.SendMail(ctx, c.ID, MailInvoice, inv.ID, tt.req); !errors.Is(err, tt.err) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
	m := f.last()
	if len(m.To) != 2 || m.Cc[0] != "sef@firma.sk" || m.Bcc[0] != "faktury@firma.sk" || m.From != "faktury@firma.sk" ||
		m.FromName != "Moja firma" || strings.ContainsAny(m.Subject, "\r\n") || len(m.Attachments) != 1 ||
		!bytes.HasPrefix(m.Attachments[0].Data, []byte("%PDF-")) {
		t.Fatalf("správa: %+v", m)
	}

	// chyba servera sa zapíše do histórie
	f.Fail = errors.New("535 zlé heslo")
	if _, err := a.SendMail(ctx, c.ID, MailInvoice, inv.ID, MailRequest{To: "odberatel@firma.sk"}); err == nil {
		t.Fatal("chyba servera")
	}
	md, _ = a.MailDraft(ctx, c.ID, MailInvoice, inv.ID)
	if len(md.History) != 2 || md.History[0].Error == "" || md.History[1].Error != "" || md.History[1].To != "odberatel@firma.sk, uctaren@firma.sk" {
		t.Fatalf("história: %+v", md.History)
	}
	all, _ := a.Emails(ctx, c.ID, 0)
	if len(all) != 2 {
		t.Fatalf("zoznam: %d", len(all))
	}
}

func TestMailQuoteReminderAndContact(t *testing.T) {
	a, c, f := mailApp(t)
	if _, err := a.SetMailSettings(ctx, c.ID, smtpOK, false); err != nil {
		t.Fatal(err)
	}
	ct := store.Contact{Name: "Dlžník s.r.o.", Email: "uctaren@dlznik.sk"}
	if err := a.Store.SaveContact(ctx, c.ID, &ct); err != nil {
		t.Fatal(err)
	}
	d := validDraft()
	d.ContactID = ct.ID
	q, err := a.SaveQuote(ctx, c.ID, quote.Quote{Customer: d.Customer, ContactID: ct.ID, IssueDate: invoice.MustDate("2026-10-20"),
		ValidUntil: invoice.MustDate("2026-11-20"), Lines: d.Lines})
	if err != nil {
		t.Fatal(err)
	}
	if q, err = a.IssueQuote(ctx, c.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	md, err := a.MailDraft(ctx, c.ID, MailQuote, q.ID)
	if err != nil || md.To != "uctaren@dlznik.sk" || !strings.Contains(md.Body, "platí do 20. 11. 2026") || md.File != "Ponuka-"+q.Number+".pdf" {
		t.Fatalf("ponuka: %+v %v", md, err)
	}
	inv := issuedOn(t, a, c.ID, "Dlžník s.r.o.", "31322832", "2026-09-30")
	r, err := a.CreateReminder(ctx, c.ID, ReminderRequest{InvoiceIDs: []int64{inv.ID}})
	if err != nil {
		t.Fatal(err)
	}
	md, err = a.MailDraft(ctx, c.ID, MailReminder, r.ID)
	if err != nil || md.To != "" || md.Subject != "Upomienka "+r.Number+" – Moja firma" || !md.Sendable {
		t.Fatalf("upomienka: %+v %v", md, err)
	}
	if _, err := a.SendMail(ctx, c.ID, MailReminder, r.ID, MailRequest{To: "dlznik@firma.sk", Body: md.Body}); err != nil {
		t.Fatal(err)
	}
	if f.last().Attachments[0].Name != "Upomienka-"+r.Number+".pdf" {
		t.Fatalf("príloha: %s", f.last().Attachments[0].Name)
	}
	if _, err := a.MailDraft(ctx, c.ID, "iné", 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("neznámy druh: %v", err)
	}
}

func TestMailPermissions(t *testing.T) {
	o := newOffice(t)
	o.a.Mailer = &fakeMailer{}
	actx := as(o.admin)
	if _, err := o.a.SetMailSettings(actx, o.alfa.ID, smtpOK, false); err != nil {
		t.Fatal(err)
	}
	// klient firmy Beta nevidí nastavenie ani doklady firmy Alfa
	if _, err := o.a.MailSettings(as(o.klient), o.alfa.ID); err == nil {
		t.Fatal("cudzia firma")
	}
	if _, err := o.a.TestMail(as(o.klient), o.beta.ID, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("klient skúša e-mail: %v", err)
	}
	if e, err := o.a.TestMail(actx, o.alfa.ID, ""); err != nil || e.To != "faktury@firma.sk" || e.DocKind != MailTest {
		t.Fatalf("skúšobný e-mail: %+v %v", e, err)
	}
	if _, err := o.a.TestMail(actx, o.beta.ID, ""); !errors.Is(err, ErrMailNotConfigured) {
		t.Fatalf("firma bez nastavenia: %v", err)
	}
	if actorName(as(o.admin)) == "automaticky" || actorName(auth.WithActor(ctx, auth.System)) != "automaticky" {
		t.Fatal("actorName")
	}
}

func TestRecurringSendsMail(t *testing.T) {
	a, c, f := mailApp(t)
	tp := validRecurring("2026-10-05")
	tp.SendEmail = true
	tp.Customer.Email = "odberatel@firma.sk"
	if _, err := a.SaveRecurring(ctx, c.ID, tp); err != nil {
		t.Fatal(err)
	}
	noMail := validRecurring("2026-10-06")
	noMail.SendEmail = true
	if _, err := a.SaveRecurring(ctx, c.ID, noMail); err != nil {
		t.Fatal(err)
	}
	// bez nastavenia e-mailu sa faktúra vystaví a chyba sa zapíše
	if _, err := a.SetMailSettings(ctx, c.ID, smtpOK, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunRecurring(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ := a.Store.ListRecurring(ctx, c.ID)
	runs := map[string]recurring.Run{}
	for _, l := range list {
		runs[l.Customer.Email] = l.History[0]
	}
	if r := runs["odberatel@firma.sk"]; !r.Issued || r.Emailed != "odberatel@firma.sk" || r.Error != "" {
		t.Fatalf("s e-mailom: %+v", r)
	}
	if r := runs[""]; !r.Issued || r.Emailed != "" || !strings.Contains(r.Error, "nemá e-mail") {
		t.Fatalf("bez e-mailu: %+v", r)
	}
	if len(f.sent) != 1 || !strings.HasPrefix(f.last().Subject, "Faktúra FA2026") {
		t.Fatalf("odoslané: %d", len(f.sent))
	}
	all, _ := a.Emails(ctx, c.ID, 2026)
	if len(all) != 1 || all[0].User != "automaticky" {
		t.Fatalf("zoznam: %+v", all)
	}
}
