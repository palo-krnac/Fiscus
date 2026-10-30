package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/printout"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

var (
	// ErrMailNotConfigured — firma ešte nemá nastavené odosielanie e-mailov.
	ErrMailNotConfigured = errors.New("odosielanie e-mailov nie je nastavené — doplňte server SMTP v časti E-maily → Nastavenie")
	// ErrNotSendable — koncept alebo stornovaný doklad sa neposiela.
	ErrNotSendable = errors.New("poslať sa dá iba vystavený doklad")
	// ErrBadRecipient — chybný príjemca správy.
	ErrBadRecipient = errors.New("neplatný príjemca")
)

// Druhy dokladov v e-mailoch (DocKind v zázname).
const (
	MailInvoice  = "invoice"
	MailQuote    = "quote"
	MailReminder = "reminder"
	MailTest     = "test"
)

func mailKey(companyID int64) string { return "mail:" + strconv.FormatInt(companyID, 10) }

// loadMail vráti nastavenie e-mailu firmy vrátane hesla (iba pre odoslanie).
func (a *App) loadMail(ctx context.Context, companyID int64) (mail.Settings, error) {
	raw, err := a.Store.GetSetting(ctx, mailKey(companyID))
	if errors.Is(err, store.ErrNotFound) {
		return mail.Settings{}, nil
	}
	if err != nil {
		return mail.Settings{}, err
	}
	var s mail.Settings
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return mail.Settings{}, err
	}
	s.PasswordSet = s.Password != ""
	return s, nil
}

// public skryje heslo a doplní predvolené šablóny pre formulár.
func public(s mail.Settings) mail.Settings {
	s.PasswordSet = s.Password != ""
	s.Password = ""
	t := map[string]mail.Template{}
	for _, k := range mail.Kinds {
		t[k] = s.Template(k)
	}
	s.Templates = t
	return s
}

// MailSettings vráti nastavenie e-mailu firmy (bez hesla).
func (a *App) MailSettings(ctx context.Context, companyID int64) (mail.Settings, error) {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return mail.Settings{}, err
	}
	s, err := a.loadMail(ctx, companyID)
	if err != nil {
		return s, err
	}
	return public(s), nil
}

// SetMailSettings overí a uloží nastavenie. Prázdne heslo ponechá uložené,
// ClearPassword ho zmaže.
func (a *App) SetMailSettings(ctx context.Context, companyID int64, s mail.Settings, clearPassword bool) (mail.Settings, error) {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return mail.Settings{}, err
	}
	old, err := a.loadMail(ctx, companyID)
	if err != nil {
		return mail.Settings{}, err
	}
	if s.Password == "" && !clearPassword {
		s.Password = old.Password
	}
	s.PasswordSet = false
	s.Normalize()
	if err := s.Validate(); err != nil {
		return mail.Settings{}, err
	}
	b, _ := json.Marshal(s)
	if err := a.Store.SetSetting(ctx, mailKey(companyID), string(b)); err != nil {
		return mail.Settings{}, err
	}
	// heslo do auditu nejde
	a.record(ctx, companyID, audit.CatSettings, "update", "mail", companyID,
		fmt.Sprintf("Nastavenie e-mailu: %s:%d, odosielateľ %s", s.Host, s.Port, s.FromEmail), public(old), public(s))
	return public(s), nil
}

// actorName vráti meno toho, kto posiela (do zoznamu e-mailov).
func actorName(ctx context.Context) string {
	act := auth.ActorFrom(ctx)
	if act.Channel == auth.ChannelSystem {
		return "automaticky"
	}
	return act.Name
}

