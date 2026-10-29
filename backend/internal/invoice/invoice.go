// Package invoice obsahuje doménový model vydanej faktúry a jej pravidlá.
// Nevie nič o databáze ani HTTP — dá sa testovať samostatne.
package invoice

import (
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/fx"
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

// PaymentMethod je spôsob úhrady.
type PaymentMethod string

const (
	PayTransfer PaymentMethod = "transfer" // prevodom na účet
	PayCash     PaymentMethod = "cash"     // v hotovosti
	PayCard     PaymentMethod = "card"     // platobnou kartou
	PayCOD      PaymentMethod = "cod"      // dobierkou
	PayOffset   PaymentMethod = "offset"   // vzájomným zápočtom
)

// PaymentMethods je číselník spôsobov úhrady (poradie = poradie v ponuke).
var PaymentMethods = []struct {
	Code  PaymentMethod `json:"code"`
	Label string        `json:"label"`
}{
	{PayTransfer, "Prevodom na účet"},
	{PayCash, "V hotovosti"},
	{PayCard, "Platobnou kartou"},
	{PayCOD, "Dobierkou"},
	{PayOffset, "Vzájomným zápočtom"},
}

// Valid vráti true pre spôsob úhrady z číselníka.
func (p PaymentMethod) Valid() bool {
	for _, m := range PaymentMethods {
		if m.Code == p {
			return true
		}
	}
	return false
}

// Line je položka faktúry.
type Line struct {
	Description string    `json:"description"`
	Quantity    money.Dec `json:"quantity"`
	Unit        string    `json:"unit"`
	UnitPrice   money.Dec `json:"unitPrice"`
	Rate        vat.Rate  `json:"vatRate"`
	Amount      money.Dec `json:"amount"` // vypočítané: množstvo × cena
	// Reverse: tuzemské prenesenie daňovej povinnosti pri položke (§ 69 ods. 12);
	// Commodity je kód colného sadzobníka (plodiny, kovy) pre kontrolný výkaz.
	Reverse   DomesticRC `json:"reverse,omitempty"`
	Commodity string     `json:"commodity,omitempty"`
}

// Invoice je vydaná faktúra.
type Invoice struct {
	ID             int64  `json:"id"`
	CompanyID      int64  `json:"companyId"`
	Kind           Kind   `json:"kind"`
	Status         Status `json:"status"`
	Number         string `json:"number"`
	VariableSymbol string `json:"variableSymbol"`
	Customer       Party  `json:"customer"`
	IssueDate      Date   `json:"issueDate"`
	DeliveryDate   Date   `json:"deliveryDate"` // dátum dodania tovaru/služby
	DueDate        Date   `json:"dueDate"`
	PaidDate       Date   `json:"paidDate"`
	// Payments sú úhrady (aj čiastočné); PaidAmount je ich súčet (dopĺňa program).
	Payments   []Payment `json:"payments,omitempty"`
	PaidAmount money.Dec `json:"paidAmount"`
	// Balance je zostatok na úhradu, BalanceEUR v eurách (pri cudzej mene
	// prepočítaný kurzom faktúry) — pre zoznamy a QR platbu (dopĺňa program).
	Balance       money.Dec     `json:"balance"`
	BalanceEUR    money.Dec     `json:"balanceEur"`
	Currency      string        `json:"currency"`
	PriceMode     vat.PriceMode `json:"priceMode"`
	ReverseCharge bool          `json:"reverseCharge"`
	// SupplyKind: tovar alebo služba — rozhoduje pri zahraničnom odberateľovi
	// (dodanie do EÚ / vývoz, súhrnný výkaz).
	SupplyKind SupplyKind `json:"supplyKind"`
	// NonVATPayer: dodávateľ nie je platiteľ DPH — faktúra je bez DPH.
	NonVATPayer bool  `json:"nonVatPayer"`
	ContactID   int64 `json:"contactId"` // odkaz do adresára (0 = zadané ručne)
	// PaymentMethod je spôsob úhrady (číselník PaymentMethods).
	PaymentMethod PaymentMethod `json:"paymentMethod"`
	Note          string        `json:"note"`
	// Correction sú údaje dobropisu (opravnej faktúry); nil pri bežnej faktúre.
	Correction *Correction `json:"correction,omitempty"`
	// Advance sú väzby záloh (zálohová faktúra, doklad k platbe, vyúčtovanie).
	Advance *Advance    `json:"advance,omitempty"`
	Lines   []Line      `json:"lines"`
	Summary vat.Summary `json:"summary"`

	// Cudzia mena: FXRate je kurz ECB (jednotiek meny za 1 EUR) vyhlásený
	// v deň FXDate; FXManual = kurz zadal používateľ (inak ho dopĺňa program).
	// Local sú sumy v eurách pre DPH a priznanie (iba pri cudzej mene).
	FXRate   money.Dec    `json:"fxRate"`
	FXDate   Date         `json:"fxDate"`
	FXManual bool         `json:"fxManual"`
	Local    *vat.Summary `json:"local,omitempty"`
}

// Foreign vráti true pre doklad v cudzej mene.
func (inv *Invoice) Foreign() bool { return inv.Currency != "" && inv.Currency != "EUR" }

// TaxSummary vráti sumy v eurách pre DPH (pri cudzej mene prepočítané
// kurzom); false, ak doklad v cudzej mene ešte nemá kurz.
func (inv *Invoice) TaxSummary() (vat.Summary, bool) {
	if !inv.Foreign() {
		return inv.Summary, true
	}
	if inv.Local == nil {
		return vat.Summary{}, false
	}
	return *inv.Local, true
}

// ToEUR prepočíta rekapituláciu kurzom (jednotiek meny za 1 EUR): základ
// a daň každej sadzby sa zaokrúhlia na centy (§ 26 ods. 3), spolu je ich súčet.
func ToEUR(s vat.Summary, rate money.Dec) vat.Summary {
	out := vat.Summary{Rates: make([]vat.RateSummary, len(s.Rates))}
	for i, r := range s.Rates {
		base, tax := r.Base.DivCents(rate), r.VAT.DivCents(rate)
		out.Rates[i] = vat.RateSummary{Rate: r.Rate, Base: base, VAT: tax, Total: base.Add(tax), Reverse: r.Reverse}
		out.Base, out.VAT = out.Base.Add(base), out.VAT.Add(tax)
	}
	out.Total = out.Base.Add(out.VAT)
	return out
}

// Recalculate prepočíta sumy položiek a rekapituláciu DPH.
func (inv *Invoice) Recalculate() error {
	if inv.PriceMode == "" {
		inv.PriceMode = vat.PriceNet
	}
	if inv.Currency == "" {
		inv.Currency = "EUR"
	}
	if inv.PaymentMethod == "" {
		inv.PaymentMethod = PayTransfer
	}
	lines := make([]vat.Line, len(inv.Lines))
	for i := range inv.Lines {
		l := &inv.Lines[i]
		lines[i] = vat.Line{Quantity: l.Quantity, UnitPrice: l.UnitPrice, Rate: l.Rate, Reverse: l.Reverse != ""}
		l.Amount = vat.LineAmount(lines[i])
	}
	s, err := vat.Calculate(lines, inv.PriceMode, inv.ReverseCharge || inv.NonVATPayer)
	if err != nil {
		return err
	}
	inv.Summary = s
	inv.applyDeductions()
	inv.SyncPaid()
	inv.Local = nil
	if inv.Foreign() && inv.FXRate.Sign() > 0 {
		local := ToEUR(inv.Summary, inv.FXRate)
		inv.Local = &local
	} else if !inv.Foreign() {
		inv.FXRate, inv.FXDate, inv.FXManual = money.Zero, Date{}, false
	}
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
	if !inv.SupplyKind.Valid() {
		add("supplyKind", "Neznámy druh plnenia")
	} else if party.Foreign(c.Country) && inv.SupplyKind == "" && !inv.NonVATPayer {
		add("supplyKind", "Pri zahraničnom odberateľovi vyberte, či ide o tovar alebo službu")
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
		if rateDate := inv.RateDate(); !inv.ReverseCharge && l.Reverse == "" && !inv.NonVATPayer && !rateDate.IsZero() && !l.Rate.ValidOn(rateDate.String()) {
			add(fmt.Sprintf("lines[%d].vatRate", i),
				fmt.Sprintf("Sadzba %d %% neplatí k dátumu dodania %s", l.Rate, rateDate.Display()))
		}
	}
	if inv.PaymentMethod != "" && !inv.PaymentMethod.Valid() {
		add("paymentMethod", "Neznámy spôsob úhrady")
	}
	if inv.Foreign() {
		switch {
		case !fx.Known(inv.Currency):
			add("currency", "Neznáma mena")
		case inv.FXRate.Sign() <= 0:
			add("fxRate", "Chýba kurz ECB k dátumu dodania — aktualizujte kurzy alebo zadajte kurz ručne")
		}
	}
	if inv.IssueDate.IsZero() {
		add("issueDate", "Zadajte dátum vystavenia")
	}
	if inv.DeliveryDate.IsZero() {
		add("deliveryDate", "Zadajte dátum dodania")
	}
	errs = append(errs, inv.validateDomesticRC()...)
	errs = append(errs, inv.validateCorrection()...)
	errs = append(errs, inv.validateAdvance()...)
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
		inv.VariableSymbol = VariableSymbol(number)
	}
	inv.Status = StatusIssued
	return nil
}

