package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Číselné rady pokladničných dokladov.
const (
	CashInFormatKey   = "cash_in"
	CashOutFormatKey  = "cash_out"
	QuoteFormatKey    = "quote"
	ReminderFormatKey = "reminder"
)

// ExtraFormats sú predvolené formáty radov mimo vydaných faktúr.
var ExtraFormats = map[string]string{
	PurchaseFormatKey: DefaultPurchaseFormat,
	CashInFormatKey:   "PPD{RRRR}{NNNN}",
	CashOutFormatKey:  "VPD{RRRR}{NNNN}",
	QuoteFormatKey:    "CP{RRRR}{NNNN}",
	ReminderFormatKey: "UP{RRRR}{NNNN}",
}

// CashBook je pokladničná kniha za rok so zostatkom (v mene pokladne).
type CashBook struct {
	Register  cash.Register   `json:"register"`
	Documents []cash.Document `json:"documents"`
	Opening   money.Dec       `json:"opening"` // zostatok na začiatku roka
	Balance   money.Dec       `json:"balance"` // aktuálny zostatok (všetky roky)
	In        money.Dec       `json:"in"`      // príjmy v roku
	Out       money.Dec       `json:"out"`     // výdavky v roku
}

// CashBook vráti doklady hlavnej pokladne za rok so zostatkami.
func (a *App) CashBook(ctx context.Context, companyID int64, year int) (CashBook, error) {
	return a.CashBookOf(ctx, companyID, year, 0)
}

// CashBookOf vráti doklady pokladne za rok, počiatočný a aktuálny zostatok.
func (a *App) CashBookOf(ctx context.Context, companyID int64, year int, registerID int64) (CashBook, error) {
	if err := a.Require(ctx, companyID, auth.PermCashRead); err != nil {
		return CashBook{}, err
	}
	reg, err := a.cashRegister(ctx, companyID, registerID)
	if err != nil {
		return CashBook{}, err
	}
	all, err := a.registerDocs(ctx, companyID, registerID)
	if err != nil {
		return CashBook{}, err
	}
	book := CashBook{Register: reg, Documents: []cash.Document{}}
	for _, d := range all {
		if d.Cancelled {
			if d.Date.Year() == year {
				book.Documents = append(book.Documents, d)
			}
			continue
		}
		book.Balance = book.Balance.Add(d.Signed())
		switch {
		case d.Date.Year() < year:
			book.Opening = book.Opening.Add(d.Signed())
		case d.Date.Year() == year:
			book.Documents = append(book.Documents, d)
			if d.Kind == cash.In {
				book.In = book.In.Add(d.Amount)
			} else {
				book.Out = book.Out.Add(d.Amount)
			}
		}
	}
	return book, nil
}

// registerDocs vráti všetky doklady jednej pokladne.
func (a *App) registerDocs(ctx context.Context, companyID, registerID int64) ([]cash.Document, error) {
	all, err := a.Store.ListCashDocuments(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	out := all[:0:0]
	for _, d := range all {
		if d.RegisterID == registerID {
			out = append(out, d)
		}
	}
	return out, nil
}

// CashCandidates vráti neuhradené doklady, ktoré možno zaplatiť v hotovosti.
func (a *App) CashCandidates(ctx context.Context, companyID int64, kind cash.Kind, registerID int64) ([]bank.Candidate, error) {
	if err := a.Require(ctx, companyID, auth.PermCashRead); err != nil {
		return nil, err
	}
	reg, err := a.cashRegister(ctx, companyID, registerID)
	if err != nil {
		return nil, err
	}
	cands, err := a.candidatesIn(ctx, companyID, reg.Currency)
	if err != nil {
		return nil, err
	}
	dir := bank.In
	if kind == cash.Out {
		dir = bank.Out
	}
	out := []bank.Candidate{}
	for _, c := range cands {
		if c.Direction == dir {
			out = append(out, c)
		}
	}
	return out, nil
}

// checkLink overí doklad, ktorý má pokladničný doklad uhradiť.
func (a *App) checkLink(ctx context.Context, companyID int64, d *cash.Document) error {
	bad := func(msg string) error {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "linkId", Message: msg}}}
	}
	switch d.LinkKind {
	case "":
		d.LinkID, d.LinkNumber = 0, ""
		return nil
	case "invoice":
		inv, err := a.Store.GetInvoice(ctx, companyID, d.LinkID)
		if err != nil {
			return err
		}
		if (inv.Kind == invoice.KindCreditNote) != (d.Kind == cash.Out) {
			return bad("Príjem uhrádza vydanú faktúru, výdavok dobropis alebo prijatú faktúru")
		}
		if inv.Status != invoice.StatusIssued {
			return bad("Faktúra " + inv.Number + " nie je vystavená a neuhradená")
		}
		if cur := currencyCode(inv.Currency); cur != d.Currency {
			return bad("Faktúra " + inv.Number + " je v mene " + cur + " — pokladňa je v " + d.Currency)
		}
		rest := inv.Remaining()
		if inv.Kind == invoice.KindCreditNote {
			rest = rest.Neg()
		}
		// pokladňou sa dá uhradiť aj časť faktúry, nie viac ako zostatok
		if d.Amount.Cmp(rest) > 0 {
			return bad(fmt.Sprintf("Suma je vyššia ako zostatok dokladu %s (%s €)", inv.Number, rest.Display()))
		}
		d.LinkNumber = inv.Number
	case "purchase":
		p, err := a.Store.GetPurchase(ctx, companyID, d.LinkID)
		if err != nil {
			return err
		}
		if d.Kind != cash.Out {
			return bad("Prijatú faktúru uhrádza výdavkový doklad")
		}
		if p.Status != purchase.StatusBooked {
			return bad("Prijatá faktúra " + p.Number + " je už uhradená")
		}
		if cur := currencyCode(p.Currency); cur != d.Currency {
			return bad("Prijatá faktúra " + p.Number + " je v mene " + cur + " — pokladňa je v " + d.Currency)
		}
		if !p.Summary.Total.Equal(d.Amount) {
			return bad(fmt.Sprintf("Suma sa musí rovnať sume dokladu %s (%s €)", p.Number, p.Summary.Total.Display()))
		}
		d.LinkNumber = p.Number
	default:
		return bad("Neznámy druh dokladu")
	}
	return nil
}

