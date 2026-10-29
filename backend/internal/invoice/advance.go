package invoice

import (
	"errors"
	"fmt"

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
	// na zálohovej faktúre: doklad k platbe a vyúčtovanie
	ReceiptID     int64  `json:"receiptId,omitempty"`
	ReceiptNumber string `json:"receiptNumber,omitempty"`
	FinalID       int64  `json:"finalId,omitempty"`
	FinalNumber   string `json:"finalNumber,omitempty"`
	// na vyúčtovacej faktúre: odpočítané zálohy a celá dodávka pred odpočtom
	Deductions []Deduction  `json:"deductions,omitempty"`
	Supply     *vat.Summary `json:"supply,omitempty"`
}

// Deduction je odpočet zálohy vo vyúčtovacej faktúre: základ a daň po sadzbách
// presne podľa dokladu k prijatej platbe (nie prepočítané).
type Deduction struct {
	ReceiptID int64             `json:"receiptId,omitempty"` // 0 = neplatiteľ DPH (bez dokladu k platbe)
	Number    string            `json:"number"`              // číslo dokladu k platbe, inak zálohovej faktúry
	Date      Date              `json:"date"`                // deň prijatia platby
	Rates     []vat.RateSummary `json:"rates"`
	Total     money.Dec         `json:"total"`
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
	return inv.Kind == KindProforma && !inv.NonVATPayer && !inv.ReverseCharge && !party.Foreign(inv.Customer.Country) && !inv.Foreign()
}

// NewAdvanceReceipt vyhotoví daňový doklad k prijatej platbe za uhradenú
// zálohovú faktúru. Daň sa vypočíta z prijatej platby („zhora“) po sadzbách
// zálohovej faktúry; dátum dodania = deň prijatia platby.
func NewAdvanceReceipt(proforma Invoice, issue Date) (Invoice, error) {
	if proforma.Kind != KindProforma {
		return Invoice{}, ErrNotAdvance
	}
	if proforma.Status != StatusPaid || proforma.PaidDate.IsZero() {
		return Invoice{}, ErrAdvanceNotPaid
	}
	if !proforma.NeedsReceipt() {
		return Invoice{}, ErrForeignAdvance
	}
	paid := proforma.PaidDate
	r := Invoice{
		Kind: KindAdvanceReceipt, Status: StatusDraft,
		Customer: proforma.Customer, ContactID: proforma.ContactID,
		IssueDate: issue, DeliveryDate: paid, DueDate: issue, // už uhradené
		Currency: proforma.Currency, PriceMode: vat.PriceGross, PaymentMethod: proforma.PaymentMethod,
		VariableSymbol: proforma.VariableSymbol,
		Note:           "Daňový doklad k prijatej platbe k zálohovej faktúre " + proforma.Number + ".",
		Advance:        &Advance{ProformaID: proforma.ID, ProformaNumber: proforma.Number},
	}
	what := "Prijatá platba k zálohovej faktúre " + proforma.Number
	if len(proforma.Lines) > 0 && proforma.Lines[0].Description != "" {
		what += " – " + proforma.Lines[0].Description
	}
	for _, rs := range proforma.Summary.Rates {
		if rs.Total.IsZero() {
			continue
		}
		r.Lines = append(r.Lines, Line{Description: what, Quantity: money.FromInt(1), Unit: "", UnitPrice: rs.Total, Rate: rs.Rate})
	}
	if len(r.Lines) == 0 {
		return Invoice{}, errors.New("zálohová faktúra nemá sumu")
	}
	if err := r.Recalculate(); err != nil {
		return Invoice{}, err
	}
	return r, nil
}

// NewFinalInvoice pripraví koncept vyúčtovacej faktúry k uhradenej zálohe:
// položky zálohy (dajú sa upraviť) a odpočet zálohy podľa dokladu k prijatej
// platbe. receipt je nil pri neplatiteľovi DPH (odpočíta sa uhradená suma).
func NewFinalInvoice(proforma Invoice, receipt *Invoice, issue Date, dueDays int) (Invoice, error) {
	if proforma.Kind != KindProforma {
		return Invoice{}, ErrNotAdvance
	}
	if proforma.Status != StatusPaid {
		return Invoice{}, ErrAdvanceNotPaid
	}
	if proforma.Advance != nil && proforma.Advance.FinalID != 0 {
		return Invoice{}, ErrAlreadySettled
	}
	if proforma.ReverseCharge || party.Foreign(proforma.Customer.Country) || proforma.Foreign() {
		return Invoice{}, ErrForeignAdvance
	}
	d := Deduction{Number: proforma.Number, Date: proforma.PaidDate}
	switch {
	case receipt != nil:
		d.ReceiptID, d.Number, d.Date = receipt.ID, receipt.Number, receipt.DeliveryDate
		d.Rates, d.Total = append([]vat.RateSummary(nil), receipt.Summary.Rates...), receipt.Summary.Total
	case proforma.NonVATPayer:
		d.Rates, d.Total = append([]vat.RateSummary(nil), proforma.Summary.Rates...), proforma.Summary.Total
	default:
		return Invoice{}, errors.New("chýba daňový doklad k prijatej platbe")
	}
	inv := Invoice{
		Kind: KindInvoice, Status: StatusDraft,
		Customer: proforma.Customer, ContactID: proforma.ContactID,
		IssueDate: issue, DeliveryDate: issue, DueDate: issue.AddDays(dueDays),
		Currency: proforma.Currency, PriceMode: proforma.PriceMode, SupplyKind: proforma.SupplyKind,
		NonVATPayer: proforma.NonVATPayer, PaymentMethod: proforma.PaymentMethod,
		Note:    "Vyúčtovanie zálohovej faktúry " + proforma.Number + ".",
		Lines:   append([]Line(nil), proforma.Lines...),
		Advance: &Advance{ProformaID: proforma.ID, ProformaNumber: proforma.Number, Deductions: []Deduction{d}},
	}
	if err := inv.Recalculate(); err != nil {
		return Invoice{}, err
	}
	return inv, nil
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
	byRate := map[vat.Rate]vat.RateSummary{}
	order := []vat.Rate{}
	for _, r := range full.Rates {
		byRate[r.Rate] = r
		order = append(order, r.Rate)
	}
	for _, d := range inv.Advance.Deductions {
		for _, r := range d.Rates {
			cur, ok := byRate[r.Rate]
			if !ok {
				order = append(order, r.Rate)
				cur.Rate = r.Rate
			}
			cur.Base, cur.VAT, cur.Total = cur.Base.Sub(r.Base), cur.VAT.Sub(r.VAT), cur.Total.Sub(r.Total)
			byRate[r.Rate] = cur
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
