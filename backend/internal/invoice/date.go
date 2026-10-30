package invoice

import (
	"fmt"
	"strings"
	"time"
)

// Date je kalendárny dátum bez času (JSON "2026-10-29").
type Date struct{ t time.Time }

const dateLayout = "2006-01-02"

// NewDate vytvorí dátum.
func NewDate(y int, m time.Month, d int) Date { return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)} }

// ParseDate prečíta "2026-10-29".
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	return Date{t}, err
}

// MustDate je ParseDate pre testy.
func MustDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Today vráti dnešný dátum v slovenskom časovom pásme.
func Today() Date {
	loc, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		loc = time.UTC
	}
	n := time.Now().In(loc)
	return NewDate(n.Year(), n.Month(), n.Day())
}

func (d Date) IsZero() bool         { return d.t.IsZero() }
func (d Date) Year() int            { return d.t.Year() }
func (d Date) Month() int           { return int(d.t.Month()) }
func (d Date) Day() int             { return d.t.Day() }
func (d Date) Before(o Date) bool   { return d.t.Before(o.t) }
func (d Date) After(o Date) bool    { return d.t.After(o.t) }
func (d Date) AddDays(n int) Date   { return Date{d.t.AddDate(0, 0, n)} }
func (d Date) DaysUntil(o Date) int { return int(o.t.Sub(d.t).Hours() / 24) }

// Weekday vráti deň v týždni.
func (d Date) Weekday() time.Weekday { return d.t.Weekday() }

// AddMonths pridá mesiace; deň presahujúci kratší mesiac sa zníži na jeho posledný deň.
func (d Date) AddMonths(n int) Date {
	first := time.Date(d.t.Year(), d.t.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return NewDate(first.Year(), first.Month(), min(d.t.Day(), last))
}
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.t.Format(dateLayout)
}

// Display vráti dátum v slovenskom zápise „20. 12. 2024“.
func (d Date) Display() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d. %d. %d", d.t.Day(), int(d.t.Month()), d.t.Year())
}

func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.String() + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*d = Date{}
		return nil
	}
	v, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}
