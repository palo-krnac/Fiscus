// Package vat počíta DPH podľa slovenských pravidiel.
//
// Sadzby od 1. 1. 2025: 23 % základná, 19 % a 5 % znížené, 0 % (oslobodené
// plnenie alebo prenesenie daňovej povinnosti). DPH sa počíta zo súčtu
// základov za každú sadzbu a zaokrúhľuje na centy — tak, ako ju
// zobrazuje rekapitulácia na faktúre.
package vat

import (
	"fmt"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Rate je sadzba DPH v percentách.
type Rate int

const (
	Rate0  Rate = 0
	Rate5  Rate = 5
	Rate19 Rate = 19
	Rate23 Rate = 23
)

// Valid vráti true pre sadzby platné na Slovensku od 2025.
func (r Rate) Valid() bool {
	switch r {
	case Rate0, Rate5, Rate19, Rate23:
		return true
	}
	return false
}

// PriceMode určuje, či sú jednotkové ceny zadané bez DPH alebo s DPH.
type PriceMode string

const (
	PriceNet   PriceMode = "net"   // ceny bez DPH — DPH sa pripočíta
	PriceGross PriceMode = "gross" // ceny s DPH — DPH sa vypočíta „zhora“
)

// Line je jedna položka pre výpočet DPH.
type Line struct {
	Quantity  money.Dec
	UnitPrice money.Dec
	Rate      Rate
}

// RateSummary je jeden riadok rekapitulácie DPH.
type RateSummary struct {
	Rate  Rate      `json:"rate"`
	Base  money.Dec `json:"base"`
	VAT   money.Dec `json:"vat"`
	Total money.Dec `json:"total"`
}

// Summary je výsledok výpočtu dokladu.
type Summary struct {
	Rates []RateSummary `json:"rates"`
	Base  money.Dec     `json:"base"`
	VAT   money.Dec     `json:"vat"`
	Total money.Dec     `json:"total"`
}

// LineAmount vráti sumu položky (množstvo × cena) zaokrúhlenú na centy.
func LineAmount(l Line) money.Dec { return l.Quantity.Mul(l.UnitPrice).RoundCents() }

// Calculate spočíta rekapituláciu DPH podľa sadzieb.
//
// reverseCharge = prenesenie daňovej povinnosti (§ 69 ods. 12 zákona o DPH):
// faktúra sa vystaví bez DPH, daň priznáva odberateľ.
func Calculate(lines []Line, mode PriceMode, reverseCharge bool) (Summary, error) {
	sums := map[Rate]money.Dec{}
	for i, l := range lines {
		if !l.Rate.Valid() {
			return Summary{}, fmt.Errorf("položka %d: neplatná sadzba DPH %d %%", i+1, l.Rate)
		}
		r := l.Rate
		if reverseCharge {
			r = Rate0
		}
		sums[r] = sums[r].Add(LineAmount(l))
	}

	rates := make([]Rate, 0, len(sums))
	for r := range sums {
		rates = append(rates, r)
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i] > rates[j] })

	var out Summary
	hundred := money.FromInt(100)
	for _, r := range rates {
		amount := sums[r]
		rate := money.FromInt(int64(r))
		var rs RateSummary
		rs.Rate = r
		switch mode {
		case PriceGross:
			// DPH = suma × sadzba / (100 + sadzba)
			rs.VAT = amount.MulRatio(rate, hundred.Add(rate)).RoundCents()
			rs.Total = amount
			rs.Base = amount.Sub(rs.VAT)
		default:
			rs.Base = amount
			rs.VAT = amount.MulRatio(rate, hundred).RoundCents()
			rs.Total = amount.Add(rs.VAT)
		}
		out.Rates = append(out.Rates, rs)
		out.Base = out.Base.Add(rs.Base)
		out.VAT = out.VAT.Add(rs.VAT)
		out.Total = out.Total.Add(rs.Total)
	}
	return out, nil
}
