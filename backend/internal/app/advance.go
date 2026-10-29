package app

import (
	"context"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
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
	var receipts []invoice.Invoice
	for _, ref := range p.AllReceipts() {
		r, err := a.Store.GetInvoice(ctx, companyID, ref.ID)
		if err != nil {
			return p, err
		}
		receipts = append(receipts, r)
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

// linkFinal zapíše číslo vystavenej vyúčtovacej faktúry do zálohovej faktúry.
func (a *App) linkFinal(ctx context.Context, companyID int64, final invoice.Invoice) error {
	p, err := a.Store.GetInvoice(ctx, companyID, final.Advance.ProformaID)
	if err != nil {
		return err
	}
	if p.Advance == nil {
		p.Advance = &invoice.Advance{}
	}
	p.Advance.FinalID, p.Advance.FinalNumber = final.ID, final.Number
	return a.Store.SaveInvoice(ctx, companyID, &p)
}
