// Package reminder sú upomienky k neuhradeným faktúram po splatnosti:
// výber faktúr jedného odberateľa, stupeň upomienky, úrok z omeškania
// a paušálna náhrada nákladov.
package reminder

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// InterestMode určuje, ako sa počíta úrok z omeškania.
type InterestMode string

const (
	InterestStatutory InterestMode = "statutory" // zákonná sadzba podľa ECB
	InterestCustom    InterestMode = "custom"    // dohodnutá sadzba % p. a.
	InterestNone      InterestMode = "none"      // bez úroku
)

// MaxLevel je najvyšší stupeň upomienky (posledná pred vymáhaním).
const MaxLevel = 3

// Law sú zákonné parametre upomienok (predvolené v DefaultLaw, platné
// v Nastaveniach → Legislatíva).
type Law struct {
	ECB ECBTable
	// BusinessMargin a ConsumerMargin sú prirážky k sadzbe ECB v p. b.:
	// podnikatelia § 369 ObZ a NV č. 21/2013 Z. z. (9), spotrebitelia
	// § 517 OZ a NV č. 87/1995 Z. z. (5).
	BusinessMargin money.Dec
	ConsumerMargin money.Dec
	// FlatFee je paušálna náhrada nákladov spojených s uplatnením pohľadávky
	// (§ 369c ObZ) — iba medzi podnikateľmi, za každú faktúru.
	FlatFee money.Dec
}

// DefaultLaw vráti predvolené zákonné parametre.
func DefaultLaw() Law {
	return Law{ECB: BuiltinECB(), BusinessMargin: money.FromInt(9), ConsumerMargin: money.FromInt(5), FlatFee: money.FromInt(40)}
}

// Item je jedna faktúra na upomienke.
type Item struct {
	InvoiceID int64        `json:"invoiceId"`
	Number    string       `json:"number"`
	IssueDate invoice.Date `json:"issueDate"`
	DueDate   invoice.Date `json:"dueDate"`
	VS        string       `json:"variableSymbol"`
	Amount    money.Dec    `json:"amount"`   // dlžná suma
	Days      int          `json:"days"`     // dni omeškania k dátumu upomienky
	Rate      money.Dec    `json:"rate"`     // % p. a.
	Interest  money.Dec    `json:"interest"` // úrok z omeškania k dátumu upomienky
	Fee       money.Dec    `json:"fee"`      // paušálna náhrada
}

// Reminder je upomienka.
type Reminder struct {
	ID         int64         `json:"id"`
	CompanyID  int64         `json:"companyId"`
	Number     string        `json:"number"`
	Date       invoice.Date  `json:"date"`
	PayBy      invoice.Date  `json:"payBy"` // dodatočná lehota na úhradu
	Level      int           `json:"level"` // 1–3
	Customer   invoice.Party `json:"customer"`
	ContactID  int64         `json:"contactId"`
	Business   bool          `json:"business"` // odberateľ je podnikateľ (má IČO)
	Interest   InterestMode  `json:"interestMode"`
	CustomRate money.Dec     `json:"customRate"`
	ChargeFee  bool          `json:"chargeFee"`
	// Margin je prirážka k sadzbe ECB (p. b.) a FeeEach paušálna náhrada za
	// faktúru platné v deň upomienky — tlač ich uvedie aj po zmene legislatívy.
	Margin  money.Dec `json:"margin"`
	FeeEach money.Dec `json:"feeEach"`
	Note    string    `json:"note"`
	Items   []Item    `json:"items"`

	Principal     money.Dec `json:"principal"`
	InterestTotal money.Dec `json:"interestTotal"`
	Fees          money.Dec `json:"fees"`
	Total         money.Dec `json:"total"`
}

// Options sú voľby pri vytváraní upomienky.
type Options struct {
	Date       invoice.Date `json:"date"`
	PayBy      invoice.Date `json:"payBy"`
	Interest   InterestMode `json:"interestMode"`
	CustomRate money.Dec    `json:"customRate"`
	ChargeFee  bool         `json:"chargeFee"`
	Note       string       `json:"note"`
	// Law sú zákonné parametre (nil = predvolené).
	Law *Law `json:"-"`
}

// IsBusiness rozhodne, či ide o obchodnoprávny vzťah (odberateľ s IČO).
// Inak sa odberateľ považuje za spotrebiteľa.
func IsBusiness(p invoice.Party) bool { return strings.TrimSpace(p.ICO) != "" }

// StatutoryRate vráti zákonnú sadzbu úroku z omeškania (% p. a.) pre omeškanie,
// ktoré začalo dňom firstDay: sadzba ECB k prvému dňu omeškania + prirážka
// (podnikateľ / spotrebiteľ). Sadzba sa nemení počas celého omeškania.
func (l Law) StatutoryRate(firstDay invoice.Date, business bool) money.Dec {
	add := l.ConsumerMargin
	if business {
		add = l.BusinessMargin
	}
	return l.ECB.On(firstDay).Add(add)
}

// Interest vypočíta úrok z omeškania zo sumy za dni od from do to (vrátane)
// pri sadzbe ratePct % p. a. Každý kalendárny rok sa delí počtom svojich dní.
func Interest(amount, ratePct money.Dec, from, to invoice.Date) money.Dec {
	if amount.Sign() <= 0 || ratePct.Sign() <= 0 || to.Before(from) {
		return money.Zero
	}
	total := money.Zero
	for y := from.Year(); y <= to.Year(); y++ {
		start, end := from, to
		if y > from.Year() {
			start = invoice.NewDate(y, time.January, 1)
		}
		if y < to.Year() {
			end = invoice.NewDate(y, time.December, 31)
		}
		days := start.DaysUntil(end) + 1
		yearDays := invoice.NewDate(y, time.January, 1).DaysUntil(invoice.NewDate(y+1, time.January, 1))
		total = total.Add(amount.MulRatio(ratePct.Mul(money.FromInt(int64(days))), money.FromInt(int64(100*yearDays))))
	}
	return total.RoundCents()
}

