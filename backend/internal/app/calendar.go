package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/calendar"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

// CalendarView je kalendár termínov firmy za obdobie.
type CalendarView struct {
	From     invoice.Date     `json:"from"`
	To       invoice.Date     `json:"to"`
	Today    invoice.Date     `json:"today"`
	Events   []calendar.Event `json:"events"`
	Holidays []calendar.Event `json:"holidays"`
	// Overdue sú nesplnené termíny s dátumom pred From (po splatnosti).
	Overdue []calendar.Event `json:"overdue"`
	// CanEdit: používateľ smie spravovať vlastné termíny.
	CanEdit bool `json:"canEdit"`
}

// MaxCalendarDays je najdlhší rozsah kalendára naraz.
const MaxCalendarDays = 400

// Calendar zostaví termíny firmy: daňové lehoty (posunuté na pracovný deň),
// splatnosti vystavených a prijatých faktúr, pravidelné faktúry, koniec
// platnosti ponúk a vlastné termíny. Každá časť sa ukáže, iba ak na ňu má
// používateľ právo.
func (a *App) Calendar(ctx context.Context, companyID int64, from, to invoice.Date) (CalendarView, error) {
	if err := a.Require(ctx, companyID, ""); err != nil {
		return CalendarView{}, err
	}
	if from.IsZero() || to.IsZero() || to.Before(from) || from.DaysUntil(to) > MaxCalendarDays {
		return CalendarView{}, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "to", Message: fmt.Sprintf("Zadajte rozsah najviac %d dní", MaxCalendarDays)}}}
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return CalendarView{}, err
	}
	today := a.Today()
	days := calendar.NewDays(a.legalParams().Holidays)
	can := func(p auth.Perm) bool { return a.Require(ctx, companyID, p) == nil }
	v := CalendarView{From: from, To: to, Today: today, Events: []calendar.Event{}, Overdue: []calendar.Event{},
		Holidays: days.InRange(from, to), CanEdit: can(auth.PermLedgerWrite)}
	if v.Holidays == nil {
		v.Holidays = []calendar.Event{}
	}
	var all []calendar.Event
	add := func(es ...calendar.Event) { all = append(all, es...) }

	if can(auth.PermVATRead) && co.VATPayer {
		es, err := a.vatDeadlines(ctx, co, from, to, today, days)
		if err != nil {
			return CalendarView{}, err
		}
		add(es...)
	}
	if can(auth.PermLedgerRead) {
		es, err := a.yearDeadlines(ctx, companyID, from, to, days)
		if err != nil {
			return CalendarView{}, err
		}
		add(es...)
	}
	if can(auth.PermInvoicesRead) {
		es, err := a.invoiceEvents(ctx, companyID, from, to, today)
		if err != nil {
			return CalendarView{}, err
		}
		add(es...)
	}
	if can(auth.PermPurchasesRead) {
		ps, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
		if err != nil {
			return CalendarView{}, err
		}
		for _, p := range ps {
			if p.Status != purchase.StatusBooked || p.DueDate.IsZero() || p.CreditNote() {
				continue
			}
			amt := p.Summary.Total
			who := p.Supplier.Name
			num := p.SupplierNumber
			if num == "" {
				num = p.Number
			}
			add(calendar.Event{ID: fmt.Sprintf("pur-%d", p.ID), Date: p.DueDate, Kind: calendar.KindPay,
				Title: "Úhrada " + num + " – " + who, Amount: &amt, Curr: currencyCode(p.Currency),
				Link: fmt.Sprintf("/prijate/%d", p.ID), Overdue: p.Overdue(today)})
		}
	}
	own, err := a.ownEvents(ctx, companyID)
	if err != nil {
		return CalendarView{}, err
	}
	// vlastné termíny aj spred rozsahu (nesplnené po termíne) — ako zmeškané
	// sa ukážu iba mesiac dozadu, staršie výskyty sú história
	for _, o := range own {
		for _, e := range o.Occurrences(minDate(from, today.AddDays(-31)), to, days) {
			e.Overdue = taxOverdue(e.Done, e.Date, today)
			add(e)
		}
	}

	for _, e := range all {
		switch {
		case !e.Date.Before(from) && !e.Date.After(to):
			v.Events = append(v.Events, e)
		case e.Date.Before(from) && e.Overdue:
			v.Overdue = append(v.Overdue, e)
		}
	}
	calendar.Sort(v.Events)
	calendar.Sort(v.Overdue)
	return v, nil
}

