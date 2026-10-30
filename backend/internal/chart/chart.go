// Package chart je účtový rozvrh pre podvojné účtovníctvo: syntetické účty
// z rámcovej účtovej osnovy pre podnikateľov (opatrenie MF SR
// č. 23054/2002-92, príloha č. 1), analytické účty firmy, strediská
// a zákazky. Rozvrh je predvyplnený a firma ho upravuje.
package chart

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// Type je druh účtu.
type Type string

const (
	Asset     Type = "A" // aktívny (súvahový)
	Liability Type = "P" // pasívny (súvahový)
	Both      Type = "X" // aktívny aj pasívny podľa zostatku (zúčtovacie vzťahy)
	Expense   Type = "N" // nákladový
	Revenue   Type = "V" // výnosový
	Closing   Type = "Z" // závierkový
	OffBook   Type = "S" // podsúvahový
)

// Types sú druhy účtov s popisom.
var Types = []struct {
	Code  Type   `json:"code"`
	Label string `json:"label"`
}{
	{Asset, "aktívny"}, {Liability, "pasívny"}, {Both, "aktívny / pasívny"},
	{Expense, "nákladový"}, {Revenue, "výnosový"}, {Closing, "závierkový"}, {OffBook, "podsúvahový"},
}

// Valid vráti true pre známy druh.
func (t Type) Valid() bool {
	for _, x := range Types {
		if x.Code == t {
			return true
		}
	}
	return false
}

// Account je účet rozvrhu: syntetický (3 číslice) alebo analytický
// (syntetický účet + 1 až 6 číslic, napr. 221001).
type Account struct {
	Number string `json:"number"`
	Name   string `json:"name"`
	Type   Type   `json:"type"`
	// NonTax: náklad alebo výnos, ktorý nie je daňový (napr. reprezentácia,
	// dary) — pre daňové priznanie.
	NonTax bool `json:"nonTax,omitempty"`
	// Active: účet sa ponúka pri účtovaní (nepoužívané syntetické účty
	// osnovy možno skryť).
	Active bool   `json:"active"`
	Note   string `json:"note,omitempty"`
}

// Synthetic vráti syntetický účet (prvé 3 číslice).
func (a Account) Synthetic() string {
	if len(a.Number) < 3 {
		return a.Number
	}
	return a.Number[:3]
}

// Analytic vráti true pre analytický účet.
func (a Account) Analytic() bool { return len(a.Number) > 3 }

// Class vráti účtovú triedu (0 – 7).
func (a Account) Class() int {
	if a.Number == "" {
		return -1
	}
	return int(a.Number[0] - '0')
}

// Group vráti účtovú skupinu (prvé 2 číslice).
func (a Account) Group() string {
	if len(a.Number) < 2 {
		return a.Number
	}
	return a.Number[:2]
}

// Classes sú názvy účtových tried.
var Classes = []string{
	"Dlhodobý majetok",
	"Zásoby",
	"Finančné účty",
	"Zúčtovacie vzťahy",
	"Kapitálové účty a dlhodobé záväzky",
	"Náklady",
	"Výnosy",
	"Závierkové a podsúvahové účty",
}

// ErrNotFound — účet, stredisko alebo zákazka neexistuje.
var ErrNotFound = errors.New("neexistuje")

// Chart je účtový rozvrh firmy.
type Chart struct {
	Accounts []Account `json:"accounts"`
}

// Default vráti rozvrh predvyplnený rámcovou účtovou osnovou.
func Default() Chart {
	return Chart{Accounts: append([]Account(nil), framework...)}
}

// Sort zoradí účty podľa čísla (analytické za syntetickým).
func (c *Chart) Sort() {
	sort.Slice(c.Accounts, func(i, j int) bool { return c.Accounts[i].Number < c.Accounts[j].Number })
}

// Find vráti účet podľa čísla.
func (c Chart) Find(number string) (Account, bool) {
	for _, a := range c.Accounts {
		if a.Number == number {
			return a, true
		}
	}
	return Account{}, false
}

// Seed doplní syntetické účty osnovy, ktoré v rozvrhu chýbajú (nová verzia
// programu). Vráti počet doplnených.
func (c *Chart) Seed() int {
	have := map[string]bool{}
	for _, a := range c.Accounts {
		have[a.Number] = true
	}
	n := 0
	for _, a := range framework {
		if !have[a.Number] {
			c.Accounts = append(c.Accounts, a)
			n++
		}
	}
	if n > 0 {
		c.Sort()
	}
	return n
}

