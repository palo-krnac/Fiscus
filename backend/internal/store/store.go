// Package store definuje úložisko dát. Biznis pravidlá tu nie sú — tie sú
// v balíku app. Implementácie: memstore (testy, demo) a sqlite.
//
// Každá metóda s dátami firmy dostáva companyID a implementácia MUSÍ
// vracať iba dáta tejto firmy. Overuje to spoločná sada testov
// storetest.Run, ktorú musí prejsť každá implementácia.
package store

import (
	"context"
	"errors"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
)

// ErrNotFound — záznam neexistuje (alebo patrí inej firme).
var ErrNotFound = errors.New("záznam neexistuje")

// ErrDuplicate — záznam s rovnakým jedinečným kľúčom už existuje (napr. e-mail).
var ErrDuplicate = errors.New("záznam už existuje")

// Company je účtovná jednotka (firma alebo živnostník).
type Company struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Street   string `json:"street"`
	City     string `json:"city"`
	Zip      string `json:"zip"`
	ICO      string `json:"ico"`
	DIC      string `json:"dic"`
	ICDPH    string `json:"icdph"`
	IBAN     string `json:"iban"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	VATPayer bool   `json:"vatPayer"`
	// DefaultDueDays je predvolená splatnosť nových faktúr v dňoch.
	DefaultDueDays int `json:"defaultDueDays"`
	// Units sú merné jednotky ponúkané pri položkách (číselník firmy).
	Units []string `json:"units"`
	// NumberFormats sú formáty číselných radov podľa druhu dokladu
	// ("invoice", "credit_note", "proforma"); chýbajúci = predvolený.
	NumberFormats map[string]string `json:"numberFormats"`
	// TaxOffice je daňový úrad (hlavička priznania k DPH).
	TaxOffice string `json:"taxOffice"`
	// VATPeriod je zdaňovacie obdobie DPH: "month" alebo "quarter".
	VATPeriod string `json:"vatPeriod"`
}

// Zdaňovacie obdobia DPH.
const (
	VATPeriodMonth   = "month"
	VATPeriodQuarter = "quarter"
)

// DefaultUnits je predvolený číselník merných jednotiek.
var DefaultUnits = []string{"ks", "hod", "m", "m²", "m³", "kg", "t", "l", "km", "bal", "súb", "deň", "mes", "paušál"}

// Contact je záznam v adresári (odberateľ alebo dodávateľ).
type Contact struct {
	ID        int64  `json:"id"`
	CompanyID int64  `json:"companyId"`
	Name      string `json:"name"`
	Street    string `json:"street"`
	City      string `json:"city"`
	Zip       string `json:"zip"`
	Country   string `json:"country"`
	ICO       string `json:"ico"`
	DIC       string `json:"dic"`
	ICDPH     string `json:"icdph"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	IBAN      string `json:"iban"`
	// DueDays prepíše predvolenú splatnosť firmy pre tohto odberateľa (0 = nie).
	DueDays int    `json:"dueDays"`
	Note    string `json:"note"`
}

// InvoiceFilter obmedzuje zoznam faktúr.
type InvoiceFilter struct {
	Year   int            // 0 = všetky roky
	Status invoice.Status // "" = všetky
	Search string         // číslo, odberateľ, IČO
}

// PurchaseFilter obmedzuje zoznam prijatých faktúr.
type PurchaseFilter struct {
	Year   int    // podľa dátumu doručenia; 0 = všetky
	Search string // interné číslo, číslo dodávateľa, dodávateľ, IČO
}

// BankFilter obmedzuje zoznam bankových pohybov.
type BankFilter struct {
	Year   int         // podľa dátumu zaúčtovania; 0 = všetky
	Status bank.Status // "" = všetky
}

// Logo je obrázok loga firmy.
type Logo struct {
	ContentType string
	Data        []byte
	Updated     time.Time
}