func minDate(a, b invoice.Date) invoice.Date {
	if b.Before(a) {
		return b
	}
	return a
}

// vatDeadlines: priznanie k DPH a KV (a SV, ak sú v období dodania do EÚ)
// do 25 dní po skončení obdobia, posunuté na pracovný deň.
func (a *App) vatDeadlines(ctx context.Context, co store.Company, from, to, today invoice.Date, days *calendar.Days) ([]calendar.Event, error) {
	quarterly := co.VATPeriod == store.VATPeriodQuarter
	var out []calendar.Event
	// obdobia, ktorých lehota môže padnúť do rozsahu (lehota je najviac ~40 dní po konci)
	p := vatreturn.PeriodOf(from.AddDays(-45), quarterly)
	for i := 0; i < 40; i++ {
		base := p.Deadline()
		due := days.NextWorkday(base)
		if due.After(to) {
			break
		}
		out = append(out, a.vatEvents(ctx, co, p, base, due, today)...)
		p = nextPeriod(p)
	}
	return out, nil
}

// taxOverdue: nesplnený termín sa ako zmeškaný ukáže iba mesiac po lehote —
// staršie obdobia mohli byť podané či zaplatené mimo Fiškusa.
func taxOverdue(done bool, due, today invoice.Date) bool {
	return !done && today.After(due) && due.DaysUntil(today) <= 31
}

func nextPeriod(p vatreturn.Period) vatreturn.Period {
	return vatreturn.PeriodOf(p.End().AddDays(1), p.Quarter > 0)
}

func (a *App) vatEvents(ctx context.Context, co store.Company, p vatreturn.Period, base, due, today invoice.Date) []calendar.Event {
	e := calendar.Event{ID: "vat-" + p.String(), Date: due, Kind: calendar.KindTax,
		Title: "Priznanie k DPH a kontrolný výkaz za " + p.Label(), Link: "/dph"}
	if due != base {
		e.Shifted = base
	}
	var sv *calendar.Event
	// po skončení obdobia: výsledok priznania, SV a či už bolo podané
	if today.After(p.End()) && p.Supported() {
		if r, _, err := a.VATReturn(ctx, co.ID, p.String(), money.Zero); err == nil {
			if res := r.Result(); res.Sign() > 0 {
				e.Amount, e.Curr = &res, "EUR"
				e.Detail = "Daň na úhradu: " + res.Display() + " € — splatná v lehote na podanie priznania."
			} else if res.Sign() < 0 {
				e.Detail = "Nadmerný odpočet " + res.Neg().Display() + " €."
			}
			if len(r.SV) > 0 {
				s := calendar.Event{ID: "sv-" + p.String(), Date: due, Kind: calendar.KindTax, Shifted: e.Shifted,
					Title: "Súhrnný výkaz za " + p.Label(), Link: "/suhrnny-vykaz",
					Detail: fmt.Sprintf("Dodania do iných štátov EÚ: %d riadkov.", len(r.SV))}
				sv = &s
			}
		}
		if fs, err := a.VATFilings(ctx, co.ID, p.String()); err == nil && len(fs) > 0 {
			e.Done = true
			e.Detail = strings.TrimSpace(e.Detail + " Podané " + fs[0].Filed.Display() + ".")
			if sv != nil {
				sv.Done = true
			}
		}
	}
	e.Overdue = taxOverdue(e.Done, due, today)
	out := []calendar.Event{e}
	if sv != nil {
		sv.Overdue = taxOverdue(sv.Done, due, today)
		out = append(out, *sv)
	}
	return out
}