var (
	ErrNoInvoices    = errors.New("Vyberte aspoň jednu faktúru po splatnosti")
	ErrMixedCustomer = errors.New("Upomienka môže obsahovať faktúry iba jedného odberateľa")
	// ErrNotRemindable: faktúru nemožno upomenúť (uhradená, dobropis, pred splatnosťou).
	ErrNotRemindable = errors.New("faktúru nemožno upomenúť")
)

// notRemindable je chyba s konkrétnou správou, ktorá spĺňa errors.Is(ErrNotRemindable).
type notRemindable string

func (e notRemindable) Error() string        { return string(e) }
func (e notRemindable) Is(target error) bool { return target == ErrNotRemindable }

// customerKey spojí faktúry toho istého odberateľa: kontakt z adresára,
// inak IČO, inak názov.
func CustomerKey(inv invoice.Invoice) string {
	switch {
	case inv.ContactID != 0:
		return fmt.Sprintf("c%d", inv.ContactID)
	case strings.TrimSpace(inv.Customer.ICO) != "":
		return "i" + strings.ReplaceAll(inv.Customer.ICO, " ", "")
	}
	return "n" + strings.ToLower(strings.TrimSpace(inv.Customer.Name))
}

// Build zostaví upomienku z faktúr jedného odberateľa. Faktúry musia byť
// vystavené, neuhradené a k dátumu upomienky po splatnosti.
func Build(invs []invoice.Invoice, level int, o Options) (Reminder, error) {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if len(invs) == 0 {
		return Reminder{}, ErrNoInvoices
	}
	if o.Date.IsZero() {
		add("date", "Zadajte dátum upomienky")
	}
	if o.PayBy.IsZero() {
		add("payBy", "Zadajte, dokedy má odberateľ zaplatiť")
	} else if !o.Date.IsZero() && o.PayBy.Before(o.Date) {
		add("payBy", "Lehota na úhradu nemôže byť pred dátumom upomienky")
	}
	switch o.Interest {
	case InterestStatutory, InterestNone:
	case InterestCustom:
		if o.CustomRate.Sign() <= 0 || o.CustomRate.Cmp(money.FromInt(100)) > 0 {
			add("customRate", "Sadzba musí byť väčšia ako 0 a najviac 100 % ročne")
		}
	default:
		add("interestMode", "Vyberte spôsob úroku z omeškania")
	}
	if len(fe) > 0 {
		return Reminder{}, &invoice.ValidationError{Fields: fe}
	}
	if level < 1 {
		level = 1
	}
	if level > MaxLevel {
		level = MaxLevel
	}
	first := invs[0]
	r := Reminder{
		Date: o.Date, PayBy: o.PayBy, Level: level, Customer: first.Customer, ContactID: first.ContactID,
		Business: IsBusiness(first.Customer), Interest: o.Interest, CustomRate: o.CustomRate,
		Note: strings.TrimSpace(o.Note), Items: []Item{},
	}
	law := DefaultLaw()
	if o.Law != nil {
		law = *o.Law
	}
	r.ChargeFee = o.ChargeFee && r.Business
	if o.Interest == InterestStatutory {
		r.Margin = law.ConsumerMargin
		if r.Business {
			r.Margin = law.BusinessMargin
		}
	}
	if r.ChargeFee {
		r.FeeEach = law.FlatFee
	}
	if o.Interest != InterestCustom {
		r.CustomRate = money.Zero
	}
	key := CustomerKey(first)
	for _, inv := range invs {
		if CustomerKey(inv) != key {
			return r, ErrMixedCustomer
		}
		if inv.Kind != invoice.KindInvoice || inv.Status != invoice.StatusIssued {
			return r, notRemindable(fmt.Sprintf("Faktúra %s nie je vystavená a neuhradená", inv.Number))
		}
		if !inv.Overdue(o.Date) {
			return r, notRemindable(fmt.Sprintf("Faktúra %s k %s ešte nie je po splatnosti", inv.Number, o.Date.Display()))
		}
		it := Item{InvoiceID: inv.ID, Number: inv.Number, IssueDate: inv.IssueDate, DueDate: inv.DueDate,
			VS: inv.VariableSymbol, Amount: inv.Remaining(), Days: inv.DueDate.DaysUntil(o.Date)}
		firstDay := inv.DueDate.AddDays(1)
		switch o.Interest {
		case InterestStatutory:
			it.Rate = law.StatutoryRate(firstDay, r.Business)
		case InterestCustom:
			it.Rate = o.CustomRate
		}
		it.Interest = Interest(it.Amount, it.Rate, firstDay, o.Date)
		if r.ChargeFee {
			it.Fee = law.FlatFee
		}
		r.Items = append(r.Items, it)
		r.Principal = r.Principal.Add(it.Amount)
		r.InterestTotal = r.InterestTotal.Add(it.Interest)
		r.Fees = r.Fees.Add(it.Fee)
	}
	r.Total = r.Principal.Add(r.InterestTotal).Add(r.Fees)
	return r, nil
}