// NormalizeNumber odstráni z čísla účtu medzery, bodky a pomlčky (221.001 → 221001).
func NormalizeNumber(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Display vráti číslo účtu na zobrazenie: 221001 → „221.001“.
func Display(number string) string {
	if len(number) <= 3 {
		return number
	}
	return number[:3] + "." + number[3:]
}

// Validate overí účet voči rozvrhu (old = pôvodné číslo pri úprave, "" = nový).
func (c Chart) Validate(a *Account, old string) error {
	var fe []invoice.FieldError
	bad := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	a.Number = NormalizeNumber(a.Number)
	a.Name = strings.TrimSpace(a.Name)
	a.Note = strings.TrimSpace(a.Note)
	switch {
	case len(a.Number) < 3 || len(a.Number) > 9:
		bad("number", "Číslo účtu má 3 číslice (syntetický) alebo 4 až 9 (analytický, napr. 221.001)")
	case a.Number[0] > '7':
		bad("number", "Účtové triedy sú 0 až 7 (8 a 9 sú pre vnútropodnikové účtovníctvo)")
	}
	if a.Name == "" || len([]rune(a.Name)) > 120 {
		bad("name", "Zadajte názov účtu (najviac 120 znakov)")
	}
	if len(fe) == 0 {
		if a.Number != old {
			if _, dup := c.Find(a.Number); dup {
				bad("number", "Účet "+Display(a.Number)+" už v rozvrhu je")
			}
		}
		if a.Analytic() {
			syn, ok := c.Find(a.Synthetic())
			if !ok {
				bad("number", "Syntetický účet "+a.Synthetic()+" nie je v rozvrhu")
			} else {
				a.Type = syn.Type // analytický účet preberá druh syntetického
				if !a.NonTax {
					a.NonTax = syn.NonTax && old == ""
				}
			}
		} else if old != "" && a.Number != old {
			bad("number", "Číslo syntetického účtu sa nemení — pridajte nový účet")
		}
	}
	if !a.Type.Valid() {
		bad("type", "Neznámy druh účtu")
	}
	if a.NonTax && a.Class() != 5 && a.Class() != 6 {
		a.NonTax = false
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Save pridá alebo upraví účet (old = pôvodné číslo, "" = nový).
func (c *Chart) Save(a Account, old string) (Account, error) {
	if err := c.Validate(&a, old); err != nil {
		return a, err
	}
	if old == "" {
		c.Accounts = append(c.Accounts, a)
	} else {
		i := c.index(old)
		if i < 0 {
			return a, ErrNotFound
		}
		c.Accounts[i] = a
		if !a.Analytic() {
			// zmena druhu syntetického účtu platí aj pre jeho analytické účty
			for j := range c.Accounts {
				if c.Accounts[j].Analytic() && c.Accounts[j].Synthetic() == a.Number {
					c.Accounts[j].Type = a.Type
				}
			}
		}
	}
	c.Sort()
	return a, nil
}

// ErrInUse — účet má analytické účty alebo je v osnove.
var ErrInUse = errors.New("účet sa nedá zmazať")

// Delete zmaže analytický účet. Syntetický účet osnovy sa iba skryje
// (Active = false), vlastný syntetický bez analytík sa zmaže.
func (c *Chart) Delete(number string) error {
	i := c.index(number)
	if i < 0 {
		return ErrNotFound
	}
	a := c.Accounts[i]
	if !a.Analytic() {
		for _, x := range c.Accounts {
			if x.Analytic() && x.Synthetic() == number {
				return fmt.Errorf("%w: syntetický účet %s má analytické účty", ErrInUse, number)
			}
		}
		if _, inFramework := frameworkIndex[number]; inFramework {
			c.Accounts[i].Active = false
			return nil
		}
	}
	c.Accounts = append(c.Accounts[:i], c.Accounts[i+1:]...)
	return nil
}

func (c Chart) index(number string) int {
	for i, a := range c.Accounts {
		if a.Number == number {
			return i
		}
	}
	return -1
}

// InFramework vráti true pre syntetický účet rámcovej účtovej osnovy.
func InFramework(number string) bool {
	_, ok := frameworkIndex[number]
	return ok
}

var frameworkIndex = func() map[string]int {
	m := map[string]int{}
	for i, a := range framework {
		m[a.Number] = i
	}
	return m
}()
