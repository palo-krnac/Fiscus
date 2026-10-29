// Package recurring je pravidelná fakturácia: šablóna faktúry, z ktorej program
// v nastavenom intervale vytvára faktúry (paušály, nájmy, servisné zmluvy).
package recurring

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Interval je perióda fakturácie.
type Interval string

const (
	Monthly   Interval = "monthly"
	Quarterly Interval = "quarterly"
	HalfYear  Interval = "halfyear"
	Yearly    Interval = "yearly"
)

// Months vráti dĺžku intervalu v mesiacoch (0 pre neznámy).
func (i Interval) Months() int {
	switch i {
	case Monthly:
		return 1
	case Quarterly:
		return 3
	case HalfYear:
		return 6
	case Yearly:
		return 12
	}
	return 0
}

const (
	// MaxCatchUp je najviac faktúr, ktoré sa dobehnú naraz (program dlho nebežal).
	MaxCatchUp = 12
	// historyLimit je počet posledných behov uložených v šablóne.
	historyLimit = 24
)

// Run je jedna vytvorená faktúra.
type Run struct {
	Period    invoice.Date `json:"period"`    // dátum obdobia podľa rozvrhu
	InvoiceID int64        `json:"invoiceId"` // 0, ak sa faktúra nevytvorila
	Number    string       `json:"number"`    // dopĺňa aplikácia pri čítaní
	Issued    bool         `json:"issued"`
	Error     string       `json:"error,omitempty"` // prečo sa nevystavila (ostal koncept)
	Manual    bool         `json:"manual,omitempty"`
}

// Template je šablóna pravidelnej faktúry.
type Template struct {
	ID        int64    `json:"id"`
	CompanyID int64    `json:"companyId"`
	Name      string   `json:"name"`
	Active    bool     `json:"active"`
	Interval  Interval `json:"interval"`
	// Rozvrh: StartDate je začiatok rozvrhu a Count počet období od neho
	// (vytvorených alebo preskočených). Deň v mesiaci zo StartDate platí pre
	// všetky obdobia (31. → posledný deň kratšieho mesiaca). Používateľ zadáva
	// iba NextDate — pri uložení sa z neho stane nový začiatok rozvrhu.
	StartDate invoice.Date `json:"startDate"`
	Count     int          `json:"count"`
	EndDate   invoice.Date `json:"endDate"`   // posledná možná faktúra (nepovinné)
	NextDate  invoice.Date `json:"nextDate"`  // ďalšia faktúra; prázdne po skončení
	Generated int          `json:"generated"` // počet vytvorených faktúr celkovo
	DueDays   int          `json:"dueDays"`   // 0 = podľa kontaktu alebo firmy
	AutoIssue bool         `json:"autoIssue"`

	Customer      invoice.Party         `json:"customer"`
	ContactID     int64                 `json:"contactId"`
	PriceMode     vat.PriceMode         `json:"priceMode"`
	ReverseCharge bool                  `json:"reverseCharge"`
	SupplyKind    invoice.SupplyKind    `json:"supplyKind"`
	NonVATPayer   bool                  `json:"nonVatPayer"`
	PaymentMethod invoice.PaymentMethod `json:"paymentMethod"`
	Note          string                `json:"note"`
	Lines         []invoice.Line        `json:"lines"`
	Summary       vat.Summary           `json:"summary"`

	History []Run `json:"history"` // najnovšie prvé
}

// ErrFinished: šablóna už nemá ďalšie obdobie (uplynul koniec).
var ErrFinished = errors.New("pravidelná faktúra skončila — predĺžte dátum konca")

// PeriodDate vráti dátum k-teho obdobia (0 = prvé).
func (t *Template) PeriodDate(k int) invoice.Date {
	return addMonths(t.StartDate, k*t.Interval.Months(), t.StartDate.Day())
}

