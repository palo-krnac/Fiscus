// Package quote obsahuje cenovú ponuku. Ponuka nie je daňový doklad: nezapočítava
// sa do DPH, nepáruje sa s platbami a nie je po splatnosti. Z prijatej ponuky
// vznikne koncept faktúry.
package quote

import (
	"errors"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Status je stav ponuky.
type Status string

const (
	StatusDraft    Status = "draft"    // koncept — dá sa upravovať, nemá číslo
	StatusIssued   Status = "issued"   // vystavená (odoslaná zákazníkovi) — uzamknutá
	StatusAccepted Status = "accepted" // zákazník ju prijal
	StatusRejected Status = "rejected" // zákazník ju odmietol
	StatusInvoiced Status = "invoiced" // vyfakturovaná
)

// DefaultValidDays je predvolená platnosť ponuky.
const DefaultValidDays = 30

// Quote je cenová ponuka.
type Quote struct {
	ID            int64              `json:"id"`
	CompanyID     int64              `json:"companyId"`
	Status        Status             `json:"status"`
	Number        string             `json:"number"`
	Customer      invoice.Party      `json:"customer"`
	ContactID     int64              `json:"contactId"`
	IssueDate     invoice.Date       `json:"issueDate"`
	ValidUntil    invoice.Date       `json:"validUntil"`
	Currency      string             `json:"currency"`
	PriceMode     vat.PriceMode      `json:"priceMode"`
	ReverseCharge bool               `json:"reverseCharge"`
	SupplyKind    invoice.SupplyKind `json:"supplyKind"`
	NonVATPayer   bool               `json:"nonVatPayer"`
	Note          string             `json:"note"`
	Lines         []invoice.Line     `json:"lines"`
	Summary       vat.Summary        `json:"summary"`
	// InvoiceID: faktúra vytvorená z ponuky. InvoiceNumber dopĺňa aplikácia
	// pri čítaní (koncept faktúry číslo ešte nemá).
	InvoiceID     int64  `json:"invoiceId"`
	InvoiceNumber string `json:"invoiceNumber"`
}

var (
	// ErrLocked: vystavená ponuka sa nemení — vytvorí sa kópia.
	ErrLocked = errors.New("ponuka je vystavená a nedá sa meniť — vytvorte kópiu")
	// ErrBadTransition je nepovolená zmena stavu.
	ErrBadTransition = errors.New("nepovolená zmena stavu ponuky")
)

// asInvoice prevedie ponuku na faktúru kvôli spoločnému výpočtu a kontrole.
// Sadzby DPH sa posudzujú k dátumu ponuky.
func (q *Quote) asInvoice() invoice.Invoice {
	return invoice.Invoice{
		Kind: invoice.KindInvoice, Customer: q.Customer, ContactID: q.ContactID,
		IssueDate: q.IssueDate, DeliveryDate: q.IssueDate, DueDate: q.ValidUntil,
		Currency: q.Currency, PriceMode: q.PriceMode, ReverseCharge: q.ReverseCharge,
		SupplyKind: q.SupplyKind, NonVATPayer: q.NonVATPayer, Note: q.Note,
		Lines: append([]invoice.Line(nil), q.Lines...),
	}
}

// Recalculate prepočíta sumy položiek a rekapituláciu DPH (rovnako ako faktúra).
func (q *Quote) Recalculate() error {
	inv := q.asInvoice()
	if err := inv.Recalculate(); err != nil {
		return err
	}
	q.Lines, q.Summary, q.PriceMode, q.Currency = inv.Lines, inv.Summary, inv.PriceMode, inv.Currency
	return nil
}

// Validate skontroluje ponuku pred vystavením. Pravidlá sú rovnaké ako pri
// faktúre (odberateľ, položky, sadzby, prenesenie), namiesto splatnosti je platnosť.
func (q *Quote) Validate() error {
	var out []invoice.FieldError
	var ve *invoice.ValidationError
	inv := q.asInvoice()
	if err := inv.Validate(); errors.As(err, &ve) {
		for _, f := range ve.Fields {
			switch {
			case f.Field == "deliveryDate", f.Field == "dueDate":
				continue // platnosť kontrolujeme nižšie
			case f.Field == "fxRate":
				continue // ponuka nie je daňový doklad — kurz sa určí až na faktúre
			case f.Field == "lines":
				f.Message = "Ponuka musí mať aspoň jednu položku"
			case strings.HasSuffix(f.Field, ".vatRate"):
				f.Message = strings.Replace(f.Message, "dátumu dodania", "dátumu ponuky", 1)
			}
			out = append(out, f)
		}
	} else if err != nil {
		return err
	}
	if q.ValidUntil.IsZero() {
		out = append(out, invoice.FieldError{Field: "validUntil", Message: "Zadajte, dokedy ponuka platí"})
	} else if !q.IssueDate.IsZero() && q.ValidUntil.Before(q.IssueDate) {
		out = append(out, invoice.FieldError{Field: "validUntil", Message: "Platnosť nemôže skončiť pred dátumom ponuky"})
	}
	if len(out) > 0 {
		return &invoice.ValidationError{Fields: out}
	}
	return nil
}

// Editable vráti true, ak sa ponuka dá upravovať.
func (q *Quote) Editable() bool { return q.Status == "" || q.Status == StatusDraft }

// Issue vystaví koncept: pridelí číslo a uzamkne ho.
func (q *Quote) Issue(number string) error {
	if !q.Editable() {
		return ErrBadTransition
	}
	if err := q.Recalculate(); err != nil {
		return err
	}
	if err := q.Validate(); err != nil {
		return err
	}
	q.Number, q.Status = number, StatusIssued
	return nil
}

// Accept označí ponuku ako prijatú zákazníkom.
func (q *Quote) Accept() error {
	if q.Status != StatusIssued && q.Status != StatusRejected {
		return ErrBadTransition
	}
	q.Status = StatusAccepted
	return nil
}

// Reject označí ponuku ako odmietnutú.
func (q *Quote) Reject() error {
	if q.Status != StatusIssued && q.Status != StatusAccepted {
		return ErrBadTransition
	}
	q.Status = StatusRejected
	return nil
}

// Invoiceable vráti true, ak sa z ponuky dá vytvoriť faktúra.
func (q *Quote) Invoiceable() bool { return q.Status == StatusIssued || q.Status == StatusAccepted }

// MarkInvoiced zapíše faktúru vytvorenú z ponuky.
func (q *Quote) MarkInvoiced(id int64, number string) error {
	if !q.Invoiceable() {
		return ErrBadTransition
	}
	q.Status, q.InvoiceID, q.InvoiceNumber = StatusInvoiced, id, number
	return nil
}

// ToInvoice vytvorí koncept faktúry s odberateľom a položkami ponuky.
// Dátumy faktúry sa zadávajú nanovo (fakturuje sa k dnešku).
func (q *Quote) ToInvoice(today invoice.Date, dueDays int) invoice.Invoice {
	inv := q.asInvoice()
	inv.Status = invoice.StatusDraft
	inv.IssueDate, inv.DeliveryDate, inv.DueDate = today, today, today.AddDays(dueDays)
	inv.Note = q.Note
	if q.Number != "" {
		ref := "Podľa cenovej ponuky " + q.Number + "."
		if inv.Note == "" {
			inv.Note = ref
		} else {
			inv.Note = ref + "\n" + inv.Note
		}
	}
	return inv
}

// Copy vytvorí nový koncept s rovnakým obsahom a dnešným dátumom.
func (q *Quote) Copy(today invoice.Date, validDays int) Quote {
	c := *q
	c.ID, c.Number, c.Status, c.InvoiceID, c.InvoiceNumber = 0, "", StatusDraft, 0, ""
	c.IssueDate, c.ValidUntil = today, today.AddDays(validDays)
	c.Lines = append([]invoice.Line(nil), q.Lines...)
	return c
}

// Expired vráti true, ak vystavenej ponuke k danému dňu uplynula platnosť.
func (q *Quote) Expired(today invoice.Date) bool {
	return q.Status == StatusIssued && today.After(q.ValidUntil)
}
