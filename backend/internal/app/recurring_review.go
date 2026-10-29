package app

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// MailNotice je druh záznamu e-mailu s upozornením pre firmu (nie pre odberateľa).
const MailNotice = "notice"

// RecurringDraft je koncept z pravidelnej fakturácie, ktorý čaká na kontrolu
// a vystavenie.
type RecurringDraft struct {
	InvoiceID  int64        `json:"invoiceId"`
	TemplateID int64        `json:"templateId"`
	Template   string       `json:"template"`
	Customer   string       `json:"customer"`
	Period     string       `json:"period"` // „október 2026“
	IssueDate  invoice.Date `json:"issueDate"`
	Total      money.Dec    `json:"total"`
	Currency   string       `json:"currency"`
	Reason     string       `json:"reason,omitempty"` // prečo sa nevystavila sama
}

// RecurringDrafts vráti koncepty z pravidelnej fakturácie, ktoré ešte nie sú
// vystavené, od najstaršieho.
func (a *App) RecurringDrafts(ctx context.Context, companyID int64) ([]RecurringDraft, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	list, err := a.Store.ListRecurring(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := []RecurringDraft{}
	for i := range list {
		t := &list[i]
		for _, r := range t.History {
			if d, ok := a.pendingDraft(ctx, companyID, t, r); ok {
				out = append(out, d)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].IssueDate.Before(out[j].IssueDate) })
	return out, nil
}

// pendingDraft vráti koncept z behu šablóny, ak faktúra stále nie je vystavená.
func (a *App) pendingDraft(ctx context.Context, companyID int64, t *recurring.Template, r recurring.Run) (RecurringDraft, bool) {
	if r.InvoiceID == 0 {
		return RecurringDraft{}, false
	}
	inv, err := a.Store.GetInvoice(ctx, companyID, r.InvoiceID)
	if err != nil || inv.Status != invoice.StatusDraft {
		return RecurringDraft{}, false
	}
	reason := r.Error
	if reason == "" && !t.AutoIssue {
		reason = "Šablóna vytvára koncepty — skontrolujte a vystavte"
	}
	return RecurringDraft{
		InvoiceID: inv.ID, TemplateID: t.ID, Template: strings.Trim(recurringName(*t), "„“"), Customer: inv.Customer.Name,
		Period: t.Interval.PeriodLabel(r.Period), IssueDate: inv.IssueDate, Total: inv.Summary.Total,
		Currency: inv.Currency, Reason: reason,
	}, true
}

// notifyDrafts pošle firme upozornenie na nové koncepty z automatického
// behu, ak má nastavené odosielanie e-mailov a adresu pre upozornenia.
func (a *App) notifyDrafts(ctx context.Context, companyID int64, drafts []RecurringDraft) {
	if len(drafts) == 0 {
		return
	}
	s, err := a.loadMail(ctx, companyID)
	if err != nil || !s.Configured() || strings.TrimSpace(s.NotifyDrafts) == "" {
		return
	}
	to, err := mail.ParseList(s.NotifyDrafts)
	if err != nil || len(to) == 0 {
		log.Printf("upozornenie na koncepty (firma %d): neplatná adresa %q", companyID, s.NotifyDrafts)
		return
	}
	s.BccSelf = false
	m := mail.Message{To: to, Subject: draftNoticeSubject(len(drafts)), Body: a.draftNoticeBody(ctx, companyID, drafts)}
	if _, err := a.deliver(ctx, companyID, s, m, store.Email{DocKind: MailNotice}); err != nil {
		log.Printf("upozornenie na koncepty (firma %d): %v", companyID, err)
	}
}

func draftNoticeSubject(n int) string {
	switch {
	case n == 1:
		return "Fiškus: 1 koncept faktúry čaká na kontrolu"
	case n < 5:
		return fmt.Sprintf("Fiškus: %d koncepty faktúr čakajú na kontrolu", n)
	}
	return fmt.Sprintf("Fiškus: %d konceptov faktúr čaká na kontrolu", n)
}

func (a *App) draftNoticeBody(ctx context.Context, companyID int64, drafts []RecurringDraft) string {
	var b strings.Builder
	name := ""
	if co, err := a.Store.GetCompany(ctx, companyID); err == nil {
		name = " firmy " + co.Name
	}
	fmt.Fprintf(&b, "Dobrý deň,\n\npravidelná fakturácia%s pripravila faktúry, ktoré treba skontrolovať a vystaviť:\n\n", name)
	for _, d := range drafts {
		fmt.Fprintf(&b, "• %s — %s, %s, %s\n", d.Customer, d.Template, d.Period, money2(d.Total, d.Currency))
		if d.Reason != "" {
			fmt.Fprintf(&b, "  %s\n", d.Reason)
		}
	}
	b.WriteString("\nKoncepty nájdete v programe Fiškus v časti Pravidelná fakturácia")
	if u := strings.TrimRight(a.PublicURL, "/"); u != "" {
		b.WriteString(": " + u + "/pravidelne")
	}
	b.WriteString(".\n\nTúto správu poslal Fiškus automaticky. Adresu pre upozornenia zmeníte v časti E-maily → Nastavenie.\n")
	return b.String()
}