// addMonths posunie dátum o n mesiacov a deň zarovná na koniec kratšieho mesiaca.
func addMonths(d invoice.Date, n, day int) invoice.Date {
	if d.IsZero() {
		return d
	}
	first := time.Date(d.Year(), time.Month(d.Month())+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return invoice.NewDate(first.Year(), first.Month(), day)
}

// Refresh prepočíta dátum ďalšej faktúry.
func (t *Template) Refresh() {
	t.NextDate = invoice.Date{}
	if t.Interval.Months() == 0 || t.StartDate.IsZero() {
		return
	}
	next := t.PeriodDate(t.Count)
	if !t.EndDate.IsZero() && next.After(t.EndDate) {
		return
	}
	t.NextDate = next
}

// Finished vráti true, ak šablóna nemá ďalšie obdobie.
func (t *Template) Finished() bool {
	t.Refresh()
	return t.NextDate.IsZero()
}

// DueDates vráti obdobia, ktoré sa k dnešku majú vyfakturovať (najviac MaxCatchUp).
func (t *Template) DueDates(today invoice.Date) []invoice.Date {
	if !t.Active || t.Interval.Months() == 0 || t.StartDate.IsZero() {
		return nil
	}
	var out []invoice.Date
	for k := t.Count; len(out) < MaxCatchUp; k++ {
		d := t.PeriodDate(k)
		if d.After(today) || (!t.EndDate.IsZero() && d.After(t.EndDate)) {
			break
		}
		out = append(out, d)
	}
	return out
}

// Record zapíše vytvorenú faktúru a posunie rozvrh na ďalšie obdobie.
func (t *Template) Record(r Run) {
	t.History = append([]Run{r}, t.History...)
	if len(t.History) > historyLimit {
		t.History = t.History[:historyLimit]
	}
	t.Count++
	t.Generated++
	t.Refresh()
	if t.NextDate.IsZero() {
		t.Active = false
	}
}

// Reschedule začne rozvrh od nového dátumu ďalšej faktúry (úprava šablóny).
func (t *Template) Reschedule(next invoice.Date) {
	t.StartDate, t.Count = next, 0
	t.Refresh()
}

// Resume obnoví pozastavenú šablónu. Obdobia počas pozastavenia (pred dneškom)
// sa preskočia — nevyfakturujú sa dodatočne.
func (t *Template) Resume(today invoice.Date) error {
	for t.Interval.Months() > 0 && !t.StartDate.IsZero() && t.PeriodDate(t.Count).Before(today) {
		t.Count++
	}
	t.Refresh()
	if t.NextDate.IsZero() {
		return ErrFinished
	}
	t.Active = true
	return nil
}

var months = [...]string{"január", "február", "marec", "apríl", "máj", "jún", "júl", "august",
	"september", "október", "november", "december"}

// PeriodLabel vráti názov obdobia: „október 2026“, „4. štvrťrok 2026“, „2. polrok 2026“, „2026“.
func (i Interval) PeriodLabel(d invoice.Date) string {
	switch i {
	case Quarterly:
		return fmt.Sprintf("%d. štvrťrok %d", (d.Month()-1)/3+1, d.Year())
	case HalfYear:
		return fmt.Sprintf("%d. polrok %d", (d.Month()-1)/6+1, d.Year())
	case Yearly:
		return fmt.Sprintf("%d", d.Year())
	}
	return fmt.Sprintf("%s %d", months[d.Month()-1], d.Year())
}

// Expand nahradí zástupné texty podľa obdobia:
// {mesiac}, {rok}, {obdobie}, {minule_obdobie}.
func (t *Template) Expand(s string, period invoice.Date) string {
	if !strings.Contains(s, "{") {
		return s
	}
	prev := addMonths(period, -t.Interval.Months(), period.Day())
	return strings.NewReplacer(
		"{mesiac}", months[period.Month()-1],
		"{rok}", fmt.Sprint(period.Year()),
		"{obdobie}", t.Interval.PeriodLabel(period),
		"{minule_obdobie}", t.Interval.PeriodLabel(prev),
	).Replace(s)
}

// Build vytvorí koncept faktúry za obdobie s dátumom vystavenia issue.
// Splatnosť doplní aplikácia (DueDays alebo podľa kontaktu / firmy).
func (t *Template) Build(period, issue invoice.Date) invoice.Invoice {
	inv := invoice.Invoice{
		Kind: invoice.KindInvoice, Status: invoice.StatusDraft,
		Customer: t.Customer, ContactID: t.ContactID,
		IssueDate: issue, DeliveryDate: issue,
		PriceMode: t.PriceMode, ReverseCharge: t.ReverseCharge, SupplyKind: t.SupplyKind,
		NonVATPayer: t.NonVATPayer, PaymentMethod: t.PaymentMethod,
		Note:  t.Expand(t.Note, period),
		Lines: make([]invoice.Line, len(t.Lines)),
	}
	for i, l := range t.Lines {
		l.Description = t.Expand(l.Description, period)
		inv.Lines[i] = l
	}
	if t.DueDays > 0 {
		inv.DueDate = issue.AddDays(t.DueDays)
	}
	return inv
}

// Recalculate prepočíta sumy šablóny (rovnako ako faktúra).
func (t *Template) Recalculate() error {
	inv := t.Build(t.StartDate, t.StartDate)
	inv.Lines = append([]invoice.Line(nil), t.Lines...) // bez nahradenia textov
	if err := inv.Recalculate(); err != nil {
		return err
	}
	t.Lines, t.Summary, t.PriceMode, t.PaymentMethod = inv.Lines, inv.Summary, inv.PriceMode, inv.PaymentMethod
	return nil
}

// Validate skontroluje šablónu. Pravidlá faktúry (odberateľ, položky, sadzby)
// sa overia na faktúre za začiatok rozvrhu.
func (t *Template) Validate() error {
	var out []invoice.FieldError
	add := func(f, m string) { out = append(out, invoice.FieldError{Field: f, Message: m}) }
	if t.Interval.Months() == 0 {
		add("interval", "Vyberte, ako často sa fakturuje")
	}
	if t.StartDate.IsZero() {
		add("nextDate", "Zadajte dátum ďalšej faktúry")
	} else if !t.EndDate.IsZero() && t.EndDate.Before(t.StartDate) {
		add("endDate", "Koniec nemôže byť pred ďalšou faktúrou")
	}
	if t.DueDays < 0 || t.DueDays > 365 {
		add("dueDays", "Splatnosť musí byť 0 až 365 dní")
	}
	if len([]rune(t.Name)) > 80 {
		add("name", "Názov môže mať najviac 80 znakov")
	}
	start := t.StartDate
	if start.IsZero() {
		start = invoice.NewDate(2000, 1, 1)
	}
	inv := t.Build(start, start)
	inv.DueDate = start
	var ve *invoice.ValidationError
	if err := inv.Validate(); errors.As(err, &ve) {
		for _, f := range ve.Fields {
			switch {
			case f.Field == "issueDate", f.Field == "deliveryDate", f.Field == "dueDate":
				continue
			case strings.HasSuffix(f.Field, ".vatRate"):
				f.Message = strings.Replace(f.Message, "dátumu dodania", "dátumu ďalšej faktúry", 1)
			}
			out = append(out, f)
		}
	} else if err != nil {
		return err
	}
	if len(out) > 0 {
		return &invoice.ValidationError{Fields: out}
	}
	return nil
}
