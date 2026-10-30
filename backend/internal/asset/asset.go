// Package asset je evidencia dlhodobého majetku a daňové odpisy podľa
// zákona č. 595/2003 Z. z. o dani z príjmov (§ 22 – § 29):
//
//   - hmotný majetok so vstupnou cenou nad 1 700 € a nehmotný nad 2 400 €;
//   - odpisové skupiny 0 – 6 s dobou odpisovania 2, 4, 6, 8, 12, 20, 40 rokov;
//   - rovnomerné odpisovanie (§ 27): ročný odpis = vstupná cena / doba;
//   - zrýchlené odpisovanie (§ 28) iba v skupinách 2 a 3: prvý rok
//     VC / k1, ďalšie roky 2 × ZC / (k − n), koeficienty 6/7 a 8/9;
//   - v prvom roku sa uplatní pomerná časť ročného odpisu podľa mesiacov
//     od mesiaca zaradenia do užívania; neuplatnená časť sa doodpíše v roku
//     nasledujúcom po uplynutí doby odpisovania;
//   - v roku vyradenia (predaj, likvidácia) sa odpis neuplatní, zostatková
//     cena je výdavkom (§ 22 ods. 11, § 24 ods. 2);
//   - pri čiastočnom používaní na podnikanie sa uplatní pomerná časť.
package asset

