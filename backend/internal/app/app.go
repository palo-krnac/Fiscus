// Package app je aplikačná vrstva: spája doménu (invoice), úložisko (store)
// a pravidlá edície (edition). HTTP API volá iba tento balík.
package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/postman"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/legal"
	"github.com/palo-krnac/fiscus/backend/internal/license"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// ErrCompanyLimit — v tejto edícii sa nedá založiť ďalšia firma.
var ErrCompanyLimit = errors.New("desktopová verzia podporuje iba jednu firmu — viac firiem je dostupných v serverovej verzii")

// App je aplikačná služba.
type App struct {
	Store store.Store
	Caps  edition.Capabilities // režim bez licencie; aktuálne schopnosti vracia Capabilities()
	Today func() invoice.Date  // dá sa podvrhnúť v testoch
	Now   func() time.Time     // čas pre licenciu; nil = time.Now
	// PublicURL je verejná adresa servera (odkazy v upozorneniach); prázdna = bez odkazu.
	PublicURL string

	// LicenseKeys sú verejné kľúče na overenie licencie; nil = license.TrustedKeys.
	LicenseKeys []ed25519.PublicKey
	// SaveLicense trvalo uloží licenčný kľúč (nastaví main); nil = iba v pamäti.
	SaveLicense func(key string) error
	// Backups spravuje šifrované zálohy (nil = vypnuté, dôvod v BackupsOff).
	Backups    *backup.Manager
	BackupsOff string
	// BackupSetup umožní nastaviť priečinok a heslo záloh v programe (nil = iba parametrami).
	BackupSetup *BackupSetup
	// ECBURL je adresa kurzov ECB ("" = nesťahovať); FetchRates ju v testoch nahradí.
	ECBURL string
	// ECBRateURL je séria základnej úrokovej sadzby ECB (CSV) pre úrok
	// z omeškania ("" = nesťahovať, sadzby sa zadávajú ručne).
	ECBRateURL string
	FetchRates func(ctx context.Context, url string) ([]fx.Rate, error)
	// AIBaseURL je adresa Claude API ("" = api.anthropic.com; v testoch falošný server).
	AIBaseURL string

	// Postman vytvorí digitálneho poštára z nastavenia firmy (nil = postman.New;
	// v testoch falošný poštár).
	Postman func(postman.Settings) (postman.Provider, error)
	// Mailer posiela e-maily (nil = SMTP podľa nastavenia firmy).
	Mailer  mail.Sender
	lic     atomic.Pointer[license.License]
	recurMu sync.Mutex // jeden beh pravidelnej fakturácie naraz
	quoteMu sync.Mutex // odpovede zákazníkov na ponuky
	legal   atomic.Pointer[legal.Params]
	// mfaTickets sú rozpracované prihlásenia, ktoré čakajú na kód 2FA.
	mfaTickets mfaTickets
	// oauth: podpisový kľúč a autorizačné kódy OAuth (oauth.go)
	oauth oauthState
	// pins sú odomknuté okná desktopu chráneného PIN-om (security.go)
	pins pinSessions
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
	if !a.Capabilities().CanAddCompany(n) {
		return store.Company{}, a.companyLimitError()
	}
	normalizeCompany(&c)
	if err := validateCompany(c); err != nil {
		return store.Company{}, err
	}
	c, err = a.Store.CreateCompany(ctx, c)
	if err != nil {
		return c, err
	}
	// zakladateľ (prihlásený používateľ) je správcom novej firmy
	if act := auth.ActorFrom(ctx); act.UserID != 0 {
		if err := a.Store.SetMembership(ctx, auth.Membership{UserID: act.UserID, CompanyID: c.ID, Role: auth.RoleAdmin}); err != nil {
			return c, err
		}
	}
	a.record(ctx, c.ID, audit.CatSettings, "create", "company", c.ID, "Založená firma "+c.Name, nil, c)
	return c, nil
}

