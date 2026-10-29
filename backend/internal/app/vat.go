package app

import (
	"context"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

// Chyby priznania k DPH.
var (
	ErrNotVATPayer    = errors.New("firma nie je platiteľom DPH — priznanie k DPH sa nepodáva")
	ErrPeriodTooOld   = errors.New("obdobia pred júlom 2025 sa podávajú na staršom tlačive, ktoré Fiškus nepodporuje")
	ErrPeriodInFuture = errors.New("obdobie ešte nezačalo")
	ErrMissingVATData = errors.New("v nastaveniach firmy doplňte IČ DPH a DIČ")
)

// VATReturn zostaví priznanie k DPH a kontrolný výkaz za obdobie.
func (a *App) VATReturn(ctx context.Context, companyID int64, period string, excess money.Dec) (vatreturn.Return, store.Company, error) {
	if err := a.Require(ctx, companyID, auth.PermVATRead); err != nil {
		return vatreturn.Return{}, store.Company{}, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return vatreturn.Return{}, c, err
	}
	if !c.VATPayer {
		return vatreturn.Return{}, c, ErrNotVATPayer
	}
	p, err := vatreturn.ParsePeriod(period)
	if err != nil {
		return vatreturn.Return{}, c, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "period", Message: err.Error()}}}
	}
	if !p.Supported() {
		return vatreturn.Return{}, c, ErrPeriodTooOld
	}
	if a.Today().Before(p.Start()) {
		return vatreturn.Return{}, c, ErrPeriodInFuture
	}
	// faktúra dodaná v decembri môže byť vystavená v januári → aj nasledujúci rok
	var invs []invoice.Invoice
	var purs []purchase.Purchase
	for _, y := range []int{p.Year, p.Year + 1} {
		list, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{Year: y})
		if err != nil {
			return vatreturn.Return{}, c, err
		}
		invs = append(invs, list...)
		pl, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{Year: y})
		if err != nil {
			return vatreturn.Return{}, c, err
		}
		purs = append(purs, pl...)
	}
	if excess.Sign() < 0 {
		excess = money.Zero
	}
	r := vatreturn.Compute(vatreturn.Input{Period: p, Invoices: invs, Purchases: purs, ExcessCarried: excess})
	return r, c, nil
}

// Taxpayer pripraví údaje platiteľa do hlavičky.
func Taxpayer(c store.Company) vatreturn.Taxpayer {
	return vatreturn.Taxpayer{Name: c.Name, Street: c.Street, City: c.City, Zip: c.Zip, Phone: c.Phone, Email: c.Email,
		DIC: c.DIC, ICDPH: c.ICDPH, TaxOffice: c.TaxOffice}
}

// VATExport vytvorí XML pre eDane ("dph" alebo "kv") a zapíše to do auditu.
func (a *App) VATExport(ctx context.Context, companyID int64, kind, period string, excess money.Dec) ([]byte, string, error) {
	r, c, err := a.VATReturn(ctx, companyID, period, excess)
	if err != nil {
		return nil, "", err
	}
	if c.ICDPH == "" || c.DIC == "" {
		return nil, "", ErrMissingVATData
	}
	tp := Taxpayer(c)
	var out []byte
	var name, what string
	switch kind {
	case "kv":
		out, name, what = vatreturn.KVXML(r, tp), "kv-dph-"+r.Period.String()+".xml", "kontrolný výkaz"
	default:
		out, name, what = vatreturn.DPHXML(r, tp, a.Today()), "dph-"+r.Period.String()+".xml", "priznanie k DPH"
	}
	a.record(ctx, companyID, audit.CatTaxes, "export", "vat", 0,
		"Export "+what+" za "+r.Label+" (XML pre eDane), vlastná daňová povinnosť "+r.Rows["r32"].Display()+" €, nadmerný odpočet "+r.Rows["r33"].Display()+" €",
		nil, map[string]any{"period": r.Period.String(), "kind": kind, "r32": r.Rows["r32"], "r33": r.Rows["r33"]})
	return out, name, nil
}