// yearDeadlines: daňové priznanie k dani z príjmov (31. 3.) a k dani
// z motorových vozidiel (31. 1., ak má firma vozidlá v knihe jázd).
func (a *App) yearDeadlines(ctx context.Context, companyID int64, from, to invoice.Date, days *calendar.Days) ([]calendar.Event, error) {
	vehicles, err := a.Store.ListVehicles(ctx, companyID)
	if err != nil {
		return nil, err
	}
	var out []calendar.Event
	for y := from.Year(); y <= to.Year(); y++ {
		dp := invoice.NewDate(y, time.March, 31)
		e := calendar.Event{ID: fmt.Sprintf("dp-%d", y-1), Date: days.NextWorkday(dp), Kind: calendar.KindTax,
			Title: fmt.Sprintf("Daňové priznanie k dani z príjmov za %d", y-1), Link: "/vykazy-ju",
			Detail: "Podanie a úhrada dane. Lehotu možno predĺžiť oznámením správcovi dane do 31. 3. (najviac o 3 mesiace, pri príjmoch zo zahraničia o 6)."}
		if e.Date != dp {
			e.Shifted = dp
		}
		out = append(out, e)
		if len(vehicles) > 0 {
			dm := invoice.NewDate(y, time.January, 31)
			names := make([]string, 0, len(vehicles))
			for _, v := range vehicles {
				names = append(names, strings.TrimSpace(v.Name+" "+v.Plate))
			}
			e := calendar.Event{ID: fmt.Sprintf("dmv-%d", y-1), Date: days.NextWorkday(dm), Kind: calendar.KindTax,
				Title:  fmt.Sprintf("Daň z motorových vozidiel za %d", y-1),
				Detail: "Priznanie a úhrada, ak sa vozidlo v roku používalo na podnikanie: " + strings.Join(names, ", ") + ".",
				Link:   "/kniha-jazd"}
			if e.Date != dm {
				e.Shifted = dm
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// invoiceEvents: splatnosti vystavených faktúr, pravidelné faktúry a ponuky.
func (a *App) invoiceEvents(ctx context.Context, companyID int64, from, to, today invoice.Date) ([]calendar.Event, error) {
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{Status: invoice.StatusIssued})
	if err != nil {
		return nil, err
	}
	var out []calendar.Event
	for _, inv := range invs {
		if inv.Kind == invoice.KindCreditNote || inv.Settled() || inv.DueDate.IsZero() {
			continue
		}
		inv.SyncPaid()
		rest := inv.Remaining()
		if rest.Sign() <= 0 {
			continue
		}
		what := "Splatnosť "
		if inv.Kind == invoice.KindProforma {
			what = "Splatnosť zálohy "
		}
		out = append(out, calendar.Event{ID: fmt.Sprintf("inv-%d", inv.ID), Date: inv.DueDate, Kind: calendar.KindReceive,
			Title: what + inv.Number + " – " + inv.Customer.Name, Amount: &rest, Curr: currencyCode(inv.Currency),
			Link: fmt.Sprintf("/faktury/%d", inv.ID), Overdue: inv.Overdue(today)})
	}

	tpls, err := a.Store.ListRecurring(ctx, companyID)
	if err != nil {
		return nil, err
	}
	for _, t := range tpls {
		if !t.Active || t.Interval.Months() == 0 || t.StartDate.IsZero() {
			continue
		}
		for k := t.Count; k < t.Count+60; k++ {
			d := t.PeriodDate(k)
			if d.After(to) || (!t.EndDate.IsZero() && d.After(t.EndDate)) {
				break
			}
			if d.Before(from) {
				continue
			}
			amt := t.Summary.Total
			out = append(out, calendar.Event{ID: fmt.Sprintf("rec-%d-%s", t.ID, d), Date: d, Kind: calendar.KindRecurring,
				Title: "Pravidelná faktúra: " + t.Name + " – " + t.Customer.Name, Amount: &amt,
				Detail: recurringNote(t.AutoIssue), Link: fmt.Sprintf("/pravidelne/%d", t.ID)})
		}
	}

	for y := from.Year() - 1; y <= to.Year(); y++ {
		qs, err := a.Store.ListQuotes(ctx, companyID, y)
		if err != nil {
			return nil, err
		}
		for _, q := range qs {
			if q.Status != quote.StatusIssued || q.ValidUntil.IsZero() {
				continue
			}
			amt := q.Summary.Total
			out = append(out, calendar.Event{ID: fmt.Sprintf("quo-%d", q.ID), Date: q.ValidUntil, Kind: calendar.KindQuote,
				Title: "Koniec platnosti ponuky " + q.Number + " – " + q.Customer.Name, Amount: &amt,
				Detail: "Zákazník ešte neodpovedal.", Link: fmt.Sprintf("/ponuky/%d", q.ID)})
		}
	}
	return out, nil
}

func recurringNote(auto bool) string {
	if auto {
		return "Vystaví sa automaticky."
	}
	return "Vytvorí sa koncept na kontrolu."
}

// --- vlastné termíny ---

func ownKey(companyID int64) string { return fmt.Sprintf("calendar.own.%d", companyID) }

func (a *App) ownEvents(ctx context.Context, companyID int64) ([]calendar.Own, error) {
	raw, err := a.Store.GetSetting(ctx, ownKey(companyID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return []calendar.Own{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []calendar.Own
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (a *App) saveOwn(ctx context.Context, companyID int64, list []calendar.Own) error {
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, ownKey(companyID), string(b))
}

// OwnEvents vráti vlastné termíny firmy.
func (a *App) OwnEvents(ctx context.Context, companyID int64) ([]calendar.Own, error) {
	if err := a.Require(ctx, companyID, ""); err != nil {
		return nil, err
	}
	return a.ownEvents(ctx, companyID)
}

// SaveOwnEvent uloží vlastný termín (ID 0 = nový).
func (a *App) SaveOwnEvent(ctx context.Context, companyID int64, o calendar.Own) (calendar.Own, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return calendar.Own{}, err
	}
	if err := o.Validate(); err != nil {
		return calendar.Own{}, err
	}
	a.calMu.Lock()
	defer a.calMu.Unlock()
	list, err := a.ownEvents(ctx, companyID)
	if err != nil {
		return calendar.Own{}, err
	}
	action := "create"
	if o.ID == 0 {
		for _, x := range list {
			o.ID = max(o.ID, x.ID)
		}
		o.ID++
		o.Done = nil
		list = append(list, o)
	} else {
		i := ownIndex(list, o.ID)
		if i < 0 {
			return calendar.Own{}, store.ErrNotFound
		}
		// splnené výskyty, ktoré po zmene rozvrhu už neexistujú, sa zahodia
		var done []invoice.Date
		for _, d := range list[i].Done {
			if o.Occurs(d) {
				done = append(done, d)
			}
		}
		o.Done = done
		list[i] = o
		action = "update"
	}
	if err := a.saveOwn(ctx, companyID, list); err != nil {
		return calendar.Own{}, err
	}
	a.record(ctx, companyID, audit.CatSettings, action, "calendar", o.ID, "Vlastný termín „"+o.Title+"“ ("+o.Date.Display()+")", nil, o)
	return o, nil
}

// DeleteOwnEvent zmaže vlastný termín.
func (a *App) DeleteOwnEvent(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	a.calMu.Lock()
	defer a.calMu.Unlock()
	list, err := a.ownEvents(ctx, companyID)
	if err != nil {
		return err
	}
	i := ownIndex(list, id)
	if i < 0 {
		return store.ErrNotFound
	}
	gone := list[i]
	list = append(list[:i], list[i+1:]...)
	if err := a.saveOwn(ctx, companyID, list); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatSettings, "delete", "calendar", id, "Zmazaný vlastný termín „"+gone.Title+"“", gone, nil)
	return nil
}

// MarkOwnEvent označí výskyt vlastného termínu (pôvodný dátum) ako splnený.
func (a *App) MarkOwnEvent(ctx context.Context, companyID, id int64, base invoice.Date, done bool) (calendar.Own, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return calendar.Own{}, err
	}
	a.calMu.Lock()
	defer a.calMu.Unlock()
	list, err := a.ownEvents(ctx, companyID)
	if err != nil {
		return calendar.Own{}, err
	}
	i := ownIndex(list, id)
	if i < 0 {
		return calendar.Own{}, store.ErrNotFound
	}
	if !list[i].Occurs(base) {
		return calendar.Own{}, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "date", Message: "Termín v tento deň nie je"}}}
	}
	list[i].SetDone(base, done)
	if err := a.saveOwn(ctx, companyID, list); err != nil {
		return calendar.Own{}, err
	}
	state := "splnený"
	if !done {
		state = "nesplnený"
	}
	a.record(ctx, companyID, audit.CatSettings, "update", "calendar", id, "Termín „"+list[i].Title+"“ "+base.Display()+" označený ako "+state, nil, nil)
	return list[i], nil
}

