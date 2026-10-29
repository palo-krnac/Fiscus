package invoice

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Formát čísla dokladu (číselný rad):
//
//	{RRRR} rok štvorciferne   {RR} rok dvojciferne   {MM} mesiac vystavenia
//	{NNNN} poradové číslo doplnené nulami na počet N (1 až 8)
//
// Príklady: "FA{RRRR}{NNNN}" → FA20260007, "{RRRR}/{NNN}" → 2026/007,
// "DB-{RR}-{NNNNN}" → DB-26-00012. Poradie sa čísluje od 1 každý rok.

// DefaultFormats sú predvolené formáty podľa druhu dokladu.
var DefaultFormats = map[Kind]string{
	KindInvoice:    "FA{RRRR}{NNNN}",
	KindCreditNote: "DB{RRRR}{NNNN}",
	KindProforma:   "ZF{RRRR}{NNNN}",
	// daňový doklad k prijatej platbe
	KindAdvanceReceipt: "ZD{RRRR}{NNNN}",
}

var (
	seqToken   = regexp.MustCompile(`\{N{1,8}\}`)
	anyToken   = regexp.MustCompile(`\{[^}]*\}`)
	knownToken = regexp.MustCompile(`^\{(RRRR|RR|MM|N{1,8})\}$`)
	allowed    = regexp.MustCompile(`^[A-Za-z0-9/\-_.{}]+$`)
)

// ValidateFormat overí formát číselného radu.
func ValidateFormat(f string) error {
	if f == "" {
		return errors.New("Zadajte formát čísla")
	}
	if len(f) > 30 {
		return errors.New("Formát je príliš dlhý")
	}
	if !allowed.MatchString(f) {
		return errors.New("Povolené sú písmená bez diakritiky, číslice a znaky / - _ .")
	}
	for _, t := range anyToken.FindAllString(f, -1) {
		if !knownToken.MatchString(t) {
			return fmt.Errorf("Neznámy zástupný znak %s (povolené {RRRR}, {RR}, {MM}, {NNNN})", t)
		}
	}
	if n := len(seqToken.FindAllString(f, -1)); n != 1 {
		return errors.New("Formát musí obsahovať práve jedno poradové číslo, napr. {NNNN}")
	}
	// rad sa čísluje každý rok od 1 — bez roka by čísla v ďalšom roku kolidovali
	if !strings.Contains(f, "{RRRR}") && !strings.Contains(f, "{RR}") {
		return errors.New("Formát musí obsahovať rok ({RRRR} alebo {RR}), inak by sa čísla každý rok opakovali")
	}
	return nil
}

// FormatNumberWith zloží číslo dokladu podľa formátu.
func FormatNumberWith(f string, year, month, seq int) string {
	out := seqToken.ReplaceAllStringFunc(f, func(t string) string {
		return fmt.Sprintf("%0*d", len(t)-2, seq)
	})
	return strings.NewReplacer(
		"{RRRR}", fmt.Sprintf("%04d", year),
		"{RR}", fmt.Sprintf("%02d", year%100),
		"{MM}", fmt.Sprintf("%02d", month),
	).Replace(out)
}

// VariableSymbol odvodí variabilný symbol z čísla dokladu: iba číslice,
// najviac 10 (pri dlhšom čísle sa ponechá posledných 10).
func VariableSymbol(number string) string {
	d := digitsOnly(number)
	if len(d) > 10 {
		d = d[len(d)-10:]
	}
	return d
}