import (
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Kind je druh majetku.
type Kind string

const (
	Tangible   Kind = "tangible"   // hmotný
	Intangible Kind = "intangible" // nehmotný
)

// Method je spôsob odpisovania.
type Method string

const (
	Straight    Method = "straight"    // rovnomerné (§ 27)
	Accelerated Method = "accelerated" // zrýchlené (§ 28)
)

// Disposal je dôvod vyradenia.
type Disposal string

const (
	DisposalSale        Disposal = "sale"        // predaj
	DisposalLiquidation Disposal = "liquidation" // likvidácia, škoda
	DisposalOther       Disposal = "other"       // vklad, dar, iné
)

// Hranice vstupnej ceny (§ 22 ods. 1 a 2).
var (
	TangibleLimit   = money.FromInt(1700)
	IntangibleLimit = money.FromInt(2400)
)

// GroupYears je doba odpisovania podľa odpisovej skupiny (§ 26 ods. 1).
var GroupYears = []int{2, 4, 6, 8, 12, 20, 40}

// accel sú koeficienty zrýchleného odpisovania (§ 28 ods. 1): [prvý rok, ďalšie roky].
var accel = map[int][2]int64{2: {6, 7}, 3: {8, 9}}

// Asset je položka dlhodobého majetku.
type Asset struct {
	ID        int64        `json:"id"`
	CompanyID int64        `json:"companyId"`
	Number    string       `json:"number"` // inventárne číslo
	Name      string       `json:"name"`
	Kind      Kind         `json:"kind"`
	Group     int          `json:"group"` // odpisová skupina 0–6 (hmotný)
	Years     int          `json:"years"` // doba odpisovania nehmotného majetku
	Method    Method       `json:"method"`
	Acquired  invoice.Date `json:"acquired"` // zaradenie do užívania
	Cost      money.Dec    `json:"cost"`     // vstupná cena
	// Share je percento používania na podnikanie (1–100).
	Share        int          `json:"share"`
	PurchaseID   int64        `json:"purchaseId,omitempty"` // prijatá faktúra
	PurchaseNo   string       `json:"purchaseNumber,omitempty"`
	Disposed     invoice.Date `json:"disposed"` // vyradenie (prázdne = v užívaní)
	DisposalKind Disposal     `json:"disposal,omitempty"`
	Note         string       `json:"note"`
}

// Period vráti dobu odpisovania v rokoch.
func (a *Asset) Period() int {
	if a.Kind == Intangible {
		return a.Years
	}
	if a.Group < 0 || a.Group >= len(GroupYears) {
		return 0
	}
	return GroupYears[a.Group]
}

// Normalize doplní predvolené hodnoty.
func (a *Asset) Normalize() {
	a.Name, a.Number, a.Note = strings.TrimSpace(a.Name), strings.TrimSpace(a.Number), strings.TrimSpace(a.Note)
	if a.Kind == "" {
		a.Kind = Tangible
	}
	if a.Method == "" {
		a.Method = Straight
	}
	if a.Share == 0 {
		a.Share = 100
	}
	if a.Kind == Intangible {
		a.Group, a.Method = 0, Straight
	} else {
		a.Years = 0
	}
	if a.Disposed.IsZero() {
		a.DisposalKind = ""
	}
}

// Validate skontroluje položku majetku.
func (a *Asset) Validate() error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if a.Name == "" {
		add("name", "Zadajte názov majetku")
	}
	costSet := a.Cost.Sign() > 0
	if !costSet {
		add("cost", "Zadajte vstupnú cenu")
	}
	switch a.Kind {
	case Tangible:
		if a.Group < 0 || a.Group >= len(GroupYears) {
			add("group", "Vyberte odpisovú skupinu 0 až 6")
		}
		if costSet && a.Cost.Cmp(TangibleLimit) <= 0 {
			add("cost", "Hmotný majetok sa odpisuje od vstupnej ceny nad 1\u00a0700\u00a0€ — lacnejší je výdavok naraz")
		}
	case Intangible:
		if a.Years < 1 || a.Years > 40 {
			add("years", "Zadajte dobu odpisovania nehmotného majetku (podľa odpisového plánu)")
		}
		if costSet && a.Cost.Cmp(IntangibleLimit) <= 0 {
			add("cost", "Nehmotný majetok sa odpisuje od vstupnej ceny nad 2\u00a0400\u00a0€ — lacnejší je výdavok naraz")
		}
	default:
		add("kind", "Vyberte hmotný alebo nehmotný majetok")
	}
	switch a.Method {
	case Straight:
	case Accelerated:
		if _, ok := accel[a.Group]; !ok || a.Kind != Tangible {
			add("method", "Zrýchlene sa odpisuje iba hmotný majetok v skupine 2 a 3")
		}
	default:
		add("method", "Neznámy spôsob odpisovania")
	}
	if a.Acquired.IsZero() {
		add("acquired", "Zadajte dátum zaradenia do užívania")
	}
	if !a.Cost.Equal(a.Cost.RoundCents()) {
		add("cost", "Vstupná cena môže mať najviac dve desatinné miesta")
	}
	if a.Share < 1 || a.Share > 100 {
		add("share", "Podiel na podnikaní musí byť 1 až 100 %")
	}
	if !a.Disposed.IsZero() {
		switch {
		case a.Disposed.Before(a.Acquired):
			add("disposed", "Vyradenie nemôže byť pred zaradením")
		case a.DisposalKind != DisposalSale && a.DisposalKind != DisposalLiquidation && a.DisposalKind != DisposalOther:
			add("disposal", "Vyberte dôvod vyradenia")
		}
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Year je rok v odpisovom pláne.
type Year struct {
	Year     int       `json:"year"`
	Annual   money.Dec `json:"annual"`   // ročný odpis podľa zákona (pred krátením podielom)
	Claimed  money.Dec `json:"claimed"`  // uplatnený odpis (× podiel na podnikaní)
	Residual money.Dec `json:"residual"` // daňová zostatková cena na konci roka
	// Disposal je zostatková cena, ktorá je výdavkom pri vyradení (× podiel).
	Disposal money.Dec `json:"disposal"`
	Note     string    `json:"note,omitempty"`
}

// ErrNoSchedule — majetok nemá platné údaje na odpisový plán.
var ErrNoSchedule = errors.New("odpisový plán sa nedá zostaviť")

// Schedule zostaví daňový odpisový plán od roku zaradenia po posledný odpis
// alebo vyradenie.
func (a *Asset) Schedule() ([]Year, error) {
	n := a.Period()
	if n <= 0 || a.Acquired.IsZero() || a.Cost.Sign() <= 0 {
		return nil, ErrNoSchedule
	}
	months := money.FromInt(int64(13 - a.Acquired.Month())) // od mesiaca zaradenia do decembra
	twelve := money.FromInt(12)
	share := func(d money.Dec) money.Dec {
		if a.Share >= 100 {
			return d
		}
		return d.MulRatio(money.FromInt(int64(a.Share)), money.FromInt(100)).RoundCents()
	}
	start := a.Acquired.Year()
	var out []Year
	residual := a.Cost // daňová ZC (pri zrýchlenom „zvýšená“ ZC podľa § 28)
	var unclaimed money.Dec

	// ročné odpisy podľa spôsobu, rok i = 0..n-1
	annual := make([]money.Dec, 0, n)
	switch a.Method {
	case Accelerated:
		k := accel[a.Group]
		first := a.Cost.MulRatio(money.FromInt(1), money.FromInt(k[0])).RoundCents()
		annual = append(annual, first)
		zc := a.Cost.Sub(first)
		for i := 1; i < n; i++ {
			var v money.Dec
			if int64(i) >= k[1]-1 || i == n-1 {
				v = zc // posledný rok: celá zostatková cena
			} else {
				v = zc.MulRatio(money.FromInt(2), money.FromInt(k[1]-int64(i))).RoundCents()
			}
			annual = append(annual, v)
			zc = zc.Sub(v)
		}
	default:
		per := a.Cost.MulRatio(money.FromInt(1), money.FromInt(int64(n))).RoundCents()
		sum := money.Zero
		for i := 0; i < n; i++ {
			v := per
			if i == n-1 {
				v = a.Cost.Sub(sum) // posledný rok vyrovná zaokrúhlenie
			}
			annual = append(annual, v)
			sum = sum.Add(v)
		}
	}

	for i := 0; i <= n; i++ {
		y := Year{Year: start + i}
		if !a.Disposed.IsZero() && a.Disposed.Year() == y.Year {
			// rok vyradenia: odpis sa neuplatní, ZC (vrátane neuplatnenej
			// časti prvého roka) je výdavkom
			y.Disposal = share(residual.Add(unclaimed))
			y.Note = map[Disposal]string{DisposalSale: "predaj", DisposalLiquidation: "likvidácia", DisposalOther: "vyradenie"}[a.DisposalKind]
			out = append(out, y)
			return out, nil
		}
		switch {
		case i == 0:
			y.Annual = annual[0]
			claimed := annual[0].Mul(months).MulRatio(money.FromInt(1), twelve).RoundCents()
			if months.Equal(twelve) {
				claimed = annual[0]
			}
			unclaimed = annual[0].Sub(claimed)
			y.Claimed = share(claimed)
			residual = residual.Sub(annual[0])
			if !unclaimed.IsZero() {
				y.Note = fmt.Sprintf("pomerná časť %d/12", 13-a.Acquired.Month())
			}
		case i < n:
			y.Annual = annual[i]
			y.Claimed = share(annual[i])
			residual = residual.Sub(annual[i])
		default:
			if unclaimed.IsZero() {
				return out, nil
			}
			y.Annual = unclaimed
			y.Claimed = share(unclaimed)
			y.Note = "neuplatnená časť prvého roka"
			unclaimed = money.Zero
		}
		// zostatková cena na konci roka: VC mínus odpisy (neuplatnená časť prvého
		// roka ostáva v ZC, kým sa nedoodpíše)
		y.Residual = residual.Add(unclaimed)
		out = append(out, y)
	}
	return out, nil
}

// InYear vráti riadok plánu za rok (nulový, ak v tom roku nič nie je).
func (a *Asset) InYear(year int) Year {
	plan, err := a.Schedule()
	if err != nil {
		return Year{Year: year}
	}
	for _, y := range plan {
		if y.Year == year {
			return y
		}
	}
	// pred zaradením: nič; po poslednom odpise: ZC 0
	return Year{Year: year}
}

// ResidualAt vráti daňovú zostatkovú cenu na konci roka (0 po vyradení
// alebo doodpisovaní, VC pred zaradením sa neráta — majetok ešte nie je).
func (a *Asset) ResidualAt(year int) money.Dec {
	if a.Acquired.IsZero() || year < a.Acquired.Year() {
		return money.Zero
	}
	if !a.Disposed.IsZero() && year >= a.Disposed.Year() {
		return money.Zero
	}
	plan, err := a.Schedule()
	if err != nil {
		return money.Zero
	}
	for _, y := range plan {
		if y.Year == year {
			return y.Residual
		}
	}
	return money.Zero
}
