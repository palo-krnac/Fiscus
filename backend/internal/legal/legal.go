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
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/calendar"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/logbook"
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
	// KmRates sú sadzby základnej náhrady za 1 km (kniha jázd, súkromné vozidlo).
	KmRates []logbook.KmRate `json:"kmRates"`
	// Holidays sú dni pracovného pokoja — posun lehôt v kalendári termínov.
	Holidays []calendar.Holiday `json:"holidays"`
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
		KmRates:        append([]logbook.KmRate(nil), logbook.DefaultKmRates...),
		Holidays:       append([]calendar.Holiday(nil), calendar.DefaultHolidays...),
	}
	keys := []string{"param:businessMargin", "param:consumerMargin", "param:flatFee"}
	for _, v := range p.VATPeriods {
		keys = append(keys, "vat:"+v.From)
	}
	for _, r := range p.ECB {
		keys = append(keys, "ecb:"+r.From.String())
	}
	for _, r := range p.KmRates {
		keys = append(keys, "km:"+r.From.String())
	}
	for _, h := range p.Holidays {
		keys = append(keys, h.Key())
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
		case len(k) > 3 && k[:3] == "km:":
			from := k[3:]
			if !slices.ContainsFunc(p.KmRates, func(r logbook.KmRate) bool { return r.From.String() == from }) {
				for _, r := range def.KmRates {
					if r.From.String() == from {
						p.KmRates = append(p.KmRates, r)
					}
				}
			}
		case len(k) > 4 && k[:4] == "hol:":
			if !slices.ContainsFunc(p.Holidays, func(h calendar.Holiday) bool { return h.Key() == k }) {
				for _, h := range def.Holidays {
					if h.Key() == k {
						p.Holidays = append(p.Holidays, h)
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
	sortKm(p.KmRates)
	sortHolidays(p.Holidays)
	return changed
}

func sortHolidays(hs []calendar.Holiday) {
	// Veľká noc sa radí medzi marec a máj, Veľký piatok pred pondelkom
	key := func(h calendar.Holiday) string {
		if off, ok := strings.CutPrefix(h.Day, "easter"); ok {
			n, _ := strconv.Atoi(off)
			return fmt.Sprintf("04-00 %03d", n+100)
		}
		return h.Day
	}
	sort.SliceStable(hs, func(i, j int) bool {
		if a, b := key(hs[i]), key(hs[j]); a != b {
			return a < b
		}
		return hs[i].From < hs[j].From
	})
}

func sortKm(rs []logbook.KmRate) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].From.Before(rs[j].From) })
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
	kmSeen := map[string]bool{}
	for i, r := range p.KmRates {
		switch {
		case r.From.IsZero():
			add(fmt.Sprintf("kmRates[%d].from", i), "Zadajte, odkedy sadzba platí")
		case kmSeen[r.From.String()]:
			add(fmt.Sprintf("kmRates[%d].from", i), "Sadzba od "+r.From.Display()+" je zadaná dvakrát")
		}
		kmSeen[r.From.String()] = true
		for f, v := range map[string]money.Dec{"car": r.Car, "moto": r.Moto} {
			if v.Sign() <= 0 || v.Cmp(money.FromInt(5)) > 0 {
				add(fmt.Sprintf("kmRates[%d].%s", i, f), "Sadzba za 1 km musí byť 0 až 5 €")
			}
		}
	}
	holSeen := map[string]bool{}
	for i, h := range p.Holidays {
		if err := h.Validate(); err != nil {
			add(fmt.Sprintf("holidays[%d].day", i), "Sviatky: "+err.Error())
		} else if holSeen[h.Key()] {
			add(fmt.Sprintf("holidays[%d].day", i), "Sviatok "+h.Day+" je zadaný dvakrát s rovnakým rokom od")
		}
		holSeen[h.Key()] = true
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Normalize zoradí tabuľky (pred uložením).
func (p *Params) Normalize() {
	p.ECB = reminder.Sorted(p.ECB)
	sortKm(p.KmRates)
	sortHolidays(p.Holidays)
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
