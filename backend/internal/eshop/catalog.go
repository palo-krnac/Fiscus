package eshop

import (
	"context"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Riadenie obchodu z Fiškusu: cenník (produkty a ceny) a zákazníci.
// Platforma, ktorá to vie, implementuje Catalog.

// Product je produkt (alebo variant) obchodu podľa kódu.
type Product struct {
	SKU    string    `json:"sku"`
	Name   string    `json:"name"`
	Net    money.Dec `json:"net"`   // cena bez DPH
	Gross  money.Dec `json:"gross"` // cena s DPH
	Rate   int       `json:"rate"`  // sadzba DPH v %; -1 = neznáma
	Active bool      `json:"active"`
	// Create: ak ho obchod nemá, založiť ho (inak ErrNoSKU).
	Create bool `json:"create"`
}

// Catalog je obchod, ktorý vie vydať a prijať cenník a zákazníkov.
type Catalog interface {
	// Products vráti produkty a varianty s kódom.
	Products(ctx context.Context) ([]Product, error)
	// Customers vráti registrovaných zákazníkov.
	Customers(ctx context.Context) ([]Address, error)
	// PushProduct upraví produkt s kódom p.SKU. Ak ho obchod nemá a p.Create,
	// založí ho ako koncept (bez fotiek a popisu ho obchod nemá hneď
	// zobraziť); inak vráti ErrNoSKU.
	PushProduct(ctx context.Context, p Product) (created bool, err error)
}

// MaxProducts je najviac produktov načítaných z obchodu.
const MaxProducts = 20000