func ownIndex(list []calendar.Own, id int64) int {
	for i, o := range list {
		if o.ID == id {
			return i
		}
	}
	return -1
}

// --- odber kalendára (iCalendar) ---

// calendarFeedPrefix označuje kľúč na odber kalendára v zozname kľúčov.
const calendarFeedPrefix = "Kalendár"

// CalendarTokenPrefix je začiatok tajného odkazu na odber.
const CalendarTokenPrefix = "fkc_"

// ErrBadFeed — neplatný alebo zrušený odkaz na odber kalendára.
var ErrBadFeed = errors.New("neplatný alebo zrušený odkaz na kalendár")

// CalendarFeedStatus hovorí, či má používateľ aktívny odkaz na odber.
type CalendarFeedStatus struct {
	Active    bool  `json:"active"`
	CreatedAt int64 `json:"createdAt,omitempty"`
	LastUsed  int64 `json:"lastUsedAt,omitempty"`
}

func (a *App) feedKeys(ctx context.Context) ([]auth.APIKey, error) {
	keys, err := a.Store.ListAPIKeys(ctx, auth.ActorFrom(ctx).UserID)
	if err != nil {
		return nil, err
	}
	var out []auth.APIKey
	for _, k := range keys {
		if k.Prefix == calendarFeedPrefix && !k.Revoked {
			out = append(out, k)
		}
	}
	return out, nil
}

