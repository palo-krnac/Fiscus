package app

import (
	"context"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
)

// quoteSeries je kľúč číselného radu ponúk (nezávislý od formátu).
const quoteSeries = "CP"

// Quotes vráti cenové ponuky firmy (year 0 = všetky).
func (a *App) Quotes(ctx context.Context, companyID int64, year int) ([]quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	list, err := a.Store.ListQuotes(ctx, companyID, year)
	if err != nil {
		return nil, err
	}
	for i := range list {
		a.linkInvoice(ctx, companyID, &list[i])
	}
	return list, nil
}

// linkInvoice doplní aktuálne číslo faktúry vytvorenej z ponuky (koncept
// faktúry číslo ešte nemá, dostane ho až pri vystavení).
func (a *App) linkInvoice(ctx context.Context, companyID int64, q *quote.Quote) {
	if q.Share != nil {
		q.Share.URL = a.QuoteURL(*q)
	}
	if q.InvoiceID == 0 {
		return
	}
	if inv, err := a.Store.GetInvoice(ctx, companyID, q.InvoiceID); err == nil {
		q.InvoiceNumber = inv.Number
	}
}

// Quote vráti jednu ponuku.
func (a *App) Quote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err == nil {
		a.linkInvoice(ctx, companyID, &q)
	}
	return q, err
}

// prepareQuote doplní údaje odvodené od firmy a prepočíta sumy.
func (a *App) prepareQuote(ctx context.Context, companyID int64, q *quote.Quote) error {
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}
	q.NonVATPayer = !c.VATPayer
	if q.NonVATPayer {
		q.ReverseCharge = false
	}
	q.Note = strings.TrimSpace(q.Note)
	return q.Recalculate()
}

// PreviewQuote prepočíta ponuku bez uloženia (živý výpočet v editore).
func (a *App) PreviewQuote(ctx context.Context, companyID int64, q quote.Quote) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return q, err
	}
	err := a.prepareQuote(ctx, companyID, &q)
	return q, err
}

// SaveQuote uloží nový alebo upravený koncept ponuky. Koncept sa uloží aj
// neúplný — kontroluje sa pri vystavení.
func (a *App) SaveQuote(ctx context.Context, companyID int64, q quote.Quote) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return q, err
	}
	var before *quote.Quote
	if q.ID != 0 {
		old, err := a.Store.GetQuote(ctx, companyID, q.ID)
		if err != nil {
			return q, err
		}
		if !old.Editable() {
			return q, quote.ErrLocked
		}
		before = &old
	}
	q.Status, q.Number, q.InvoiceID, q.InvoiceNumber, q.Share = quote.StatusDraft, "", 0, "", nil
	if q.IssueDate.IsZero() {
		q.IssueDate = a.Today()
	}
	if q.ValidUntil.IsZero() {
		q.ValidUntil = q.IssueDate.AddDays(quote.DefaultValidDays)
	}
	if err := a.prepareQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	if before == nil {
		a.record(ctx, companyID, audit.CatQuotes, "create", "quote", q.ID, "Nový koncept ponuky pre "+q.Customer.Name, nil, q)
	} else {
		a.record(ctx, companyID, audit.CatQuotes, "update", "quote", q.ID, "Úprava konceptu ponuky pre "+q.Customer.Name, before, q)
	}
	return q, nil
}

