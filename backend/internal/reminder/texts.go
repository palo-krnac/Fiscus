package reminder

import (
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Texty listu upomienky — rovnaké ako lib/reminders.js vo frontende.

// Title vráti nadpis upomienky podľa stupňa.
func Title(level int) string {
	switch level {
	case 2:
		return "2. upomienka"
	case 3:
		return "Posledná upomienka pred vymáhaním"
	}
	return "Upomienka"
}

// Intro vráti úvodný text listu podľa stupňa.
func (r Reminder) Intro() string {
	by := r.PayBy.Display()
	switch r.Level {
	case 2:
		return "Napriek predchádzajúcej upomienke stále evidujeme neuhradené faktúry uvedené nižšie. Žiadame Vás o ich úhradu najneskôr do " + by + "."
	case 3:
		return "Toto je posledná upomienka. Ak dlžnú sumu neuhradíte najneskôr do " + by + ", budeme nútení uplatniť pohľadávku súdnou cestou alebo prostredníctvom inkasnej spoločnosti, čo Vám spôsobí ďalšie náklady."
	}
	return "Dovoľujeme si Vás upozorniť, že podľa našej evidencie nemáme uhradené faktúry uvedené nižšie, ktorých splatnosť už uplynula. Prosíme Vás o ich úhradu najneskôr do " + by + ". Ak ste ich medzičasom uhradili, považujte túto upomienku za bezpredmetnú."
}

// plain zapíše číslo bez zbytočných núl a s desatinnou čiarkou (9.00 → "9", 8.50 → "8,5").
func plain(d money.Dec) string {
	s := d.String()
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return strings.ReplaceAll(s, ".", ",")
}

// LegalNotes vráti poznámky o úroku z omeškania a paušálnej náhrade.
func (r Reminder) LegalNotes() []string {
	var out []string
	interest := r.InterestTotal.Sign() > 0
	switch {
	case r.Interest == InterestStatutory && interest:
		law := "§ 517 Občianskeho zákonníka a nariadenia vlády č. 87/1995 Z. z."
		if r.Business {
			law = "§ 369 Obchodného zákonníka a nariadenia vlády č. 21/2013 Z. z."
		}
		out = append(out, "Úrok z omeškania podľa "+law+" (základná úroková sadzba ECB platná k prvému dňu omeškania zvýšená o "+plain(r.Margin)+" percentuálnych bodov), vypočítaný ku dňu vystavenia upomienky.")
	case r.Interest == InterestCustom && interest:
		out = append(out, "Úrok z omeškania vo výške "+plain(r.CustomRate)+" % ročne, vypočítaný ku dňu vystavenia upomienky.")
	}
	if r.Fees.Sign() > 0 {
		out = append(out, "Paušálna náhrada nákladov spojených s uplatnením pohľadávky podľa § 369c Obchodného zákonníka ("+r.FeeEach.Display()+" € za každú faktúru).")
	}
	if interest {
		out = append(out, "Úrok z omeškania plynie až do zaplatenia; uvedená suma je k dátumu upomienky.")
	}
	return out
}