// currencyCode vráti kód meny dokladu (prázdna = EUR).
func currencyCode(c string) string {
	if c == "" {
		return "EUR"
	}
	return c
}

// CashPreview je doklad po prepočte DPH s chybami (pred vystavením).
type CashPreview struct {
	cash.Document
	Errors []invoice.FieldError `json:"errors"`
}

// PreviewCash prepočíta DPH dokladu bez uloženia.
func (a *App) PreviewCash(ctx context.Context, companyID int64, d cash.Document) (CashPreview, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return CashPreview{}, err
	}
	out := CashPreview{Errors: []invoice.FieldError{}}
	err := a.prepareCash(ctx, companyID, &d)
	out.Document = d
	if ve, ok := err.(*invoice.ValidationError); ok && ve != nil {
		out.Errors = ve.Fields
	} else if err != nil {
		return out, err
	}
	return out, nil
}

// prepareCash upraví texty, prepočíta DPH a overí doklad.
func (a *App) prepareCash(ctx context.Context, companyID int64, d *cash.Document) error {
	d.Partner, d.Purpose, d.Note = strings.TrimSpace(d.Partner), strings.TrimSpace(d.Purpose), strings.TrimSpace(d.Note)
	reg, err := a.cashRegister(ctx, companyID, d.RegisterID)
	if err != nil {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "registerId", Message: "Pokladňa neexistuje"}}}
	}
	if reg.Archived {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "registerId", Message: "Pokladňa " + reg.Name + " je archivovaná"}}}
	}
	d.Currency = reg.Currency
	if d.Foreign() {
		// kurz ECB ku dňu dokladu (alebo ručne zadaný)
		if err := a.fillRate(ctx, d.Currency, d.Date, d.Date, &d.FXRate, &d.FXDate, &d.FXManual); err != nil {
			return err
		}
	}
	d.SyncEUR()
	d.NormalizeTax()
	if d.Tax != nil && d.Tax.VehicleID != 0 {
		pct, err := a.vehicleVAT(ctx, companyID, d.Tax.VehicleID, d.Date)
		if err != nil {
			return err
		}
		d.Tax.DeductPercent = pct
	}
	if d.Tax != nil {
		company, err := a.Store.GetCompany(ctx, companyID)
		if err != nil {
			return err
		}
		if !company.VATPayer {
			return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "tax", Message: "DPH na pokladničnom doklade uvádza iba platiteľ DPH"}}}
		}
	}
	return d.Validate()
}

