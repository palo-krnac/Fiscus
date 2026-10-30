package calendar

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

var d = invoice.MustDate

func TestEaster(t *testing.T) {
	tests := []struct {
		year int
		want string
	}{
		{2024, "2024-03-31"}, {2025, "2025-04-20"}, {2026, "2026-04-05"}, {2027, "2027-03-28"}, {2028, "2028-04-16"}, {2038, "2038-04-25"},
	}
	for _, tt := range tests {
		if got := Easter(tt.year).String(); got != tt.want {
			t.Errorf("Veľká noc %d: %s, čakám %s", tt.year, got, tt.want)
		}
	}
}

func TestWorkdays(t *testing.T) {
	days := NewDays(DefaultHolidays)
	tests := []struct {
		day     string
		workday bool
		holiday string
	}{
		{"2026-01-01", false, "Deň vzniku Slovenskej republiky"},
		{"2026-04-03", false, "Veľký piatok"},
		{"2026-04-06", false, "Veľkonočný pondelok"},
		{"2026-05-08", true, ""}, // v roku 2026 nie je dňom pracovného pokoja (§ 4b)
		{"2027-05-08", false, "Deň víťazstva nad fašizmom"},
		{"2026-09-01", true, ""}, // štátny sviatok, nie deň pracovného pokoja
		{"2023-09-01", false, "Deň Ústavy SR"},
		{"2026-09-15", true, ""},
		{"2027-09-15", false, "Sedembolestná Panna Mária"},
		{"2026-10-28", true, ""},
		{"2026-11-17", true, ""},
		{"2024-11-17", false, "Deň boja za slobodu a demokraciu"},
		{"2026-12-24", false, "Štedrý deň"},
		{"2026-10-24", false, ""}, // sobota
		{"2026-10-26", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.day, func(t *testing.T) {
			if got := days.Workday(d(tt.day)); got != tt.workday {
				t.Errorf("pracovný deň = %v", got)
			}
			if got := days.Holiday(d(tt.day)); got != tt.holiday {
				t.Errorf("sviatok = %q", got)
			}
		})
	}
}

func TestNextWorkday(t *testing.T) {
	days := NewDays(DefaultHolidays)
	tests := []struct{ in, want string }{
		{"2026-10-25", "2026-10-26"}, // nedeľa → pondelok (DPH za september 2026)
		{"2026-10-26", "2026-10-26"},
		{"2026-04-03", "2026-04-07"}, // Veľký piatok → po Veľkonočnom pondelku
		{"2026-12-24", "2026-12-28"}, // Vianoce + víkend
		{"2027-01-31", "2027-02-01"}, // DMV
		{"2027-03-31", "2027-03-31"},
		{"2026-05-08", "2026-05-08"},
	}
	for _, tt := range tests {
		if got := days.NextWorkday(d(tt.in)).String(); got != tt.want {
			t.Errorf("%s → %s, čakám %s", tt.in, got, tt.want)
		}
	}
	if in := days.InRange(d("2026-12-01"), d("2026-12-31")); len(in) != 3 || in[0].Title != "Štedrý deň" {
		t.Errorf("sviatky v decembri: %+v", in)
	}
}

func TestHolidayValidate(t *testing.T) {
	tests := []struct {
		h    Holiday
		want string
	}{
		{Holiday{Day: "05-01", Name: "Sviatok práce"}, ""},
		{Holiday{Day: "easter-2", Name: "Veľký piatok"}, ""},
		{Holiday{Day: "5-1", Name: "x"}, "MM-DD"},
		{Holiday{Day: "02-30", Name: "x"}, "MM-DD"},
		{Holiday{Day: "easter+x", Name: "x"}, "posun"},
		{Holiday{Day: "05-01"}, "názov"},
		{Holiday{Day: "05-01", Name: "x", From: 2027, To: 2026}, "pred rokom"},
	}
	for _, tt := range tests {
		err := tt.h.Validate()
		if (tt.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%+v: %v, čakám %q", tt.h, err, tt.want)
		}
	}
	for _, h := range DefaultHolidays {
		if err := h.Validate(); err != nil {
			t.Errorf("predvolený sviatok: %v", err)
		}
	}
}

