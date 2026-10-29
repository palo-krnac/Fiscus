// Package invoice obsahuje doménový model vydanej faktúry a jej pravidlá.
// Nevie nič o databáze ani HTTP — dá sa testovať samostatne.
package invoice

import (
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Status je stav faktúry.
type Status string

const (
	StatusDraft     Status = "draft"     // koncept — dá sa upravovať, nemá číslo
	StatusIssued    Status = "issued"    // vystavená — uzamknutá
	StatusPaid      Status = "paid"      // uhradená
	StatusCancelled Status = "cancelled" // stornovaná
)

// Kind je druh dokladu.
type Kind string

const (
	KindInvoice    Kind = "invoice"     // faktúra
	KindCreditNote Kind = "credit_note" // dobropis
	KindProforma   Kind = "proforma"    // zálohová faktúra
)

// Party je odberateľ alebo dodávateľ.
type Party struct {
	Name    string `json:"name"`
	Street  string `json:"street"`
	City    string `json:"city"`
	Zip     string `json:"zip"`
	Country string `json:"country"` // ISO kód, napr. "SK"
	ICO     string `json:"ico"`
	DIC     string `json:"dic"`
	ICDPH   string `json:"icdph"`
	Email   string `json:"email"`
}

// Line je položka faktúry.
type Line struct {
	Description string    `json:"description"`
	Quantity    money.Dec `json:"quantity"`
	Unit        string    `json:"unit"`
	UnitPrice   money.Dec `json:"unitPrice"`
	Rate        vat.Rate  `json:"vatRate"`
	Amount      money.Dec `json:"amount"` // vypočítané: množstvo × cena
}

// Invoice je vydaná faktúra.
type Invoice struct {
	ID             int64         `json:"id"`
	CompanyID      int64         `json:"companyId"`
	Kind           Kind          `json:"kind"`
	Status         Status        `json:"status"`
	Number         string        `json:"number"`
	VariableSymbol string        `json:"variableSymbol"`
	Customer       Party         `json:"customer"`
	IssueDate      Date          `json:"issueDate"`
	DeliveryDate   Date          `json:"deliveryDate"` // dátum dodania tovaru/služby
	DueDate        Date          `json:"dueDate"`
	PaidDate       Date          `json:"paidDate"`
	Currency       string        `json:"currency"`
	PriceMode      vat.PriceMode `json:"priceMode"`
	ReverseCharge  bool          `json:"reverseCharge"`
	// NonVATPayer: dodávateľ nie je platiteľ DPH — faktúra je bez DPH.
	NonVATPayer bool        `json:"nonVatPayer"`
	ContactID   int64       `json:"contactId"` // odkaz do adresára (0 = zadané ručne)
	Note        string      `json:"note"`
	Lines       []Line      `json:"lines"`
	Summary     vat.Summary `json:"summary"`
}

// Recalculate prepočíta sumy položiek a rekapituláciu DPH.
func (inv *Invoice) Recalculate() error {
	if inv.PriceMode == "" {
		inv.PriceMode = vat.PriceNet
	}
	if inv.Currency == "" {
		inv.Currency = "EUR"
	}
	lines := make([]vat.Line, len(inv.Lines))
	for i := range inv.Lines {
		l := &inv.Lines[i]
		lines[i] = vat.Line{Quantity: l.Quantity, UnitPrice: l.UnitPrice, Rate: l.Rate}
		l.Amount = vat.LineAmount(lines[i])
	}
	s, err := vat.Calculate(lines, inv.PriceMode, inv.ReverseCharge || inv.NonVATPayer)
	if err != nil {
		return err
	}
	inv.Summary = s
	return nil
}

// FieldError je chyba konkrétneho poľa — frontend ju zobrazí pri poli.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError zoskupuje chyby polí.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = f.Field + ": " + f.Message
	}
	return "neplatná faktúra: " + strings.Join(msgs, "; ")
}

