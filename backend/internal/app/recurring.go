package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
)

// Recurring vráti šablóny pravidelnej fakturácie s číslami vytvorených faktúr.
func (a *App) Recurring(ctx context.Context, companyID int64) ([]recurring.Template, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	list, err := a.Store.ListRecurring(ctx, companyID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		a.linkRuns(ctx, companyID, &list[i])
	}
	return list, nil
}

// RecurringTemplate vráti jednu šablónu.
func (a *App) RecurringTemplate(ctx context.Context, companyID, id int64) (recurring.Template, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return recurring.Template{}, err
	}
	t, err := a.Store.GetRecurring(ctx, companyID, id)
	if err == nil {
		a.linkRuns(ctx, companyID, &t)
	}
	return t, err
}

// linkRuns doplní aktuálne čísla faktúr v histórii (koncept číslo nemá).
func (a *App) linkRuns(ctx context.Context, companyID int64, t *recurring.Template) {
	for i := range t.History {
		r := &t.History[i]
		if r.InvoiceID == 0 {
			continue
		}
		if inv, err := a.Store.GetInvoice(ctx, companyID, r.InvoiceID); err == nil {
			r.Number, r.Issued = inv.Number, inv.Status != invoice.StatusDraft
		}
	}
}

func (a *App) prepareRecurring(ctx context.Context, companyID int64, t *recurring.Template) error {
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}
	t.NonVATPayer = !c.VATPayer
	if t.NonVATPayer {
		t.ReverseCharge = false
	}
	t.Name, t.Note = strings.TrimSpace(t.Name), strings.TrimSpace(t.Note)
	return t.Recalculate()
}

// RecurringSample je ukážka faktúry za ďalšie obdobie (texty po nahradení).
type RecurringSample struct {
	Period string   `json:"period"`
	Lines  []string `json:"lines"`
	Note   string   `json:"note"`
}

// PreviewRecurring prepočíta šablónu bez uloženia a ukáže texty ďalšej faktúry.
func (a *App) PreviewRecurring(ctx context.Context, companyID int64, t recurring.Template) (recurring.Template, RecurringSample, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return t, RecurringSample{}, err
	}
	t.Reschedule(t.NextDate)
	if err := a.prepareRecurring(ctx, companyID, &t); err != nil {
		return t, RecurringSample{}, err
	}
	var smp RecurringSample
	if !t.NextDate.IsZero() && t.Interval.Months() > 0 {
		inv := t.Build(t.NextDate, t.NextDate)
		smp.Period = t.Interval.PeriodLabel(t.NextDate)
		smp.Note = inv.Note
		for _, l := range inv.Lines {
			smp.Lines = append(smp.Lines, l.Description)
		}
	}
	return t, smp, nil
}

// SaveRecurring uloží šablónu. Rozvrh začne od zadaného dátumu ďalšej faktúry;
// počet a história vytvorených faktúr sa z klienta nemenia.
func (a *App) SaveRecurring(ctx context.Context, companyID int64, t recurring.Template) (recurring.Template, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return t, err
	}
	t.Generated, t.History = 0, nil
	var before *recurring.Template
	if t.ID != 0 {
		old, err := a.Store.GetRecurring(ctx, companyID, t.ID)
		if err != nil {
			return t, err
		}
		before = &old
		t.Generated, t.History = old.Generated, old.History
	}
	t.Reschedule(t.NextDate)
	if err := a.prepareRecurring(ctx, companyID, &t); err != nil {
		return t, err
	}
	// šablóna beží bez dozoru, preto sa kontroluje celá už pri uložení
	if err := t.Validate(); err != nil {
		return t, err
	}
	if err := a.Store.SaveRecurring(ctx, companyID, &t); err != nil {
		return t, err
	}
	if before == nil {
		a.record(ctx, companyID, audit.CatInvoices, "create", "recurring", t.ID,
			"Nová pravidelná faktúra "+recurringName(t), nil, t)
	} else {
		a.record(ctx, companyID, audit.CatInvoices, "update", "recurring", t.ID,
			"Úprava pravidelnej faktúry "+recurringName(t), before, t)
	}
	return t, nil
}

func recurringName(t recurring.Template) string {
	name := t.Name
	if name == "" {
		name = t.Customer.Name
	}
	return "„" + name + "“"
}

