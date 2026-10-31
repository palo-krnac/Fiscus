// Package omega číta textový export z programu KROS Omega („Export do
// textového súboru“): partnerov (T04), skladové karty (T03), fakturáciu
// (T01) a doklady EUD (T00, odoslané a došlé faktúry s úhradami).
//
// Formát podľa špecifikácie KROS (ImportExport, verzia 20.60): každý
// riadok je veta R00 (typ údajov Txx), R01 (hlavička) alebo R02 (položka),
// hodnoty sú oddelené tabulátorom, kódovanie Windows-1250, dátumy
// DD.MM.RRRR. Stĺpce sa číslujú od 1 ako v špecifikácii (A = 1 = R01).
package omega

import (
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/table"
)

// Rec je veta: hodnoty podľa čísla stĺpca zo špecifikácie (1 = R01…).
type Rec []string

// Col vráti hodnotu stĺpca n (číslovanie od 1), prázdnu ak chýba.
func (r Rec) Col(n int) string {
	if n < 1 || n > len(r) {
		return ""
	}
	return strings.TrimSpace(r[n-1])
}

// Doc je hlavička dokladu (R01) s položkami (R02).
type Doc struct {
	Line  int
	Head  Rec
	Items []Rec
}

// Section je časť súboru jedného typu údajov (R00 Txx).
type Section struct {
	Type string // T00, T01, T03, T04…
	Docs []Doc
}

// File je načítaný export.
type File struct {
	Sections []Section
}

// Docs vráti doklady všetkých častí daného typu.
func (f File) Docs(typ string) []Doc {
	var out []Doc
	for _, s := range f.Sections {
		if s.Type == typ {
			out = append(out, s.Docs...)
		}
	}
	return out
}

// ErrNotOmega — súbor nie je textový export z Omegy.
var ErrNotOmega = errors.New("súbor nie je textový export z programu KROS Omega (prvý riadok má začínať R00)")

// Parse prečíta export.
func Parse(data []byte) (File, error) {
	text := table.Text(data)
	var f File
	var cur *Section
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		rec := Rec(strings.Split(line, "\t"))
		switch rec.Col(1) {
		case "R00":
			typ := strings.ToUpper(rec.Col(2))
			if typ == "T020" {
				typ = "T20"
			}
			f.Sections = append(f.Sections, Section{Type: typ})
			cur = &f.Sections[len(f.Sections)-1]
		case "R01":
			if cur == nil {
				return f, ErrNotOmega
			}
			cur.Docs = append(cur.Docs, Doc{Line: i + 1, Head: rec})
		case "R02", "R03":
			if cur == nil {
				return f, ErrNotOmega
			}
			if len(cur.Docs) == 0 {
				return f, fmt.Errorf("riadok %d: položka bez hlavičky dokladu", i+1)
			}
			if rec.Col(1) == "R02" {
				d := &cur.Docs[len(cur.Docs)-1]
				d.Items = append(d.Items, rec)
			}
		default:
			if cur == nil {
				return f, ErrNotOmega
			}
			return f, fmt.Errorf("riadok %d: neznáma veta %q (očakáva sa R00, R01, R02 alebo R03)", i+1, truncate(rec.Col(1), 10))
		}
	}
	if len(f.Sections) == 0 {
		return f, ErrNotOmega
	}
	return f, nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// Stĺpce viet podľa špecifikácie KROS (číslovanie od 1).
const (
	// T04 Partneri, R01
	PName     = 2
	PICO      = 3
	PStreet   = 4
	PZip      = 5
	PCity     = 6
	PCountry  = 8
	PCode     = 9
	PVATPayer = 13 // T/F
	PEmail    = 14
	PNote     = 23
	PVATCode  = 25 // kód štátu IČ DPH (SK)
	PICDPH    = 26
	PDIC      = 27

	// T03 Skladové karty, R02 (karta)
	SName     = 2
	SNumber   = 3
	SUnit     = 4
	SEAN      = 5
	SRate     = 11 // 0/N/V
	SNote     = 17
	SPrice    = 32 // predvolená predajná cena bez DPH
	SPriceVAT = 33

	// T01 Fakturácia, R01
	FNumber     = 2
	FPartner    = 3
	FICO        = 4
	FIssued     = 5 // vystavenie, pri došlých prijatie
	FDue        = 6
	FTaxDate    = 7 // DUZP
	FBaseLow    = 8
	FBaseHigh   = 9
	FBase0      = 10
	FBaseFree   = 11
	FRateLow    = 12
	FRateHigh   = 13
	FVATLow     = 14
	FVATHigh    = 15
	FRounding   = 16
	FKind       = 18 // 0 odberateľská faktúra, 4 dobropis, 14 došlá faktúra…
	FStreet     = 25
	FZip        = 26
	FCity       = 27
	FDIC        = 28
	FCurrency   = 40
	FTotal      = 43 // suma spolu v tuzemskej mene
	FNote       = 45
	FSubject    = 46
	FCountry    = 47
	FVATCode    = 48
	FICDPH      = 49
	FIssuedDF   = 69 // dátum vystavenia došlej faktúry
	FVarSymbol  = 71
	FPhone      = 84
	FItemName   = 2 // R02
	FItemQty    = 3
	FItemUnit   = 4
	FItemPrice  = 5 // jednotková cena bez DPH
	FItemRate   = 6 // 0/N/V
	FItemDisc   = 9 // % zľava
	FItemCode   = 18
	FItemGross  = 50 // -1 = ceny zadané s DPH
	FItemPriceV = 51 // jednotková cena s DPH

	// T00 Doklady EUD, R01
	EKind       = 2 // 100 OF, 130 DF…
	EInternal   = 5
	EExternal   = 6
	EPartner    = 7
	EICO        = 8
	EDIC        = 9
	EIssued     = 10
	EReceived   = 11
	EDue        = 12
	ETaxDate    = 13
	ECurrency   = 15
	ETotal      = 20 // suma spolu v tuzemskej mene
	ERateLow    = 21
	ERateHigh   = 22
	EBaseLow    = 23
	EBaseHigh   = 24
	EBase0      = 25
	EBaseFree   = 26
	EVATLow     = 27
	EVATHigh    = 28
	ENote       = 40
	EText       = 41
	EVATCode    = 44
	EICDPH      = 45
	EOriginal   = 55 // čísla pôvodných dokladov (dobropis)
	EItemType   = 2  // R02: 0 účtovný zápis, 1 úhrada, 2 odpočet
	EPayDate    = 3
	EPayAmount  = 4
	EPayDocCode = 14
)