// Validate skontroluje faktúru pred uložením alebo vystavením.
func (inv *Invoice) Validate() error {
	var errs []FieldError
	add := func(f, m string) { errs = append(errs, FieldError{f, m}) }

	c := inv.Customer
	if strings.TrimSpace(c.Name) == "" {
		add("customer.name", "Zadajte odberateľa")
	}
	if c.ICO != "" && !party.ValidICO(c.ICO) {
		add("customer.ico", "Neplatné IČO")
	}
	if c.ICDPH != "" && strings.HasPrefix(party.NormalizeID(c.ICDPH), "SK") && !party.ValidICDPH(c.ICDPH) {
		add("customer.icdph", "Neplatné IČ DPH")
	}
	if inv.ReverseCharge && c.ICDPH == "" {
		add("customer.icdph", "Pri prenesení daňovej povinnosti je IČ DPH odberateľa povinné")
	}
	if len(inv.Lines) == 0 {
		add("lines", "Faktúra musí mať aspoň jednu položku")
	}
	for i, l := range inv.Lines {
		if strings.TrimSpace(l.Description) == "" {
			add(fmt.Sprintf("lines[%d].description", i), "Zadajte popis položky")
		}
		if l.Quantity.IsZero() {
			add(fmt.Sprintf("lines[%d].quantity", i), "Množstvo nesmie byť nula")
		}
		if !l.Rate.Valid() {
			add(fmt.Sprintf("lines[%d].vatRate", i), "Neplatná sadzba DPH")
		}
	}
	if inv.IssueDate.IsZero() {
		add("issueDate", "Zadajte dátum vystavenia")
	}
	if inv.DeliveryDate.IsZero() {
		add("deliveryDate", "Zadajte dátum dodania")
	}
	if inv.DueDate.IsZero() {
		add("dueDate", "Zadajte dátum splatnosti")
	} else if !inv.IssueDate.IsZero() && inv.DueDate.Before(inv.IssueDate) {
		add("dueDate", "Splatnosť nemôže byť pred dátumom vystavenia")
	}
	if len(errs) > 0 {
		return &ValidationError{errs}
	}
	return nil
}

// ErrLocked vracia sa pri pokuse zmeniť vystavenú faktúru.
var ErrLocked = errors.New("faktúra je vystavená a nedá sa meniť — vytvorte dobropis")

// ErrBadTransition je nepovolená zmena stavu.
var ErrBadTransition = errors.New("nepovolená zmena stavu faktúry")

// Editable vráti true, ak sa faktúra dá upravovať.
func (inv *Invoice) Editable() bool { return inv.Status == "" || inv.Status == StatusDraft }

// Issue vystaví koncept: pridelí číslo a variabilný symbol a uzamkne ho.
func (inv *Invoice) Issue(number string) error {
	if !inv.Editable() {
		return ErrBadTransition
	}
	if err := inv.Recalculate(); err != nil {
		return err
	}
	if err := inv.Validate(); err != nil {
		return err
	}
	inv.Number = number
	if inv.VariableSymbol == "" {
		inv.VariableSymbol = digitsOnly(number)
	}
	inv.Status = StatusIssued
	return nil
}

// MarkPaid označí vystavenú faktúru ako uhradenú.
func (inv *Invoice) MarkPaid(on Date) error {
	if inv.Status != StatusIssued {
		return ErrBadTransition
	}
	inv.Status = StatusPaid
	inv.PaidDate = on
	return nil
}

// Cancel stornuje vystavenú, ešte neuhradenú faktúru.
func (inv *Invoice) Cancel() error {
	if inv.Status != StatusIssued {
		return ErrBadTransition
	}
	inv.Status = StatusCancelled
	return nil
}

// Overdue vráti true, ak je faktúra po splatnosti k danému dňu.
func (inv *Invoice) Overdue(today Date) bool {
	return inv.Status == StatusIssued && today.After(inv.DueDate)
}

// Series vráti prefix číselného radu podľa druhu dokladu.
func Series(k Kind) string {
	switch k {
	case KindCreditNote:
		return "DB"
	case KindProforma:
		return "ZF"
	}
	return "FA"
}

// FormatNumber zloží číslo dokladu, napr. FA20260007.
func FormatNumber(series string, year, seq int) string {
	return fmt.Sprintf("%s%d%04d", series, year, seq)
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