// deliver odošle správu a zapíše ju do zoznamu e-mailov aj pri chybe.
func (a *App) deliver(ctx context.Context, companyID int64, s mail.Settings, m mail.Message, e store.Email) (store.Email, error) {
	if s.BccSelf {
		m.Bcc = append(m.Bcc, s.FromEmail)
	}
	m.FromName, m.From, m.ReplyTo = s.FromName, s.FromEmail, s.ReplyTo
	if m.FromName == "" {
		if co, err := a.Store.GetCompany(ctx, companyID); err == nil {
			m.FromName = co.Name
		}
	}
	sender := a.Mailer
	if sender == nil {
		sender = mail.SMTP{}
	}
	sendErr := sender.Send(ctx, s, m)
	e.To, e.Cc, e.Subject = strings.Join(m.To, ", "), strings.Join(m.Cc, ", "), m.Subject
	e.Sent = a.clock()
	e.User = actorName(ctx)
	if len(m.Attachments) > 0 {
		e.File = m.Attachments[0].Name
	}
	if sendErr != nil {
		e.Error = sendErr.Error()
	}
	if err := a.Store.AddEmail(ctx, companyID, &e); err != nil {
		return e, err
	}
	return e, sendErr
}

func (a *App) clock() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// TestMail pošle skúšobnú správu podľa uloženého nastavenia.
func (a *App) TestMail(ctx context.Context, companyID int64, to string) (store.Email, error) {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return store.Email{}, err
	}
	s, err := a.loadMail(ctx, companyID)
	if err != nil {
		return store.Email{}, err
	}
	if !s.Configured() {
		return store.Email{}, ErrMailNotConfigured
	}
	if to == "" {
		to = s.FromEmail
	}
	list, err := mail.ParseList(to)
	if err != nil || len(list) == 0 {
		return store.Email{}, fmt.Errorf("%w: %v", ErrBadRecipient, err)
	}
	s.BccSelf = false
	m := mail.Message{To: list, Subject: "Skúšobná správa z programu Fiškus",
		Body: "Dobrý deň,\n\ntoto je skúšobná správa. Odosielanie e-mailov z programu Fiškus funguje.\n"}
	return a.deliver(ctx, companyID, s, m, store.Email{DocKind: MailTest})
}

// MailDraft je predvyplnená správa k dokladu.
type MailDraft struct {
	To         string        `json:"to"`
	Cc         string        `json:"cc"`
	Subject    string        `json:"subject"`
	Body       string        `json:"body"`
	File       string        `json:"file"`
	Configured bool          `json:"configured"`
	Sendable   bool          `json:"sendable"` // doklad je vystavený
	History    []store.Email `json:"history"`
}

