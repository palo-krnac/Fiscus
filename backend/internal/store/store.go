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

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
)

// ErrNotFound — záznam neexistuje (alebo patrí inej firme).
var ErrNotFound = errors.New("záznam neexistuje")

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
}

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

	// NextSequence atomicky zvýši a vráti poradové číslo radu za rok.
	NextSequence(ctx context.Context, companyID int64, series string, year int) (int, error)

	Close() error
}