// UpdateCompany uloží nastavenia firmy.
func (a *App) UpdateCompany(ctx context.Context, c store.Company) (store.Company, error) {
	old, err := a.Store.GetCompany(ctx, c.ID)
	if err != nil {
		return c, err
	}
	normalizeCompany(&c)
	if err := validateCompany(c); err != nil {
		return c, err
	}
	if err := a.Store.UpdateCompany(ctx, c); err != nil {
		return c, err
	}
	a.record(ctx, c.ID, audit.CatSettings, "update", "company", c.ID, "Zmena nastavení firmy", old, c)
	return c, nil
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
	c.TaxOffice = strings.TrimSpace(c.TaxOffice)
	if c.VATPeriod == "" {
		c.VATPeriod = store.VATPeriodMonth
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
	for k, def := range ExtraFormats {
		if f := strings.TrimSpace(c.NumberFormats[k]); f != "" {
			formats[k] = f
		} else {
			formats[k] = def
		}
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
	if c.VATPeriod != store.VATPeriodMonth && c.VATPeriod != store.VATPeriodQuarter {
		add("vatPeriod", "Zdaňovacie obdobie je mesačné alebo štvrťročné")
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
	for _, k := range []string{string(invoice.KindInvoice), string(invoice.KindCreditNote), string(invoice.KindProforma), string(invoice.KindAdvanceReceipt), PurchaseFormatKey, CashInFormatKey, CashOutFormatKey, QuoteFormatKey, ReminderFormatKey} {
		if err := invoice.ValidateFormat(c.NumberFormats[k]); err != nil {
			add("numberFormats."+k, err.Error())
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
	if err := a.applyRate(ctx, &inv); err != nil {
		return inv, err
	}
	err := inv.Recalculate()
	return inv, err
}

// SaveDraft uloží nový alebo upravený koncept.
func (a *App) SaveDraft(ctx context.Context, companyID int64, inv invoice.Invoice) (invoice.Invoice, error) {
	var before *invoice.Invoice
	if inv.ID != 0 {
		old, err := a.Store.GetInvoice(ctx, companyID, inv.ID)
		if err != nil {
			return inv, err
		}
		before = &old
		if old.Status == invoice.StatusIssued || old.Status == invoice.StatusPaid {
			// vystavená, neuhradená faktúra sa dá upraviť (číslo ostane)
			return a.amendIssued(ctx, companyID, old, inv)
		}
		if !old.Editable() {
			return inv, invoice.ErrLocked
		}
		// druh dokladu a odkaz na pôvodnú faktúru sa nedajú zmeniť z klienta,
		// meniť sa dá iba dôvod opravy
		inv.Kind = old.Kind
		inv.Advance = old.Advance // odpočet zálohy nastavuje program
		if old.Correction != nil {
			c := *old.Correction
			if inv.Correction != nil {
				c.Reason = strings.TrimSpace(inv.Correction.Reason)
			}
			inv.Correction = &c
		} else {
			inv.Correction = nil
		}
	} else if inv.Kind == invoice.KindCreditNote || inv.Correction != nil {
		return inv, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "kind",
			Message: "Dobropis sa vytvára z vystavenej faktúry tlačidlom Dobropis"}}}
	} else if inv.Kind == invoice.KindAdvanceReceipt {
		return inv, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "kind",
			Message: "Daňový doklad k prijatej platbe vznikne sám pri úhrade zálohovej faktúry"}}}
	} else {
		inv.Advance = nil
	}
	saved, err := a.saveDraft(ctx, companyID, inv)
	if err != nil {
		return saved, err
	}
	if before == nil {
		a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", saved.ID,
			"Nový koncept "+docName(saved)+" pre "+saved.Customer.Name, nil, saved)
	} else {
		a.record(ctx, companyID, audit.CatInvoices, "update", "invoice", saved.ID,
			"Úprava konceptu "+docName(saved)+" pre "+saved.Customer.Name, before, saved)
	}
	return saved, nil
}

// docName vráti „faktúry FA…“ / „dobropisu DB…“ na popis v audite.
func docName(inv invoice.Invoice) string {
	kind := "faktúry"
	if inv.Kind == invoice.KindCreditNote {
		kind = "dobropisu"
	}
	if inv.Number != "" {
		return kind + " " + inv.Number
	}
	return kind
}

