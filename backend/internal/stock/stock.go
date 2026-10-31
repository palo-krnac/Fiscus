// Package stock je sklad: skladové karty položiek cenníka, doklady
// (príjemka, výdajka, inventúrny rozdiel) a ocenenie zásob váženým
// aritmetickým priemerom (§ 25 ods. 4 písm. a) zákona o účtovníctve —
// priemer sa prepočíta pri každom príjme).
package stock

import (
	"fmt"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Kind je druh skladového dokladu.
type Kind string

const (
	Receipt  Kind = "receipt"  // príjemka
	Issue    Kind = "issue"    // výdajka
	Count    Kind = "count"    // inventúrny rozdiel (+ prebytok, − manko)
	Transfer Kind = "transfer" // presun medzi skladmi
)

// Method je spôsob ocenenia úbytku zásob (§ 25 ods. 4 písm. a) zákona
// o účtovníctve).
type Method string

const (
	Average Method = "avg"  // vážený aritmetický priemer
	FIFO    Method = "fifo" // prvá cena na ocenenie prírastku = prvá na ocenenie úbytku
)

// Methods sú spôsoby ocenenia s popisom.
var Methods = []struct {
	Code  Method `json:"code"`
	Label string `json:"label"`
}{{Average, "vážený aritmetický priemer"}, {FIFO, "FIFO (prvý do skladu, prvý zo skladu)"}}

// Valid vráti true pre známy spôsob.
func (m Method) Valid() bool { return m == Average || m == FIFO }

// Kinds sú druhy dokladov s popisom a predponou čísla.
var Kinds = []struct {
	Code   Kind   `json:"code"`
	Label  string `json:"label"`
	Prefix string `json:"prefix"`
}{
	{Receipt, "Príjemka", "PR"}, {Issue, "Výdajka", "VY"}, {Count, "Inventúrny rozdiel", "IR"}, {Transfer, "Presun", "PS"},
}

// Prefix vráti predponu čísla dokladu.
func (k Kind) Prefix() string {
	for _, x := range Kinds {
		if x.Code == k {
			return x.Prefix
		}
	}
	return ""
}

// ItemKind je druh zásoby (určuje účty v podvojnom účtovníctve).
type ItemKind string

const (
	Goods    ItemKind = "goods"    // tovar (132)
	Material ItemKind = "material" // materiál (112)
	Product  ItemKind = "product"  // výrobky (123)
)

// ItemKinds sú druhy zásob s popisom.
var ItemKinds = []struct {
	Code  ItemKind `json:"code"`
	Label string   `json:"label"`
}{{Goods, "tovar"}, {Material, "materiál"}, {Product, "výrobky"}}

// Valid vráti true pre známy druh zásoby.
func (k ItemKind) Valid() bool { return k == Goods || k == Material || k == Product }

// Line je riadok dokladu: položka cenníka, množstvo a ocenenie.
type Line struct {
	ItemID   int64     `json:"itemId"`
	Name     string    `json:"name"`
	Unit     string    `json:"unit"`
	Quantity money.Dec `json:"quantity"` // pri inventúrnom rozdiele so znamienkom
	// UnitCost je obstarávacia cena za jednotku bez DPH (príjemka, prebytok);
	// pri výdaji a manku ju dopočíta ocenenie.
	UnitCost money.Dec `json:"unitCost"`
	Total    money.Dec `json:"total"`
}

// Doc je skladový doklad.
type Doc struct {
	ID        int64        `json:"id"`
	CompanyID int64        `json:"companyId"`
	Number    string       `json:"number"`
	Kind      Kind         `json:"kind"`
	Date      invoice.Date `json:"date"`
	Warehouse string       `json:"warehouse"`
	// To je cieľový sklad presunu.
	To         string `json:"to,omitempty"`
	Partner    string `json:"partner"`
	Note       string `json:"note"`
	LinkKind   string `json:"linkKind,omitempty"` // invoice | purchase
	LinkID     int64  `json:"linkId,omitempty"`
	LinkNumber string `json:"linkNumber,omitempty"`
	// Auto: výdajku vytvoril Fiškus z vystavenej faktúry (upravuje sa s ňou).
	Auto bool `json:"auto,omitempty"`
	// Return: príjem vrátenej zásoby (dobropis) — ocení sa priemernou cenou.
	Return bool   `json:"return,omitempty"`
	Lines  []Line `json:"lines"`
	// Total je hodnota dokladu (súčet riadkov; dopĺňa ocenenie).
	Total money.Dec `json:"total"`
}

// Validate skontroluje doklad (položky overí aplikácia).
func (d *Doc) Validate() error {
	var fe []invoice.FieldError
	bad := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	d.Partner, d.Note, d.Warehouse = strings.TrimSpace(d.Partner), strings.TrimSpace(d.Note), strings.TrimSpace(d.Warehouse)
	if d.Kind.Prefix() == "" {
		bad("kind", "Neznámy druh dokladu")
	}
	if d.Date.IsZero() {
		bad("date", "Zadajte dátum")
	}
	if d.Warehouse == "" {
		bad("warehouse", "Vyberte sklad")
	}
	d.To = strings.TrimSpace(d.To)
	switch {
	case d.Kind != Transfer:
		d.To = ""
	case d.To == "":
		bad("to", "Vyberte cieľový sklad")
	case d.To == d.Warehouse:
		bad("to", "Cieľový sklad musí byť iný")
	}
	if len(d.Lines) == 0 {
		bad("lines", "Pridajte aspoň jednu položku")
	}
	for i := range d.Lines {
		l := &d.Lines[i]
		f := func(s string) string { return fmt.Sprintf("lines[%d].%s", i, s) }
		if l.ItemID == 0 {
			bad(f("itemId"), "Vyberte položku")
		}
		switch {
		case l.Quantity.IsZero():
			bad(f("quantity"), "Zadajte množstvo")
		case d.Kind != Count && l.Quantity.Sign() < 0:
			bad(f("quantity"), "Množstvo musí byť kladné")
		}
		if l.UnitCost.Sign() < 0 {
			bad(f("unitCost"), "Cena nemôže byť záporná")
		}
		if d.Kind == Issue || d.Kind == Transfer || (d.Kind == Receipt && d.Return) {
			l.UnitCost, l.Total = money.Zero, money.Zero // ocení sa priemerom
		} else if d.Kind == Receipt || l.Quantity.Sign() > 0 {
			l.Total = l.Quantity.Mul(l.UnitCost).RoundCents()
		}
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Key je skladová karta: sklad a položka.
type Key struct {
	Warehouse string `json:"warehouse"`
	ItemID    int64  `json:"itemId"`
}

// Card je stav skladovej karty.
type Card struct {
	Key
	Quantity money.Dec `json:"quantity"`
	Value    money.Dec `json:"value"`
	AvgCost  money.Dec `json:"avgCost"` // priemerná cena za jednotku
	// Moves sú pohyby na karte v poradí ocenenia.
	Moves []Move `json:"moves"`
}

// Move je pohyb na skladovej karte so stavom po pohybe.
type Move struct {
	DocID    int64        `json:"docId"`
	Number   string       `json:"number"`
	Kind     Kind         `json:"kind"`
	Date     invoice.Date `json:"date"`
	Partner  string       `json:"partner,omitempty"`
	Quantity money.Dec    `json:"quantity"` // + príjem, − výdaj
	UnitCost money.Dec    `json:"unitCost"`
	Total    money.Dec    `json:"total"` // + príjem, − výdaj
	Balance  money.Dec    `json:"balance"`
	Value    money.Dec    `json:"value"`
}

// Problem je chyba pri ocenení (napr. výdaj nad stav).
type Problem struct {
	DocID   int64  `json:"docId"`
	Number  string `json:"number"`
	Message string `json:"message"`
}

// order zoradí doklady na ocenenie: dátum, príjmy pred výdajmi, vrátenia
// na konci dňa, poradie vzniku.
func order(docs []Doc) []Doc {
	out := append([]Doc(nil), docs...)
	rank := func(d Doc) int {
		switch {
		case d.Return: // vrátenie po predaji z toho istého dňa
			return 2
		case d.Kind == Receipt || d.Kind == Count:
			return 0
		}
		return 1
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Date != b.Date {
			return a.Date.Before(b.Date)
		}
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		return a.ID < b.ID
	})
	return out
}

// layer je vrstva zásoby pri FIFO: množstvo a jeho hodnota.
type layer struct{ qty, value money.Dec }

// state je stav karty počas ocenenia.
type state struct {
	*Card
	layers   []layer
	lastCost money.Dec // posledná cena za jednotku (vrátenie, prebytok bez ceny pri FIFO)
}

func (c *state) add(m Method, q, total money.Dec) {
	short := c.Quantity.Sign() < 0
	c.Quantity, c.Value = c.Quantity.Add(q), c.Value.Add(total)
	if m == FIFO {
		if short {
			// príjem najprv pokryje výdaj nad stav; zvyšok je jedna vrstva so
			// zostatkovou hodnotou (inak by rozdiel ocenenia z karty zmizol)
			c.layers = nil
			if c.Quantity.Sign() > 0 {
				c.layers = []layer{{c.Quantity, c.Value}}
			}
		} else {
			c.layers = append(c.layers, layer{q, total})
		}
	}
	if c.Quantity.Sign() > 0 {
		c.AvgCost = c.Value.MulRatio(money.FromInt(1), c.Quantity)
	}
	if q.Sign() > 0 {
		c.lastCost = total.MulRatio(money.FromInt(1), q)
	}
}

// take vyberie množstvo: vráti ocenenie a či na sklade chýbalo.
func (c *state) take(m Method, q money.Dec) (cost money.Dec, short bool) {
	if m == FIFO {
		rest := q
		for rest.Sign() > 0 && len(c.layers) > 0 {
			l := &c.layers[0]
			if l.qty.Cmp(rest) <= 0 {
				cost, rest = cost.Add(l.value), rest.Sub(l.qty)
				c.lastCost = l.value.MulRatio(money.FromInt(1), l.qty)
				c.layers = c.layers[1:]
				continue
			}
			part := l.value.MulRatio(rest, l.qty).RoundCents()
			c.lastCost = l.value.MulRatio(money.FromInt(1), l.qty)
			l.qty, l.value = l.qty.Sub(rest), l.value.Sub(part)
			cost, rest = cost.Add(part), money.Zero
		}
		if rest.Sign() > 0 {
			short = true
			cost = cost.Add(rest.Mul(c.lastCost)).RoundCents()
		}
	} else {
		switch c.Quantity.Cmp(q) {
		case 0:
			cost = c.Value
		case 1:
			cost = c.Value.MulRatio(q, c.Quantity).RoundCents()
		default:
			short = true
			cost = q.Mul(c.AvgCost).RoundCents()
			if c.Quantity.Sign() > 0 {
				cost = c.Value.Add(q.Sub(c.Quantity).Mul(c.AvgCost)).RoundCents()
			}
		}
	}
	c.Quantity, c.Value = c.Quantity.Sub(q), c.Value.Sub(cost)
	if c.Quantity.Sign() <= 0 {
		c.layers = nil
		if c.Quantity.IsZero() {
			c.Value = money.Zero
		}
	}
	if c.Quantity.Sign() > 0 {
		c.AvgCost = c.Value.MulRatio(money.FromInt(1), c.Quantity)
	}
	return cost, short
}

// Valuate ocení doklady zvoleným spôsobom (vážený priemer alebo FIFO):
// vráti stav kariet, doklady s doplnenými cenami výdaja a problémy. Výdaj
// nad stav sa ocení poslednou známou cenou a hlási sa ako problém.
func Valuate(docs []Doc, m Method) (map[Key]*Card, []Doc, []Problem) {
	if !m.Valid() {
		m = Average
	}
	states := map[Key]*state{}
	get := func(k Key) *state {
		c := states[k]
		if c == nil {
			c = &state{Card: &Card{Key: k}}
			states[k] = c
		}
		return c
	}
	var probs []Problem
	out := order(docs)
	for di := range out {
		d := &out[di]
		d.Lines = append([]Line(nil), d.Lines...)
		for li := range d.Lines {
			l := &d.Lines[li]
			c := get(Key{d.Warehouse, l.ItemID})
			q := l.Quantity
			if q.Sign() < 0 {
				q = q.Neg()
			}
			mv := func(c *state, qty, total money.Dec) {
				c.Moves = append(c.Moves, Move{DocID: d.ID, Number: d.Number, Kind: d.Kind, Date: d.Date, Partner: d.Partner,
					Quantity: qty, UnitCost: l.UnitCost, Total: total, Balance: c.Quantity, Value: c.Value})
			}
			if d.Kind == Receipt || (d.Kind == Count && l.Quantity.Sign() > 0) {
				if d.Return || (d.Kind == Count && l.UnitCost.IsZero()) {
					// vrátenie a prebytok bez ceny: priemernou (pri FIFO poslednou) cenou
					unit := c.AvgCost
					if m == FIFO && c.lastCost.Sign() > 0 {
						unit = c.lastCost
					}
					l.UnitCost, l.Total = unit, q.Mul(unit).RoundCents()
				}
				c.add(m, q, l.Total)
				mv(c, q, l.Total)
				continue
			}
			cost, short := c.take(m, q)
			if short {
				probs = append(probs, Problem{DocID: d.ID, Number: d.Number, Message: fmt.Sprintf("%s: výdaj %s %s, na sklade %s chýba %s",
					l.Name, q.Display(), l.Unit, d.Warehouse, c.Quantity.Neg().Display())})
			}
			l.Total, l.UnitCost = cost, money.Zero
			if q.Sign() > 0 {
				l.UnitCost = cost.MulRatio(money.FromInt(1), q)
			}
			mv(c, q.Neg(), cost.Neg())
			if d.Kind == Transfer {
				to := get(Key{d.To, l.ItemID})
				to.add(m, q, cost)
				mv(to, q, cost)
			}
		}
	}
	cards := map[Key]*Card{}
	for k, c := range states {
		cards[k] = c.Card
	}
	for i := range out {
		out[i].Total = money.Zero
		for _, l := range out[i].Lines {
			out[i].Total = out[i].Total.Add(l.Total)
		}
	}
	return cards, out, probs
}

// NextNumber vráti ďalšie číslo dokladu druhu v roku: PR2026001.
func NextNumber(docs []Doc, k Kind, year int) string {
	prefix := fmt.Sprintf("%s%d", k.Prefix(), year)
	n := 0
	for _, d := range docs {
		if rest, ok := strings.CutPrefix(d.Number, prefix); ok {
			var x int
			if _, err := fmt.Sscanf(rest, "%d", &x); err == nil && x > n {
				n = x
			}
		}
	}
	return fmt.Sprintf("%s%03d", prefix, n+1)
}
