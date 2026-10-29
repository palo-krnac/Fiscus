// Package legal sú zákonné parametre spoločné pre všetky firmy: sadzby DPH,
// sadzby ECB a parametre úroku z omeškania. Nová inštalácia sa predvyplní
// hodnotami z programu, používateľ ich upravuje v Nastaveniach → Legislatíva.
//
// Keď nová verzia programu prinesie nový riadok (napr. novú sadzbu ECB),
// doplní sa sám; riadky, ktoré používateľ upravil alebo zmazal, ostanú tak,
// ako ich nechal — o tom rozhoduje zoznam Seeded.
package legal

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// SettingKey je kľúč v nastaveniach programu.
const SettingKey = "legal"

// Params sú zákonné parametre.
type Params struct {
	VATPeriods     []vat.Period    `json:"vatPeriods"`
	ECB            []reminder.Rate `json:"ecb"`
	BusinessMargin money.Dec       `json:"businessMargin"` // p. b. nad ECB, podnikatelia
	ConsumerMargin money.Dec       `json:"consumerMargin"` // p. b. nad ECB, spotrebitelia
	FlatFee        money.Dec       `json:"flatFee"`        // paušálna náhrada za faktúru
	// Seeded sú kľúče predvolených riadkov, ktoré už boli raz doplnené
	// (aby sa zmazaný riadok nevrátil). Spravuje ich program, nie používateľ.
	Seeded []string `json:"seeded"`
}

// predvolené hodnoty a ich kľúče
func defaults() (Params, []string) {
	law := reminder.DefaultLaw()
	p := Params{
		VATPeriods:     append([]vat.Period(nil), vat.DefaultPeriods...),
		ECB:            law.ECB,
		BusinessMargin: law.BusinessMargin,
		ConsumerMargin: law.ConsumerMargin,
		FlatFee:        law.FlatFee,
	}
	keys := []string{"param:businessMargin", "param:consumerMargin", "param:flatFee"}
	for _, v := range p.VATPeriods {
		keys = append(keys, "vat:"+v.From)
	}
	for _, r := range p.ECB {
		keys = append(keys, "ecb:"+r.From.String())
	}
	return p, keys
}

// Defaults vráti predvyplnenie novej inštalácie.
func Defaults() Params {
	p, keys := defaults()
	p.Seeded = keys
	return p
}

// Seed doplní predvolené riadky, ktoré ešte neboli doplnené (nová verzia
// programu). Vráti true, ak sa niečo zmenilo.
func (p *Params) Seed() bool {
	def, keys := defaults()
	seen := map[string]bool{}
	for _, k := range p.Seeded {
		seen[k] = true
	}
	changed := false
	mark := func(k string) {
		p.Seeded = append(p.Seeded, k)
		seen[k] = true
		changed = true
	}
	for _, k := range keys {
		if seen[k] {
			continue
		}
		switch {
		case k == "param:businessMargin":
			p.BusinessMargin = def.BusinessMargin
		case k == "param:consumerMargin":
			p.ConsumerMargin = def.ConsumerMargin
		case k == "param:flatFee":
			p.FlatFee = def.FlatFee
		case len(k) > 4 && k[:4] == "vat:":
			from := k[4:]
			if !slices.ContainsFunc(p.VATPeriods, func(v vat.Period) bool { return v.From == from }) {
				for _, v := range def.VATPeriods {
					if v.From == from {
						p.addVATPeriod(v)
					}
				}
			}
		case len(k) > 4 && k[:4] == "ecb:":
			from := k[4:]
			if !slices.ContainsFunc(p.ECB, func(r reminder.Rate) bool { return r.From.String() == from }) {
				for _, r := range def.ECB {
					if r.From.String() == from {
						p.ECB = append(p.ECB, r)
					}
				}
			}
		}
		mark(k)
	}
	p.ECB = reminder.Sorted(p.ECB)
	return changed
}

// addVATPeriod vloží nové obdobie z novej verzie programu: ak doteraz
// otvorené obdobie začína skôr, ukončí ho deň pred novým.
func (p *Params) addVATPeriod(v vat.Period) {
	for i := range p.VATPeriods {
		cur := &p.VATPeriods[i]
		if cur.To == "" && cur.From < v.From {
			d, err := invoice.ParseDate(v.From)
			if err == nil {
				cur.To = d.AddDays(-1).String()
			}
		}
	}
	p.VATPeriods = append(p.VATPeriods, v)
	sort.Slice(p.VATPeriods, func(i, j int) bool { return p.VATPeriods[i].From < p.VATPeriods[j].From })
}

// Validate skontroluje parametre pred uložením.
func (p Params) Validate() error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if err := vat.ValidatePeriods(p.VATPeriods); err != nil {
		add("vatPeriods", "Sadzby DPH: "+err.Error())
	}
	if len(p.ECB) == 0 {
		add("ecb", "Zadajte aspoň jednu sadzbu ECB")
	}
	seen := map[string]bool{}
	for i, r := range p.ECB {
		var ve *invoice.ValidationError
		if err := reminder.ValidateRate(r); errors.As(err, &ve) {
			add(fmt.Sprintf("ecb[%d].%s", i, ve.Fields[0].Field), ve.Fields[0].Message)
		}
		if k := r.From.String(); k != "" && seen[k] {
			add(fmt.Sprintf("ecb[%d].from", i), "Sadzba od "+r.From.Display()+" je zadaná dvakrát")
		} else {
			seen[k] = true
		}
	}
	margin := func(f string, v money.Dec) {
		if v.Sign() < 0 || v.Cmp(money.FromInt(50)) > 0 {
			add(f, "Prirážka musí byť 0 až 50 percentuálnych bodov")
		}
	}
	margin("businessMargin", p.BusinessMargin)
	margin("consumerMargin", p.ConsumerMargin)
	if p.FlatFee.Sign() < 0 || p.FlatFee.Cmp(money.FromInt(10000)) > 0 {
		add("flatFee", "Paušálna náhrada musí byť 0 až 10 000 €")
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Normalize zoradí tabuľky (pred uložením).
func (p *Params) Normalize() {
	p.ECB = reminder.Sorted(p.ECB)
	sort.SliceStable(p.VATPeriods, func(i, j int) bool { return p.VATPeriods[i].From < p.VATPeriods[j].From })
	for i := range p.VATPeriods {
		rs := p.VATPeriods[i].Rates
		sort.Slice(rs, func(a, b int) bool { return rs[a] > rs[b] })
	}
}

// Law vráti parametre upomienok.
func (p Params) Law() reminder.Law {
	return reminder.Law{ECB: reminder.Sorted(p.ECB), BusinessMargin: p.BusinessMargin, ConsumerMargin: p.ConsumerMargin, FlatFee: p.FlatFee}
}
