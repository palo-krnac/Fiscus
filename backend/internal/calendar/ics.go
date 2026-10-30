package calendar

import (
	"strings"
	"time"
	"unicode/utf8"
)

// ICS vytvorí kalendár vo formáte iCalendar (RFC 5545) — celodenné udalosti
// na odber v Google Kalendári, Outlooku alebo v telefóne. Daňové lehoty
// a vlastné termíny majú pripomienku 3 dni a 1 deň vopred.
func ICS(name string, events []Event, now time.Time) []byte {
	var b strings.Builder
	line := func(s string) { b.WriteString(fold(s)); b.WriteString("\r\n") }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Fiškus//Kalendár termínov//SK")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:" + escape(name))
	line("X-WR-TIMEZONE:Europe/Bratislava")
	line("REFRESH-INTERVAL;VALUE=DURATION:PT6H")
	line("X-PUBLISHED-TTL:PT6H")
	stamp := now.UTC().Format("20060102T150405Z")
	for _, e := range events {
		if e.Kind == KindHoliday {
			continue
		}
		title := e.Title
		if e.Done {
			title = "✓ " + title
		}
		day := strings.ReplaceAll(e.Date.String(), "-", "")
		next := strings.ReplaceAll(e.Date.AddDays(1).String(), "-", "")
		line("BEGIN:VEVENT")
		line("UID:" + e.ID + "@fiskus")
		line("DTSTAMP:" + stamp)
		line("DTSTART;VALUE=DATE:" + day)
		line("DTEND;VALUE=DATE:" + next)
		line("SUMMARY:" + escape(title))
		if d := description(e); d != "" {
			line("DESCRIPTION:" + escape(d))
		}
		line("CATEGORIES:" + escape(kindLabel[e.Kind]))
		line("TRANSP:TRANSPARENT")
		if !e.Done && (e.Kind == KindTax || e.Kind == KindOwn || e.Kind == KindPay) {
			for _, before := range []string{"-P3D", "-P1D"} {
				line("BEGIN:VALARM")
				line("ACTION:DISPLAY")
				line("DESCRIPTION:" + escape(e.Title))
				line("TRIGGER;RELATED=START:" + before)
				line("END:VALARM")
			}
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return []byte(b.String())
}

var kindLabel = map[Kind]string{
	KindTax: "Dane", KindReceive: "Pohľadávky", KindPay: "Záväzky", KindRecurring: "Pravidelné faktúry",
	KindQuote: "Ponuky", KindOwn: "Vlastné termíny", KindHoliday: "Sviatky",
}

func description(e Event) string {
	var parts []string
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	if e.Amount != nil {
		c := e.Curr
		if c == "" {
			c = "EUR"
		}
		parts = append(parts, "Suma: "+e.Amount.Display()+" "+c)
	}
	if !e.Shifted.IsZero() {
		parts = append(parts, "Lehota pripadla na "+e.Shifted.Display()+" (deň pracovného pokoja), posunutá na najbližší pracovný deň.")
	}
	if e.Done {
		parts = append(parts, "Splnené.")
	}
	return strings.Join(parts, "\n")
}

// escape upraví text podľa RFC 5545 (3.3.11).
func escape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)
	return r.Replace(s)
}

// fold zalomí riadok na 75 bajtov bez rozdelenia znaku UTF-8.
func fold(s string) string {
	if len(s) <= 75 {
		return s
	}
	var b strings.Builder
	n := 0
	limit := 75
	for _, r := range s {
		l := utf8.RuneLen(r)
		if n+l > limit {
			b.WriteString("\r\n ")
			n = 0
			limit = 74 // úvodná medzera sa počíta
		}
		b.WriteRune(r)
		n += l
	}
	return b.String()
}

// Summary vráti názov, popis a pripomienku termínu na prenos do iného
// kalendára (rovnako ako v súbore .ics); sviatky sa neprenášajú (ok = false).
func Summary(e Event) (title, desc string, remind, ok bool) {
	if e.Kind == KindHoliday {
		return "", "", false, false
	}
	title = e.Title
	if e.Done {
		title = "✓ " + title
	}
	desc = description(e)
	if c := kindLabel[e.Kind]; c != "" {
		if desc != "" {
			desc += "\n"
		}
		desc += "Fiškus · " + c
	}
	remind = !e.Done && (e.Kind == KindTax || e.Kind == KindOwn || e.Kind == KindPay)
	return title, desc, remind, true
}