// Email je záznam odoslaného (alebo neúspešne odosielaného) e-mailu.
type Email struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"companyId"`
	DocKind   string    `json:"docKind"` // invoice | quote | reminder | test
	DocID     int64     `json:"docId"`
	DocNumber string    `json:"docNumber"`
	To        string    `json:"to"` // adresy oddelené čiarkou
	Cc        string    `json:"cc"`
	Subject   string    `json:"subject"`
	File      string    `json:"file"` // názov prílohy
	Sent      time.Time `json:"sent"`
	User      string    `json:"user"`  // kto poslal; „automaticky“ pri pravidelnej faktúre
	Error     string    `json:"error"` // prázdne = odoslané
}

// EInvoice je záznam e-faktúry odoslanej cez digitálneho poštára alebo
// prijatej z jeho schránky (prijatá aj s obsahom XML, aby sa dala načítať znova).
type EInvoice struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"companyId"`
	Direction string    `json:"direction"` // out | in
	DocID     int64     `json:"docId"`     // vydaná faktúra (out) alebo prijatá faktúra po zaevidovaní (in)
	DocNumber string    `json:"docNumber"` // číslo faktúry
	Partner   string    `json:"partner"`   // odberateľ / dodávateľ
	PeppolID  string    `json:"peppolId"`  // 0245:DIČ protistrany
	File      string    `json:"file"`
	MessageID string    `json:"messageId"` // identifikátor u poštára (súbor v priečinku)
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	Created   time.Time `json:"created"`
	User      string    `json:"user"`
	Data      []byte    `json:"-"` // XML (iba prijaté)
}

// EInvoiceFilter obmedzuje zoznam e-faktúr.
type EInvoiceFilter struct {
	Direction string // "" = obe
	DocID     int64  // 0 = všetky
	MessageID string // "" = všetky
}

// EmailFilter obmedzuje zoznam e-mailov.
type EmailFilter struct {
	Year    int    // 0 = všetky
	DocKind string // "" = všetky
	DocID   int64  // 0 = všetky (iba s DocKind)
}

