package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

const reminderSeries = "UP"

// OverdueInvoice je faktúra po splatnosti s históriou upomienok.
type OverdueInvoice struct {
	ID         int64        `json:"id"`
	Number     string       `json:"number"`
	IssueDate  invoice.Date `json:"issueDate"`
	DueDate    invoice.Date `json:"dueDate"`
	Total      money.Dec    `json:"total"`
	Days       int          `json:"days"`
	Reminded   int          `json:"reminded"`     // najvyšší stupeň doterajších upomienok (0 = žiadna)
	LastRemind invoice.Date `json:"lastReminded"` // dátum poslednej upomienky
}

// OverdueGroup sú faktúry po splatnosti jedného odberateľa.
type OverdueGroup struct {
	Key       string           `json:"key"`
	Customer  invoice.Party    `json:"customer"`
	ContactID int64            `json:"contactId"`
	Business  bool             `json:"business"`
	Invoices  []OverdueInvoice `json:"invoices"`
	Total     money.Dec        `json:"total"`
	MaxDays   int              `json:"maxDays"`
	NextLevel int              `json:"nextLevel"` // stupeň ďalšej upomienky
}

// remindedLevels vráti pre každú faktúru najvyšší stupeň a dátum poslednej upomienky.
func (a *App) remindedLevels(ctx context.Context, companyID int64) (map[int64]int, map[int64]invoice.Date, error) {
	list, err := a.Store.ListReminders(ctx, companyID, 0)
	if err != nil {
		return nil, nil, err
	}
	level, last := map[int64]int{}, map[int64]invoice.Date{}
	for _, r := range list {
		for _, it := range r.Items {
			if r.Level > level[it.InvoiceID] {
				level[it.InvoiceID] = r.Level
			}
			if r.Date.After(last[it.InvoiceID]) {
				last[it.InvoiceID] = r.Date
			}
		}
	}
	return level, last, nil
}

// Overdue vráti neuhradené faktúry po splatnosti zoskupené podľa odberateľa,
// odberatelia s najväčším dlhom prví.
func (a *App) Overdue(ctx context.Context, companyID int64) ([]OverdueGroup, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{Status: invoice.StatusIssued})
	if err != nil {
		return nil, err
	}
	level, last, err := a.remindedLevels(ctx, companyID)
	if err != nil {
		return nil, err
	}
	today := a.Today()
	groups := map[string]*OverdueGroup{}
	var order []string
	for _, inv := range invs {
		if inv.Kind != invoice.KindInvoice || !inv.Overdue(today) {
			continue
		}
		key := reminder.CustomerKey(inv)
		g := groups[key]
		if g == nil {
			g = &OverdueGroup{Key: key, Customer: inv.Customer, ContactID: inv.ContactID, Business: reminder.IsBusiness(inv.Customer),
				Invoices: []OverdueInvoice{}}
			groups[key] = g
			order = append(order, key)
		}
		oi := OverdueInvoice{ID: inv.ID, Number: inv.Number, IssueDate: inv.IssueDate, DueDate: inv.DueDate,
			Total: inv.Summary.Total, Days: inv.DueDate.DaysUntil(today), Reminded: level[inv.ID], LastRemind: last[inv.ID]}
		g.Invoices = append(g.Invoices, oi)
		g.Total = g.Total.Add(oi.Total)
		if oi.Days > g.MaxDays {
			g.MaxDays = oi.Days
		}
		if n := min(oi.Reminded+1, reminder.MaxLevel); n > g.NextLevel {
			g.NextLevel = n
		}
	}
	out := make([]OverdueGroup, 0, len(order))
	for _, k := range order {
		g := groups[k]
		sort.Slice(g.Invoices, func(i, j int) bool { return g.Invoices[i].DueDate.Before(g.Invoices[j].DueDate) })
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Total.Cmp(out[j].Total) > 0 })
	return out, nil
}

// ReminderRequest je požiadavka na upomienku.
type ReminderRequest struct {
	InvoiceIDs []int64 `json:"invoiceIds"`
	reminder.Options
}

// buildReminder načíta faktúry a zostaví upomienku; stupeň je o jeden vyšší
// ako najvyššia doterajšia upomienka niektorej z faktúr.
func (a *App) buildReminder(ctx context.Context, companyID int64, req ReminderRequest) (reminder.Reminder, error) {
	if req.Date.IsZero() {
		req.Date = a.Today()
	}
	if req.PayBy.IsZero() {
		req.PayBy = req.Date.AddDays(7)
	}
	if req.Interest == "" {
		req.Interest = reminder.InterestStatutory
	}
	level, _, err := a.remindedLevels(ctx, companyID)
	if err != nil {
		return reminder.Reminder{}, err
	}
	invs := make([]invoice.Invoice, 0, len(req.InvoiceIDs))
	next := 1
	seen := map[int64]bool{}
	for _, id := range req.InvoiceIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		inv, err := a.Store.GetInvoice(ctx, companyID, id)
		if err != nil {
			return reminder.Reminder{}, err
		}
		invs = append(invs, inv)
		next = max(next, level[id]+1)
	}
	return reminder.Build(invs, next, req.Options)
}

// PreviewReminder zostaví upomienku bez uloženia (výpočet úrokov v rozhraní).
func (a *App) PreviewReminder(ctx context.Context, companyID int64, req ReminderRequest) (reminder.Reminder, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return reminder.Reminder{}, err
	}
	return a.buildReminder(ctx, companyID, req)
}

// CreateReminder vytvorí a očísluje upomienku.
func (a *App) CreateReminder(ctx context.Context, companyID int64, req ReminderRequest) (reminder.Reminder, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return reminder.Reminder{}, err
	}
	r, err := a.buildReminder(ctx, companyID, req)
	if err != nil {
		return r, err
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return r, err
	}
	seq, err := a.Store.NextSequence(ctx, companyID, reminderSeries, r.Date.Year())
	if err != nil {
		return r, err
	}
	format := company.NumberFormats[ReminderFormatKey]
	if format == "" {
		format = ExtraFormats[ReminderFormatKey]
	}
	r.Number = invoice.FormatNumberWith(format, r.Date.Year(), r.Date.Month(), seq)
	if err := a.Store.SaveReminder(ctx, companyID, &r); err != nil {
		return r, err
	}
	nums := ""
	for i, it := range r.Items {
		if i > 0 {
			nums += ", "
		}
		nums += it.Number
	}
	a.record(ctx, companyID, audit.CatInvoices, "remind", "reminder", r.ID,
		fmt.Sprintf("%d. upomienka %s pre %s (%s), spolu %s €", r.Level, r.Number, r.Customer.Name, nums, r.Total.Display()), nil, r)
	return r, nil
}

// Reminders vráti upomienky za rok (0 = všetky).
func (a *App) Reminders(ctx context.Context, companyID int64, year int) ([]reminder.Reminder, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	return a.Store.ListReminders(ctx, companyID, year)
}

// Reminder vráti jednu upomienku.
func (a *App) Reminder(ctx context.Context, companyID, id int64) (reminder.Reminder, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return reminder.Reminder{}, err
	}
	return a.Store.GetReminder(ctx, companyID, id)
}

// DeleteReminder zmaže upomienku vytvorenú omylom (záznam ostáva v audite).
func (a *App) DeleteReminder(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	r, err := a.Store.GetReminder(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteReminder(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "delete", "reminder", id, "Zmazaná upomienka "+r.Number+" pre "+r.Customer.Name, r, nil)
	return nil
}
