package app

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/calendar"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/logbook"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

var dt = invoice.MustDate

func findEvent(es []calendar.Event, id string) *calendar.Event {
	for i := range es {
		if es[i].ID == id {
			return &es[i]
		}
	}
	return nil
}

func TestCalendar(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026, mesačný platiteľ DPH

	issue := func(issueDate, due string) invoice.Invoice {
		d := validDraft()
		d.IssueDate, d.DeliveryDate, d.DueDate = dt(issueDate), dt(issueDate), dt(due)
		d, err := a.SaveDraft(ctx, c.ID, d)
		if err != nil {
			t.Fatal(err)
		}
		inv, err := a.Issue(ctx, c.ID, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		return inv
	}
	late := issue("2026-09-20", "2026-10-15")
	soon := issue("2026-10-28", "2026-11-12")
	p, _, err := a.SavePurchase(ctx, c.ID, validPurchase()) // splatná 19. 10.
	if err != nil {
		t.Fatal(err)
	}
	tp, err := a.SaveRecurring(ctx, c.ID, validRecurring("2026-11-05"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "Škoda Octavia", Plate: "BB123AB", Fuel: logbook.Diesel,
		Consumption: m.MustParse("5.2"), Ownership: logbook.Business, Method: logbook.MethodLog}); err != nil {
		t.Fatal(err)
	}
	odvody, err := a.SaveOwnEvent(ctx, c.ID, calendar.Own{Title: "Odvody do Sociálnej poisťovne", Date: dt("2026-01-08"),
		Repeat: calendar.RepeatMonthly, Workday: true})
	if err != nil || odvody.ID != 1 {
		t.Fatalf("vlastný termín: %+v %v", odvody, err)
	}

	// október
	v, err := a.Calendar(ctx, c.ID, dt("2026-10-01"), dt("2026-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	vat := findEvent(v.Events, "vat-2026-09")
	if vat == nil || vat.Date != dt("2026-10-26") || vat.Shifted != dt("2026-10-25") || !vat.Overdue || vat.Done || vat.Kind != calendar.KindTax {
		t.Fatalf("DPH za september (25. 10. je nedeľa): %+v", vat)
	}
	if e := findEvent(v.Events, "vat-2026-08"); e != nil {
		t.Errorf("lehota za august (25. 9.) nepatrí do októbra: %+v", e)
	}
	if e := findEvent(v.Events, "inv-"+id64(late.ID)); e == nil || !e.Overdue || e.Amount.String() != "492.00" || e.Link != "/faktury/"+id64(late.ID) {
		t.Errorf("faktúra po splatnosti: %+v", e)
	}
	if e := findEvent(v.Events, "pur-"+id64(p.ID)); e == nil || !e.Overdue || e.Kind != calendar.KindPay || !strings.Contains(e.Title, "FV-2026/0815") {
		t.Errorf("prijatá faktúra po splatnosti: %+v", e)
	}
	if e := findEvent(v.Events, "own-1-2026-10-08"); e == nil || !e.Overdue || !e.Shifted.IsZero() {
		t.Errorf("odvody 8. 10.: %+v", e)
	}
	if !v.CanEdit || len(v.Holidays) != 0 {
		t.Errorf("október: canEdit %v, sviatky %+v", v.CanEdit, v.Holidays)
	}

	// podanie zaznamenané → splnené
	if _, err := a.RecordVATFiling(ctx, c.ID, "2026-09", m.Zero, vatreturn.Options{}, dt("2026-10-20")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.MarkOwnEvent(ctx, c.ID, odvody.ID, dt("2026-10-08"), true); err != nil {
		t.Fatal(err)
	}
	v, _ = a.Calendar(ctx, c.ID, dt("2026-10-01"), dt("2026-10-31"))
	if e := findEvent(v.Events, "vat-2026-09"); e == nil || !e.Done || e.Overdue || !strings.Contains(e.Detail, "Podané 20. 10. 2026") {
		t.Errorf("podané priznanie: %+v", e)
	}
	if e := findEvent(v.Events, "own-1-2026-10-08"); e == nil || !e.Done || e.Overdue {
		t.Errorf("splnené odvody: %+v", e)
	}
	if _, err := a.MarkOwnEvent(ctx, c.ID, odvody.ID, dt("2026-10-09"), true); !fieldErr(err, "date") {
		t.Errorf("deň bez termínu: %v", err)
	}

	// november: nesplnené z októbra sú v Po splatnosti, sviatky, posun odvodov
	v, err = a.Calendar(ctx, c.ID, dt("2026-11-01"), dt("2026-11-30"))
	if err != nil {
		t.Fatal(err)
	}
	if findEvent(v.Overdue, "inv-"+id64(late.ID)) == nil || findEvent(v.Overdue, "pur-"+id64(p.ID)) == nil {
		t.Errorf("po splatnosti: %+v", v.Overdue)
	}
	if findEvent(v.Overdue, "own-1-2026-10-08") != nil || findEvent(v.Overdue, "vat-2026-09") != nil {
		t.Errorf("splnené nepatria medzi zmeškané: %+v", v.Overdue)
	}
	if e := findEvent(v.Events, "own-1-2026-11-08"); e == nil || e.Date != dt("2026-11-09") {
		t.Errorf("odvody 8. 11. (nedeľa) → 9. 11.: %+v", e)
	}
	if e := findEvent(v.Events, "inv-"+id64(soon.ID)); e == nil || e.Overdue || e.Date != dt("2026-11-12") {
		t.Errorf("faktúra pred splatnosťou: %+v", e)
	}
	if e := findEvent(v.Events, "rec-"+id64(tp.ID)+"-2026-11-05"); e == nil || e.Kind != calendar.KindRecurring || e.Amount.String() != "123.00" {
		t.Errorf("pravidelná faktúra: %+v", e)
	}
	if e := findEvent(v.Events, "vat-2026-10"); e == nil || e.Date != dt("2026-11-25") || e.Overdue {
		t.Errorf("DPH za október: %+v", e)
	}
	if len(v.Holidays) != 1 || v.Holidays[0].Date != dt("2026-11-01") {
		t.Errorf("sviatky v novembri (17. 11. už nie je voľno): %+v", v.Holidays)
	}

	// rok dopredu: daň z motorových vozidiel a daň z príjmov
	v, err = a.Calendar(ctx, c.ID, dt("2027-01-01"), dt("2027-04-30"))
	if err != nil {
		t.Fatal(err)
	}
	if e := findEvent(v.Events, "dmv-2026"); e == nil || e.Date != dt("2027-02-01") || !strings.Contains(e.Detail, "BB123AB") {
		t.Errorf("DMV: %+v", e)
	}
	if e := findEvent(v.Events, "dp-2026"); e == nil || e.Date != dt("2027-03-31") {
		t.Errorf("daň z príjmov: %+v", e)
	}
	if _, err := a.Calendar(ctx, c.ID, dt("2026-01-01"), dt("2027-06-30")); !fieldErr(err, "to") {
		t.Errorf("priveľký rozsah: %v", err)
	}

	// úprava rozvrhu zahodí splnené výskyty, ktoré už neexistujú
	odvody.Date = dt("2026-01-10")
	odvody, err = a.SaveOwnEvent(ctx, c.ID, odvody)
	if err != nil || len(odvody.Done) != 0 {
		t.Fatalf("zmena rozvrhu: %+v %v", odvody, err)
	}
	if err := a.DeleteOwnEvent(ctx, c.ID, odvody.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := a.OwnEvents(ctx, c.ID); len(list) != 0 {
		t.Errorf("po zmazaní: %+v", list)
	}
	if err := a.DeleteOwnEvent(ctx, c.ID, 99); err == nil {
		t.Error("zmazanie neexistujúceho")
	}
}

func id64(n int64) string { return strconv.FormatInt(n, 10) }

func TestCalendarPermissions(t *testing.T) {
	o := newOffice(t)
	// klient firmy Beta nevidí kalendár firmy Alfa
	if _, err := o.a.Calendar(as(o.klient), o.alfa.ID, dt("2026-10-01"), dt("2026-10-31")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("cudzia firma: %v", err)
	}
	// klient kancelárie vidí lehoty svojej firmy, ale termíny nepridáva
	v, err := o.a.Calendar(as(o.klient), o.beta.ID, dt("2027-01-01"), dt("2027-04-30"))
	if err != nil {
		t.Fatal(err)
	}
	if v.CanEdit || findEvent(v.Events, "dp-2026") == nil || findEvent(v.Events, "vat-2027-01") != nil {
		t.Errorf("klient: canEdit %v, udalosti %+v", v.CanEdit, v.Events)
	}
	if _, err := o.a.SaveOwnEvent(as(o.klient), o.beta.ID, calendar.Own{Title: "x", Date: dt("2026-11-01")}); !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrReadOnly) {
		t.Errorf("klient pridáva termín: %v", err)
	}
	// účtovníčka vidí daňové lehoty
	v, err = o.a.Calendar(as(o.jana), o.alfa.ID, dt("2026-11-01"), dt("2026-11-30"))
	if err != nil || findEvent(v.Events, "vat-2026-10") == nil || !v.CanEdit {
		t.Errorf("účtovníčka: %+v %v", v, err)
	}
}

func TestCalendarFeed(t *testing.T) {
	o := newOffice(t)
	jctx := as(o.jana)
	if st, err := o.a.CalendarFeed(jctx); err != nil || st.Active {
		t.Fatalf("bez odberu: %+v %v", st, err)
	}
	token, err := o.a.CreateCalendarFeed(jctx)
	if err != nil || !strings.HasPrefix(token, CalendarTokenPrefix) {
		t.Fatalf("odkaz: %q %v", token, err)
	}
	ics, err := o.a.CalendarICS(ctx, o.alfa.ID, token, "1.2.3.4")
	if err != nil || !strings.Contains(string(ics), "BEGIN:VCALENDAR") || !strings.Contains(string(ics), "X-WR-CALNAME:Fiškus – Alfa") ||
		!strings.Contains(string(ics), "Priznanie k DPH") {
		t.Fatalf("iCalendar: %v\n%s", err, ics)
	}
	// Jana nie je členkou firmy Beta
	if _, err := o.a.CalendarICS(ctx, o.beta.ID, token, ""); !errors.Is(err, ErrBadFeed) {
		t.Errorf("cudzia firma: %v", err)
	}
	// token na odber nie je API kľúč
	if _, err := o.a.KeyActor(ctx, token, auth.ChannelAPI, ""); !errors.Is(err, ErrBadKey) {
		t.Errorf("token ako API kľúč: %v", err)
	}
	for _, bad := range []string{"", "fkc_zly", "fk_" + strings.TrimPrefix(token, CalendarTokenPrefix)} {
		if _, err := o.a.CalendarICS(ctx, o.alfa.ID, bad, ""); !errors.Is(err, ErrBadFeed) {
			t.Errorf("token %q: %v", bad, err)
		}
	}
	if st, _ := o.a.CalendarFeed(jctx); !st.Active {
		t.Error("odber má byť aktívny")
	}
	// nový odkaz zruší starý
	token2, err := o.a.CreateCalendarFeed(jctx)
	if err != nil || token2 == token {
		t.Fatal(err)
	}
	if _, err := o.a.CalendarICS(ctx, o.alfa.ID, token, ""); !errors.Is(err, ErrBadFeed) {
		t.Errorf("starý odkaz: %v", err)
	}
	if err := o.a.RevokeCalendarFeed(jctx); err != nil {
		t.Fatal(err)
	}
	if _, err := o.a.CalendarICS(ctx, o.alfa.ID, token2, ""); !errors.Is(err, ErrBadFeed) {
		t.Errorf("zrušený odkaz: %v", err)
	}
	// stiahnutie súboru
	doc, err := o.a.CalendarDownload(jctx, o.alfa.ID)
	if err != nil || doc.Name != "fiskus-terminy.ics" || len(doc.Data) == 0 {
		t.Errorf("stiahnutie: %v", err)
	}
}