// IssueQuote vystaví koncept a pridelí mu číslo z radu ponúk.
func (a *App) IssueQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return q, err
	}
	if !q.Editable() {
		return q, quote.ErrBadTransition
	}
	// kontrola pred pridelením čísla, aby v rade nevznikali diery
	if err := a.prepareQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	if err := q.Validate(); err != nil {
		return q, err
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return q, err
	}
	year, month := q.IssueDate.Year(), q.IssueDate.Month()
	seq, err := a.Store.NextSequence(ctx, companyID, quoteSeries, year)
	if err != nil {
		return q, err
	}
	format := company.NumberFormats[QuoteFormatKey]
	if format == "" {
		format = ExtraFormats[QuoteFormatKey]
	}
	if err := q.Issue(invoice.FormatNumberWith(format, year, month, seq)); err != nil {
		return q, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	a.record(ctx, companyID, audit.CatQuotes, "issue", "quote", q.ID,
		"Vystavená ponuka "+q.Number+" pre "+q.Customer.Name+", "+q.Summary.Total.Display()+" €", nil, q)
	if a.PublicURL != "" {
		// na serveri s verejnou adresou dostane ponuka hneď odkaz pre zákazníka (do e-mailu)
		if err := a.share(ctx, companyID, &q); err != nil {
			return q, err
		}
		a.linkInvoice(ctx, companyID, &q)
	}
	return q, nil
}

// quoteTransition zmení stav ponuky a zapíše to do auditu.
func (a *App) quoteTransition(ctx context.Context, companyID, id int64, action, what string, fn func(*quote.Quote) error) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return q, err
	}
	before := q.Status
	if err := fn(&q); err != nil {
		return q, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	type state struct {
		Status quote.Status `json:"status"`
	}
	a.record(ctx, companyID, audit.CatQuotes, action, "quote", q.ID, what+q.Number+" ("+q.Customer.Name+")",
		state{before}, state{q.Status})
	return q, nil
}

// AcceptQuote označí ponuku ako prijatú zákazníkom.
func (a *App) AcceptQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	return a.quoteTransition(ctx, companyID, id, "accept", "Prijatá ponuka ", (*quote.Quote).Accept)
}

// RejectQuote označí ponuku ako odmietnutú.
func (a *App) RejectQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	return a.quoteTransition(ctx, companyID, id, "reject", "Odmietnutá ponuka ", (*quote.Quote).Reject)
}

// CopyQuote vytvorí z ponuky nový koncept s dnešným dátumom (nová verzia ponuky).
func (a *App) CopyQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return q, err
	}
	days := quote.DefaultValidDays
	if !q.IssueDate.IsZero() && !q.ValidUntil.IsZero() {
		if d := q.IssueDate.DaysUntil(q.ValidUntil); d > 0 {
			days = d
		}
	}
	c := q.Copy(a.Today(), days)
	if err := a.prepareQuote(ctx, companyID, &c); err != nil {
		return c, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &c); err != nil {
		return c, err
	}
	src := q.Number
	if src == "" {
		src = "konceptu"
	}
	a.record(ctx, companyID, audit.CatQuotes, "create", "quote", c.ID, "Kópia ponuky "+src+" pre "+c.Customer.Name, nil, c)
	return c, nil
}

// InvoiceQuote vytvorí z vystavenej alebo prijatej ponuky koncept faktúry.
// Ponuka sa označí ako vyfakturovaná a odkáže na faktúru.
func (a *App) InvoiceQuote(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return invoice.Invoice{}, err
	}
	if !q.Invoiceable() {
		return invoice.Invoice{}, quote.ErrBadTransition
	}
	draft := q.ToInvoice(a.Today(), 0)
	draft.DueDate = invoice.Date{} // splatnosť podľa kontaktu alebo firmy
	inv, err := a.saveDraft(ctx, companyID, a.withDocExtras(ctx, companyID, draft))
	if err != nil {
		return inv, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", inv.ID,
		"Koncept faktúry z ponuky "+q.Number+" pre "+inv.Customer.Name, nil, inv)
	before := q.Status
	if err := q.MarkInvoiced(inv.ID, ""); err != nil {
		return inv, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return inv, err
	}
	type state struct {
		Status    quote.Status `json:"status"`
		InvoiceID int64        `json:"invoiceId,omitempty"`
	}
	a.record(ctx, companyID, audit.CatQuotes, "invoice", "quote", q.ID, "Vyfakturovaná ponuka "+q.Number,
		state{Status: before}, state{q.Status, inv.ID})
	return inv, nil
}
