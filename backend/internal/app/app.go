// Package app je aplikačná vrstva: spája doménu (invoice), úložisko (store)
// a pravidlá edície (edition). HTTP API volá iba tento balík.
package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// ErrCompanyLimit — v tejto edícii sa nedá založiť ďalšia firma.
var ErrCompanyLimit = errors.New("desktopová verzia podporuje iba jednu firmu — viac firiem je dostupných v serverovej verzii")

// App je aplikačná služba.
type App struct {
	Store store.Store
	Caps  edition.Capabilities
	Today func() invoice.Date // dá sa podvrhnúť v testoch
}

// New vytvorí službu.
func New(s store.Store, caps edition.Capabilities) *App {
	return &App{Store: s, Caps: caps, Today: invoice.Today}
}

// CreateCompany založí firmu, ak to edícia dovoľuje.
func (a *App) CreateCompany(ctx context.Context, c store.Company) (store.Company, error) {
	n, err := a.Store.CountCompanies(ctx)
	if err != nil {
		return store.Company{}, err
	}
	if !a.Caps.CanAddCompany(n) {
		return store.Company{}, ErrCompanyLimit
	}
	var fe []invoice.FieldError
	if c.Name == "" {
		fe = append(fe, invoice.FieldError{Field: "name", Message: "Zadajte názov firmy"})
	}
	if c.ICO != "" && !party.ValidICO(c.ICO) {
		fe = append(fe, invoice.FieldError{Field: "ico", Message: "Neplatné IČO"})
	}
	if c.ICDPH != "" && !party.ValidICDPH(c.ICDPH) {
		fe = append(fe, invoice.FieldError{Field: "icdph", Message: "Neplatné IČ DPH"})
	}
	if len(fe) > 0 {
		return store.Company{}, &invoice.ValidationError{Fields: fe}
	}
	return a.Store.CreateCompany(ctx, c)
}

// Preview prepočíta faktúru bez uloženia (živý výpočet v editore).
func (a *App) Preview(inv invoice.Invoice) (invoice.Invoice, error) {
	err := inv.Recalculate()
	return inv, err
}

// SaveDraft uloží nový alebo upravený koncept.
func (a *App) SaveDraft(ctx context.Context, companyID int64, inv invoice.Invoice) (invoice.Invoice, error) {
	if inv.ID != 0 {
		old, err := a.Store.GetInvoice(ctx, companyID, inv.ID)
		if err != nil {
			return inv, err
		}
		if !old.Editable() {
			return inv, invoice.ErrLocked
		}
	}
	inv.Status = invoice.StatusDraft
	inv.Number = ""
	if inv.Kind == "" {
		inv.Kind = invoice.KindInvoice
	}
	today := a.Today()
	if inv.IssueDate.IsZero() {
		inv.IssueDate = today
	}
	if inv.DeliveryDate.IsZero() {
		inv.DeliveryDate = inv.IssueDate
	}
	if inv.DueDate.IsZero() {
		inv.DueDate = inv.IssueDate.AddDays(14)
	}
	if err := inv.Recalculate(); err != nil {
		return inv, err
	}
	// Koncept sa uloží aj neúplný — validuje sa až pri vystavení.
	err := a.Store.SaveInvoice(ctx, companyID, &inv)
	return inv, err
}

// Issue vystaví koncept a pridelí mu ďalšie číslo z radu.
func (a *App) Issue(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	inv, err := a.Store.GetInvoice(ctx, companyID, id)
	if err != nil {
		return inv, err
	}
	if !inv.Editable() {
		return inv, invoice.ErrBadTransition
	}
	// Validovať pred pridelením čísla, aby v rade nevznikali diery.
	if err := inv.Recalculate(); err != nil {
		return inv, err
	}
	if err := inv.Validate(); err != nil {
		return inv, err
	}
	series := invoice.Series(inv.Kind)
	seq, err := a.Store.NextSequence(ctx, companyID, series, inv.IssueDate.Year())
	if err != nil {
		return inv, err
	}
	if err := inv.Issue(invoice.FormatNumber(series, inv.IssueDate.Year(), seq)); err != nil {
		return inv, fmt.Errorf("vystavenie: %w", err)
	}
	return inv, a.Store.SaveInvoice(ctx, companyID, &inv)
}

// MarkPaid označí faktúru ako uhradenú.
func (a *App) MarkPaid(ctx context.Context, companyID, id int64, on invoice.Date) (invoice.Invoice, error) {
	return a.transition(ctx, companyID, id, func(inv *invoice.Invoice) error {
		if on.IsZero() {
			on = a.Today()
		}
		return inv.MarkPaid(on)
	})
}

// Cancel stornuje faktúru.
func (a *App) Cancel(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	return a.transition(ctx, companyID, id, (*invoice.Invoice).Cancel)
}

func (a *App) transition(ctx context.Context, companyID, id int64, fn func(*invoice.Invoice) error) (invoice.Invoice, error) {
	inv, err := a.Store.GetInvoice(ctx, companyID, id)
	if err != nil {
		return inv, err
	}
	if err := fn(&inv); err != nil {
		return inv, err
	}
	return inv, a.Store.SaveInvoice(ctx, companyID, &inv)
}
