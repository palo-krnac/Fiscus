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
	VATPayer bool   `json:"vatPayer"`
}

// InvoiceFilter obmedzuje zoznam faktúr.
type InvoiceFilter struct {
	Year   int            // 0 = všetky roky
	Status invoice.Status // "" = všetky
	Search string         // číslo, odberateľ, IČO
}

// Store je úložisko.
type Store interface {
	ListCompanies(ctx context.Context) ([]Company, error)
	CountCompanies(ctx context.Context) (int, error)
	CreateCompany(ctx context.Context, c Company) (Company, error)
	GetCompany(ctx context.Context, id int64) (Company, error)

	ListInvoices(ctx context.Context, companyID int64, f InvoiceFilter) ([]invoice.Invoice, error)
	GetInvoice(ctx context.Context, companyID, id int64) (invoice.Invoice, error)
	// SaveInvoice vloží (ID == 0) alebo prepíše faktúru. Nastaví ID.
	SaveInvoice(ctx context.Context, companyID int64, inv *invoice.Invoice) error
	// NextSequence atomicky zvýši a vráti poradové číslo radu za rok.
	NextSequence(ctx context.Context, companyID int64, series string, year int) (int, error)

	Close() error
}