// MailRequest je správa, ktorú používateľ odošle.
type MailRequest struct {
	To      string `json:"to"`
	Cc      string `json:"cc"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// mailDoc sú údaje dokladu pre e-mail.
type mailDoc struct {
	kind     string // invoice | quote | reminder
	id       int64
	number   string
	to       string // e-mail odberateľa
	file     string // názov prílohy
	sendable bool
	vars     map[string]string
	pdf      func() (Document, error)
}

func money2(d money.Dec, currency string) string {
	if currency == "" || currency == "EUR" {
		return d.Display() + " €"
	}
	return d.Display() + " " + currency
}

// docNames sú názvy dokladu: v 1. páde s veľkým písmenom (predmet) a vo 4. páde (text).
func docNames(k invoice.Kind) (nominative, accusative string) {
	switch k {
	case invoice.KindCreditNote:
		return "Dobropis", "dobropis"
	case invoice.KindProforma:
		return "Zálohová faktúra", "zálohovú faktúru"
	case invoice.KindAdvanceReceipt:
		return "Daňový doklad k prijatej platbe", "daňový doklad k prijatej platbe"
	}
	return "Faktúra", "faktúru"
}

// contactEmail vráti e-mail odberateľa z dokladu, inak z adresára.
func (a *App) contactEmail(ctx context.Context, companyID int64, p invoice.Party, contactID int64) string {
	if p.Email != "" {
		return p.Email
	}
	if contactID != 0 {
		if c, err := a.Store.GetContact(ctx, companyID, contactID); err == nil {
			return c.Email
		}
	}
	return ""
}

func (a *App) mailDoc(ctx context.Context, companyID int64, kind string, id int64) (mailDoc, error) {
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return mailDoc{}, err
	}
	vars := map[string]string{"firma": co.Name, "iban": co.IBAN}
	switch kind {
	case MailInvoice:
		if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
			return mailDoc{}, err
		}
		inv, err := a.Store.GetInvoice(ctx, companyID, id)
		if err != nil {
			return mailDoc{}, err
		}
		nom, acc := docNames(inv.Kind)
		vars["Doklad"], vars["doklad"] = nom, acc
		vars["cislo"], vars["vs"] = inv.Number, inv.VariableSymbol
		vars["suma"] = money2(inv.Summary.Total, inv.Currency)
		vars["splatnost"], vars["datum"] = inv.DueDate.Display(), inv.IssueDate.Display()
		vars["odberatel"] = inv.Customer.Name
		vars["platba"] = paymentSentence(inv, co.IBAN)
		return mailDoc{kind: kind, id: id, number: inv.Number, file: printout.FileName(fileKind(inv.Kind), inv.Number), to: a.contactEmail(ctx, companyID, inv.Customer, inv.ContactID),
			sendable: inv.Status == invoice.StatusIssued || inv.Status == invoice.StatusPaid, vars: vars,
			pdf: func() (Document, error) { return a.InvoicePDF(ctx, companyID, id) }}, nil
	case MailQuote:
		q, err := a.Quote(ctx, companyID, id)
		if err != nil {
			return mailDoc{}, err
		}
		vars["Doklad"], vars["doklad"] = "Cenová ponuka", "cenovú ponuku"
		vars["cislo"], vars["suma"] = q.Number, money2(q.Summary.Total, q.Currency)
		vars["platna_do"], vars["datum"] = q.ValidUntil.Display(), q.IssueDate.Display()
		vars["prijatie"] = ""
		if u := a.QuoteURL(q); u != "" && q.Status == quote.StatusIssued {
			vars["prijatie"] = "Ponuku si môžete pozrieť a prijať alebo odmietnuť aj online: " + u + "\n\n"
		}
		vars["odberatel"] = q.Customer.Name
		return mailDoc{kind: kind, id: id, number: q.Number, file: printout.FileName("Ponuka", q.Number), to: a.contactEmail(ctx, companyID, q.Customer, q.ContactID),
			sendable: q.Status != "" && q.Status != "draft", vars: vars,
			pdf: func() (Document, error) { return a.QuotePDF(ctx, companyID, id) }}, nil
	case MailReminder:
		r, err := a.Reminder(ctx, companyID, id)
		if err != nil {
			return mailDoc{}, err
		}
		vars["Doklad"], vars["doklad"] = reminder.Title(r.Level), "upomienku"
		vars["cislo"], vars["suma"] = r.Number, money2(r.Total, "")
		vars["splatnost"], vars["datum"] = r.PayBy.Display(), r.Date.Display()
		vars["odberatel"] = r.Customer.Name
		return mailDoc{kind: kind, id: id, number: r.Number, file: printout.FileName("Upomienka", r.Number), to: a.contactEmail(ctx, companyID, r.Customer, r.ContactID),
			sendable: true, vars: vars,
			pdf: func() (Document, error) { return a.ReminderPDF(ctx, companyID, id) }}, nil
	}
	return mailDoc{}, store.ErrNotFound
}

// paymentSentence je veta o úhrade do textu e-mailu k faktúre.
func paymentSentence(inv invoice.Invoice, iban string) string {
	switch {
	case inv.Kind == invoice.KindCreditNote || inv.Kind == invoice.KindAdvanceReceipt:
		return ""
	case inv.Status == invoice.StatusPaid:
		return "Doklad je už uhradený, ďakujeme.\n"
	case inv.Summary.Total.Sign() <= 0:
		return ""
	case inv.PaymentMethod != "" && inv.PaymentMethod != invoice.PayTransfer:
		return ""
	case iban == "":
		return fmt.Sprintf("Prosíme o úhradu do %s, variabilný symbol %s.\n", inv.DueDate.Display(), inv.VariableSymbol)
	}
	return fmt.Sprintf("Prosíme o úhradu do %s na účet %s, variabilný symbol %s.\n", inv.DueDate.Display(), iban, inv.VariableSymbol)
}

// MailDraft pripraví správu k dokladu podľa šablóny a vráti históriu odoslaní.
func (a *App) MailDraft(ctx context.Context, companyID int64, kind string, id int64) (MailDraft, error) {
	doc, err := a.mailDoc(ctx, companyID, kind, id)
	if err != nil {
		return MailDraft{}, err
	}
	s, err := a.loadMail(ctx, companyID)
	if err != nil {
		return MailDraft{}, err
	}
	hist, err := a.Store.ListEmails(ctx, companyID, store.EmailFilter{DocKind: kind, DocID: id})
	if err != nil {
		return MailDraft{}, err
	}
	t := s.Template(kind)
	return MailDraft{
		To: doc.to, Subject: mail.Expand(t.Subject, doc.vars), Body: mail.Expand(t.Body, doc.vars), File: doc.file,
		Configured: s.Configured(), Sendable: doc.sendable, History: hist,
	}, nil
}

// SendMail pošle doklad e-mailom s PDF v prílohe.
func (a *App) SendMail(ctx context.Context, companyID int64, kind string, id int64, req MailRequest) (store.Email, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return store.Email{}, err
	}
	doc, err := a.mailDoc(ctx, companyID, kind, id)
	if err != nil {
		return store.Email{}, err
	}
	if !doc.sendable {
		return store.Email{}, ErrNotSendable
	}
	s, err := a.loadMail(ctx, companyID)
	if err != nil {
		return store.Email{}, err
	}
	if !s.Configured() {
		return store.Email{}, ErrMailNotConfigured
	}
	to, err := mail.ParseList(req.To)
	if err != nil {
		return store.Email{}, fmt.Errorf("%w: %v", ErrBadRecipient, err)
	}
	if len(to) == 0 {
		return store.Email{}, fmt.Errorf("%w: %v", ErrBadRecipient, mail.ErrNoRecipient)
	}
	cc, err := mail.ParseList(req.Cc)
	if err != nil {
		return store.Email{}, fmt.Errorf("%w: %v", ErrBadRecipient, err)
	}
	subject := strings.Join(strings.Fields(req.Subject), " ") // bez koncov riadkov
	if subject == "" {
		subject = mail.Expand(s.Template(kind).Subject, doc.vars)
	}
	pdf, err := doc.pdf()
	if err != nil {
		return store.Email{}, err
	}
	m := mail.Message{To: to, Cc: cc, Subject: subject, Body: req.Body,
		Attachments: []mail.Attachment{{Name: pdf.Name, ContentType: "application/pdf", Data: pdf.Data}}}
	e, sendErr := a.deliver(ctx, companyID, s, m, store.Email{DocKind: kind, DocID: id, DocNumber: doc.number})
	cat := map[string]audit.Category{MailInvoice: audit.CatInvoices, MailQuote: audit.CatQuotes, MailReminder: audit.CatInvoices}[kind]
	summary := fmt.Sprintf("E-mail %s %s → %s", doc.vars["Doklad"], doc.number, e.To)
	if sendErr != nil {
		summary += " (neodoslaný: " + sendErr.Error() + ")"
	}
	a.record(ctx, companyID, cat, "email", kind, id, summary, nil, e)
	return e, sendErr
}

// Emails vráti zoznam odoslaných e-mailov firmy.
func (a *App) Emails(ctx context.Context, companyID int64, year int) ([]store.Email, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	return a.Store.ListEmails(ctx, companyID, store.EmailFilter{Year: year})
}

// autoMail pošle vystavenú faktúru odberateľovi podľa šablóny (pravidelná fakturácia).
// Vráti adresu, na ktorú bola poslaná.
func (a *App) autoMail(ctx context.Context, companyID, id int64) (string, error) {
	d, err := a.MailDraft(ctx, companyID, MailInvoice, id)
	if err != nil {
		return "", err
	}
	if !d.Configured {
		return "", ErrMailNotConfigured
	}
	if d.To == "" {
		return "", errors.New("odberateľ nemá e-mail — doplňte ho v adresári")
	}
	e, err := a.SendMail(ctx, companyID, MailInvoice, id, MailRequest{To: d.To, Subject: d.Subject, Body: d.Body})
	if err != nil {
		return "", err
	}
	return e.To, nil
}
