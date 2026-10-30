package vatreturn

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// Period je zdaňovacie obdobie: mesiac alebo štvrťrok.
type Period struct {
	Year    int `json:"year"`
	Month   int `json:"month,omitempty"`   // 1–12 pri mesačnom období
	Quarter int `json:"quarter,omitempty"` // 1–4 pri štvrťročnom období
}

// FirstSupported je prvé obdobie, pre ktoré platí tlačivo DPHv25 (od 1. 7. 2025).
var FirstSupported = invoice.NewDate(2025, time.July, 1)

// ParsePeriod prečíta "2026-10" (mesiac) alebo "2026-Q3" (štvrťrok).
func ParsePeriod(s string) (Period, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	y, rest, ok := strings.Cut(s, "-")
	year, err := strconv.Atoi(y)
	if !ok || err != nil || year < 2000 || year > 2100 {
		return Period{}, fmt.Errorf("neplatné obdobie %q (napr. 2026-10 alebo 2026-Q3)", s)
	}
	if q, isQ := strings.CutPrefix(rest, "Q"); isQ {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 4 {
			return Period{}, fmt.Errorf("neplatný štvrťrok %q", s)
		}
		return Period{Year: year, Quarter: n}, nil
	}
	m, err := strconv.Atoi(rest)
	if err != nil || m < 1 || m > 12 {
		return Period{}, fmt.Errorf("neplatný mesiac %q", s)
	}
	return Period{Year: year, Month: m}, nil
}

// PeriodOf vráti zdaňovacie obdobie, do ktorého patrí deň.
func PeriodOf(d invoice.Date, quarterly bool) Period {
	if quarterly {
		return Period{Year: d.Year(), Quarter: (d.Month()-1)/3 + 1}
	}
	return Period{Year: d.Year(), Month: d.Month()}
}

func (p Period) String() string {
	if p.Quarter > 0 {
		return fmt.Sprintf("%d-Q%d", p.Year, p.Quarter)
	}
	return fmt.Sprintf("%d-%02d", p.Year, p.Month)
}

var months = []string{"január", "február", "marec", "apríl", "máj", "jún", "júl", "august", "september", "október", "november", "december"}

// Label je názov obdobia: „október 2026“, „3. štvrťrok 2026“.
func (p Period) Label() string {
	if p.Quarter > 0 {
		return fmt.Sprintf("%d. štvrťrok %d", p.Quarter, p.Year)
	}
	return fmt.Sprintf("%s %d", months[p.Month-1], p.Year)
}

// Start je prvý deň obdobia.
func (p Period) Start() invoice.Date {
	if p.Quarter > 0 {
		return invoice.NewDate(p.Year, time.Month(3*p.Quarter-2), 1)
	}
	return invoice.NewDate(p.Year, time.Month(p.Month), 1)
}

// End je posledný deň obdobia.
func (p Period) End() invoice.Date {
	n := 1
	if p.Quarter > 0 {
		n = 3
	}
	s := p.Start()
	return invoice.NewDate(s.Year(), time.Month(s.Month()+n), 1).AddDays(-1)
}

// Contains vráti true, ak dátum patrí do obdobia.
func (p Period) Contains(d invoice.Date) bool {
	return !d.IsZero() && !d.Before(p.Start()) && !d.After(p.End())
}

// Supported vráti true, ak sa obdobie dá podať na tlačive DPHv25 / KVDPHv25.
func (p Period) Supported() bool { return !p.Start().Before(FirstSupported) }
