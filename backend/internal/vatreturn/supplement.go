package vatreturn

import (
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Druh priznania (DPHv25, hlavička „Druh priznania“; poučenie bod 11).
//   - riadne: prvé priznanie za obdobie;
//   - opravné: oprava pred uplynutím lehoty na podanie — nahrádza riadne;
//   - dodatočné: po lehote (§ 16 daňového poriadku), s dátumom zistenia
//     skutočností, rozdielom v r. 36 a sumou na vysporiadanie v r. 37.
type Kind string

const (
	KindRegular       Kind = "riadne"
	KindCorrective    Kind = "opravne"
	KindSupplementary Kind = "dodatocne"
)

// Valid vráti true pre známy druh (prázdny = riadne).
func (k Kind) Valid() bool {
	return k == "" || k == KindRegular || k == KindCorrective || k == KindSupplementary
}

// Deadline je lehota na podanie priznania a KV: 25. deň po skončení obdobia.
func (p Period) Deadline() invoice.Date { return p.End().AddDays(25) }

// Options sú údaje opravného alebo dodatočného priznania.
type Options struct {
	Kind       Kind
	Discovered invoice.Date // dodatočné: dátum zistenia skutočností
	// LastKnown je posledná známa daňová povinnosť (+) alebo nadmerný
	// odpočet (−) za obdobie z podaných priznaní alebo kontroly.
	LastKnown *money.Dec
	R37       *money.Dec // dodatočné: suma na vysporiadanie, ak sa líši od r. 36
}

// Chyby údajov dodatočného priznania.
var (
	ErrBadKind        = errors.New("neznámy druh priznania")
	ErrNoDiscovery    = errors.New("pri dodatočnom priznaní zadajte dátum zistenia skutočností")
	ErrNoLastKnown    = errors.New("pri dodatočnom priznaní zadajte poslednú známu daňovú povinnosť alebo nadmerný odpočet")
	ErrSupplementaryK = errors.New("dodatočný kontrolný výkaz obsahuje iba opravované záznamy — Fiškus ho zatiaľ nevytvára, opravte KV v eDane")
)

// Result je výsledok priznania so znamienkom: vlastná daňová povinnosť
// kladná (r. 32), nadmerný odpočet záporný (r. 33).
func (r *Return) Result() money.Dec { return r.Rows["r32"].Sub(r.Rows["r33"]) }

// Apply nastaví druh priznania. Pri dodatočnom vypočíta r. 36 (rozdiel
// oproti poslednej známej povinnosti; + zvýšenie povinnosti alebo zníženie
// nadmerného odpočtu, − naopak) a r. 37 (suma na vysporiadanie — väčšinou
// rovná r. 36; ak nie, zadá ju používateľ). Údaje dodatočného priznania
// nahrádzajú údaje posledného priznania (poučenie bod 51, 52).
func (r *Return) Apply(o Options) error {
	if !o.Kind.Valid() {
		return ErrBadKind
	}
	r.Kind = o.Kind
	if r.Kind == "" {
		r.Kind = KindRegular
	}
	delete(r.Rows, "r36")
	delete(r.Rows, "r37")
	if r.Kind != KindSupplementary {
		r.Discovered, r.LastKnown = invoice.Date{}, nil
		return nil
	}
	if o.Discovered.IsZero() {
		return ErrNoDiscovery
	}
	if o.LastKnown == nil {
		return ErrNoLastKnown
	}
	last := o.LastKnown.RoundCents()
	r.Discovered, r.LastKnown = o.Discovered, &last
	r36 := r.Result().Sub(last).RoundCents()
	r37 := r36
	if o.R37 != nil {
		r37 = o.R37.RoundCents()
	}
	if !r36.IsZero() {
		r.Rows["r36"] = r36
	}
	if !r37.IsZero() {
		r.Rows["r37"] = r37
	}
	return nil
}