// CalendarFeed vráti stav odberu prihláseného používateľa.
func (a *App) CalendarFeed(ctx context.Context) (CalendarFeedStatus, error) {
	ks, err := a.feedKeys(ctx)
	if err != nil || len(ks) == 0 {
		return CalendarFeedStatus{}, err
	}
	return CalendarFeedStatus{Active: true, CreatedAt: ks[0].CreatedAt, LastUsed: ks[0].LastUsedAt}, nil
}

// CreateCalendarFeed vytvorí nový tajný odkaz na odber (starý zruší).
// Token sa vráti iba raz; uloží sa len jeho odtlačok. Odkaz má práva
// používateľa iba na čítanie a platí len pre kalendár.
func (a *App) CreateCalendarFeed(ctx context.Context) (string, error) {
	act := auth.ActorFrom(ctx)
	if act.KeyID != 0 || act.Channel == auth.ChannelSystem {
		return "", ErrForbidden
	}
	if err := a.RevokeCalendarFeed(ctx); err != nil {
		return "", err
	}
	raw, _, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	token := CalendarTokenPrefix + raw
	k := auth.APIKey{UserID: act.UserID, Name: "Odber kalendára termínov", Prefix: calendarFeedPrefix,
		Hash: auth.TokenHash(token), ReadOnly: true, CreatedAt: time.Now().Unix()}
	if err := a.Store.CreateAPIKey(ctx, &k); err != nil {
		return "", err
	}
	a.record(ctx, 0, audit.CatSettings, "create", "calendar_feed", k.ID, "Vytvorený odkaz na odber kalendára termínov", nil, nil)
	return token, nil
}

// RevokeCalendarFeed zruší odkazy na odber prihláseného používateľa.
func (a *App) RevokeCalendarFeed(ctx context.Context) error {
	if auth.ActorFrom(ctx).KeyID != 0 {
		return ErrForbidden
	}
	ks, err := a.feedKeys(ctx)
	if err != nil {
		return err
	}
	for _, k := range ks {
		if err := a.Store.RevokeAPIKey(ctx, k.UserID, k.ID); err != nil {
			return err
		}
		a.record(ctx, 0, audit.CatSettings, "delete", "calendar_feed", k.ID, "Zrušený odkaz na odber kalendára termínov", nil, nil)
	}
	return nil
}

// CalendarICS vráti kalendár firmy vo formáte iCalendar pre tajný odkaz
// (od 2 mesiacov dozadu po rok dopredu).
func (a *App) CalendarICS(ctx context.Context, companyID int64, token, ip string) ([]byte, error) {
	if !strings.HasPrefix(token, CalendarTokenPrefix) {
		return nil, ErrBadFeed
	}
	k, err := a.Store.GetAPIKeyByHash(ctx, auth.TokenHash(token))
	if errors.Is(err, store.ErrNotFound) || (err == nil && (k.Revoked || k.Prefix != calendarFeedPrefix)) {
		return nil, ErrBadFeed
	}
	if err != nil {
		return nil, err
	}
	act, err := a.keyActor(ctx, k, auth.ChannelAPI, ip)
	if err != nil {
		return nil, ErrBadFeed
	}
	ctx = auth.WithActor(ctx, act)
	return a.calendarFile(ctx, companyID)
}

// CalendarDownload vráti kalendár ako súbor .ics (jednorazový import).
func (a *App) CalendarDownload(ctx context.Context, companyID int64) (Document, error) {
	b, err := a.calendarFile(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: "fiskus-terminy.ics", Data: b}, nil
}

func (a *App) calendarFile(ctx context.Context, companyID int64) ([]byte, error) {
	today := a.Today()
	v, err := a.Calendar(ctx, companyID, today.AddDays(-62), today.AddDays(MaxCalendarDays-62))
	if err != nil {
		if errors.Is(err, ErrForbidden) || errors.Is(err, store.ErrNotFound) {
			return nil, ErrBadFeed
		}
		return nil, err
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return calendar.ICS("Fiškus – "+co.Name, append(v.Overdue, v.Events...), a.clock()), nil
}
