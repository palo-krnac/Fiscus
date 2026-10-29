// Package app je aplikačná vrstva: spája doménu (invoice), úložisko (store)
// a pravidlá edície (edition). HTTP API volá iba tento balík.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	normalizeCompany(&c)
	if err := validateCompany(c); err != nil {
		return store.Company{}, err
	}
	return a.Store.CreateCompany(ctx, c)
}

// UpdateCompany uloží nastavenia firmy.
func (a *App) UpdateCompany(ctx context.Context, c store.Company) (store.Company, error) {
	if _, err := a.Store.GetCompany(ctx, c.ID); err != nil {
		return c, err
	}
	normalizeCompany(&c)
	if err := validateCompany(c); err != nil {
		return c, err
	}
	return c, a.Store.UpdateCompany(ctx, c)
}

func normalizeCompany(c *store.Company) {
	c.Name = strings.TrimSpace(c.Name)
	c.ICO = party.NormalizeID(c.ICO)
	c.DIC = party.NormalizeID(c.DIC)
	c.ICDPH = party.NormalizeID(c.ICDPH)
	c.IBAN = formatIBAN(c.IBAN)
	if c.DefaultDueDays == 0 {
		c.DefaultDueDays = defaultDueDays
	}
	// merné jednotky: bez prázdnych a duplicít, v zadanom poradí
	seen := map[string]bool{}
	units := []string{}
	for _, u := range c.Units {
		u = strings.TrimSpace(u)
		if u != "" && !seen[strings.ToLower(u)] {
			seen[strings.ToLower(u)] = true
			units = append(units, u)
		}
	}
	if len(units) == 0 {
		units = append(units, store.DefaultUnits...)
	}
	c.Units = units
	formats := map[string]string{}
	for k, def := range invoice.DefaultFormats {
		f := strings.TrimSpace(c.NumberFormats[string(k)])
		if f == "" {
			f = def
		}
		formats[string(k)] = f
	}
	c.NumberFormats = formats
}

// NumberFormat vráti formát číselného radu firmy pre druh dokladu.
func NumberFormat(c store.Company, k invoice.Kind) string {
	if f := c.NumberFormats[string(k)]; f != "" {
		return f
	}
	if f, ok := invoice.DefaultFormats[k]; ok {
		return f
	}
	return invoice.DefaultFormats[invoice.KindInvoice]
}

func validateCompany(c store.Company) error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if c.Name == "" {
		add("name", "Zadajte názov firmy")
	}
	if c.ICO != "" && !party.ValidICO(c.ICO) {
		add("ico", "Neplatné IČO")
	}
	if c.DIC != "" && !party.ValidDIC(c.DIC) {
		add("dic", "DIČ má mať 10 číslic")
	}
	if c.ICDPH != "" && !party.ValidICDPH(c.ICDPH) {
		add("icdph", "Neplatné IČ DPH")
	}
	if c.VATPayer && c.ICDPH == "" {
		add("icdph", "Platiteľ DPH musí mať IČ DPH")
	}
	if c.IBAN != "" && !party.ValidIBAN(c.IBAN) {
		add("iban", "Neplatný IBAN")
	}
	if c.Email != "" && !validEmail(c.Email) {
		add("email", "Neplatný e-mail")
	}
	if c.DefaultDueDays < 0 || c.DefaultDueDays > 365 {
		add("defaultDueDays", "Splatnosť musí byť 0 až 365 dní")
	}
	for _, k := range []invoice.Kind{invoice.KindInvoice, invoice.KindCreditNote, invoice.KindProforma} {
		if err := invoice.ValidateFormat(c.NumberFormats[string(k)]); err != nil {
			add("numberFormats."+string(k), err.Error())
		}
	}
	for _, u := range c.Units {
		if len([]rune(u)) > 10 {
			add("units", "Merná jednotka môže mať najviac 10 znakov: "+u)
			break
		}
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

const defaultDueDays = 14

// applyCompany nastaví na faktúru údaje odvodené od firmy a kontaktu.
func (a *App) applyCompany(ctx context.Context, companyID int64, inv *invoice.Invoice) error {
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}
	inv.NonVATPayer = !c.VATPayer
	if inv.NonVATPayer {
		inv.ReverseCharge = false
	}
	return nil
}

// dueDays vráti splatnosť pre faktúru: kontakt má prednosť pred firmou.
func (a *App) dueDays(ctx context.Context, companyID int64, inv invoice.Invoice) int {
	if inv.ContactID != 0 {
		if ct, err := a.Store.GetContact(ctx, companyID, inv.ContactID); err == nil && ct.DueDays > 0 {
			return ct.DueDays
		}
	}
	if c, err := a.Store.GetCompany(ctx, companyID); err == nil && c.DefaultDueDays > 0 {
		return c.DefaultDueDays
	}
	return defaultDueDays
}

// Preview prepočíta faktúru bez uloženia (živý výpočet v editore).
func (a *App) Preview(ctx context.Context, companyID int64, inv invoice.Invoice) (invoice.Invoice, error) {
	if err := a.applyCompany(ctx, companyID, &inv); err != nil {
		return inv, err
	}
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
	if err := a.applyCompany(ctx, companyID, &inv); err != nil {
		return inv, err
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
		inv.DueDate = inv.IssueDate.AddDays(a.dueDays(ctx, companyID, inv))
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
	if err := a.applyCompany(ctx, companyID, &inv); err != nil {
		return inv, err
	}
	if err := inv.Recalculate(); err != nil {
		return inv, err
	}
	if err := inv.Validate(); err != nil {
		return inv, err
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return inv, err
	}
	year, month := inv.IssueDate.Year(), inv.IssueDate.Month()
	seq, err := a.Store.NextSequence(ctx, companyID, invoice.SequenceKey(inv.Kind), year)
	if err != nil {
		return inv, err
	}
	number := invoice.FormatNumberWith(NumberFormat(company, inv.Kind), year, month, seq)
	if err := inv.Issue(number); err != nil {
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