// Store je úložisko.
type Store interface {
	ListCompanies(ctx context.Context) ([]Company, error)
	CountCompanies(ctx context.Context) (int, error)
	CreateCompany(ctx context.Context, c Company) (Company, error)
	GetCompany(ctx context.Context, id int64) (Company, error)
	UpdateCompany(ctx context.Context, c Company) error

	// ListContacts vráti adresár firmy zoradený podľa názvu; search filtruje
	// názov, IČO a obec (bez ohľadu na veľkosť písmen a diakritiku).
	ListContacts(ctx context.Context, companyID int64, search string) ([]Contact, error)
	GetContact(ctx context.Context, companyID, id int64) (Contact, error)
	// SaveContact vloží (ID == 0) alebo prepíše kontakt. Nastaví ID.
	SaveContact(ctx context.Context, companyID int64, c *Contact) error
	DeleteContact(ctx context.Context, companyID, id int64) error

	// ListItems vráti cenník firmy zoradený podľa názvu.
	ListItems(ctx context.Context, companyID int64) ([]catalog.Item, error)
	GetItem(ctx context.Context, companyID, id int64) (catalog.Item, error)
	// SaveItem vloží (ID == 0) alebo prepíše položku. Rovnaký neprázdny kód
	// v jednej firme → ErrDuplicate.
	SaveItem(ctx context.Context, companyID int64, it *catalog.Item) error
	DeleteItem(ctx context.Context, companyID, id int64) error

	// Kurzy mien sú spoločné pre všetky firmy. SaveRates vloží alebo prepíše
	// kurzy; kurz z ECB neprepíše ručne zadaný. RateBefore vráti posledný kurz
	// meny vyhlásený pred dňom taxPoint (ErrNotFound, ak nie je). ListRates
	// vráti najnovšie prvé.
	SaveRates(ctx context.Context, rates []fx.Rate) error
	RateBefore(ctx context.Context, currency, taxPoint string) (fx.Rate, error)
	ListRates(ctx context.Context, f fx.Filter) ([]fx.Rate, error)
	DeleteRate(ctx context.Context, currency, date string) error

	// AddEmail zapíše odoslaný e-mail (nastaví ID). ListEmails vráti
	// najnovšie prvé.
	AddEmail(ctx context.Context, companyID int64, e *Email) error
	ListEmails(ctx context.Context, companyID int64, f EmailFilter) ([]Email, error)
	// AddEInvoice zapíše odoslanú alebo prijatú e-faktúru (nastaví ID).
	// ListEInvoices vráti záznamy bez obsahu XML, najnovšie prvé;
	// GetEInvoice vráti záznam aj s obsahom.
	AddEInvoice(ctx context.Context, companyID int64, e *EInvoice) error
	ListEInvoices(ctx context.Context, companyID int64, f EInvoiceFilter) ([]EInvoice, error)
	GetEInvoice(ctx context.Context, companyID, id int64) (EInvoice, error)

	// Logo firmy na dokladoch. GetLogo a DeleteLogo vrátia ErrNotFound, ak logo nie je.
	GetLogo(ctx context.Context, companyID int64) (Logo, error)
	SaveLogo(ctx context.Context, companyID int64, l Logo) error
	DeleteLogo(ctx context.Context, companyID int64) error

	ListInvoices(ctx context.Context, companyID int64, f InvoiceFilter) ([]invoice.Invoice, error)
	GetInvoice(ctx context.Context, companyID, id int64) (invoice.Invoice, error)
	// SaveInvoice vloží (ID == 0) alebo prepíše faktúru. Nastaví ID.
	SaveInvoice(ctx context.Context, companyID int64, inv *invoice.Invoice) error
	// ListPurchases vráti prijaté faktúry, najnovšie (podľa doručenia) prvé.
	ListPurchases(ctx context.Context, companyID int64, f PurchaseFilter) ([]purchase.Purchase, error)
	GetPurchase(ctx context.Context, companyID, id int64) (purchase.Purchase, error)
	// SavePurchase vloží (ID == 0) alebo prepíše prijatú faktúru. Nastaví ID.
	SavePurchase(ctx context.Context, companyID int64, p *purchase.Purchase) error
	DeletePurchase(ctx context.Context, companyID, id int64) error

	// SaveBankTransactions vloží nové pohyby; pohyby s už známym odtlačkom
	// (opakovaný import) preskočí. Vráti vložené pohyby (s ID) a počet preskočených.
	SaveBankTransactions(ctx context.Context, companyID int64, txs []bank.Transaction) ([]bank.Transaction, int, error)
	// ListBankTransactions vráti pohyby, najnovšie prvé.
	ListBankTransactions(ctx context.Context, companyID int64, f BankFilter) ([]bank.Transaction, error)
	GetBankTransaction(ctx context.Context, companyID, id int64) (bank.Transaction, error)
	// UpdateBankTransaction uloží stav, spárovanie a poznámku pohybu.
	UpdateBankTransaction(ctx context.Context, companyID int64, t bank.Transaction) error

	// ListCashDocuments vráti pokladničné doklady (year 0 = všetky), najnovšie prvé.
	ListCashDocuments(ctx context.Context, companyID int64, year int) ([]cash.Document, error)
	GetCashDocument(ctx context.Context, companyID, id int64) (cash.Document, error)
	// SaveCashDocument vloží (ID == 0) alebo prepíše doklad. Nastaví ID.
	SaveCashDocument(ctx context.Context, companyID int64, d *cash.Document) error

	// ListQuotes vráti cenové ponuky (year 0 = všetky, podľa dátumu ponuky), najnovšie prvé.
	ListQuotes(ctx context.Context, companyID int64, year int) ([]quote.Quote, error)
	GetQuote(ctx context.Context, companyID, id int64) (quote.Quote, error)
	// SaveQuote vloží (ID == 0) alebo prepíše ponuku. Nastaví ID.
	SaveQuote(ctx context.Context, companyID int64, q *quote.Quote) error

	// ListRecurring vráti šablóny pravidelnej fakturácie firmy zoradené podľa ID.
	ListRecurring(ctx context.Context, companyID int64) ([]recurring.Template, error)
	GetRecurring(ctx context.Context, companyID, id int64) (recurring.Template, error)
	// SaveRecurring vloží (ID == 0) alebo prepíše šablónu. Nastaví ID.
	SaveRecurring(ctx context.Context, companyID int64, t *recurring.Template) error
	DeleteRecurring(ctx context.Context, companyID, id int64) error

	// ListReminders vráti upomienky (year 0 = všetky), najnovšie prvé.
	ListReminders(ctx context.Context, companyID int64, year int) ([]reminder.Reminder, error)
	GetReminder(ctx context.Context, companyID, id int64) (reminder.Reminder, error)
	// SaveReminder vloží (ID == 0) alebo prepíše upomienku. Nastaví ID.
	SaveReminder(ctx context.Context, companyID int64, r *reminder.Reminder) error
	DeleteReminder(ctx context.Context, companyID, id int64) error

	// Nastavenia programu spoločné pre všetky firmy (napr. legislatíva), hodnota
	// je JSON. GetSetting vráti ErrNotFound, ak kľúč ešte nie je uložený.
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	// ---- používatelia a prístupy ----
	CreateUser(ctx context.Context, u auth.User) (auth.User, error) // ErrDuplicate pri rovnakom e-maile
	GetUser(ctx context.Context, id int64) (auth.User, error)
	GetUserByEmail(ctx context.Context, email string) (auth.User, error) // e-mail bez ohľadu na veľkosť písmen
	UpdateUser(ctx context.Context, u auth.User) error
	ListUsers(ctx context.Context) ([]auth.User, error)
	SetMembership(ctx context.Context, m auth.Membership) error // vloží alebo zmení rolu
	RemoveMembership(ctx context.Context, userID, companyID int64) error
	UserMemberships(ctx context.Context, userID int64) ([]auth.Membership, error)
	CompanyMembers(ctx context.Context, companyID int64) ([]auth.Membership, error)
	CreateSession(ctx context.Context, s auth.Session) error
	GetSession(ctx context.Context, tokenHash string) (auth.Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUserSessions(ctx context.Context, userID int64) error

	// ---- API kľúče (AI asistenti cez MCP) ----
	CreateAPIKey(ctx context.Context, k *auth.APIKey) error               // nastaví ID
	ListAPIKeys(ctx context.Context, userID int64) ([]auth.APIKey, error) // kľúče používateľa, najnovšie prvé
	GetAPIKeyByHash(ctx context.Context, hash string) (auth.APIKey, error)
	GetAPIKey(ctx context.Context, id int64) (auth.APIKey, error)
	// RotateAPIKey vymení odtlačok, iba ak je stále oldHash (obnovovací token
	// OAuth); inak ErrNotFound — starý token bol už použitý.
	RotateAPIKey(ctx context.Context, id int64, oldHash, newHash string) error
	TouchAPIKey(ctx context.Context, id int64, at int64) error // čas posledného použitia
	RevokeAPIKey(ctx context.Context, userID, id int64) error  // ErrNotFound pre cudzí kľúč

	// ---- audit (iba pridávanie, nikdy úprava ani mazanie) ----
	// AppendAudit atomicky pridá záznam na koniec reťazca (nastaví ID, PrevHash, Hash).
	AppendAudit(ctx context.Context, e *audit.Entry) error
	// ListAudit vráti záznamy podľa filtra, najnovšie prvé, a celkový počet.
	ListAudit(ctx context.Context, f audit.Filter) ([]audit.Entry, int, error)
	// AuditChain vráti celý reťazec od najstaršieho (na overenie integrity).
	AuditChain(ctx context.Context) ([]audit.Entry, error)

	// NextSequence atomicky zvýši a vráti poradové číslo radu za rok.
	NextSequence(ctx context.Context, companyID int64, series string, year int) (int, error)

	Close() error
}
