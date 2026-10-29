package reminder

import (
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Základná úroková sadzba ECB (hlavné refinančné operácie) od daného dňa.
// Zdroj: ECB, prehľad sadzieb úrokov z omeškania na najpravo.sk.
// Pri zmene sadzby ECB treba doplniť riadok (a ECBUpdated).
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

// ECBUpdated je dátum poslednej známej zmeny sadzby ECB v tabuľke.
var ECBUpdated = invoice.MustDate(ecbRates[len(ecbRates)-1].from)

// ECBRate vráti základnú úrokovú sadzbu ECB (% p. a.) platnú v daný deň.
func ECBRate(d invoice.Date) money.Dec {
	rate := money.MustParse(ecbRates[0].rate)
	for _, r := range ecbRates {
		if d.Before(invoice.MustDate(r.from)) {
			break
		}
		rate = money.MustParse(r.rate)
	}
	return rate
}