// SetRecurringActive pozastaví alebo obnoví šablónu. Pri obnovení sa obdobia
// z čias pozastavenia preskočia.
func (a *App) SetRecurringActive(ctx context.Context, companyID, id int64, active bool) (recurring.Template, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return recurring.Template{}, err
	}
	t, err := a.Store.GetRecurring(ctx, companyID, id)
	if err != nil {
		return t, err
	}
	if t.Active == active {
		return t, nil
	}
	action, what := "pause", "Pozastavená"
	if active {
		if err := t.Resume(a.Today()); err != nil {
			return t, err
		}
		action, what = "resume", "Obnovená"
	} else {
		t.Active = false
	}
	if err := a.Store.SaveRecurring(ctx, companyID, &t); err != nil {
		return t, err
	}
	type state struct {
		Active   bool         `json:"active"`
		NextDate invoice.Date `json:"nextDate"`
	}
	a.record(ctx, companyID, audit.CatInvoices, action, "recurring", t.ID, what+" pravidelná faktúra "+recurringName(t),
		nil, state{t.Active, t.NextDate})
	a.linkRuns(ctx, companyID, &t)
	return t, nil
}

// DeleteRecurring zmaže šablónu. Vytvorené faktúry zostávajú.
func (a *App) DeleteRecurring(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	t, err := a.Store.GetRecurring(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteRecurring(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "delete", "recurring", id, "Zmazaná pravidelná faktúra "+recurringName(t), t, nil)
	return nil
}

// GenerateRecurringNow vytvorí faktúru za najbližšie obdobie hneď, s dnešným
// dátumom vystavenia (napr. zákazník chce faktúru skôr).
func (a *App) GenerateRecurringNow(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	a.recurMu.Lock()
	defer a.recurMu.Unlock()
	t, err := a.Store.GetRecurring(ctx, companyID, id)
	if err != nil {
		return invoice.Invoice{}, err
	}
	if t.Finished() {
		return invoice.Invoice{}, recurring.ErrFinished
	}
	return a.generate(ctx, companyID, &t, t.NextDate, a.Today(), true)
}

// RecurringResult je výsledok automatického behu.
type RecurringResult struct {
	Created int // vytvorené faktúry
	Issued  int // z toho vystavené
}

// RunRecurring vytvorí faktúry za všetky obdobia, ktoré k dnešku nastali, vo
// všetkých firmách. Volá ho program pri štarte a potom pravidelne.
//
// Automaticky sa vystaví iba faktúra za obdobie v aktuálnom mesiaci. Dobehnuté
// staršie obdobia (program dlho nebežal) ostanú ako koncepty — mohli by patriť
// do už podaného priznania k DPH.
func (a *App) RunRecurring(ctx context.Context) (RecurringResult, error) {
	a.recurMu.Lock()
	defer a.recurMu.Unlock()
	var res RecurringResult
	companies, err := a.Store.ListCompanies(ctx)
	if err != nil {
		return res, err
	}
	today := a.Today()
	for _, c := range companies {
		list, err := a.Store.ListRecurring(ctx, c.ID)
		if err != nil {
			return res, err
		}
		for i := range list {
			t := &list[i]
			for _, period := range t.DueDates(today) {
				inv, err := a.generate(ctx, c.ID, t, period, period, false)
				if err != nil {
					log.Printf("pravidelná fakturácia %s (firma %d): %v", recurringName(*t), c.ID, err)
					break
				}
				res.Created++
				if inv.Status == invoice.StatusIssued {
					res.Issued++
				}
			}
		}
	}
	return res, nil
}

// generate vytvorí faktúru za obdobie, prípadne ju vystaví, a posunie rozvrh.
func (a *App) generate(ctx context.Context, companyID int64, t *recurring.Template, period, issue invoice.Date, manual bool) (invoice.Invoice, error) {
	inv, err := a.saveDraft(ctx, companyID, t.Build(period, issue))
	if err != nil {
		return inv, err
	}
	label := t.Interval.PeriodLabel(period)
	a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", inv.ID,
		fmt.Sprintf("Koncept z pravidelnej faktúry %s za %s pre %s", recurringName(*t), label, inv.Customer.Name), nil, inv)
	run := recurring.Run{Period: period, InvoiceID: inv.ID, Manual: manual}
	switch {
	case !t.AutoIssue:
	case issue.Year() != a.Today().Year() || issue.Month() != a.Today().Month():
		run.Error = "Dobehnuté obdobie z minulého mesiaca — skontrolujte dátumy a vystavte ručne"
	default:
		issued, err := a.Issue(ctx, companyID, inv.ID)
		var ve *invoice.ValidationError
		switch {
		case errors.As(err, &ve):
			msgs := make([]string, len(ve.Fields))
			for i, f := range ve.Fields {
				msgs[i] = f.Message
			}
			run.Error = "Nevystavená: " + strings.Join(msgs, "; ")
		case err != nil:
			return inv, err
		default:
			inv, run.Issued, run.Number = issued, true, issued.Number
			if t.SendEmail {
				to, err := a.autoMail(ctx, companyID, inv.ID)
				if err != nil {
					run.Error = "Vystavená, ale e-mail sa neodoslal: " + err.Error()
				}
				run.Emailed = to
			}
		}
	}
	t.Record(run)
	if err := a.Store.SaveRecurring(ctx, companyID, t); err != nil {
		return inv, err
	}
	return inv, nil
}
