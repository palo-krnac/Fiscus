// Package fx sú referenčné výmenné kurzy ECB: koľko jednotiek cudzej meny
// je 1 euro. Pri faktúre v cudzej mene sa základ a daň prepočítajú kurzom
// vyhláseným v deň predchádzajúci dňu vzniku daňovej povinnosti (§ 26 ods. 1
// zákona o DPH); ak v ten deň ECB kurz nevyhlásila (víkend, sviatok), platí
// posledný vyhlásený.
package fx

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Zdroje kurzov ECB.
const (
	ECBDaily = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"
	ECB90    = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist-90d.xml"
)

// Pôvod kurzu.
const (
	SourceECB    = "ecb"
	SourceManual = "manual" // zadaný ručne (napr. kurz NBS alebo výpadok ECB)
)

// Rate je kurz meny k dátumu vyhlásenia.
type Rate struct {
	Currency string    `json:"currency"`
	Date     string    `json:"date"` // YYYY-MM-DD
	Rate     money.Dec `json:"rate"` // jednotiek meny za 1 EUR
	Source   string    `json:"source"`
}

// Filter obmedzuje zoznam kurzov.
type Filter struct {
	Currency string // "" = všetky
	Year     int    // 0 = všetky
}

// Currency je mena v číselníku.
type Currency struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Currencies sú meny, pre ktoré ECB vyhlasuje referenčný kurz.
var Currencies = []Currency{
	{"CZK", "česká koruna"}, {"USD", "americký dolár"}, {"GBP", "britská libra"}, {"HUF", "maďarský forint"},
	{"PLN", "poľský zlotý"}, {"CHF", "švajčiarsky frank"}, {"RON", "rumunský lei"}, {"DKK", "dánska koruna"},
	{"SEK", "švédska koruna"}, {"NOK", "nórska koruna"}, {"ISK", "islandská koruna"}, {"TRY", "turecká líra"},
	{"JPY", "japonský jen"}, {"CNY", "čínsky jüan"}, {"CAD", "kanadský dolár"}, {"AUD", "austrálsky dolár"},
	{"NZD", "novozélandský dolár"}, {"HKD", "hongkonský dolár"}, {"SGD", "singapurský dolár"}, {"KRW", "kórejský won"},
	{"INR", "indická rupia"}, {"IDR", "indonézska rupia"}, {"MYR", "malajzijský ringgit"}, {"PHP", "filipínske peso"},
	{"THB", "thajský baht"}, {"ILS", "izraelský šekel"}, {"MXN", "mexické peso"}, {"BRL", "brazílsky real"},
	{"ZAR", "juhoafrický rand"},
}

// Known vráti true pre menu z číselníka (EUR nie je cudzia mena).
func Known(code string) bool {
	for _, c := range Currencies {
		if c.Code == code {
			return true
		}
	}
	return false
}

// Validate overí ručne zadaný kurz.
func (r Rate) Validate() error {
	if !Known(r.Currency) {
		return fmt.Errorf("neznáma mena %q", r.Currency)
	}
	if _, err := time.Parse(time.DateOnly, r.Date); err != nil {
		return errors.New("neplatný dátum kurzu")
	}
	if r.Rate.Sign() <= 0 {
		return errors.New("kurz musí byť väčší ako nula")
	}
	return nil
}

// TaxDay vráti posledný deň, ktorého kurz sa smie použiť pre daňovú
// povinnosť vzniknutú v deň date: deň predtým.
func TaxDay(date string) string {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return ""
	}
	return d.AddDate(0, 0, -1).Format(time.DateOnly)
}

// Before vráti platný kurz meny pre daňovú povinnosť v deň date: posledný
// vyhlásený najneskôr v predchádzajúci deň. rates môžu byť v ľubovoľnom poradí.
func Before(rates []Rate, currency, date string) (Rate, bool) {
	day := TaxDay(date)
	var best Rate
	found := false
	for _, r := range rates {
		if r.Currency == currency && r.Date <= day && (!found || r.Date > best.Date) {
			best, found = r, true
		}
	}
	return best, found
}

// ParseECB prečíta XML kurzov ECB (denný súbor aj história).
func ParseECB(r io.Reader) ([]Rate, error) {
	var doc struct {
		Days []struct {
			Time  string `xml:"time,attr"`
			Rates []struct {
				Currency string `xml:"currency,attr"`
				Rate     string `xml:"rate,attr"`
			} `xml:"Cube"`
		} `xml:"Cube>Cube"`
	}
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("kurzy ECB: %w", err)
	}
	var out []Rate
	for _, d := range doc.Days {
		if _, err := time.Parse(time.DateOnly, d.Time); err != nil {
			return nil, fmt.Errorf("kurzy ECB: neplatný dátum %q", d.Time)
		}
		for _, x := range d.Rates {
			if !Known(x.Currency) {
				continue // meny mimo číselníka (napr. zrušené)
			}
			v, err := money.Parse(strings.TrimSpace(x.Rate))
			if err != nil || v.Sign() <= 0 {
				return nil, fmt.Errorf("kurzy ECB: neplatný kurz %s %q", x.Currency, x.Rate)
			}
			out = append(out, Rate{Currency: x.Currency, Date: d.Time, Rate: v, Source: SourceECB})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("kurzy ECB: súbor neobsahuje žiadne kurzy")
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].Currency < out[j].Currency
	})
	return out, nil
}