// CreateCreditNote vytvorí koncept dobropisu k vystavenej faktúre.
func (a *App) CreateCreditNote(ctx context.Context, companyID, originalID int64) (invoice.Invoice, error) {
	orig, err := a.Store.GetInvoice(ctx, companyID, originalID)
	if err != nil {
		return orig, err
	}
	cn, err := invoice.NewCreditNote(orig, a.Today())
	if err != nil {
		return cn, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "kind", Message: err.Error()}}}
	}
	saved, err := a.saveDraft(ctx, companyID, cn)
	if err == nil {
		a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", saved.ID,
			"Koncept dobropisu k faktúre "+orig.Number, nil, saved)
	}
	return saved, err
}

// offsetCreditNote započíta vystavený dobropis s neuhradenou časťou pôvodnej
// faktúry (pozri invoice.OffsetCreditNote) a uloží oba doklady.
func (a *App) offsetCreditNote(ctx context.Context, companyID int64, cn *invoice.Invoice) error {
	orig, err := a.Store.GetInvoice(ctx, companyID, cn.Correction.OriginalID)
	if err != nil {
		return err
	}
	amount, err := invoice.OffsetCreditNote(&orig, cn)
	if err != nil || amount.IsZero() {
		return err
	}
	if err := a.Store.SaveInvoice(ctx, companyID, &orig); err != nil {
		return err
	}
	if err := a.Store.SaveInvoice(ctx, companyID, cn); err != nil {
		return err
	}
	state := "zostáva uhradiť " + orig.Remaining().Display() + " €"
	if orig.Status == invoice.StatusPaid {
		state = "faktúra je vyrovnaná"
	}
	a.record(ctx, companyID, audit.CatInvoices, "pay", "invoice", orig.ID,
		"Zápočet dobropisu "+cn.Number+" s faktúrou "+orig.Number+": "+amount.Display()+" €, "+state, nil, orig.Payments)
	return nil
}

// creditNotesOf vráti dobropisy k danej faktúre.
func (a *App) creditNotesOf(ctx context.Context, companyID, originalID int64) ([]invoice.Invoice, error) {
	all, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
	if err != nil {
		return nil, err
	}
	var out []invoice.Invoice
	for _, x := range all {
		if x.Kind == invoice.KindCreditNote && x.Correction != nil && x.Correction.OriginalID == originalID &&
			x.Status != invoice.StatusDraft {
			out = append(out, x)
		}
	}
	return out, nil
}

