package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// AmendError — vystavenú faktúru už nemožno upraviť (opravuje sa dobropisom).
type AmendError struct{ Reason string }

func (e *AmendError) Error() string { return e.Reason }

// ErrAmend je spoločný predok AmendError pre errors.Is.
var ErrAmend = errors.New("vystavenú faktúru už nemožno upraviť")

func (e *AmendError) Is(target error) bool { return target == ErrAmend }

// VATDeadline vráti posledný deň podania priznania k DPH za obdobie, do
// ktorého patrí dátum (25. deň po skončení mesiaca alebo štvrťroka).
func VATDeadline(d invoice.Date, quarterly bool) time.Time {
	t := time.Date(d.Year(), time.Month(d.Month()), 1, 0, 0, 0, 0, time.UTC)
	if quarterly {
		t = time.Date(t.Year(), time.Month((int(t.Month())-1)/3*3+3), 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(t.Year(), t.Month()+1, 25, 0, 0, 0, 0, time.UTC)
}

// AmendBlock vráti dôvod, prečo sa vystavená faktúra nedá upraviť ("" = dá sa).
// Vystavená faktúra sa smie upravovať, kým nie je uhradená a kým nie je
// zahrnutá v podanom priznaní k DPH, odoslaná ako e-faktúra ani opravená
// dobropisom. Potom sa opravuje už iba dobropisom.
func (a *App) AmendBlock(ctx context.Context, companyID int64, inv invoice.Invoice) string {
	switch {
	case inv.Status == invoice.StatusDraft:
		return ""
	case inv.Status == invoice.StatusPaid:
		return "Faktúra je uhradená — opravuje sa dobropisom"
	case inv.Status != invoice.StatusIssued:
		return "Stornovaná faktúra sa nemení"
	case inv.Kind != invoice.KindInvoice && inv.Kind != invoice.KindCreditNote:
		return "Zálohové doklady sa po vystavení nemenia"
	case inv.Advance != nil:
		return "Faktúra je naviazaná na zálohu — opravuje sa dobropisom"
	case len(inv.Payments) > 0 || inv.PaidAmount.Sign() != 0:
		return "Faktúra má zaplatenú časť — opravuje sa dobropisom"
	}
	if sent, err := a.Store.ListEInvoices(ctx, companyID, store.EInvoiceFilter{Direction: "out", DocID: inv.ID}); err == nil {
		for _, e := range sent {
			if e.Error == "" {
				return "Faktúra bola odoslaná ako e-faktúra (údaje dostala finančná správa) — opravuje sa dobropisom"
			}
		}
	}
	if inv.Kind == invoice.KindInvoice {
		if all, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{}); err == nil {
			for _, x := range all {
				if x.Kind == invoice.KindCreditNote && x.Correction != nil && x.Correction.OriginalID == inv.ID {
					return "K faktúre existuje dobropis " + firstNonEmptyStr(x.Number, "(koncept)")
				}
			}
		}
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err == nil && co.VATPayer && !inv.NonVATPayer {
		point := inv.DeliveryDate
		if inv.Kind == invoice.KindCreditNote || point.IsZero() {
			point = inv.IssueDate
		}
		deadline := VATDeadline(point, co.VATPeriod == store.VATPeriodQuarter)
		today := a.Today()
		if !time.Date(today.Year(), time.Month(today.Month()), today.Day(), 0, 0, 0, 0, time.UTC).Before(deadline) {
			return fmt.Sprintf("Obdobie DPH faktúry už bolo priznané (termín %s) — opravuje sa dobropisom", deadline.Format("2. 1. 2006"))
		}
	}
	return ""
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// amendIssued uloží úpravu vystavenej, ešte neuhradenej faktúry: číslo,
// variabilný symbol a stav ostanú, doklad sa skontroluje ako pri vystavení
// a zmena sa zapíše do auditu (pred / po).
func (a *App) amendIssued(ctx context.Context, companyID int64, old, inv invoice.Invoice) (invoice.Invoice, error) {
	if reason := a.AmendBlock(ctx, companyID, old); reason != "" {
		return old, &AmendError{reason}
	}
	inv.ID, inv.CompanyID, inv.Kind, inv.Status = old.ID, old.CompanyID, old.Kind, old.Status
	inv.Number, inv.VariableSymbol = old.Number, old.VariableSymbol
	// odkaz dobropisu na pôvodnú faktúru ostáva, zmeniť sa dá iba dôvod
	reason := ""
	if inv.Correction != nil {
		reason = strings.TrimSpace(inv.Correction.Reason)
	}
	inv.Correction, inv.Advance = nil, old.Advance
	if old.Correction != nil {
		c := *old.Correction
		if reason != "" {
			c.Reason = reason
		}
		inv.Correction = &c
	}
	inv.Payments, inv.PaidAmount, inv.PaidDate = old.Payments, old.PaidAmount, old.PaidDate
	if err := a.applyCompany(ctx, companyID, &inv); err != nil {
		return old, err
	}
	if err := a.applyRate(ctx, &inv); err != nil {
		return old, err
	}
	if err := inv.Recalculate(); err != nil {
		return old, err
	}
	if err := inv.Validate(); err != nil {
		return old, err
	}
	if err := a.Store.SaveInvoice(ctx, companyID, &inv); err != nil {
		return old, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "amend", "invoice", inv.ID,
		"Úprava vystavenej faktúry "+inv.Number+" pre "+inv.Customer.Name, old, inv)
	a.syncInvoiceStock(ctx, companyID, inv)
	return inv, nil
}
