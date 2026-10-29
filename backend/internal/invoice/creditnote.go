package invoice

import (
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Correction sú údaje dobropisu (opravnej faktúry podľa § 71 ods. 2 zákona
// o DPH): odkaz na pôvodnú faktúru a dôvod opravy.
type Correction struct {
	OriginalID           int64  `json:"originalId"`
	OriginalNumber       string `json:"originalNumber"`
	OriginalIssueDate    Date   `json:"originalIssueDate"`
	OriginalDeliveryDate Date   `json:"originalDeliveryDate"`
	Reason               string `json:"reason"`
}

// RateDate je dátum, ku ktorému sa posudzuje sadzba DPH. Dobropis opravuje
// pôvodné plnenie, preto platí sadzba k pôvodnému dátumu dodania.
func (inv *Invoice) RateDate() Date {
	if inv.Kind == KindCreditNote && inv.Correction != nil && !inv.Correction.OriginalDeliveryDate.IsZero() {
		return inv.Correction.OriginalDeliveryDate
	}
	return inv.DeliveryDate
}

func (inv *Invoice) validateCorrection() []FieldError {
	if inv.Kind != KindCreditNote {
		if inv.Correction != nil {
			return []FieldError{{"correction", "Odkaz na pôvodnú faktúru má iba dobropis"}}
		}
		return nil
	}
	var errs []FieldError
	c := inv.Correction
	if c == nil || c.OriginalID == 0 {
		return []FieldError{{"correction", "Dobropis musí odkazovať na pôvodnú faktúru"}}
	}
	if strings.TrimSpace(c.Reason) == "" {
		errs = append(errs, FieldError{"correction.reason", "Zadajte dôvod opravy (napr. vrátenie tovaru, zľava)"})
	}
	if len(inv.Lines) > 0 && inv.Summary.Total.Sign() >= 0 {
		errs = append(errs, FieldError{"lines", "Dobropis musí znižovať sumu — zadajte záporné množstvá"})
	}
	return errs
}

// ErrNotCreditable — z faktúry v tomto stave sa nedá vystaviť dobropis.
var ErrNotCreditable = errors.New("dobropis sa dá vystaviť iba k vystavenej alebo uhradenej faktúre")

// NewCreditNote pripraví koncept dobropisu k vystavenej faktúre: rovnaký
// odberateľ a položky so zápornými množstvami (celý dobropis). Používateľ
// potom môže položky upraviť na čiastočný dobropis.
func NewCreditNote(orig Invoice, issueDate Date) (Invoice, error) {
	if orig.Kind == KindCreditNote {
		return Invoice{}, errors.New("k dobropisu sa nedá vystaviť ďalší dobropis")
	}
	if orig.Status != StatusIssued && orig.Status != StatusPaid {
		return Invoice{}, ErrNotCreditable
	}
	cn := Invoice{
		Kind:          KindCreditNote,
		Status:        StatusDraft,
		Customer:      orig.Customer,
		ContactID:     orig.ContactID,
		IssueDate:     issueDate,
		DeliveryDate:  issueDate, // dátum vzniku dôvodu opravy
		DueDate:       issueDate.AddDays(14),
		Currency:      orig.Currency,
		PriceMode:     orig.PriceMode,
		ReverseCharge: orig.ReverseCharge,
		SupplyKind:    orig.SupplyKind,
		NonVATPayer:   orig.NonVATPayer,
		PaymentMethod: orig.PaymentMethod,
		Correction: &Correction{
			OriginalID:           orig.ID,
			OriginalNumber:       orig.Number,
			OriginalIssueDate:    orig.IssueDate,
			OriginalDeliveryDate: orig.DeliveryDate,
		},
	}
	for _, l := range orig.Lines {
		l.Quantity = l.Quantity.Neg()
		cn.Lines = append(cn.Lines, l)
	}
	if err := cn.Recalculate(); err != nil {
		return Invoice{}, err
	}
	return cn, nil
}

// CheckCreditLimit overí, že dobropisy spolu nepresiahnu sumu pôvodnej
// faktúry. issued sú už vystavené dobropisy k tej istej faktúre.
func CheckCreditLimit(orig Invoice, issued []Invoice, cn Invoice) error {
	used := money.Zero
	for _, x := range issued {
		if x.ID != cn.ID && x.Status != StatusCancelled {
			used = used.Add(x.Summary.Total.Neg())
		}
	}
	left := orig.Summary.Total.Sub(used)
	if cn.Summary.Total.Neg().Cmp(left) > 0 {
		return &ValidationError{[]FieldError{{"lines", fmt.Sprintf(
			"Dobropisy by presiahli sumu faktúry %s — zostáva %s €", orig.Number, left.Display())}}}
	}
	return nil
}
