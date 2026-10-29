// Package vat počíta DPH podľa slovenských pravidiel.
//
// Sadzby sú v číselníku Periods() s obdobím platnosti (od 1. 1. 2025: 23 %
// základná, 19 % a 5 % znížené; do 31. 12. 2024: 20 % a 10 %, 5 %).
// 0 % = oslobodené plnenie alebo prenesenie daňovej povinnosti. DPH sa počíta zo súčtu
// základov za každú sadzbu a zaokrúhľuje na centy — tak, ako ju
// zobrazuje rekapitulácia na faktúre.
package vat

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Rate je sadzba DPH v percentách.
type Rate int

const (
	Rate0  Rate = 0
	Rate5  Rate = 5
	Rate10 Rate = 10
	Rate19 Rate = 19
	Rate20 Rate = 20
	Rate23 Rate = 23
)

// Period je obdobie, v ktorom platí sada sadzieb DPH (číselník).
type Period struct {
	From  string `json:"from"`  // vrátane, YYYY-MM-DD
	To    string `json:"to"`    // vrátane; "" = doteraz
	Rates []Rate `json:"rates"` // od najvyššej; 0 = oslobodené / prenesenie
}

// DefaultPeriods je predvolený číselník sadzieb DPH na Slovensku (predvyplnenie
// novej inštalácie). Platný číselník je v Nastaveniach → Legislatíva a do
// programu sa dostane cez SetPeriods.
var DefaultPeriods = []Period{
	{From: "2023-01-01", To: "2024-12-31", Rates: []Rate{Rate20, Rate10, Rate5, Rate0}},
	{From: "2025-01-01", Rates: []Rate{Rate23, Rate19, Rate5, Rate0}},
}

var (
	periodsMu sync.RWMutex
	periods   = DefaultPeriods
)

// Periods vráti platný číselník sadzieb DPH.
func Periods() []Period {
	periodsMu.RLock()
	defer periodsMu.RUnlock()
	return periods
}

// SetPeriods nastaví platný číselník (po kontrole cez ValidatePeriods).
func SetPeriods(ps []Period) {
	cp := make([]Period, len(ps))
	for i, p := range ps {
		cp[i] = Period{From: p.From, To: p.To, Rates: append([]Rate(nil), p.Rates...)}
	}
	periodsMu.Lock()
	periods = cp
	periodsMu.Unlock()
}

// ValidatePeriods skontroluje číselník: obdobia zoradené, neprekrývajú sa,
// iba posledné môže byť bez konca, sadzby 0–100 % bez opakovania.
func ValidatePeriods(ps []Period) error {
	if len(ps) == 0 {
		return fmt.Errorf("číselník sadzieb DPH musí mať aspoň jedno obdobie")
	}
	for i, p := range ps {
		n := i + 1
		if _, err := time.Parse("2006-01-02", p.From); err != nil {
			return fmt.Errorf("obdobie %d: neplatný začiatok", n)
		}
		if p.To != "" {
			if _, err := time.Parse("2006-01-02", p.To); err != nil {
				return fmt.Errorf("obdobie %d: neplatný koniec", n)
			}
			if p.To < p.From {
				return fmt.Errorf("obdobie %d: koniec je pred začiatkom", n)
			}
		} else if i < len(ps)-1 {
			return fmt.Errorf("obdobie %d: bez konca môže byť iba posledné obdobie", n)
		}
		if i > 0 && p.From <= ps[i-1].To {
			return fmt.Errorf("obdobie %d sa prekrýva s predchádzajúcim alebo nie je zoradené", n)
		}
		if len(p.Rates) == 0 {
			return fmt.Errorf("obdobie %d: zadajte sadzby", n)
		}
		seen := map[Rate]bool{}
		for _, r := range p.Rates {
			if r < 0 || r > 100 {
				return fmt.Errorf("obdobie %d: sadzba %d %% je mimo 0–100", n, r)
			}
			if seen[r] {
				return fmt.Errorf("obdobie %d: sadzba %d %% je dvakrát", n, r)
			}
			seen[r] = true
		}
	}
	return nil
}

// RatesOn vráti sadzby platné k dátumu dodania (YYYY-MM-DD).
// Pre dátum pred prvým obdobím vráti sadzby prvého obdobia.
func RatesOn(date string) []Rate {
	ps := Periods()
	for _, p := range ps {
		if date >= p.From && (p.To == "" || date <= p.To) {
			return p.Rates
		}
	}
	if date < ps[0].From {
		return ps[0].Rates
	}
	return ps[len(ps)-1].Rates
}

// ValidOn vráti true, ak sadzba platila k dátumu dodania.
func (r Rate) ValidOn(date string) bool {
	for _, v := range RatesOn(date) {
		if v == r {
			return true
		}
	}
	return false
}

// Known vráti true pre sadzbu, ktorá je v niektorom období číselníka.
func (r Rate) Known() bool {
	for _, p := range Periods() {
		for _, v := range p.Rates {
			if v == r {
				return true
			}
		}
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
		if !l.Rate.Known() {
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
