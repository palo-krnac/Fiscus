// Package calendar je kalendár termínov: dni pracovného pokoja, posun
// lehoty na pracovný deň, vlastné opakované termíny a export do iCalendar.
package calendar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// Holiday je deň pracovného pokoja podľa zákona č. 241/1993 Z. z. Day je
// "MM-DD" alebo "easter-2" (Veľký piatok) / "easter+1" (Veľkonočný pondelok).
// From a To sú roky platnosti (0 = bez obmedzenia), Except roky, keď deň nie
// je dňom pracovného pokoja (napr. 8. máj a 15. september v roku 2026).
type Holiday struct {
	Day    string `json:"day"`
	Name   string `json:"name"`
	From   int    `json:"from,omitempty"`
	To     int    `json:"to,omitempty"`
	Except []int  `json:"except,omitempty"`
}

// DefaultHolidays sú dni pracovného pokoja v znení účinnom od 1. 11. 2025
// (zákon č. 241/1993 Z. z., § 2 a § 4b). 1. september a 17. november sú
// štátne sviatky, ale nie dni pracovného pokoja (1. 9. od roku 2024, 17. 11.
// od 1. 11. 2025); 28. október nie
// je dňom pracovného pokoja od roku 2021.
var DefaultHolidays = []Holiday{
	{Day: "01-01", Name: "Deň vzniku Slovenskej republiky"},
	{Day: "01-06", Name: "Zjavenie Pána (Traja králi)"},
	{Day: "easter-2", Name: "Veľký piatok"},
	{Day: "easter+1", Name: "Veľkonočný pondelok"},
	{Day: "05-01", Name: "Sviatok práce"},
	{Day: "05-08", Name: "Deň víťazstva nad fašizmom", Except: []int{2026}},
	{Day: "07-05", Name: "Sviatok svätého Cyrila a Metoda"},
	{Day: "08-29", Name: "Výročie SNP"},
	{Day: "09-01", Name: "Deň Ústavy SR", To: 2023},
	{Day: "09-15", Name: "Sedembolestná Panna Mária", Except: []int{2026}},
	{Day: "10-28", Name: "Deň vzniku samostatného česko-slovenského štátu", To: 2020},
	{Day: "11-01", Name: "Sviatok všetkých svätých"},
	{Day: "11-17", Name: "Deň boja za slobodu a demokraciu", To: 2024},
	{Day: "12-24", Name: "Štedrý deň"},
	{Day: "12-25", Name: "Prvý sviatok vianočný"},
	{Day: "12-26", Name: "Druhý sviatok vianočný"},
}

// Key je kľúč predvoleného riadka (pre doplnenie novou verziou programu).
func (h Holiday) Key() string { return "hol:" + h.Day + ":" + strconv.Itoa(h.From) }

// Easter vráti Veľkonočnú nedeľu (gregoriánsky kalendár, anonymný algoritmus).
func Easter(year int) invoice.Date {
	a := year % 19
	b, c := year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return invoice.NewDate(year, time.Month(month), day)
}

// Date vráti dátum sviatku v roku; ok = false, ak v tom roku neplatí.
func (h Holiday) Date(year int) (invoice.Date, bool) {
	if (h.From != 0 && year < h.From) || (h.To != 0 && year > h.To) {
		return invoice.Date{}, false
	}
	for _, y := range h.Except {
		if y == year {
			return invoice.Date{}, false
		}
	}
	if off, ok := strings.CutPrefix(h.Day, "easter"); ok {
		n, err := strconv.Atoi(off)
		if err != nil {
			return invoice.Date{}, false
		}
		return Easter(year).AddDays(n), true
	}
	d, err := invoice.ParseDate(fmt.Sprintf("%04d-%s", year, h.Day))
	return d, err == nil
}

// Validate skontroluje zápis dňa.
func (h Holiday) Validate() error {
	if off, ok := strings.CutPrefix(h.Day, "easter"); ok {
		if n, err := strconv.Atoi(off); err != nil || n < -60 || n > 60 {
			return fmt.Errorf("deň %q: pri Veľkej noci zadajte posun, napr. easter-2 alebo easter+1", h.Day)
		}
	} else if _, err := invoice.ParseDate("2024-" + h.Day); err != nil || len(h.Day) != 5 {
		return fmt.Errorf("deň %q zadajte ako MM-DD, napr. 05-01", h.Day)
	}
	if strings.TrimSpace(h.Name) == "" {
		return fmt.Errorf("deň %s: zadajte názov", h.Day)
	}
	if h.From != 0 && h.To != 0 && h.To < h.From {
		return fmt.Errorf("deň %s: rok „do“ je pred rokom „od“", h.Day)
	}
	return nil
}

// Days je množina dní pracovného pokoja (okrem víkendov).
type Days struct {
	list  []Holiday
	cache map[int]map[invoice.Date]string
}

// NewDays pripraví výpočet pracovných dní zo zoznamu sviatkov.
func NewDays(hs []Holiday) *Days {
	return &Days{list: hs, cache: map[int]map[invoice.Date]string{}}
}

func (d *Days) year(y int) map[invoice.Date]string {
	if m, ok := d.cache[y]; ok {
		return m
	}
	m := map[invoice.Date]string{}
	for _, h := range d.list {
		if dt, ok := h.Date(y); ok {
			m[dt] = h.Name
		}
	}
	d.cache[y] = m
	return m
}

// Holiday vráti názov sviatku (deň pracovného pokoja) alebo "".
func (d *Days) Holiday(day invoice.Date) string { return d.year(day.Year())[day] }

// Workday vráti true pre pracovný deň (nie víkend ani deň pracovného pokoja).
func (d *Days) Workday(day invoice.Date) bool {
	w := day.Weekday()
	return w != time.Saturday && w != time.Sunday && d.Holiday(day) == ""
}

// NextWorkday posunie lehotu, ktorej koniec pripadne na sobotu, nedeľu alebo
// deň pracovného pokoja, na najbližší pracovný deň (§ 27 ods. 2 daňového
// poriadku, § 122 ods. 3 Občianskeho zákonníka).
func (d *Days) NextWorkday(day invoice.Date) invoice.Date {
	for i := 0; i < 14 && !d.Workday(day); i++ {
		day = day.AddDays(1)
	}
	return day
}

// InRange vráti dni pracovného pokoja v rozsahu (vrátane okrajov).
func (d *Days) InRange(from, to invoice.Date) []Event {
	var out []Event
	for y := from.Year(); y <= to.Year(); y++ {
		for day, name := range d.year(y) {
			if !day.Before(from) && !day.After(to) {
				out = append(out, Event{Date: day, Kind: KindHoliday, Title: name})
			}
		}
	}
	Sort(out)
	return out
}
