package invoice

import (
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// KindAdvanceReceipt je daňový doklad k prijatej platbe (§ 71 ods. 1 písm. b)
// zákona o DPH): vyhotoví sa, keď platiteľ prijme platbu (zálohu) pred dodaním.
// Daňová povinnosť vzniká dňom prijatia platby (§ 19 ods. 4) — ten je
// v doklade ako dátum dodania.
const KindAdvanceReceipt Kind = "advance_receipt"

// Advance spája zálohovú faktúru, doklad k prijatej platbe a vyúčtovaciu faktúru.
type Advance struct {
	// na doklade k prijatej platbe a vyúčtovacej faktúre: zálohová faktúra
	ProformaID     int64  `json:"proformaId,omitempty"`
	ProformaNumber string `json:"proformaNumber,omitempty"`
	// na zálohovej faktúre: doklad k platbe a vyúčtovanie. ReceiptID je prvý
	// doklad (staršie údaje); Receipts sú doklady ku všetkým platbám.
	ReceiptID     int64        `json:"receiptId,omitempty"`
	ReceiptNumber string       `json:"receiptNumber,omitempty"`
	Receipts      []ReceiptRef `json:"receipts,omitempty"`
	FinalID       int64        `json:"finalId,omitempty"`
	FinalNumber   string       `json:"finalNumber,omitempty"`
	// na vyúčtovacej faktúre: odpočítané zálohy a celá dodávka pred odpočtom
	Deductions []Deduction  `json:"deductions,omitempty"`
	Supply     *vat.Summary `json:"supply,omitempty"`
}

// ReceiptRef je daňový doklad k jednej prijatej platbe zálohy.
type ReceiptRef struct {
	ID     int64     `json:"id"`
	Number string    `json:"number"`
	Date   Date      `json:"date"`   // deň prijatia platby
	Amount money.Dec `json:"amount"` // prijatá platba
}

// AllReceipts vráti doklady k platbám zálohovej faktúry (aj zo starších
// údajov, kde bol iba jeden doklad k úhrade celej zálohy).
func (inv *Invoice) AllReceipts() []ReceiptRef {
	a := inv.Advance
	if a == nil {
		return nil
	}
	if len(a.Receipts) == 0 && a.ReceiptID != 0 {
		return []ReceiptRef{{ID: a.ReceiptID, Number: a.ReceiptNumber, Date: inv.PaidDate, Amount: inv.Summary.Total}}
	}
	return a.Receipts
}

// AddReceipt zapíše do zálohovej faktúry doklad k prijatej platbe.
func (inv *Invoice) AddReceipt(r ReceiptRef) {
	if inv.Advance == nil {
		inv.Advance = &Advance{}
	}
	if len(inv.Advance.Receipts) == 0 && inv.Advance.ReceiptID != 0 {
		inv.Advance.Receipts = inv.AllReceipts()
	}
	inv.Advance.Receipts = append(inv.Advance.Receipts, r)
	if inv.Advance.ReceiptID == 0 {
		inv.Advance.ReceiptID, inv.Advance.ReceiptNumber = r.ID, r.Number
	}
}

// Settled: zálohová faktúra je vyúčtovaná (vystavená vyúčtovacia faktúra) —
// neuhradený zvyšok zálohy sa už nevymáha, doplatok je na vyúčtovaní.
func (inv *Invoice) Settled() bool {
	return inv.Kind == KindProforma && inv.Advance != nil && inv.Advance.FinalNumber != ""
}

// Deduction je odpočet zálohy vo vyúčtovacej faktúre: základ a daň po sadzbách
// presne podľa dokladu k prijatej platbe (nie prepočítané).
type Deduction struct {
	ProformaID     int64             `json:"proformaId,omitempty"` // zálohová faktúra, ku ktorej odpočet patrí
	ProformaNumber string            `json:"proformaNumber,omitempty"`
	ReceiptID      int64             `json:"receiptId,omitempty"` // 0 = neplatiteľ DPH (bez dokladu k platbe)
	Number         string            `json:"number"`              // číslo dokladu k platbe, inak zálohovej faktúry
	Date           Date              `json:"date"`                // deň prijatia platby
	Rates          []vat.RateSummary `json:"rates"`
	Total          money.Dec         `json:"total"`
}

var (
	// ErrNotAdvance — operácia patrí iba zálohovej faktúre.
	ErrNotAdvance = errors.New("operácia je možná iba pri zálohovej faktúre")
	// ErrAdvanceNotPaid — vyúčtovať sa dá až uhradená záloha.
	ErrAdvanceNotPaid = errors.New("zálohová faktúra ešte nie je uhradená")
	// ErrAlreadySettled — záloha už bola vyúčtovaná.
	ErrAlreadySettled = errors.New("zálohová faktúra už je vyúčtovaná")
	// ErrForeignAdvance — pri zahraničí a prenesení daňovej povinnosti sa
	// záloha zatiaľ vyúčtuje ručne (pri dodaní tovaru do EÚ záloha nezakladá
	// daňovú povinnosť, celé plnenie sa vykazuje pri dodaní).
	ErrForeignAdvance = errors.New("zálohu zahraničnému odberateľovi, v cudzej mene alebo s prenesením daňovej povinnosti zatiaľ vyúčtujte ručne")
)

// NeedsReceipt vráti true, ak sa k úhrade zálohovej faktúry vyhotovuje daňový
// doklad k prijatej platbe: platiteľ DPH, tuzemský odberateľ, bez prenesenia.
func (inv *Invoice) NeedsReceipt() bool {
	return inv.Kind == KindProforma && !inv.NonVATPayer && !inv.AnyReverse() && !party.Foreign(inv.Customer.Country) && !inv.Foreign()
}

// NewAdvanceReceipt vyhotoví daňový doklad k prijatej platbe (aj čiastočnej)
// k zálohovej faktúre. Platba sa rozdelí na sadzby v pomere súm zálohovej
// faktúry a daň sa vypočíta z platby („zhora“); dátum dodania = deň prijatia
// platby (§ 19 ods. 4).
func NewAdvanceReceipt(proforma Invoice, amount money.Dec, paid, issue Date) (Invoice, error) {
	if proforma.Kind != KindProforma {
		return Invoice{}, ErrNotAdvance
	}
	if amount.Sign() <= 0 || paid.IsZero() {
		return Invoice{}, ErrAdvanceNotPaid
	}
	if !proforma.NeedsReceipt() {
		return Invoice{}, ErrForeignAdvance
	}
	r := Invoice{
		Kind: KindAdvanceReceipt, Status: StatusDraft,
		Customer: proforma.Customer, ContactID: proforma.ContactID,
		IssueDate: issue, DeliveryDate: paid, DueDate: issue, // už uhradené
		Currency: proforma.Currency, PriceMode: vat.PriceGross, PaymentMethod: proforma.PaymentMethod,
		VariableSymbol: proforma.VariableSymbol,
		Subject:        proforma.Subject,
		Note:           "Daňový doklad k prijatej platbe k zálohovej faktúre " + proforma.Number + ".",
		Advance:        &Advance{ProformaID: proforma.ID, ProformaNumber: proforma.Number},
	}
	what := "Prijatá platba k zálohovej faktúre " + proforma.Number
	if len(proforma.Lines) > 0 && proforma.Lines[0].Description != "" {
		what += " – " + proforma.Lines[0].Description
	}
	for _, part := range splitByRates(proforma.Summary, amount) {
		r.Lines = append(r.Lines, Line{Description: what, Quantity: money.FromInt(1), Unit: "", UnitPrice: part.Total, Rate: part.Rate})
	}
	if len(r.Lines) == 0 {
		return Invoice{}, errors.New("zálohová faktúra nemá sumu")
	}
	if err := r.Recalculate(); err != nil {
		return Invoice{}, err
	}
	return r, nil
}

// splitByRates rozdelí sumu na sadzby v pomere súm (s DPH) rekapitulácie;
// posledná sadzba dostane zvyšok, aby súčet sedel na cent. Celá suma dokladu
// vráti sadzby bez prepočtu.
func splitByRates(s vat.Summary, amount money.Dec) []vat.RateSummary {
	var rates []vat.RateSummary
	for _, rs := range s.Rates {
		if !rs.Total.IsZero() {
			rates = append(rates, rs)
		}
	}
	if amount.Equal(s.Total) {
		return rates
	}
	out := make([]vat.RateSummary, len(rates))
	rest := amount
	for i, rs := range rates {
		part := rest
		if i < len(rates)-1 {
			part = amount.MulRatio(rs.Total, s.Total).RoundCents()
			rest = rest.Sub(part)
		}
		out[i] = vat.RateSummary{Rate: rs.Rate, Reverse: rs.Reverse, Total: part}
	}
	return out
}

// NewFinalInvoice pripraví koncept vyúčtovacej faktúry k uhradenej zálohe:
// položky zálohy (dajú sa upraviť) a odpočet zálohy podľa dokladu k prijatej
// platbe. receipt je nil pri neplatiteľovi DPH (odpočíta sa uhradená suma).
func NewFinalInvoice(proforma Invoice, receipts []Invoice, issue Date, dueDays int) (Invoice, error) {
	if proforma.Kind != KindProforma {
		return Invoice{}, ErrNotAdvance
	}
	deductions, err := advanceDeductions(proforma, receipts)
	if err != nil {
		return Invoice{}, err
	}
	inv := Invoice{
		Kind: KindInvoice, Status: StatusDraft,
		Customer: proforma.Customer, ContactID: proforma.ContactID,
		IssueDate: issue, DeliveryDate: issue, DueDate: issue.AddDays(dueDays),
		Currency: proforma.Currency, PriceMode: proforma.PriceMode, SupplyKind: proforma.SupplyKind,
		NonVATPayer: proforma.NonVATPayer, PaymentMethod: proforma.PaymentMethod,
		Subject: proforma.Subject, Intro: proforma.Intro,
		Note:    "Vyúčtovanie zálohovej faktúry " + proforma.Number + ".",
		Lines:   append([]Line(nil), proforma.Lines...),
		Advance: &Advance{ProformaID: proforma.ID, ProformaNumber: proforma.Number, Deductions: deductions},
	}
	if err := inv.Recalculate(); err != nil {
		return Invoice{}, err
	}
	return inv, nil
}

// advanceDeductions vráti odpočty zálohovej faktúry podľa dokladov k platbám
// (neplatiteľ: uhradená suma). Záloha musí byť aspoň čiastočne uhradená,
// ešte nevyúčtovaná a tuzemská.
func advanceDeductions(proforma Invoice, receipts []Invoice) ([]Deduction, error) {
	paid := proforma.Paid()
	if (proforma.Status != StatusPaid && proforma.Status != StatusIssued) || paid.Sign() <= 0 {
		return nil, ErrAdvanceNotPaid
	}
	if proforma.Advance != nil && proforma.Advance.FinalID != 0 {
		return nil, ErrAlreadySettled
	}
	if proforma.AnyReverse() || party.Foreign(proforma.Customer.Country) || proforma.Foreign() {
		return nil, ErrForeignAdvance
	}
	var deductions []Deduction
	switch {
	case len(receipts) > 0:
		var covered money.Dec
		for _, r := range receipts {
			deductions = append(deductions, Deduction{ProformaID: proforma.ID, ProformaNumber: proforma.Number, ReceiptID: r.ID, Number: r.Number, Date: r.DeliveryDate,
				Rates: append([]vat.RateSummary(nil), r.Summary.Rates...), Total: r.Summary.Total})
			covered = covered.Add(r.Summary.Total)
		}
		if !covered.Equal(paid) {
			return nil, fmt.Errorf("k platbám zálohy (%s €) chýba daňový doklad k prijatej platbe — doklady pokrývajú %s €", paid.Display(), covered.Display())
		}
	case proforma.NonVATPayer:
		// neplatiteľ: odpočíta sa uhradená suma (bez DPH)
		date := proforma.PaidDate
		if date.IsZero() && len(proforma.Payments) > 0 {
			date = proforma.Payments[len(proforma.Payments)-1].Date
		}
		d := Deduction{ProformaID: proforma.ID, ProformaNumber: proforma.Number, Number: proforma.Number, Date: date, Total: paid}
		for _, r := range splitByRates(proforma.Summary, paid) {
			if r.Base.IsZero() && r.VAT.IsZero() {
				r.Base = r.Total // pri čiastočnej úhrade: neplatiteľ nemá DPH
			}
			d.Rates = append(d.Rates, r)
		}
		deductions = []Deduction{d}
	default:
		return nil, errors.New("chýba daňový doklad k prijatej platbe")
	}
	return deductions, nil
}

// ProformaIDs vráti zálohové faktúry odpočítané vo vyúčtovacej faktúre.
func (inv *Invoice) ProformaIDs() []int64 {
	if inv.Advance == nil {
		return nil
	}
	var ids []int64
	seen := map[int64]bool{}
	add := func(id int64) {
		if id != 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(inv.Advance.ProformaID)
	for _, d := range inv.Advance.Deductions {
		add(d.ProformaID)
	}
	return ids
}

// ErrOtherCustomer — zálohu iného odberateľa nemožno pridať do vyúčtovania.
var ErrOtherCustomer = errors.New("zálohová faktúra patrí inému odberateľovi")

// SameCustomer: zálohu možno odpočítať vo vyúčtovaní (rovnaký odberateľ
// podľa IČO alebo názvu, rovnaká mena a režim neplatiteľa).
func (inv *Invoice) SameCustomer(p Invoice) bool {
	a, b := inv.Customer, p.Customer
	same := a.ICO != "" && a.ICO == b.ICO
	if a.ICO == "" || b.ICO == "" {
		same = strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(b.Name))
	}
	return same && inv.Currency == p.Currency && inv.NonVATPayer == p.NonVATPayer
}

// AddAdvance pridá do konceptu vyúčtovacej faktúry ďalšiu zálohu: jej
// odpočty a položky (dajú sa upraviť).
func (inv *Invoice) AddAdvance(proforma Invoice, receipts []Invoice) error {
	if inv.Status != StatusDraft || inv.Kind != KindInvoice || inv.Advance == nil || len(inv.Advance.Deductions) == 0 {
		return errors.New("zálohu možno pridať iba do konceptu vyúčtovacej faktúry")
	}
	for _, id := range inv.ProformaIDs() {
		if id == proforma.ID {
			return errors.New("záloha " + proforma.Number + " už je vo vyúčtovaní")
		}
	}
	if !inv.SameCustomer(proforma) {
		return ErrOtherCustomer
	}
	ds, err := advanceDeductions(proforma, receipts)
	if err != nil {
		return err
	}
	inv.Advance.Deductions = append(inv.Advance.Deductions, ds...)
	inv.Lines = append(inv.Lines, proforma.Lines...)
	inv.Note = strings.TrimSpace(inv.Note + " Vyúčtovanie zálohovej faktúry " + proforma.Number + ".")
	return inv.Recalculate()
}

// RemoveAdvance odoberie z konceptu vyúčtovania pridanú zálohu (pôvodnú,
// z ktorej vyúčtovanie vzniklo, nie). Položky zálohy ostanú na úpravu.
func (inv *Invoice) RemoveAdvance(proformaID int64) error {
	if inv.Status != StatusDraft || inv.Advance == nil {
		return errors.New("zálohu možno odobrať iba z konceptu vyúčtovacej faktúry")
	}
	if proformaID == inv.Advance.ProformaID {
		return errors.New("vyúčtovanie vzniklo z tejto zálohy — odobrať ju nemožno")
	}
	kept := inv.Advance.Deductions[:0]
	removed := false
	for _, d := range inv.Advance.Deductions {
		if d.ProformaID == proformaID {
			removed = true
			continue
		}
		kept = append(kept, d)
	}
	if !removed {
		return errors.New("záloha nie je vo vyúčtovaní")
	}
	inv.Advance.Deductions = kept
	return inv.Recalculate()
}

// applyDeductions odpočíta zálohy od rekapitulácie po sadzbách. Do DPH
// a kontrolného výkazu ide iba zostatok; celá dodávka ostane v Advance.Supply.
func (inv *Invoice) applyDeductions() {
	if inv.Advance == nil || len(inv.Advance.Deductions) == 0 {
		if inv.Advance != nil {
			inv.Advance.Supply = nil
		}
		return
	}
	full := inv.Summary
	full.Rates = append([]vat.RateSummary(nil), full.Rates...)
	inv.Advance.Supply = &full
	byRate := map[vat.Key]vat.RateSummary{}
	order := []vat.Key{}
	for _, r := range full.Rates {
		byRate[r.Key()] = r
		order = append(order, r.Key())
	}
	for _, d := range inv.Advance.Deductions {
		for _, r := range d.Rates {
			cur, ok := byRate[r.Key()]
			if !ok {
				order = append(order, r.Key())
				cur.Rate, cur.Reverse = r.Rate, r.Reverse
			}
			cur.Base, cur.VAT, cur.Total = cur.Base.Sub(r.Base), cur.VAT.Sub(r.VAT), cur.Total.Sub(r.Total)
			byRate[r.Key()] = cur
		}
	}
	s := vat.Summary{Rates: []vat.RateSummary{}}
	for _, rate := range order {
		r := byRate[rate]
		if r.Base.IsZero() && r.VAT.IsZero() && r.Total.IsZero() {
			continue // záloha pokryla celú sadzbu — do KV sa neuvádza
		}
		s.Rates = append(s.Rates, r)
		s.Base, s.VAT, s.Total = s.Base.Add(r.Base), s.VAT.Add(r.VAT), s.Total.Add(r.Total)
	}
	inv.Summary = s
}

// validateAdvance skontroluje väzby záloh.
func (inv *Invoice) validateAdvance() []FieldError {
	if inv.Advance == nil || len(inv.Advance.Deductions) == 0 {
		return nil
	}
	if inv.Kind != KindInvoice {
		return []FieldError{{"advance", "Odpočet zálohy má iba vyúčtovacia faktúra"}}
	}
	if inv.Summary.Total.Sign() < 0 {
		return []FieldError{{"lines", fmt.Sprintf("Záloha je vyššia ako vyúčtovanie o %s € — preplatok vráťte dobropisom", inv.Summary.Total.Neg().Display())}}
	}
	return nil
}