func (a *App) saveDraft(ctx context.Context, companyID int64, inv invoice.Invoice) (invoice.Invoice, error) {
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
	if err := a.applyRate(ctx, &inv); err != nil {
		return inv, err
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
	if err := a.applyRate(ctx, &inv); err != nil {
		return inv, err
	}
	if err := inv.Recalculate(); err != nil {
		return inv, err
	}
	if err := inv.Validate(); err != nil {
		return inv, err
	}
	if inv.Kind == invoice.KindCreditNote {
		orig, err := a.Store.GetInvoice(ctx, companyID, inv.Correction.OriginalID)
		if err != nil {
			return inv, err
		}
		issued, err := a.creditNotesOf(ctx, companyID, orig.ID)
		if err != nil {
			return inv, err
		}
		if err := invoice.CheckCreditLimit(orig, issued, inv); err != nil {
			return inv, err
		}
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
	if err := a.Store.SaveInvoice(ctx, companyID, &inv); err != nil {
		return inv, err
	}
	what := "Vystavená faktúra "
	if inv.Kind == invoice.KindCreditNote {
		what = "Vystavený dobropis "
	}
	a.record(ctx, companyID, audit.CatInvoices, "issue", "invoice", inv.ID,
		what+inv.Number+" pre "+inv.Customer.Name+", "+inv.Summary.Total.Display()+" €", nil, inv)
	if inv.Kind == invoice.KindCreditNote {
		if err := a.offsetCreditNote(ctx, companyID, &inv); err != nil {
			return inv, err
		}
	}
	if adv := inv.Advance; adv != nil && len(adv.Deductions) > 0 {
		if err := a.linkFinal(ctx, companyID, inv); err != nil {
			return inv, err
		}
		if inv.Summary.Total.IsZero() {
			// celé plnenie uhradené zálohou
			return a.MarkPaid(ctx, companyID, inv.ID, inv.IssueDate)
		}
	}
	return inv, nil
}

// MarkPaid označí faktúru ako uhradenú (doplatí zostatok).
func (a *App) MarkPaid(ctx context.Context, companyID, id int64, on invoice.Date) (invoice.Invoice, error) {
	return a.AddPayment(ctx, companyID, id, invoice.Payment{Date: on, Source: invoice.PaymentManual}, true)
}

// AddPayment pripíše úhradu faktúry (aj čiastočnú). S rest=true sa suma
// doplní na celý zostatok. Pri zálohovej faktúre vznikne ku každej platbe
// daňový doklad k prijatej platbe.
func (a *App) AddPayment(ctx context.Context, companyID, id int64, p invoice.Payment, rest bool) (invoice.Invoice, error) {
	inv, err := a.transition(ctx, companyID, id, "pay", "Úhrada ", func(inv *invoice.Invoice) error {
		if p.Date.IsZero() {
			p.Date = a.Today()
		}
		if rest {
			p.Amount = inv.Remaining()
		}
		return inv.AddPayment(p)
	})
	if err == nil && inv.NeedsReceipt() && p.Amount.Sign() > 0 {
		// prijatá záloha (aj časť): daňová povinnosť vzniká dňom prijatia platby
		inv, err = a.issueAdvanceReceipt(ctx, companyID, inv, p)
	}
	return inv, err
}

// RemovePayment zruší úhradu z banky alebo pokladne (zrušené spárovanie,
// storno pokladničného dokladu). Úhrada spred evidencie úhrad sa zruší celá.
func (a *App) RemovePayment(ctx context.Context, companyID, id int64, source string, ref int64) (invoice.Invoice, error) {
	return a.transition(ctx, companyID, id, "unpay", "Zrušená úhrada ", func(inv *invoice.Invoice) error {
		if err := receiptGuard(inv); err != nil {
			return err
		}
		if inv.RemovePayment(source, ref) {
			return nil
		}
		if len(inv.Payments) == 0 && inv.Status == invoice.StatusPaid {
			return inv.Unpay()
		}
		return invoice.ErrBadTransition
	})
}

func receiptGuard(inv *invoice.Invoice) error {
	if inv.Advance != nil && inv.Advance.ReceiptID != 0 {
		return fmt.Errorf("%w %s", ErrReceiptIssued, inv.Advance.ReceiptNumber)
	}
	if inv.Kind == invoice.KindAdvanceReceipt {
		return ErrReceiptIssued
	}
	return nil
}

// Unpay zruší úhradu faktúry.
func (a *App) Unpay(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	return a.transition(ctx, companyID, id, "unpay", "Zrušená úhrada ", func(inv *invoice.Invoice) error {
		if err := receiptGuard(inv); err != nil {
			return err
		}
		return inv.Unpay()
	})
}

// Cancel stornuje faktúru.
func (a *App) Cancel(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	return a.transition(ctx, companyID, id, "cancel", "Stornovaná ", (*invoice.Invoice).Cancel)
}

func (a *App) transition(ctx context.Context, companyID, id int64, action, what string, fn func(*invoice.Invoice) error) (invoice.Invoice, error) {
	inv, err := a.Store.GetInvoice(ctx, companyID, id)
	if err != nil {
		return inv, err
	}
	type state struct {
		Status   invoice.Status    `json:"status"`
		PaidDate invoice.Date      `json:"paidDate"`
		Paid     money.Dec         `json:"paid"`
		Payments []invoice.Payment `json:"payments,omitempty"`
	}
	before := state{inv.Status, inv.PaidDate, inv.Paid(), inv.Payments}
	if err := fn(&inv); err != nil {
		return inv, err
	}
	if err := a.Store.SaveInvoice(ctx, companyID, &inv); err != nil {
		return inv, err
	}
	a.record(ctx, companyID, audit.CatInvoices, action, "invoice", inv.ID, what+docName(inv), before, state{inv.Status, inv.PaidDate, inv.Paid(), inv.Payments})
	return inv, nil
}