// CreateCash vystaví pokladničný doklad (a prípadne uhradí faktúru).
func (a *App) CreateCash(ctx context.Context, companyID int64, d cash.Document) (cash.Document, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return d, err
	}
	d.ID, d.Cancelled = 0, false
	if err := a.prepareCash(ctx, companyID, &d); err != nil {
		return d, err
	}
	if err := a.checkLink(ctx, companyID, &d); err != nil {
		return d, err
	}
	all, err := a.registerDocs(ctx, companyID, d.RegisterID)
	if err != nil {
		return d, err
	}
	if err := cash.CheckBalance(append(all, d)); err != nil {
		return d, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "amount", Message: err.Error()}}}
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return d, err
	}
	reg, err := a.cashRegister(ctx, companyID, d.RegisterID)
	if err != nil {
		return d, err
	}
	key, series := CashInFormatKey, "PPD"
	if d.Kind == cash.Out {
		key, series = CashOutFormatKey, "VPD"
	}
	format := company.NumberFormats[key]
	if format == "" {
		format = ExtraFormats[key]
	}
	if reg.ID != 0 {
		// ďalšia pokladňa: vlastný rad s predponou, napr. CZPPD20260001
		format = reg.Prefix + ExtraFormats[key]
		series = fmt.Sprintf("%s:%d", series, reg.ID)
	}
	seq, err := a.Store.NextSequence(ctx, companyID, series, d.Date.Year())
	if err != nil {
		return d, err
	}
	d.Number = invoice.FormatNumberWith(format, d.Date.Year(), d.Date.Month(), seq)
	if err := a.Store.SaveCashDocument(ctx, companyID, &d); err != nil {
		return d, err
	}
	what := "Príjmový"
	if d.Kind == cash.Out {
		what = "Výdavkový"
	}
	withVAT := ""
	if d.Tax != nil {
		_, tax, _ := d.Tax.Totals()
		withVAT = fmt.Sprintf(" (DPH %s €)", tax.Display())
	}
	a.record(ctx, companyID, audit.CatCash, "create", "cash", d.ID,
		fmt.Sprintf("%s pokladničný doklad %s, %s %s%s: %s", what, d.Number, d.Amount.Display(), currencySign(d.Currency), withVAT, d.Purpose), nil, d)

	switch d.LinkKind {
	case "invoice":
		amount := d.Amount
		if d.Kind == cash.Out {
			amount = amount.Neg() // výdavok vracia peniaze k dobropisu
		}
		_, err = a.AddPayment(ctx, companyID, d.LinkID, invoice.Payment{Date: d.Date, Amount: amount, Source: invoice.PaymentCash, Ref: d.ID}, false)
	case "purchase":
		_, err = a.MarkPurchasePaid(ctx, companyID, d.LinkID, d.Date)
	}
	return d, err
}

// UpdateCash zmení iba texty dokladu (partner, účel, poznámka); sumy a dátum
// vystaveného dokladu sa nemenia — opravuje sa stornom a novým dokladom.
func (a *App) UpdateCash(ctx context.Context, companyID, id int64, partner, purpose, note string) (cash.Document, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return cash.Document{}, err
	}
	d, err := a.Store.GetCashDocument(ctx, companyID, id)
	if err != nil {
		return d, err
	}
	before := d
	d.Partner, d.Purpose, d.Note = strings.TrimSpace(partner), strings.TrimSpace(purpose), strings.TrimSpace(note)
	if err := d.Validate(); err != nil {
		return d, err
	}
	if err := a.Store.SaveCashDocument(ctx, companyID, &d); err != nil {
		return d, err
	}
	a.record(ctx, companyID, audit.CatCash, "update", "cash", d.ID, "Úprava pokladničného dokladu "+d.Number, before, d)
	return d, nil
}

// CancelCash stornuje doklad; uhradená faktúra sa vráti medzi neuhradené.
func (a *App) CancelCash(ctx context.Context, companyID, id int64) (cash.Document, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return cash.Document{}, err
	}
	d, err := a.Store.GetCashDocument(ctx, companyID, id)
	if err != nil {
		return d, err
	}
	if d.Cancelled {
		return d, cash.ErrCancelled
	}
	all, err := a.registerDocs(ctx, companyID, d.RegisterID)
	if err != nil {
		return d, err
	}
	rest := make([]cash.Document, 0, len(all))
	for _, x := range all {
		if x.ID != d.ID {
			rest = append(rest, x)
		}
	}
	if err := cash.CheckBalance(rest); err != nil {
		return d, fmt.Errorf("doklad nemožno stornovať: %w", err)
	}
	d.Cancelled = true
	if err := a.Store.SaveCashDocument(ctx, companyID, &d); err != nil {
		return d, err
	}
	a.record(ctx, companyID, audit.CatCash, "cancel", "cash", d.ID,
		fmt.Sprintf("Stornovaný pokladničný doklad %s, %s %s", d.Number, d.Amount.Display(), currencySign(d.Currency)), nil, nil)
	switch d.LinkKind {
	case "invoice":
		_, err = a.RemovePayment(ctx, companyID, d.LinkID, invoice.PaymentCash, d.ID)
	case "purchase":
		_, err = a.UnpayPurchase(ctx, companyID, d.LinkID)
	}
	if err != nil && !errors.Is(err, invoice.ErrBadTransition) && !errors.Is(err, store.ErrNotFound) {
		return d, err
	}
	return d, nil
}

func currencySign(c string) string {
	if c == "" || c == "EUR" {
		return "€"
	}
	return c
}
