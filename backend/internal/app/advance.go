package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

// ErrReceiptIssued: k úhrade zálohy je vystavený daňový doklad — úhradu nemožno zrušiť.
var ErrReceiptIssued = errors.New("k platbe je vystavený daňový doklad k prijatej platbe — opravte ho dobropisom")

// issueAdvanceReceipt vystaví daňový doklad k prijatej platbe (aj čiastočnej)
// k zálohovej faktúre a prepojí ich.
func (a *App) issueAdvanceReceipt(ctx context.Context, companyID int64, proforma invoice.Invoice, p invoice.Payment) (invoice.Invoice, error) {
	r, err := invoice.NewAdvanceReceipt(proforma, p.Amount, p.Date, a.Today())
	if err != nil {
		return proforma, err
	}
	saved, err := a.saveDraft(ctx, companyID, r)
	if err != nil {
		return proforma, err
	}
	issued, err := a.Issue(ctx, companyID, saved.ID)
	if err != nil {
		return proforma, err
	}
	if err := issued.MarkPaid(p.Date); err != nil {
		return proforma, err
	}
	if err := a.Store.SaveInvoice(ctx, companyID, &issued); err != nil {
		return proforma, err
	}
	proforma.AddReceipt(invoice.ReceiptRef{ID: issued.ID, Number: issued.Number, Date: p.Date, Amount: issued.Summary.Total})
	if err := a.Store.SaveInvoice(ctx, companyID, &proforma); err != nil {
		return proforma, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "receipt", "invoice", issued.ID,
		"Daňový doklad k prijatej platbe "+issued.Number+" k zálohovej faktúre "+proforma.Number+", "+issued.Summary.Total.Display()+" €",
		nil, issued)
	return proforma, nil
}

// SettleAdvance vytvorí koncept vyúčtovacej faktúry k uhradenej (aj čiastočne)
// zálohovej faktúre s odpočtom záloh podľa dokladov k prijatým platbám.
func (a *App) SettleAdvance(ctx context.Context, companyID, proformaID int64) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	p, err := a.Store.GetInvoice(ctx, companyID, proformaID)
	if err != nil {
		return p, err
	}
	receipts, err := a.receiptsOf(ctx, companyID, p)
	if err != nil {
		return p, err
	}
	f, err := invoice.NewFinalInvoice(p, receipts, a.Today(), a.dueDays(ctx, companyID, p))
	if err != nil {
		return f, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "advance", Message: err.Error()}}}
	}
	saved, err := a.saveDraft(ctx, companyID, f)
	if err != nil {
		return saved, err
	}
	if p.Advance == nil {
		p.Advance = &invoice.Advance{}
	}
	p.Advance.FinalID = saved.ID
	if err := a.Store.SaveInvoice(ctx, companyID, &p); err != nil {
		return saved, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", saved.ID,
		"Koncept vyúčtovacej faktúry k zálohe "+p.Number+" pre "+saved.Customer.Name, nil, saved)
	return saved, nil
}

// linkFinal zapíše číslo vystavenej vyúčtovacej faktúry do všetkých
// odpočítaných zálohových faktúr.
func (a *App) linkFinal(ctx context.Context, companyID int64, final invoice.Invoice) error {
	for _, id := range final.ProformaIDs() {
		p, err := a.Store.GetInvoice(ctx, companyID, id)
		if err != nil {
			return err
		}
		if p.Advance == nil {
			p.Advance = &invoice.Advance{}
		}
		p.Advance.FinalID, p.Advance.FinalNumber = final.ID, final.Number
		if err := a.Store.SaveInvoice(ctx, companyID, &p); err != nil {
			return err
		}
	}
	return nil
}

// AdvanceCandidate je ďalšia záloha, ktorú možno pridať do vyúčtovania.
type AdvanceCandidate struct {
	ID       int64        `json:"id"`
	Number   string       `json:"number"`
	Date     invoice.Date `json:"date"`
	Total    money.Dec    `json:"total"`
	Paid     money.Dec    `json:"paid"`
	Currency string       `json:"currency"`
}

// AdvanceCandidates vráti uhradené (aj čiastočne) nevyúčtované zálohy
// odberateľa konceptu vyúčtovacej faktúry.
func (a *App) AdvanceCandidates(ctx context.Context, companyID, finalID int64) ([]AdvanceCandidate, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	f, err := a.Store.GetInvoice(ctx, companyID, finalID)
	if err != nil {
		return nil, err
	}
	out := []AdvanceCandidate{}
	if f.Status != invoice.StatusDraft || f.Advance == nil || len(f.Advance.Deductions) == 0 {
		return out, nil
	}
	list, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if p.Kind != invoice.KindProforma || (p.Status != invoice.StatusIssued && p.Status != invoice.StatusPaid) ||
			p.Paid().Sign() <= 0 || (p.Advance != nil && p.Advance.FinalID != 0) || !f.SameCustomer(p) {
			continue
		}
		out = append(out, AdvanceCandidate{ID: p.ID, Number: p.Number, Date: p.IssueDate, Total: p.Summary.Total, Paid: p.Paid(), Currency: p.Currency})
	}
	return out, nil
}

