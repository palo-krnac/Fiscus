package app

import (
	"context"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/printout"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Document je hotový doklad v PDF (na stiahnutie alebo do prílohy e-mailu).
type Document struct {
	Name string // názov súboru, napr. „Faktura-2026001.pdf“
	Data []byte
}

// supplier vráti údaje dodávateľa pre tlač: firmu a logo (ak je nahrané).
func (a *App) supplier(ctx context.Context, companyID int64) (printout.Supplier, error) {
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return printout.Supplier{}, err
	}
	sup := printout.Supplier{Company: co}
	if l, err := a.Store.GetLogo(ctx, companyID); err == nil {
		sup.Logo = l.Data
	} else if !errors.Is(err, store.ErrNotFound) {
		return printout.Supplier{}, err
	}
	return sup, nil
}

// fileKind vráti druh dokladu do názvu súboru (bez diakritiky).
func fileKind(k invoice.Kind) string {
	switch k {
	case invoice.KindCreditNote:
		return "Dobropis"
	case invoice.KindProforma:
		return "Zalohova-faktura"
	case invoice.KindAdvanceReceipt:
		return "Doklad-k-platbe"
	}
	return "Faktura"
}

// InvoicePDF vytvorí PDF faktúry.
func (a *App) InvoicePDF(ctx context.Context, companyID, id int64) (Document, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return Document{}, err
	}
	inv, err := a.Store.GetInvoice(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	sup, err := a.supplier(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := printout.Invoice(inv, sup)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: printout.FileName(fileKind(inv.Kind), inv.Number), Data: data}, nil
}

// QuotePDF vytvorí PDF cenovej ponuky.
func (a *App) QuotePDF(ctx context.Context, companyID, id int64) (Document, error) {
	q, err := a.Quote(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	sup, err := a.supplier(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := printout.Quote(q, sup)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: printout.FileName("Ponuka", q.Number), Data: data}, nil
}

// ReminderPDF vytvorí PDF upomienky.
func (a *App) ReminderPDF(ctx context.Context, companyID, id int64) (Document, error) {
	r, err := a.Reminder(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	sup, err := a.supplier(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := printout.Reminder(r, sup)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: printout.FileName("Upomienka", r.Number), Data: data}, nil
}
