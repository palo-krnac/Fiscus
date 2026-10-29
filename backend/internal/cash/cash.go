// Package cash je pokladňa: príjmové (PPD) a výdavkové (VPD) pokladničné doklady.
//
// Zostatok pokladne nesmie byť v žiadnom okamihu záporný. Doklad sa nemaže,
// iba stornuje (číslo zostáva v rade). Doklad môže uhrádzať faktúru — vtedy
// sa faktúra označí ako uhradená v hotovosti a storno úhradu zruší.
package cash

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Kind je druh pokladničného dokladu.
type Kind string

const (
	In  Kind = "in"  // príjmový pokladničný doklad (PPD)
	Out Kind = "out" // výdavkový pokladničný doklad (VPD)
)

// Document je pokladničný doklad.
type Document struct {
	ID         int64        `json:"id"`
	CompanyID  int64        `json:"companyId"`
	Number     string       `json:"number"`
	Kind       Kind         `json:"kind"`
	Date       invoice.Date `json:"date"`
	Amount     money.Dec    `json:"amount"`  // vždy kladná
	Partner    string       `json:"partner"` // od koho / komu
	Purpose    string       `json:"purpose"` // účel platby
	Note       string       `json:"note"`
	LinkKind   string       `json:"linkKind"` // "invoice" | "purchase" | ""
	LinkID     int64        `json:"linkId"`
	LinkNumber string       `json:"linkNumber"`
	Cancelled  bool         `json:"cancelled"`
	// Tax je DPH dokladu bez väzby na faktúru (blok, tržba z e-kasy); nil = bez DPH.
	Tax *Tax `json:"tax,omitempty"`
}

// Signed vráti sumu so znamienkom (výdavok záporný).
func (d Document) Signed() money.Dec {
	if d.Kind == Out {
		return d.Amount.Neg()
	}
	return d.Amount
}

// Chyby.
var (
	ErrNegative  = errors.New("Zostatok pokladne by bol záporný")
	ErrCancelled = errors.New("doklad je už stornovaný")
)

// Validate skontroluje doklad.
func (d *Document) Validate() error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if d.Kind != In && d.Kind != Out {
		add("kind", "Vyberte príjem alebo výdavok")
	}
	if d.Date.IsZero() {
		add("date", "Zadajte dátum")
	}
	if d.Amount.Sign() <= 0 {
		add("amount", "Suma musí byť kladná")
	} else if !d.Amount.Equal(d.Amount.RoundCents()) {
		add("amount", "Suma môže mať najviac dve desatinné miesta")
	}
	if strings.TrimSpace(d.Purpose) == "" {
		add("purpose", "Zadajte účel platby")
	}
	if len(fe) == 0 {
		fe = append(fe, d.ValidateTax(d.Date.String())...)
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Balance vráti zostatok platných dokladov.
func Balance(docs []Document) money.Dec {
	var b money.Dec
	for _, d := range docs {
		if !d.Cancelled {
			b = b.Add(d.Signed())
		}
	}
	return b
}

// CheckBalance overí, že po zoradení podľa dátumu (v rámci dňa najprv príjmy)
// zostatok nikdy neklesne pod nulu. Vráti prvý deň, keď by bol záporný.
func CheckBalance(docs []Document) error {
	list := make([]Document, 0, len(docs))
	for _, d := range docs {
		if !d.Cancelled {
			list = append(list, d)
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Date != list[j].Date {
			return list[i].Date.Before(list[j].Date)
		}
		return list[i].Kind == In && list[j].Kind == Out
	})
	var b money.Dec
	for _, d := range list {
		b = b.Add(d.Signed())
		if b.Sign() < 0 {
			return fmt.Errorf("%w (%s € dňa %s)", ErrNegative, b.Display(), d.Date.Display())
		}
	}
	return nil
}
