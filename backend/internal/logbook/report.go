package logbook

import (
	"fmt"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Month sú súčty jedného mesiaca.
type Month struct {
	Month      int       `json:"month"`
	KmBusiness int64     `json:"kmBusiness"`
	KmPrivate  int64     `json:"kmPrivate"`
	Quantity   money.Dec `json:"quantity"`
	FuelCost   money.Dec `json:"fuelCost"`
}

// TripCost sú náklady jednej služobnej jazdy: PHM a opotrebenie
// (základná náhrada pri súkromnom vozidle, odpis pri vozidle v majetku).
type TripCost struct {
	TripID int64     `json:"tripId"`
	Fuel   money.Dec `json:"fuel"`
	Wear   money.Dec `json:"wear"`
	Total  money.Dec `json:"total"`
}

// Report je vyúčtovanie vozidla za rok.
type Report struct {
	Year       int   `json:"year"`
	KmBusiness int64 `json:"kmBusiness"`
	KmPrivate  int64 `json:"kmPrivate"`
	KmTotal    int64 `json:"kmTotal"`
	OdoStart   int64 `json:"odoStart"` // stav tachometra na začiatku roka (z prvej jazdy)
	OdoEnd     int64 `json:"odoEnd"`   // na konci roka (z poslednej jazdy)
	// Share je podiel služobných km v % (2 desatinné miesta).
	Share    money.Dec `json:"share"`
	Quantity money.Dec `json:"quantity"` // natankované množstvo
	FuelCost money.Dec `json:"fuelCost"` // cena PHM s DPH z dokladov
	AvgPrice money.Dec `json:"avgPrice"` // priemerná cena za jednotku
	// Actual je skutočná priemerná spotreba na 100 km (natankované / km),
	// Norm spotreba z technického preukazu.
	Actual money.Dec `json:"actual"`
	Norm   money.Dec `json:"norm"`

	// výdavky
	FuelExpense   money.Dec `json:"fuelExpense"`   // daňový výdavok (náhrada) za PHM
	FuelPrivate   money.Dec `json:"fuelPrivate"`   // PHM na súkromné jazdy (nie je výdavok)
	FuelOveruse   money.Dec `json:"fuelOveruse"`   // nadspotreba nad technický preukaz
	BaseAllowance money.Dec `json:"baseAllowance"` // základná náhrada za km (súkromné vozidlo)
	Depreciation  money.Dec `json:"depreciation"`  // daňový odpis vozidla v roku (register majetku)
	// Allowance je suma cestovných náhrad súkromného vozidla (PHM + základná),
	// Exclude je PHM, ktoré treba vylúčiť z výdavkov vozidla v majetku
	// (súkromné jazdy, nadspotreba, nad paušál).
	Allowance money.Dec `json:"allowance"`
	Exclude   money.Dec `json:"exclude"`
	// Total sú všetky výdavky na vozidlo v roku (PHM, náhrady, odpis);
	// PerKm na služobný km.
	Total money.Dec `json:"total"`
	PerKm money.Dec `json:"perKm"`

	Months   []Month    `json:"months"`
	Trips    []TripCost `json:"trips"`
	Warnings []string   `json:"warnings"`
}

// Input sú údaje na vyúčtovanie.
type Input struct {
	Vehicle Vehicle
	Year    int
	Trips   []Trip // všetky jazdy vozidla (rok sa vyberie)
	Fuel    []Fuel // všetky tankovania vozidla
	Rates   []KmRate
	// Depreciation je daňový odpis vozidla v roku z registra majetku
	// (už krátený podielom na podnikaní), AssetShare ten podiel v %.
	Depreciation money.Dec
	AssetShare   int
}

func pct(part, whole int64) money.Dec {
	if whole <= 0 {
		return money.Zero
	}
	return money.FromInt(part).MulRatio(money.FromInt(100), money.FromInt(whole)).RoundCents()
}

// Compute zostaví vyúčtovanie vozidla za rok.
func Compute(in Input) Report {
	v := in.Vehicle
	r := Report{Year: in.Year, Norm: v.Consumption, Depreciation: in.Depreciation, Months: make([]Month, 12),
		Trips: []TripCost{}, Warnings: []string{}}
	warn := func(f string, a ...any) { r.Warnings = append(r.Warnings, fmt.Sprintf(f, a...)) }
	for i := range r.Months {
		r.Months[i].Month = i + 1
	}

	trips := make([]Trip, 0, len(in.Trips))
	for _, t := range in.Trips {
		if t.Date.Year() == in.Year {
			trips = append(trips, t)
		}
	}
	sort.SliceStable(trips, func(i, j int) bool {
		if trips[i].Date != trips[j].Date {
			return trips[i].Date.Before(trips[j].Date)
		}
		return trips[i].OdoStart < trips[j].OdoStart
	})
	var lastEnd int64
	for _, t := range trips {
		m := &r.Months[t.Date.Month()-1]
		if t.Kind == TripBusiness {
			r.KmBusiness += t.Km
			m.KmBusiness += t.Km
		} else {
			r.KmPrivate += t.Km
			m.KmPrivate += t.Km
		}
		if t.OdoStart > 0 {
			if r.OdoStart == 0 {
				r.OdoStart = t.OdoStart
			}
			if lastEnd > 0 && t.OdoStart != lastEnd {
				gap := t.OdoStart - lastEnd
				if gap > 0 {
					warn("Pred jazdou %s chýba v knihe %d km (tachometer %d → %d) — doplňte jazdu, inak podiel služobných km nesedí", t.Date.Display(), gap, lastEnd, t.OdoStart)
				} else {
					warn("Jazda %s začína na nižšom stave tachometra (%d) ako skončila predchádzajúca (%d)", t.Date.Display(), t.OdoStart, lastEnd)
				}
			}
		}
		if t.OdoEnd > 0 {
			lastEnd = t.OdoEnd
			r.OdoEnd = t.OdoEnd
		}
	}
	r.KmTotal = r.KmBusiness + r.KmPrivate
	r.Share = pct(r.KmBusiness, r.KmTotal)

	noDoc := 0
	for _, f := range in.Fuel {
		if f.Date.Year() != in.Year {
			continue
		}
		if f.DocKind == "" {
			noDoc++
		}
		r.Quantity = r.Quantity.Add(f.Quantity)
		r.FuelCost = r.FuelCost.Add(f.Amount)
		m := &r.Months[f.Date.Month()-1]
		m.Quantity = m.Quantity.Add(f.Quantity)
		m.FuelCost = m.FuelCost.Add(f.Amount)
	}
	if r.Quantity.Sign() > 0 {
		r.AvgPrice = r.FuelCost.MulRatio(money.FromInt(1), r.Quantity).RoundTo(3)
	}
	if r.KmTotal > 0 && r.Quantity.Sign() > 0 {
		r.Actual = r.Quantity.MulRatio(money.FromInt(100), money.FromInt(r.KmTotal)).RoundTo(2)
	}
	km := func(n int64) money.Dec { return money.FromInt(n) }
	// PHM podľa spotreby z technického preukazu: km × spotreba / 100 × cena
	normCost := func(n int64) money.Dec {
		return km(n).Mul(v.Consumption).MulRatio(r.AvgPrice, money.FromInt(100)).RoundCents()
	}

	switch {
	case v.Ownership != Private && v.Method == MethodLog:
		// služobná časť podľa km, najviac podľa technického preukazu
		business := r.FuelCost
		if r.KmTotal > 0 {
			business = r.FuelCost.MulRatio(km(r.KmBusiness), km(r.KmTotal)).RoundCents()
		} else if r.FuelCost.Sign() > 0 {
			business = money.Zero
			warn("Tankovanie bez jázd v knihe — bez jázd nemožno preukázať, že PHM súvisí s podnikaním")
		}
		r.FuelExpense = business
		if v.Consumption.Sign() > 0 && r.KmBusiness > 0 {
			if limit := normCost(r.KmBusiness); limit.Cmp(business) < 0 {
				r.FuelExpense = limit
				r.FuelOveruse = business.Sub(limit)
			}
		}
		r.FuelPrivate = r.FuelCost.Sub(business)
		r.Exclude = r.FuelPrivate.Add(r.FuelOveruse)
	case v.Ownership != Private:
		// paušál 80 %, pri preukázanom nižšom podiele služobných km nižší
		rate := v.FlatRate()
		if r.KmTotal > 0 && r.Share.Cmp(rate) < 0 {
			rate = r.Share
		}
		r.FuelExpense = r.FuelCost.MulRatio(rate, money.FromInt(100)).RoundCents()
		r.FuelPrivate = r.FuelCost.Sub(r.FuelExpense)
		r.Exclude = r.FuelPrivate
		if r.OdoStart == 0 && v.Odometer == 0 {
			warn("Pri paušálnych výdavkoch na PHM treba evidovať stav tachometra na začiatku a na konci roka")
		}
	case v.Method == MethodLog:
		// súkromné vozidlo: cestovné náhrady za služobné jazdy
		var base money.Dec
		missingRate := false
		for _, t := range trips {
			if t.Kind != TripBusiness {
				continue
			}
			rate, err := RateOn(in.Rates, t.Date, v.Moto)
			if err != nil {
				missingRate = true
				continue
			}
			base = base.Add(km(t.Km).Mul(rate))
		}
		if missingRate {
			warn("Pre niektoré jazdy chýba sadzba základnej náhrady za 1 km — doplňte ju v Legislatíve")
		}
		r.BaseAllowance = base.CeilCents()
		if r.KmBusiness > 0 {
			if r.AvgPrice.IsZero() {
				warn("Bez dokladu o tankovaní nemožno vypočítať náhradu za PHM — cenu PHM preukazuje doklad o kúpe, nie cena z internetu")
			} else {
				r.FuelExpense = normCost(r.KmBusiness).CeilCents()
			}
		}
		r.Allowance = r.BaseAllowance.Add(r.FuelExpense)
	default:
		// súkromné vozidlo, paušál 50 % PHM (bez základnej náhrady)
		r.FuelExpense = r.FuelCost.MulRatio(v.FlatRate(), money.FromInt(100)).RoundCents()
		r.Allowance = r.FuelExpense
	}

	if v.Consumption.Sign() > 0 && r.Actual.Sign() > 0 {
		limit := v.Consumption.MulRatio(money.FromInt(120), money.FromInt(100))
		if r.Actual.Cmp(limit) > 0 {
			warn("Skutočná spotreba %s %s/100 km je o viac ako 20 %% vyššia ako v technickom preukaze (%s) — skontrolujte, či sú v knihe všetky jazdy", r.Actual.Display(), v.Fuel.Unit(), v.Consumption.Display())
		}
	}
	if v.Ownership == Business && in.AssetShare > 0 && r.KmTotal > 0 && v.Method == MethodLog {
		diff := money.FromInt(int64(in.AssetShare)).Sub(r.Share)
		if diff.Cmp(money.FromInt(5)) > 0 {
			warn("V registri majetku je podiel na podnikaní %d %%, podľa knihy jázd %s %% — zvážte úpravu podielu pri odpisoch", in.AssetShare, r.Share.Display())
		}
	}
	if v.Ownership != Private && noDoc > 0 {
		warn("Tankovania bez priradeného dokladu (pokladničný blok alebo prijatá faktúra): %d — nákup PHM musí byť vo výdavkoch, inak sa súkromná časť vylúči z výdavkov, ktoré tam nie sú", noDoc)
	}
	if v.Ownership == Business && v.AssetID == 0 {
		warn("Vozidlo nie je prepojené s registrom majetku — odpis sa do nákladov na km nezapočíta")
	}

	r.Total = r.FuelExpense.Add(r.BaseAllowance).Add(r.Depreciation)
	if r.KmBusiness > 0 {
		r.PerKm = r.Total.MulRatio(money.FromInt(1), km(r.KmBusiness)).RoundTo(3)
	}

	// náklady jednotlivých služobných jázd
	if r.KmBusiness > 0 {
		wearTotal := r.BaseAllowance.Add(r.Depreciation)
		for _, t := range trips {
			if t.Kind != TripBusiness {
				continue
			}
			c := TripCost{TripID: t.ID,
				Fuel: r.FuelExpense.MulRatio(km(t.Km), km(r.KmBusiness)).RoundCents(),
				Wear: wearTotal.MulRatio(km(t.Km), km(r.KmBusiness)).RoundCents()}
			if v.Ownership == Private && v.Method == MethodLog {
				if rate, err := RateOn(in.Rates, t.Date, v.Moto); err == nil {
					c.Wear = km(t.Km).Mul(rate).CeilCents()
				}
			}
			c.Total = c.Fuel.Add(c.Wear)
			r.Trips = append(r.Trips, c)
		}
	}
	return r
}

// NextOdometer vráti stav tachometra po poslednej jazde (na predvyplnenie
// novej jazdy), inak počiatočný stav vozidla.
func NextOdometer(v Vehicle, trips []Trip, before invoice.Date) int64 {
	best := v.Odometer
	var bestDate invoice.Date
	for _, t := range trips {
		if t.OdoEnd > 0 && (before.IsZero() || !t.Date.After(before)) && (bestDate.IsZero() || !t.Date.Before(bestDate)) && t.OdoEnd > best {
			best, bestDate = t.OdoEnd, t.Date
		}
	}
	return best
}
