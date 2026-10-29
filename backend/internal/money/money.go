// Package money implementuje presný desatinný typ pre účtovníctvo.
//
// Dec je číslo s pevnou presnosťou 4 desatinných miest uložené ako int64
// (1.0000 = 10000). Nikdy sa nepoužíva float. Sumy na doklade sa zaokrúhľujú
// na centy (2 miesta) matematicky — polovica od nuly (0.005 → 0.01).
package money

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Scale je počet desatinných miest, ktoré Dec drží interne.
const Scale = 4

const unit int64 = 10000 // 10^Scale

// Dec je desatinné číslo s pevnou presnosťou 4 miest.
type Dec struct{ v int64 }

// Zero je nula.
var Zero = Dec{}

// FromInt vytvorí Dec z celého čísla.
func FromInt(i int64) Dec { return Dec{i * unit} }

// FromCents vytvorí Dec z počtu centov.
func FromCents(c int64) Dec { return Dec{c * (unit / 100)} }

// Raw vráti internú hodnotu (v desaťtisícinách) — pre ukladanie do DB.
func (d Dec) Raw() int64 { return d.v }

// FromRaw vytvorí Dec z internej hodnoty.
func FromRaw(v int64) Dec { return Dec{v} }

// ErrSyntax je chyba pri nesprávnom zápise čísla.
var ErrSyntax = errors.New("money: neplatné číslo")

// Parse prečíta číslo zo zápisu "1234.5", "-0,25", "1 234,50".
// Medzery sa ignorujú, desatinná čiarka aj bodka sú povolené.
// Viac ako 4 desatinné miesta sa zaokrúhli (polovica od nuly).
func Parse(s string) (Dec, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.Replace(s, ",", ".", 1)
	if s == "" {
		return Zero, ErrSyntax
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || strings.ContainsAny(s, "/eE") {
		return Zero, ErrSyntax
	}
	return fromRat(r)
}

// MustParse je Parse, ktorý pri chybe spanikári — len pre testy a konštanty.
func MustParse(s string) Dec {
	d, err := Parse(s)
	if err != nil {
		panic(fmt.Sprintf("money.MustParse(%q): %v", s, err))
	}
	return d
}

func fromRat(r *big.Rat) (Dec, error) {
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt64(unit))
	n := roundHalfAway(scaled.Num(), scaled.Denom())
	if !n.IsInt64() {
		return Zero, errors.New("money: pretečenie")
	}
	return Dec{n.Int64()}, nil
}

// roundHalfAway vydelí num/den a zaokrúhli polovicu od nuly.
func roundHalfAway(num, den *big.Int) *big.Int {
	q, m := new(big.Int).QuoRem(num, den, new(big.Int))
	// |2m| >= |den| → zaokrúhli smerom od nuly
	twice := new(big.Int).Abs(new(big.Int).Mul(m, big.NewInt(2)))
	if twice.Cmp(new(big.Int).Abs(den)) >= 0 {
		if num.Sign()*den.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

func (d Dec) Add(o Dec) Dec { return Dec{d.v + o.v} }
func (d Dec) Sub(o Dec) Dec { return Dec{d.v - o.v} }
func (d Dec) Neg() Dec      { return Dec{-d.v} }

// Mul vynásobí dve čísla, výsledok zaokrúhli na 4 miesta.
func (d Dec) Mul(o Dec) Dec {
	num := new(big.Int).Mul(big.NewInt(d.v), big.NewInt(o.v))
	return Dec{roundHalfAway(num, big.NewInt(unit)).Int64()}
}

// MulRatio vypočíta d * num / den s jediným zaokrúhlením na 4 miesta.
// Používa sa napr. na DPH z ceny s daňou: suma * sadzba / (100 + sadzba).
func (d Dec) MulRatio(num, den Dec) Dec {
	if den.v == 0 {
		panic("money: delenie nulou")
	}
	n := new(big.Int).Mul(big.NewInt(d.v), big.NewInt(num.v))
	return Dec{roundHalfAway(n, big.NewInt(den.v)).Int64()}
}

// DivCents vydelí d číslom den a výsledok zaokrúhli na centy jediným
// zaokrúhlením (polovica od nuly). Používa sa na prepočet cudzej meny na eurá.
func (d Dec) DivCents(den Dec) Dec {
	if den.v == 0 {
		panic("money: delenie nulou")
	}
	const step = unit / 100
	n := new(big.Int).Mul(big.NewInt(d.v), big.NewInt(100))
	return Dec{roundHalfAway(n, big.NewInt(den.v)).Int64() * step}
}

// RoundCents zaokrúhli na 2 desatinné miesta (centy), polovica od nuly.
func (d Dec) RoundCents() Dec {
	const step = unit / 100
	return Dec{roundHalfAway(big.NewInt(d.v), big.NewInt(step)).Int64() * step}
}

// RoundTo zaokrúhli na zadaný počet desatinných miest (0–4).
func (d Dec) RoundTo(places int) Dec {
	if places >= Scale {
		return d
	}
	step := int64(1)
	for i := places; i < Scale; i++ {
		step *= 10
	}
	return Dec{roundHalfAway(big.NewInt(d.v), big.NewInt(step)).Int64() * step}
}

func (d Dec) Cmp(o Dec) int {
	switch {
	case d.v < o.v:
		return -1
	case d.v > o.v:
		return 1
	}
	return 0
}

func (d Dec) Equal(o Dec) bool { return d.v == o.v }
func (d Dec) IsZero() bool     { return d.v == 0 }
func (d Dec) Sign() int        { return d.Cmp(Zero) }

// String vráti zápis s bodkou a 2 až 4 desatinnými miestami ("12.50", "0.1234").
func (d Dec) String() string {
	v := d.v
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	whole, frac := v/unit, v%unit
	fs := fmt.Sprintf("%04d", frac)
	fs = strings.TrimRight(fs, "0")
	for len(fs) < 2 {
		fs += "0"
	}
	return fmt.Sprintf("%s%d.%s", sign, whole, fs)
}

// Display vráti sumu v slovenskom zápise s 2 desatinnými miestami: "1 234,50".
func (d Dec) Display() string {
	s := d.RoundCents().String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	frac = (frac + "00")[:2]
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteRune('\u00a0')
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + frac
	if neg {
		out = "-" + out
	}
	return out
}

// MarshalJSON zapisuje číslo ako reťazec, aby JavaScript nestratil presnosť.
func (d Dec) MarshalJSON() ([]byte, error) { return []byte(`"` + d.String() + `"`), nil }

// UnmarshalJSON prijme reťazec aj číslo.
func (d *Dec) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		*d = Zero
		return nil
	}
	v, err := Parse(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}
