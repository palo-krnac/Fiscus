package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

// VATReturn zostaví riadne priznanie k DPH a kontrolný výkaz za obdobie.
func (a *App) VATReturn(ctx context.Context, companyID int64, period string, excess money.Dec) (vatreturn.Return, store.Company, error) {
	return a.VATReturnWith(ctx, companyID, period, excess, vatreturn.Options{})
}

// VATReturnWith zostaví priznanie zvoleného druhu (riadne, opravné, dodatočné).
func (a *App) VATReturnWith(ctx context.Context, companyID int64, period string, excess money.Dec, o vatreturn.Options) (vatreturn.Return, store.Company, error) {
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
	cashDocs, err := a.Store.ListCashDocuments(ctx, companyID, p.Year)
	if err != nil {
		return vatreturn.Return{}, c, err
	}
	r := vatreturn.Compute(vatreturn.Input{Period: p, Invoices: invs, Purchases: purs, CashDocs: cashDocs, ExcessCarried: excess})
	if err := r.Apply(o); err != nil {
		return r, c, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "kind", Message: err.Error()}}}
	}
	return r, c, nil
}

// Taxpayer pripraví údaje platiteľa do hlavičky.
func Taxpayer(c store.Company) vatreturn.Taxpayer {
	return vatreturn.Taxpayer{Name: c.Name, Street: c.Street, City: c.City, Zip: c.Zip, Phone: c.Phone, Email: c.Email,
		DIC: c.DIC, ICDPH: c.ICDPH, TaxOffice: c.TaxOffice}
}

// VATExport vytvorí XML pre eDane ("dph" alebo "kv") a zapíše to do auditu.
func (a *App) VATExport(ctx context.Context, companyID int64, kind, period string, excess money.Dec) ([]byte, string, error) {
	return a.VATExportWith(ctx, companyID, kind, period, excess, vatreturn.Options{})
}

// VATExportWith vytvorí XML zvoleného druhu priznania (opravné, dodatočné).
func (a *App) VATExportWith(ctx context.Context, companyID int64, kind, period string, excess money.Dec, o vatreturn.Options) ([]byte, string, error) {
	r, c, err := a.VATReturnWith(ctx, companyID, period, excess, o)
	if err != nil {
		return nil, "", err
	}
	if kind == "kv" && r.Kind == vatreturn.KindSupplementary {
		return nil, "", &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "kind", Message: vatreturn.ErrSupplementaryK.Error()}}}
	}
	if c.ICDPH == "" || c.DIC == "" {
		return nil, "", ErrMissingVATData
	}
	tp := Taxpayer(c)
	var out []byte
	var name, what string
	switch kind {
	case "kv":
		out, name, what = vatreturn.KVXML(r, tp), "kv-dph-"+r.Period.String()+suffix(r.Kind)+".xml", "kontrolný výkaz"+kindText(r.Kind)
	case "sv":
		out, name, what = vatreturn.SVXML(r, tp, a.Today()), "sv-dph-"+r.Period.String()+".xml", "súhrnný výkaz"
	default:
		out, name, what = vatreturn.DPHXML(r, tp, a.Today()), "dph-"+r.Period.String()+suffix(r.Kind)+".xml", "priznanie k DPH"+kindText(r.Kind)
	}
	a.record(ctx, companyID, audit.CatTaxes, "export", "vat", 0,
		"Export "+what+" za "+r.Label+" (XML pre eDane), vlastná daňová povinnosť "+r.Rows["r32"].Display()+" €, nadmerný odpočet "+r.Rows["r33"].Display()+" €",
		nil, map[string]any{"period": r.Period.String(), "kind": kind, "r32": r.Rows["r32"], "r33": r.Rows["r33"]})
	return out, name, nil
}

func suffix(k vatreturn.Kind) string {
	switch k {
	case vatreturn.KindCorrective:
		return "-opravne"
	case vatreturn.KindSupplementary:
		return "-dodatocne"
	}
	return ""
}

func kindText(k vatreturn.Kind) string {
	switch k {
	case vatreturn.KindCorrective:
		return " (opravné)"
	case vatreturn.KindSupplementary:
		return " (dodatočné)"
	}
	return ""
}

// VATFiling je záznam o podanom priznaní k DPH (posledná známa povinnosť
// pre dodatočné priznanie). Fiškus priznanie nepodáva sám — zapíše ho
// používateľ po podaní v eDane.
type VATFiling struct {
	Kind     vatreturn.Kind `json:"kind"`
	Filed    invoice.Date   `json:"filed"`
	Result   money.Dec      `json:"result"` // + vlastná daňová povinnosť, − nadmerný odpočet
	R35      money.Dec      `json:"r35"`
	R37      money.Dec      `json:"r37"`
	User     string         `json:"user"`
	Recorded time.Time      `json:"recorded"`
}

func filingKey(companyID int64, period string) string {
	return fmt.Sprintf("vat.filed.%d.%s", companyID, period)
}

// VATFilings vráti zaznamenané podania za obdobie (najstaršie prvé).
func (a *App) VATFilings(ctx context.Context, companyID int64, period string) ([]VATFiling, error) {
	if err := a.Require(ctx, companyID, auth.PermVATRead); err != nil {
		return nil, err
	}
	p, err := vatreturn.ParsePeriod(period)
	if err != nil {
		return nil, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "period", Message: err.Error()}}}
	}
	out := []VATFiling{}
	raw, err := a.Store.GetSetting(ctx, filingKey(companyID, p.String()))
	if errors.Is(err, store.ErrNotFound) || raw == "" {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal([]byte(raw), &out)
}

// RecordVATFiling zapíše, že priznanie (s aktuálnymi údajmi a zvoleným
// druhom) bolo podané v deň filed.
func (a *App) RecordVATFiling(ctx context.Context, companyID int64, period string, excess money.Dec, o vatreturn.Options, filed invoice.Date) ([]VATFiling, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return nil, err
	}
	r, _, err := a.VATReturnWith(ctx, companyID, period, excess, o)
	if err != nil {
		return nil, err
	}
	if filed.IsZero() {
		filed = a.Today()
	}
	list, err := a.VATFilings(ctx, companyID, period)
	if err != nil {
		return nil, err
	}
	list = append(list, VATFiling{Kind: r.Kind, Filed: filed, Result: r.Result(), R35: r.Rows["r35"], R37: r.Rows["r37"],
		User: actorName(ctx), Recorded: a.clock().UTC()})
	b, _ := json.Marshal(list)
	if err := a.Store.SetSetting(ctx, filingKey(companyID, r.Period.String()), string(b)); err != nil {
		return nil, err
	}
	a.record(ctx, companyID, audit.CatTaxes, "filed", "vat", 0,
		fmt.Sprintf("Podané %s priznanie k DPH za %s (%s), výsledok %s €", r.Kind, r.Label, filed.Display(), r.Result().Display()), nil, list[len(list)-1])
	return list, nil
}