// receiptsOf načíta doklady k platbám zálohovej faktúry.
func (a *App) receiptsOf(ctx context.Context, companyID int64, p invoice.Invoice) ([]invoice.Invoice, error) {
	var out []invoice.Invoice
	for _, ref := range p.AllReceipts() {
		r, err := a.Store.GetInvoice(ctx, companyID, ref.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// AddAdvanceToFinal pridá do konceptu vyúčtovacej faktúry ďalšiu zálohu
// toho istého odberateľa (odpočty a položky).
func (a *App) AddAdvanceToFinal(ctx context.Context, companyID, finalID, proformaID int64) (invoice.Invoice, error) {
	return a.changeFinalAdvances(ctx, companyID, finalID, proformaID, true)
}

// RemoveAdvanceFromFinal odoberie pridanú zálohu z konceptu vyúčtovania.
func (a *App) RemoveAdvanceFromFinal(ctx context.Context, companyID, finalID, proformaID int64) (invoice.Invoice, error) {
	return a.changeFinalAdvances(ctx, companyID, finalID, proformaID, false)
}

func (a *App) changeFinalAdvances(ctx context.Context, companyID, finalID, proformaID int64, add bool) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	f, err := a.Store.GetInvoice(ctx, companyID, finalID)
	if err != nil {
		return f, err
	}
	p, err := a.Store.GetInvoice(ctx, companyID, proformaID)
	if err != nil {
		return f, err
	}
	bad := func(err error) error {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "advance", Message: err.Error()}}}
	}
	if p.Kind != invoice.KindProforma {
		return f, bad(invoice.ErrNotAdvance)
	}
	if add {
		receipts, err := a.receiptsOf(ctx, companyID, p)
		if err != nil {
			return f, err
		}
		if err := f.AddAdvance(p, receipts); err != nil {
			return f, bad(err)
		}
	} else if err := f.RemoveAdvance(proformaID); err != nil {
		return f, bad(err)
	}
	saved, err := a.saveDraft(ctx, companyID, f)
	if err != nil {
		return saved, err
	}
	if p.Advance == nil {
		p.Advance = &invoice.Advance{}
	}
	what := "Pridaná záloha "
	if add {
		p.Advance.FinalID = saved.ID
	} else {
		p.Advance.FinalID, p.Advance.FinalNumber, what = 0, "", "Odobraná záloha "
	}
	if err := a.Store.SaveInvoice(ctx, companyID, &p); err != nil {
		return saved, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "update", "invoice", saved.ID,
		what+p.Number+" vo vyúčtovaní pre "+saved.Customer.Name, nil, saved.Advance)
	return saved, nil
}

// within15Check: záloha bez dokladu k platbe (dodanie do 15 dní) sa dá
// vyúčtovať iba faktúrou s dodaním do 15 dní od platby v tom istom
// zdaňovacom období — daňová povinnosť vzniká dňom prijatia platby.
func within15Check(inv invoice.Invoice, company store.Company) error {
	if msg := inv.Within15Error(); msg != "" {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "deliveryDate", Message: msg}}}
	}
	if inv.Advance == nil {
		return nil
	}
	quarterly := company.VATPeriod == store.VATPeriodQuarter
	at := vatreturn.PeriodOf(inv.DeliveryDate, quarterly)
	for _, d := range inv.Advance.Deductions {
		if d.Untaxed != invoice.UntaxedWithin15 {
			continue
		}
		if paid := vatreturn.PeriodOf(d.Date, quarterly); paid != at {
			return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "deliveryDate", Message: fmt.Sprintf(
				"Platba zálohy %s je v období %s, dodanie v období %s — daň z platby patrí do obdobia platby. Vystavte k platbe daňový doklad (na zálohovej faktúre).",
				d.Number, paid.Label(), at.Label())}}}
		}
	}
	return nil
}

// payFromAdvances zapíše do vystaveného vyúčtovania úhrady zo záloh bez
// daňového dokladu k platbe (peniaze už prišli na zálohovú faktúru).
func (a *App) payFromAdvances(ctx context.Context, companyID int64, inv *invoice.Invoice) error {
	changed := false
	for _, d := range inv.Advance.Deductions {
		if d.Untaxed == "" || inv.Status != invoice.StatusIssued {
			continue
		}
		if err := inv.AddPayment(invoice.Payment{Date: d.Date, Amount: d.Total, Source: invoice.PaymentAdvance, Ref: d.ProformaID}); err != nil {
			return err
		}
		changed = true
	}
	if !changed {
		return nil
	}
	return a.Store.SaveInvoice(ctx, companyID, inv)
}

// IssueMissingReceipts vystaví daňové doklady k platbám zálohy, ku ktorým
// doklad chýba — napr. keď sa dodanie do 15 dní od platby nestihlo.
func (a *App) IssueMissingReceipts(ctx context.Context, companyID, proformaID int64) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	p, err := a.Store.GetInvoice(ctx, companyID, proformaID)
	if err != nil {
		return p, err
	}
	if !p.CanIssueReceipt() {
		_, why := p.AdvanceMode()
		if why == "" {
			why = "k tejto zálohe sa daňový doklad k prijatej platbe nevyhotovuje"
		}
		return p, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "advance", Message: why}}}
	}
	if p.Settled() {
		return p, invoice.ErrAlreadySettled
	}
	missing := p.UncoveredPayments()
	if len(missing) == 0 {
		return p, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "advance", Message: "Všetky platby zálohy už majú daňový doklad"}}}
	}
	for _, pay := range missing {
		if p, err = a.issueAdvanceReceipt(ctx, companyID, p, pay); err != nil {
			return p, err
		}
	}
	// rozpracované vyúčtovanie: odpočet už podľa nových dokladov
	if p.Advance != nil && p.Advance.FinalID != 0 {
		f, err := a.Store.GetInvoice(ctx, companyID, p.Advance.FinalID)
		if err == nil && f.Status == invoice.StatusDraft {
			receipts, err := a.receiptsOf(ctx, companyID, p)
			if err != nil {
				return p, err
			}
			if err := f.RefreshAdvance(p, receipts); err != nil {
				return p, err
			}
			if _, err := a.saveDraft(ctx, companyID, f); err != nil {
				return p, err
			}
		}
	}
	return p, nil
}
