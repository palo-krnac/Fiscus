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
	// RegisterID je pokladňa (0 = hlavná). Doklad pokladne v cudzej mene má
	// sumu v mene pokladne a kurz ECB ku dňu dokladu; AmountEUR je suma
	// v eurách pre peňažný denník.
	RegisterID int64        `json:"registerId"`
	Currency   string       `json:"currency"`
	FXRate     money.Dec    `json:"fxRate"`
	FXDate     invoice.Date `json:"fxDate"`
	FXManual   bool         `json:"fxManual"`
	AmountEUR  money.Dec    `json:"amountEur"`
}

// Foreign vráti true pre doklad v cudzej mene.
func (d Document) Foreign() bool { return d.Currency != "" && d.Currency != "EUR" }

// SyncEUR prepočíta sumu v eurách (v eurách = suma dokladu; v cudzej mene
// suma / kurz, nula bez kurzu).
func (d *Document) SyncEUR() {
	if d.Currency == "" {
		d.Currency = "EUR"
	}
	if !d.Foreign() {
		d.FXRate, d.FXDate, d.FXManual = money.Zero, invoice.Date{}, false
		d.AmountEUR = d.Amount
		return
	}
	d.AmountEUR = money.Zero
	if d.FXRate.Sign() > 0 {
		d.AmountEUR = d.Amount.DivCents(d.FXRate)
	}
}

// SignedEUR vráti sumu v eurách so znamienkom (výdavok záporný).
func (d Document) SignedEUR() money.Dec {
	if d.Kind == Out {
		return d.AmountEUR.Neg()
	}
	return d.AmountEUR
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
	if d.Foreign() {
		if d.FXRate.Sign() <= 0 {
			add("fxRate", "Chýba kurz ECB ku dňu dokladu — aktualizujte kurzy alebo zadajte kurz ručne")
		}
		if d.Tax != nil {
			add("tax", "DPH sa na doklade v cudzej mene neuvádza — zahraničný blok nie je doklad na odpočet v SR")
		}
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
