package calendar

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Kind je druh termínu (určuje farbu a skupinu v kalendári).
type Kind string

const (
	KindTax       Kind = "tax"       // daňová lehota (DPH, KV, SV, daň z príjmov, DMV)
	KindReceive   Kind = "receive"   // splatnosť vystavenej faktúry
	KindPay       Kind = "pay"       // splatnosť prijatej faktúry
	KindRecurring Kind = "recurring" // pravidelná faktúra
	KindQuote     Kind = "quote"     // koniec platnosti ponuky
	KindOwn       Kind = "own"       // vlastný termín
	KindHoliday   Kind = "holiday"   // deň pracovného pokoja
)

// Event je jeden termín v kalendári.
type Event struct {
	ID      string       `json:"id"` // stabilný identifikátor (aj UID v iCalendar)
	Date    invoice.Date `json:"date"`
	Kind    Kind         `json:"kind"`
	Title   string       `json:"title"`
	Detail  string       `json:"detail,omitempty"`
	Amount  *money.Dec   `json:"amount,omitempty"`
	Curr    string       `json:"currency,omitempty"`
	Link    string       `json:"link,omitempty"`    // cesta v rozhraní (/faktury/12)
	Done    bool         `json:"done,omitempty"`    // splnené (podané, uhradené)
	Overdue bool         `json:"overdue,omitempty"` // po termíne a nesplnené
	// Shifted je pôvodný dátum lehoty, ak sa posunula na pracovný deň.
	Shifted invoice.Date `json:"shifted,omitzero"`
}

// Sort zoradí termíny podľa dátumu, druhu a názvu.
func Sort(es []Event) {
	order := map[Kind]int{KindHoliday: 0, KindTax: 1, KindOwn: 2, KindPay: 3, KindReceive: 4, KindRecurring: 5, KindQuote: 6}
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.Date != b.Date {
			return a.Date.Before(b.Date)
		}
		if order[a.Kind] != order[b.Kind] {
			return order[a.Kind] < order[b.Kind]
		}
		return a.Title < b.Title
	})
}

// Repeat je opakovanie vlastného termínu.
type Repeat string

const (
	RepeatNone      Repeat = ""
	RepeatMonthly   Repeat = "monthly"
	RepeatQuarterly Repeat = "quarterly"
	RepeatYearly    Repeat = "yearly"
)

func (r Repeat) months() int {
	switch r {
	case RepeatMonthly:
		return 1
	case RepeatQuarterly:
		return 3
	case RepeatYearly:
		return 12
	}
	return 0
}

// Own je vlastný termín firmy (odvody, preddavky, STK, poistka…).
type Own struct {
	ID     int64        `json:"id"`
	Title  string       `json:"title"`
	Note   string       `json:"note"`
	Date   invoice.Date `json:"date"`   // prvý (alebo jediný) termín
	Repeat Repeat       `json:"repeat"` // opakovanie
	Until  invoice.Date `json:"until"`  // posledný možný termín (nepovinné)
	// Workday: termín pripadajúci na víkend alebo sviatok sa posunie na
	// najbližší pracovný deň (zákonné lehoty).
	Workday bool `json:"workday"`
	// Done sú dni (pôvodné, neposunuté), ktoré používateľ označil ako splnené.
	Done []invoice.Date `json:"done"`
}

// ErrOwn — neplatný vlastný termín.
var ErrOwn = errors.New("neplatný termín")

// Validate skontroluje vlastný termín.
func (o *Own) Validate() error {
	var fe []invoice.FieldError
	o.Title = strings.TrimSpace(o.Title)
	if o.Title == "" || len([]rune(o.Title)) > 120 {
		fe = append(fe, invoice.FieldError{Field: "title", Message: "Zadajte názov (najviac 120 znakov)"})
	}
	if o.Date.IsZero() {
		fe = append(fe, invoice.FieldError{Field: "date", Message: "Zadajte dátum"})
	}
	if o.Repeat != RepeatNone && o.Repeat.months() == 0 {
		fe = append(fe, invoice.FieldError{Field: "repeat", Message: "Neznáme opakovanie"})
	}
	if !o.Until.IsZero() && !o.Date.IsZero() && o.Until.Before(o.Date) {
		fe = append(fe, invoice.FieldError{Field: "until", Message: "Koniec je pred prvým termínom"})
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// maxOccurrences chráni pred nekonečným cyklom.
const maxOccurrences = 1200

// Occurrences vráti výskyty termínu v rozsahu [from, to]. Pôvodný dátum
// výskytu (pred posunom) je v Shifted, ak sa posunul.
func (o Own) Occurrences(from, to invoice.Date, days *Days) []Event {
	var out []Event
	step := o.Repeat.months()
	done := map[invoice.Date]bool{}
	for _, d := range o.Done {
		done[d] = true
	}
	for k := 0; k < maxOccurrences; k++ {
		base := o.Date
		if step > 0 {
			base = o.Date.AddMonths(k * step)
		} else if k > 0 {
			break
		}
		if !o.Until.IsZero() && base.After(o.Until) {
			break
		}
		day := base
		if o.Workday && days != nil {
			day = days.NextWorkday(base)
		}
		if day.After(to) {
			break
		}
		if day.Before(from) {
			continue
		}
		e := Event{ID: fmt.Sprintf("own-%d-%s", o.ID, base), Date: day, Kind: KindOwn, Title: o.Title, Detail: o.Note,
			Done: done[base], Link: "/kalendar"}
		if day != base {
			e.Shifted = base
		}
		out = append(out, e)
	}
	return out
}

// SetDone označí výskyt (pôvodný dátum) ako splnený alebo nesplnený.
func (o *Own) SetDone(base invoice.Date, done bool) {
	out := o.Done[:0]
	for _, d := range o.Done {
		if d != base {
			out = append(out, d)
		}
	}
	if done {
		out = append(out, base)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	o.Done = out
}

// Occurs vráti true, ak termín pripadá (pred posunom) na daný deň.
func (o Own) Occurs(base invoice.Date) bool {
	step := o.Repeat.months()
	for k := 0; k < maxOccurrences; k++ {
		d := o.Date
		if step > 0 {
			d = o.Date.AddMonths(k * step)
		} else if k > 0 {
			return false
		}
		if d == base {
			return true
		}
		if d.After(base) || (!o.Until.IsZero() && d.After(o.Until)) {
			return false
		}
	}
	return false
}
