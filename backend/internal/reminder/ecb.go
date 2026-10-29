package reminder

import (
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Zabudovaná tabuľka: základná úroková sadzba ECB (hlavné refinančné operácie)
// od daného dňa. Zdroj: ECB, prehľad sadzieb úrokov z omeškania na najpravo.sk.
// Je to iba predvyplnenie: platné sadzby sú v Nastaveniach → Legislatíva
// (balík legal). Nové riadky doplnené sem pri novej verzii sa doplnia aj tam.
var ecbRates = []struct {
	from string
	rate string
}{
	{"2014-09-10", "0.05"},
	{"2016-03-16", "0.00"},
	{"2022-07-27", "0.50"},
	{"2022-09-14", "1.25"},
	{"2022-11-02", "2.00"},
	{"2022-12-21", "2.50"},
	{"2023-02-08", "3.00"},
	{"2023-03-22", "3.50"},
	{"2023-05-10", "3.75"},
	{"2023-06-21", "4.00"},
	{"2023-08-02", "4.25"},
	{"2023-09-20", "4.50"},
	{"2024-06-12", "4.25"},
	{"2024-09-18", "3.65"},
	{"2024-10-23", "3.40"},
	{"2024-12-18", "3.15"},
	{"2025-02-05", "2.90"},
	{"2025-03-12", "2.65"},
	{"2025-04-23", "2.40"},
	{"2025-06-11", "2.15"},
	{"2026-06-17", "2.40"},
	{"2026-09-16", "2.65"},
}

// Rate je základná úroková sadzba ECB platná od daného dňa.
type Rate struct {
	From invoice.Date `json:"from"`
	Rate money.Dec    `json:"rate"`
}

// ECBTable sú sadzby zoradené podľa dátumu.
type ECBTable []Rate

// BuiltinECB vráti zabudovanú tabuľku sadzieb (predvolené hodnoty novej inštalácie).
func BuiltinECB() ECBTable {
	out := make(ECBTable, len(ecbRates))
	for i, r := range ecbRates {
		out[i] = Rate{From: invoice.MustDate(r.from), Rate: money.MustParse(r.rate)}
	}
	return out
}

// Sorted vráti tabuľku zoradenú podľa dátumu.
func Sorted(rates []Rate) ECBTable {
	out := append(ECBTable(nil), rates...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].From.Before(out[j].From) })
	return out
}

// On vráti sadzbu platnú v daný deň (pred prvým záznamom platí prvý záznam).
func (t ECBTable) On(d invoice.Date) money.Dec {
	if len(t) == 0 {
		return money.Zero
	}
	rate := t[0].Rate
	for _, r := range t {
		if d.Before(r.From) {
			break
		}
		rate = r.Rate
	}
	return rate
}

// Updated vráti dátum poslednej zmeny sadzby v tabuľke.
func (t ECBTable) Updated() invoice.Date {
	if len(t) == 0 {
		return invoice.Date{}
	}
	return t[len(t)-1].From
}

// ValidateRate skontroluje ručne zadanú sadzbu.
func ValidateRate(r Rate) error {
	var fe []invoice.FieldError
	if r.From.IsZero() {
		fe = append(fe, invoice.FieldError{Field: "from", Message: "Zadajte dátum, od ktorého sadzba platí"})
	}
	if r.Rate.Cmp(money.FromInt(-1)) < 0 || r.Rate.Cmp(money.FromInt(20)) > 0 {
		fe = append(fe, invoice.FieldError{Field: "rate", Message: "Sadzba ECB musí byť od −1 do 20 %"})
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}
