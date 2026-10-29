package recurring

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func sample() Template {
	return Template{
		Name: "Servisný paušál", Active: true, Interval: Monthly,
		StartDate: invoice.MustDate("2026-01-31"),
		Customer:  invoice.Party{Name: "Odberateľ s.r.o.", Country: "SK"},
		Note:      "Fakturujeme za {obdobie}.",
		Lines: []invoice.Line{{Description: "Servisný paušál {mesiac} {rok}", Quantity: m.MustParse("1"),
			Unit: "mes", UnitPrice: m.MustParse("150"), Rate: 23}},
	}
}

func dates(ds []invoice.Date) string {
	s := make([]string, len(ds))
	for i, d := range ds {
		s[i] = d.String()
	}
	return strings.Join(s, " ")
}

func TestSchedule(t *testing.T) {
	cases := []struct {
		name     string
		interval Interval
		start    string
		want     string // prvých 5 období
	}{
		{"mesačne od 31. — koniec kratších mesiacov", Monthly, "2026-01-31", "2026-01-31 2026-02-28 2026-03-31 2026-04-30 2026-05-31"},
		{"mesačne cez prestupný rok", Monthly, "2027-12-29", "2027-12-29 2028-01-29 2028-02-29 2028-03-29 2028-04-29"},
		{"štvrťročne", Quarterly, "2026-01-15", "2026-01-15 2026-04-15 2026-07-15 2026-10-15 2027-01-15"},
		{"polročne", HalfYear, "2026-08-31", "2026-08-31 2027-02-28 2027-08-31 2028-02-29 2028-08-31"},
		{"ročne 29. 2.", Yearly, "2028-02-29", "2028-02-29 2029-02-28 2030-02-28 2031-02-28 2032-02-29"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tp := Template{Interval: c.interval, StartDate: invoice.MustDate(c.start)}
			var got []invoice.Date
			for k := 0; k < 5; k++ {
				got = append(got, tp.PeriodDate(k))
			}
			if dates(got) != c.want {
				t.Fatalf("\n got %s\nwant %s", dates(got), c.want)
			}
		})
	}
}

func TestDueDatesAndRecord(t *testing.T) {
	tp := sample()
	tp.StartDate = invoice.MustDate("2026-07-01")
	if got := dates(tp.DueDates(invoice.MustDate("2026-06-30"))); got != "" {
		t.Fatalf("pred začiatkom: %s", got)
	}
	// program 3 mesiace nebežal → dobehne každé obdobie samostatne
	if got := dates(tp.DueDates(invoice.MustDate("2026-09-15"))); got != "2026-07-01 2026-08-01 2026-09-01" {
		t.Fatalf("dobehnutie: %s", got)
	}
	tp.Record(Run{Period: invoice.MustDate("2026-07-01"), InvoiceID: 5, Issued: true})
	if tp.Count != 1 || tp.Generated != 1 || tp.NextDate.String() != "2026-08-01" || len(tp.History) != 1 {
		t.Fatalf("po zápise: %d %s", tp.Count, tp.NextDate)
	}
	if got := dates(tp.DueDates(invoice.MustDate("2026-09-15"))); got != "2026-08-01 2026-09-01" {
		t.Fatalf("po zápise: %s", got)
	}
	// najviac MaxCatchUp naraz
	if n := len(tp.DueDates(invoice.MustDate("2030-01-01"))); n != MaxCatchUp {
		t.Fatalf("strop dobehnutia: %d", n)
	}
	// pozastavená šablóna nič nevytvorí
	tp.Active = false
	if len(tp.DueDates(invoice.MustDate("2030-01-01"))) != 0 {
		t.Fatal("pozastavená")
	}
	// história má strop a najnovšie prvé
	tp.Active = true
	for i := 0; i < 30; i++ {
		tp.Record(Run{InvoiceID: int64(100 + i)})
	}
	if len(tp.History) != historyLimit || tp.History[0].InvoiceID != 129 {
		t.Fatalf("história: %d %d", len(tp.History), tp.History[0].InvoiceID)
	}
}

func TestEndDate(t *testing.T) {
	tp := sample()
	tp.StartDate, tp.EndDate = invoice.MustDate("2026-10-01"), invoice.MustDate("2026-11-30")
	if got := dates(tp.DueDates(invoice.MustDate("2027-03-01"))); got != "2026-10-01 2026-11-01" {
		t.Fatalf("koniec: %s", got)
	}
	tp.Record(Run{})
	tp.Record(Run{})
	if !tp.Finished() || tp.Active || !tp.NextDate.IsZero() {
		t.Fatalf("po poslednej faktúre: active=%v next=%s", tp.Active, tp.NextDate)
	}
}