// MarkPaid označí vystavenú faktúru ako uhradenú.
func (inv *Invoice) MarkPaid(on Date) error {
	return inv.AddPayment(Payment{Date: on, Amount: inv.Remaining(), Source: PaymentManual})
}

// Unpay zruší všetky úhrady (napr. platba bola omylom priradená k inej faktúre).
func (inv *Invoice) Unpay() error {
	if inv.Status != StatusPaid && len(inv.Payments) == 0 {
		return ErrBadTransition
	}
	if inv.Status != StatusPaid && inv.Status != StatusIssued {
		return ErrBadTransition
	}
	inv.Payments = nil
	inv.Status = StatusIssued
	inv.PaidDate = Date{}
	inv.SyncPaid()
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
	// dobropis je záväzok voči odberateľovi, nie pohľadávka po splatnosti
	return inv.Status == StatusIssued && (inv.Kind == KindInvoice || inv.Kind == KindProforma) && today.After(inv.DueDate)
}

// SequenceKey je kľúč číselného radu podľa druhu dokladu. Je nezávislý
// od formátu, takže zmena formátu počas roka nezačne číslovať od 1.
func SequenceKey(k Kind) string {
	switch k {
	case KindCreditNote:
		return "DB"
	case KindProforma:
		return "ZF"
	case KindAdvanceReceipt:
		return "ZD"
	}
	return "FA"
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

// SupplyKind je druh plnenia.
type SupplyKind string

const (
	SupplyGoods    SupplyKind = "goods"    // dodanie tovaru
	SupplyServices SupplyKind = "services" // poskytnutie služby
)

// Valid vráti true pre prázdny alebo známy druh.
func (k SupplyKind) Valid() bool { return k == "" || k == SupplyGoods || k == SupplyServices }
