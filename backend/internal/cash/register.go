package cash

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// Register je pokladňa firmy. Hlavná pokladňa má ID 0, je v eurách a
// jej doklady majú pôvodné rady PPD/VPD. Ďalšie pokladne (napr. v
// prevádzke alebo v cudzej mene) majú vlastnú predponu radov.
type Register struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"` // EUR, CZK, USD…
	// Prefix je predpona čísiel dokladov ďalšej pokladne (napr. „CZ“ → CZPPD20260001).
	Prefix   string `json:"prefix"`
	Archived bool   `json:"archived"`
}

// MainName je predvolený názov hlavnej pokladne (aj v peňažnom denníku).
const MainName = "Pokladnica"

// Main je hlavná pokladňa.
func Main() Register { return Register{ID: 0, Name: MainName, Currency: "EUR"} }

// Foreign vráti true pre pokladňu v cudzej mene.
func (r Register) Foreign() bool { return r.Currency != "" && r.Currency != "EUR" }

// ErrRegisterUsed — pokladňu s dokladmi nemožno zmeniť na inú menu ani zmazať.
var ErrRegisterUsed = errors.New("pokladňa už má doklady — menu ani predponu nemožno zmeniť, pokladňu môžete archivovať")

// Normalize upraví texty a predvolené hodnoty.
func (r *Register) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Prefix = strings.ToUpper(strings.TrimSpace(r.Prefix))
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
	if r.Currency == "" {
		r.Currency = "EUR"
	}
	if r.ID == 0 {
		r.Currency, r.Prefix, r.Archived = "EUR", "", false
		if r.Name == "" {
			r.Name = MainName
		}
	}
}

// ValidateRegisters skontroluje zoznam pokladní (vrátane hlavnej).
func ValidateRegisters(list []Register) error {
	var fe []invoice.FieldError
	add := func(i int, f, m string) {
		fe = append(fe, invoice.FieldError{Field: fmt.Sprintf("registers[%d].%s", i, f), Message: m})
	}
	names, prefixes := map[string]bool{}, map[string]bool{}
	for i, r := range list {
		if r.Name == "" {
			add(i, "name", "Zadajte názov pokladne")
		} else if names[strings.ToLower(r.Name)] {
			add(i, "name", "Pokladňa s týmto názvom už je")
		}
		names[strings.ToLower(r.Name)] = true
		if !fx.Known(r.Currency) && r.Currency != "EUR" {
			add(i, "currency", "Neznáma mena")
		}
		if r.ID == 0 {
			continue
		}
		switch {
		case r.Prefix == "":
			add(i, "prefix", "Zadajte predponu čísiel dokladov (napr. P2 alebo CZ)")
		case len(r.Prefix) > 6 || strings.IndexFunc(r.Prefix, func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsDigit(c) }) >= 0:
			add(i, "prefix", "Predpona môže mať najviac 6 písmen a číslic")
		case r.Prefix == "PPD" || r.Prefix == "VPD" || prefixes[r.Prefix]:
			add(i, "prefix", "Predponu "+r.Prefix+" už používa iná pokladňa")
		}
		prefixes[r.Prefix] = true
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}