func TestOwnOccurrences(t *testing.T) {
	days := NewDays(DefaultHolidays)
	tests := []struct {
		name string
		own  Own
		from string
		to   string
		want []string // dni výskytov
	}{
		{"jednorazový", Own{ID: 1, Title: "STK", Date: d("2026-11-12")}, "2026-11-01", "2026-12-31", []string{"2026-11-12"}},
		{"jednorazový mimo", Own{ID: 1, Title: "STK", Date: d("2026-11-12")}, "2026-12-01", "2026-12-31", nil},
		{"mesačne s posunom", Own{ID: 2, Title: "Odvody SP", Date: d("2026-01-08"), Repeat: RepeatMonthly, Workday: true},
			"2026-10-01", "2027-01-31", []string{"2026-10-08", "2026-11-09", "2026-12-08", "2027-01-08"}},
		{"mesačne bez posunu", Own{ID: 2, Title: "x", Date: d("2026-01-08"), Repeat: RepeatMonthly},
			"2026-11-01", "2026-11-30", []string{"2026-11-08"}},
		{"31. v mesiaci", Own{ID: 3, Title: "x", Date: d("2026-01-31"), Repeat: RepeatMonthly}, "2026-02-01", "2026-04-30",
			[]string{"2026-02-28", "2026-03-31", "2026-04-30"}},
		{"štvrťročne do konca", Own{ID: 4, Title: "x", Date: d("2026-03-31"), Repeat: RepeatQuarterly, Until: d("2026-09-30")},
			"2026-01-01", "2027-12-31", []string{"2026-03-31", "2026-06-30", "2026-09-30"}},
		{"ročne", Own{ID: 5, Title: "Poistka", Date: d("2024-02-29"), Repeat: RepeatYearly}, "2025-01-01", "2028-12-31",
			[]string{"2025-02-28", "2026-02-28", "2027-02-28", "2028-02-29"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, e := range tt.own.Occurrences(d(tt.from), d(tt.to), days) {
				got = append(got, e.Date.String())
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("výskyty %v, čakám %v", got, tt.want)
			}
		})
	}
	o := Own{ID: 2, Title: "Odvody", Date: d("2026-01-08"), Repeat: RepeatMonthly, Workday: true}
	o.SetDone(d("2026-11-08"), true)
	es := o.Occurrences(d("2026-11-01"), d("2026-11-30"), days)
	if len(es) != 1 || !es[0].Done || es[0].Shifted != d("2026-11-08") || es[0].ID != "own-2-2026-11-08" {
		t.Fatalf("splnený posunutý výskyt: %+v", es)
	}
	if !o.Occurs(d("2026-11-08")) || o.Occurs(d("2026-11-09")) {
		t.Error("Occurs")
	}
	o.SetDone(d("2026-11-08"), false)
	if len(o.Done) != 0 {
		t.Error("zrušenie splnenia")
	}
}

func TestOwnValidate(t *testing.T) {
	tests := []struct {
		own   Own
		field string
	}{
		{Own{Title: "x", Date: d("2026-01-01")}, ""},
		{Own{Date: d("2026-01-01")}, "title"},
		{Own{Title: "x"}, "date"},
		{Own{Title: "x", Date: d("2026-01-01"), Repeat: "weekly"}, "repeat"},
		{Own{Title: "x", Date: d("2026-01-01"), Until: d("2025-01-01")}, "until"},
	}
	for _, tt := range tests {
		err := tt.own.Validate()
		var ve *invoice.ValidationError
		if tt.field == "" {
			if err != nil {
				t.Errorf("%+v: %v", tt.own, err)
			}
			continue
		}
		if !errors.As(err, &ve) || ve.Fields[0].Field != tt.field {
			t.Errorf("%+v: %v, čakám pole %s", tt.own, err, tt.field)
		}
	}
}

func TestICS(t *testing.T) {
	amt := money.MustParse("1234.5")
	es := []Event{
		{ID: "vat-2026-09", Date: d("2026-10-26"), Kind: KindTax, Title: "Priznanie k DPH a kontrolný výkaz za september 2026", Shifted: d("2026-10-25")},
		{ID: "inv-7", Date: d("2026-11-03"), Kind: KindReceive, Title: "Splatnosť FA2026007; Firma, s. r. o.", Amount: &amt, Done: true},
		{ID: "hol", Date: d("2026-12-24"), Kind: KindHoliday, Title: "Štedrý deň"},
	}
	raw := string(ICS("Fiškus – Demo", es, time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)))
	out := strings.ReplaceAll(raw, "\r\n ", "")
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "X-WR-CALNAME:Fiškus – Demo\r\n", "UID:vat-2026-09@fiskus\r\n",
		"DTSTART;VALUE=DATE:20261026\r\n", "DTEND;VALUE=DATE:20261027\r\n", "TRIGGER;RELATED=START:-P3D",
		`SUMMARY:✓ Splatnosť FA2026007\; Firma\, s. r. o.`, "Suma: 1 234\\,50 EUR", "posunutá", "END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("chýba %q v\n%s", want, out)
		}
	}
	if strings.Contains(out, "Štedrý") {
		t.Error("sviatky nepatria do odberu")
	}
	if strings.Count(out, "BEGIN:VALARM") != 2 {
		t.Error("pripomienky iba pri nesplnenej daňovej lehote")
	}
	for _, l := range strings.Split(raw, "\r\n") {
		if len(l) > 75 {
			t.Errorf("riadok dlhší ako 75 bajtov: %q", l)
		}
	}
}

func TestFold(t *testing.T) {
	s := "SUMMARY:" + strings.Repeat("ščťž", 30)
	f := fold(s)
	for i, l := range strings.Split(f, "\r\n") {
		if len(l) > 75 {
			t.Errorf("riadok %d má %d bajtov", i, len(l))
		}
	}
	if strings.ReplaceAll(f, "\r\n ", "") != s {
		t.Error("zalomenie zmenilo text")
	}
}