func TestExpandAndBuild(t *testing.T) {
	cases := []struct {
		interval Interval
		period   string
		text     string
		want     string
	}{
		{Monthly, "2026-10-01", "Paušál {mesiac} {rok}", "Paušál október 2026"},
		{Monthly, "2027-01-05", "Za {minule_obdobie}", "Za december 2026"},
		{Quarterly, "2026-10-01", "{obdobie} / predtým {minule_obdobie}", "4. štvrťrok 2026 / predtým 3. štvrťrok 2026"},
		{HalfYear, "2026-01-10", "{obdobie}", "1. polrok 2026"},
		{Yearly, "2027-01-01", "Licencia {obdobie}", "Licencia 2027"},
		{Monthly, "2026-10-01", "Bez zástupcov {iny}", "Bez zástupcov {iny}"},
	}
	for _, c := range cases {
		tp := Template{Interval: c.interval}
		if got := tp.Expand(c.text, invoice.MustDate(c.period)); got != c.want {
			t.Errorf("%s %s: %q, chcel som %q", c.interval, c.text, got, c.want)
		}
	}

	tp := sample()
	tp.DueDays = 10
	tp.Subject, tp.Intro = "Paušál {mesiac}", "Za {obdobie} fakturujeme:"
	inv := tp.Build(invoice.MustDate("2026-10-31"), invoice.MustDate("2026-11-02"))
	if inv.Lines[0].Description != "Servisný paušál október 2026" || inv.Note != "Fakturujeme za október 2026." {
		t.Fatalf("texty: %q %q", inv.Lines[0].Description, inv.Note)
	}
	if inv.Subject != "Paušál október" || inv.Intro != "Za október 2026 fakturujeme:" {
		t.Fatalf("predmet a úvod: %q %q", inv.Subject, inv.Intro)
	}
	if inv.IssueDate.String() != "2026-11-02" || inv.DeliveryDate.String() != "2026-11-02" || inv.DueDate.String() != "2026-11-12" {
		t.Fatalf("dátumy: %s %s %s", inv.IssueDate, inv.DeliveryDate, inv.DueDate)
	}
	if tp.Lines[0].Description != "Servisný paušál {mesiac} {rok}" {
		t.Fatal("faktúra zmenila šablónu")
	}
	tp.DueDays = 0
	if inv := tp.Build(invoice.MustDate("2026-10-31"), invoice.MustDate("2026-10-31")); !inv.DueDate.IsZero() {
		t.Fatal("splatnosť 0 = doplní aplikácia")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*Template)
		field string
	}{
		{"platná", func(*Template) {}, ""},
		{"bez intervalu", func(t *Template) { t.Interval = "weekly" }, "interval"},
		{"bez začiatku", func(t *Template) { t.StartDate = invoice.Date{} }, "nextDate"},
		{"koniec pred začiatkom", func(t *Template) { t.EndDate = invoice.MustDate("2025-12-31") }, "endDate"},
		{"splatnosť", func(t *Template) { t.DueDays = 400 }, "dueDays"},
		{"bez odberateľa", func(t *Template) { t.Customer.Name = "" }, "customer.name"},
		{"bez položiek", func(t *Template) { t.Lines = nil }, "lines"},
		{"stará sadzba", func(t *Template) { t.Lines[0].Rate = 20 }, "lines[0].vatRate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tp := sample()
			c.edit(&tp)
			err := tp.Validate()
			var ve *invoice.ValidationError
			if c.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.As(err, &ve) {
				t.Fatalf("bez chyby: %v", err)
			}
			for _, f := range ve.Fields {
				if f.Field == c.field {
					return
				}
				if f.Field == "dueDate" || f.Field == "issueDate" {
					t.Fatalf("chyba dátumu faktúry nepatrí k šablóne: %v", ve.Fields)
				}
			}
			t.Fatalf("chýba %s: %v", c.field, ve.Fields)
		})
	}
}

func TestRecalculateKeepsPlaceholders(t *testing.T) {
	tp := sample()
	if err := tp.Recalculate(); err != nil {
		t.Fatal(err)
	}
	if tp.Summary.Total.String() != "184.50" || tp.Lines[0].Amount.String() != "150.00" || !strings.Contains(tp.Lines[0].Description, "{mesiac}") {
		t.Fatalf("%s %s %q", tp.Summary.Total, tp.Lines[0].Amount, tp.Lines[0].Description)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	tp := sample()
	_ = tp.Recalculate()
	tp.Record(Run{Period: invoice.MustDate("2026-01-31"), InvoiceID: 3, Issued: true})
	tp.Record(Run{Period: invoice.MustDate("2026-02-28"), InvoiceID: 4, Error: "x", Manual: true})
	b, _ := json.Marshal(tp)
	var back Template
	if err := json.Unmarshal(b, &back); err != nil || !reflect.DeepEqual(back, tp) {
		t.Fatalf("JSON:\n got %+v\nwant %+v", back, tp)
	}
}

func TestRescheduleAndResume(t *testing.T) {
	tp := sample()
	tp.StartDate = invoice.MustDate("2026-01-10")
	tp.Record(Run{})
	tp.Record(Run{})
	tp.Reschedule(invoice.MustDate("2026-06-30"))
	if tp.Count != 0 || tp.Generated != 2 || tp.NextDate.String() != "2026-06-30" || tp.PeriodDate(1).String() != "2026-07-30" {
		t.Fatalf("nový rozvrh: %d %d %s", tp.Count, tp.Generated, tp.NextDate)
	}
	// pozastavená od júla do októbra → po obnovení sa júl–september preskočí
	tp.Active = false
	if err := tp.Resume(invoice.MustDate("2026-10-05")); err != nil {
		t.Fatal(err)
	}
	if !tp.Active || tp.NextDate.String() != "2026-10-30" || tp.Generated != 2 || len(tp.DueDates(invoice.MustDate("2026-10-05"))) != 0 {
		t.Fatalf("po obnovení: %v %s", tp.Active, tp.NextDate)
	}
	// obdobie práve dnes sa nepreskočí
	tp.Active = false
	if _ = tp.Resume(invoice.MustDate("2026-10-30")); tp.NextDate.String() != "2026-10-30" {
		t.Fatalf("dnešné obdobie: %s", tp.NextDate)
	}
	tp.EndDate = invoice.MustDate("2026-11-15")
	tp.Active = false
	if err := tp.Resume(invoice.MustDate("2026-12-01")); !errors.Is(err, ErrFinished) || tp.Active {
		t.Fatalf("skončená: %v", err)
	}
}
