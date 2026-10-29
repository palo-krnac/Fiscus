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

// CashBook je pokladničná kniha za rok so zostatkom.
type CashBook struct {
	Documents []cash.Document `json:"documents"`
	Opening   money.Dec       `json:"opening"` // zostatok na začiatku roka
	Balance   money.Dec       `json:"balance"` // aktuálny zostatok (všetky roky)
	In        money.Dec       `json:"in"`      // príjmy v roku
	Out       money.Dec       `json:"out"`     // výdavky v roku
}

// CashBook vráti doklady za rok, počiatočný a aktuálny zostatok.
func (a *App) CashBook(ctx context.Context, companyID int64, year int) (CashBook, error) {
	if err := a.Require(ctx, companyID, auth.PermCashRead); err != nil {
		return CashBook{}, err
	}
	all, err := a.Store.ListCashDocuments(ctx, companyID, 0)
	if err != nil {
		return CashBook{}, err
	}
	book := CashBook{Documents: []cash.Document{}}
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

// CashCandidates vráti neuhradené doklady, ktoré možno zaplatiť v hotovosti.
func (a *App) CashCandidates(ctx context.Context, companyID int64, kind cash.Kind) ([]bank.Candidate, error) {
	if err := a.Require(ctx, companyID, auth.PermCashRead); err != nil {
		return nil, err
	}
	cands, err := a.candidates(ctx, companyID)
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
		total := inv.Summary.Total
		if inv.Kind == invoice.KindCreditNote {
			total = total.Neg()
		}
		if !total.Equal(d.Amount) {
			return bad(fmt.Sprintf("Suma sa musí rovnať sume dokladu %s (%s €)", inv.Number, total.Display()))
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
		if !p.Summary.Total.Equal(d.Amount) {
			return bad(fmt.Sprintf("Suma sa musí rovnať sume dokladu %s (%s €)", p.Number, p.Summary.Total.Display()))
		}
		d.LinkNumber = p.Number
	default:
		return bad("Neznámy druh dokladu")
	}
	return nil
}

// CreateCash vystaví pokladničný doklad (a prípadne uhradí faktúru).
func (a *App) CreateCash(ctx context.Context, companyID int64, d cash.Document) (cash.Document, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return d, err
	}
	d.ID, d.Cancelled = 0, false
	d.Partner, d.Purpose, d.Note = strings.TrimSpace(d.Partner), strings.TrimSpace(d.Purpose), strings.TrimSpace(d.Note)
	if err := d.Validate(); err != nil {
		return d, err
	}
	if err := a.checkLink(ctx, companyID, &d); err != nil {
		return d, err
	}
	all, err := a.Store.ListCashDocuments(ctx, companyID, 0)
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
	key, series := CashInFormatKey, "PPD"
	if d.Kind == cash.Out {
		key, series = CashOutFormatKey, "VPD"
	}
	seq, err := a.Store.NextSequence(ctx, companyID, series, d.Date.Year())
	if err != nil {
		return d, err
	}
	format := company.NumberFormats[key]
	if format == "" {
		format = ExtraFormats[key]
	}
	d.Number = invoice.FormatNumberWith(format, d.Date.Year(), d.Date.Month(), seq)
	if err := a.Store.SaveCashDocument(ctx, companyID, &d); err != nil {
		return d, err
	}
	what := "Príjmový"
	if d.Kind == cash.Out {
		what = "Výdavkový"
	}
	a.record(ctx, companyID, audit.CatCash, "create", "cash", d.ID,
		fmt.Sprintf("%s pokladničný doklad %s, %s €: %s", what, d.Number, d.Amount.Display(), d.Purpose), nil, d)

	switch d.LinkKind {
	case "invoice":
		_, err = a.MarkPaid(ctx, companyID, d.LinkID, d.Date)
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
	all, err := a.Store.ListCashDocuments(ctx, companyID, 0)
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
		fmt.Sprintf("Stornovaný pokladničný doklad %s, %s €", d.Number, d.Amount.Display()), nil, nil)
	switch d.LinkKind {
	case "invoice":
		_, err = a.Unpay(ctx, companyID, d.LinkID)
	case "purchase":
		_, err = a.UnpayPurchase(ctx, companyID, d.LinkID)
	}
	if err != nil && !errors.Is(err, invoice.ErrBadTransition) && !errors.Is(err, store.ErrNotFound) {
		return d, err
	}
	return d, nil
}
