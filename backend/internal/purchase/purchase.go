// Package purchase je evidencia prijatých faktúr (nákup). Prijatá faktúra
// sa neprepočítava z položiek — opisuje sa rekapitulácia DPH z dokladu
// dodávateľa (základ a DPH podľa sadzieb), presne ako ju treba do
// priznania DPH a kontrolného výkazu (časť B.2).
package purchase

import (
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Status prijatej faktúry.
type Status string

const (
	StatusBooked Status = "booked" // zaevidovaná, neuhradená
	StatusPaid   Status = "paid"   // uhradená
)

// RateRow je riadok rekapitulácie DPH z dokladu dodávateľa.
type RateRow struct {
	Rate vat.Rate  `json:"vatRate"`
	Base money.Dec `json:"base"`
	VAT  money.Dec `json:"vat"`
}

// Summary sú súčty dokladu.
type Summary struct {
	Base  money.Dec `json:"base"`
	VAT   money.Dec `json:"vat"`   // DPH na odpočet / samozdanenie
	Total money.Dec `json:"total"` // suma na úhradu dodávateľovi
}

// Purchase je prijatá faktúra.
type Purchase struct {
	ID             int64                 `json:"id"`
	CompanyID      int64                 `json:"companyId"`
	Number         string                `json:"number"`         // interné poradové číslo (PF…)
	SupplierNumber string                `json:"supplierNumber"` // číslo faktúry dodávateľa
	VariableSymbol string                `json:"variableSymbol"`
	Status         Status                `json:"status"`
	ContactID      int64                 `json:"contactId"`
	Supplier       invoice.Party         `json:"supplier"`
	SupplierIBAN   string                `json:"supplierIban"`
	IssueDate      invoice.Date          `json:"issueDate"`    // dátum vyhotovenia
	DeliveryDate   invoice.Date          `json:"deliveryDate"` // dátum dodania
	ReceivedDate   invoice.Date          `json:"receivedDate"` // dátum doručenia — obdobie odpočtu
	DueDate        invoice.Date          `json:"dueDate"`
	PaidDate       invoice.Date          `json:"paidDate"`
	PaymentMethod  invoice.PaymentMethod `json:"paymentMethod"`
	// ReverseCharge: tuzemské prenesenie daňovej povinnosti (§ 69 ods. 12) —
	// dodávateľ fakturuje bez DPH, daň priznávame my (a zároveň ju odpočítavame).
	ReverseCharge bool `json:"reverseCharge"`
	// SupplyKind: tovar alebo služba — pri zahraničnom dodávateľovi rozhoduje
	// medzi nadobudnutím tovaru z EÚ (r. 05–08) a službou (r. 09–10).
	SupplyKind invoice.SupplyKind `json:"supplyKind"`
	// NoDeduction: DPH sa neodpočítava (napr. reprezentácia, súkromná spotreba).
	NoDeduction bool   `json:"noDeduction"`
	Description string `json:"description"` // predmet plnenia
	// CorrectsNumber je číslo pôvodnej faktúry dodávateľa, ktorú dobropis
	// (záporné sumy) opravuje — do kontrolného výkazu C.2.
	CorrectsNumber string `json:"correctsNumber"`
	// Expense je druh výdavku pre peňažný denník (zásoby, služby…); prázdny = služby.
	Expense ExpenseKind `json:"expense"`
	Note    string      `json:"note"`
	Rates   []RateRow   `json:"rates"`
	Summary Summary     `json:"summary"`

	// Cudzia mena: sumy dokladu sú v mene Currency, do DPH sa prepočítajú
	// kurzom ECB (FXRate jednotiek meny za 1 EUR, vyhlásený FXDate).
	Currency string       `json:"currency"`
	FXRate   money.Dec    `json:"fxRate"`
	FXDate   invoice.Date `json:"fxDate"`
	FXManual bool         `json:"fxManual"`
	Local    *Amounts     `json:"local,omitempty"` // sumy v eurách
}

// ExpenseKind je druh výdavku v peňažnom denníku (opatrenie MF SR
// č. MF/27076/2007-74, § 4 ods. 6 písm. e a g).
type ExpenseKind string

const (
	ExpenseServices ExpenseKind = "services" // služby (energie, nájom, telefón…)
	ExpenseStock    ExpenseKind = "stock"    // zásoby: materiál a tovar
	ExpenseOther    ExpenseKind = "other"    // ostatné výdavky
	ExpenseAsset    ExpenseKind = "asset"    // dlhodobý majetok — do základu dane cez odpisy
)

// ExpenseKinds je číselník druhov výdavku (poradie v ponuke).
var ExpenseKinds = []struct {
	Code  ExpenseKind `json:"code"`
	Label string      `json:"label"`
}{
	{ExpenseServices, "Služby"},
	{ExpenseStock, "Zásoby (materiál, tovar)"},
	{ExpenseOther, "Ostatné výdavky"},
	{ExpenseAsset, "Dlhodobý majetok"},
}

// Valid vráti true pre prázdny alebo známy druh.
func (k ExpenseKind) Valid() bool {
	if k == "" {
		return true
	}
	for _, x := range ExpenseKinds {
		if x.Code == k {
			return true
		}
	}
	return false
}

// Amounts sú rekapitulácia a súčty (v eurách pri doklade v cudzej mene).
type Amounts struct {
	Rates   []RateRow `json:"rates"`
	Summary Summary   `json:"summary"`
}

// Foreign vráti true pre doklad v cudzej mene.
func (p *Purchase) Foreign() bool { return p.Currency != "" && p.Currency != "EUR" }

// Tax vráti rekapituláciu a súčty v eurách pre DPH; false, ak doklad
// v cudzej mene ešte nemá kurz.
func (p *Purchase) Tax() (Amounts, bool) {
	if !p.Foreign() {
		return Amounts{Rates: p.Rates, Summary: p.Summary}, true
	}
	if p.Local == nil {
		return Amounts{}, false
	}
	return *p.Local, true
}

// TotalEUR je suma na úhradu v eurách (nula pri cudzej mene bez kurzu).
func (p *Purchase) TotalEUR() money.Dec {
	t, _ := p.Tax()
	return t.Summary.Total
}

// toEUR prepočíta riadky kurzom (každý základ a DPH zaokrúhlené na centy).
func (p *Purchase) toEUR() Amounts {
	out := Amounts{Rates: make([]RateRow, len(p.Rates))}
	for i, r := range p.Rates {
		e := RateRow{Rate: r.Rate, Base: r.Base.DivCents(p.FXRate), VAT: r.VAT.DivCents(p.FXRate)}
		out.Rates[i] = e
		out.Summary.Base = out.Summary.Base.Add(e.Base)
		out.Summary.VAT = out.Summary.VAT.Add(e.VAT)
	}
	out.Summary.Total = out.Summary.Base.Add(out.Summary.VAT)
	if p.ReverseCharge {
		out.Summary.Total = out.Summary.Base
	}
	return out
}

// Warning je upozornenie, ktoré nebráni uloženiu (napr. iné zaokrúhlenie DPH u dodávateľa).
type Warning struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Tolerance rozdielu medzi DPH z dokladu a vypočítanou DPH.
var tolerance = money.MustParse("0.05")

// ExpectedVAT je DPH vypočítaná zo základu (zaokrúhlená na centy).
func ExpectedVAT(r RateRow) money.Dec {
	return r.Base.MulRatio(money.FromInt(int64(r.Rate)), money.FromInt(100)).RoundCents()
}

// Normalize doplní chýbajúce hodnoty a spočíta súčty. Ak riadok nemá
// vyplnenú DPH, dopočíta ju zo základu; vyplnenú DPH z dokladu nemení.
func (p *Purchase) Normalize() []Warning {
	if p.PaymentMethod == "" {
		p.PaymentMethod = invoice.PayTransfer
	}
	if p.Status == "" {
		p.Status = StatusBooked
	}
	if p.DeliveryDate.IsZero() {
		p.DeliveryDate = p.IssueDate
	}
	var warns []Warning
	var s Summary
	for i := range p.Rates {
		r := &p.Rates[i]
		r.Base = r.Base.RoundCents()
		if r.Rate == vat.Rate0 {
			r.VAT = money.Zero
		} else if r.VAT.IsZero() {
			r.VAT = ExpectedVAT(*r)
		}
		r.VAT = r.VAT.RoundCents()
		if diff := r.VAT.Sub(ExpectedVAT(*r)); diff.Cmp(tolerance) > 0 || diff.Neg().Cmp(tolerance) > 0 {
			warns = append(warns, Warning{fmt.Sprintf("rates[%d].vat", i), fmt.Sprintf(
				"DPH %s € sa líši od %d %% zo základu (%s €) — skontrolujte doklad", r.VAT.Display(), r.Rate, ExpectedVAT(*r).Display())})
		}
		s.Base = s.Base.Add(r.Base)
		s.VAT = s.VAT.Add(r.VAT)
	}
	if p.ReverseCharge {
		s.Total = s.Base // dodávateľovi platíme bez DPH, DPH je samozdanenie
	} else {
		s.Total = s.Base.Add(s.VAT)
	}
	p.Summary = s
	p.Local = nil
	switch {
	case !p.Foreign():
		p.Currency, p.FXRate, p.FXDate, p.FXManual = "", money.Zero, invoice.Date{}, false
	case p.FXRate.Sign() > 0:
		local := p.toEUR()
		p.Local = &local
	}
	return warns
}

// Deductible je DPH v eurách, ktorú si možno odpočítať.
func (p *Purchase) Deductible() money.Dec {
	t, ok := p.Tax()
	if p.NoDeduction || !ok {
		return money.Zero
	}
	return t.Summary.VAT
}

// CreditNote: doklad je dobropis od dodávateľa (záporné sumy).
func (p *Purchase) CreditNote() bool {
	var b money.Dec
	for _, r := range p.Rates {
		b = b.Add(r.Base)
	}
	return b.Sign() < 0
}

// Validate skontroluje doklad.
func (p *Purchase) Validate() error {
	var errs []invoice.FieldError
	add := func(f, m string) { errs = append(errs, invoice.FieldError{Field: f, Message: m}) }

	s := p.Supplier
	if strings.TrimSpace(s.Name) == "" {
		add("supplier.name", "Zadajte dodávateľa")
	}
	if !p.Expense.Valid() {
		add("expense", "Neznámy druh výdavku")
	}
	if s.ICO != "" && (s.Country == "" || s.Country == "SK") && !party.ValidICO(s.ICO) {
		add("supplier.ico", "Neplatné IČO")
	}
	if s.ICDPH != "" && strings.HasPrefix(party.NormalizeID(s.ICDPH), "SK") && !party.ValidICDPH(s.ICDPH) {
		add("supplier.icdph", "Neplatné IČ DPH")
	}
	if strings.TrimSpace(p.SupplierNumber) == "" {
		add("supplierNumber", "Zadajte číslo faktúry dodávateľa")
	}
	if p.SupplierIBAN != "" && !party.ValidIBAN(p.SupplierIBAN) {
		add("supplierIban", "Neplatný IBAN")
	}
	if p.IssueDate.IsZero() {
		add("issueDate", "Zadajte dátum vyhotovenia")
	}
	if p.ReceivedDate.IsZero() {
		add("receivedDate", "Zadajte dátum doručenia")
	} else if !p.IssueDate.IsZero() && p.ReceivedDate.Before(p.IssueDate) {
		add("receivedDate", "Faktúra nemohla prísť skôr, ako bola vystavená")
	}
	if !p.DueDate.IsZero() && !p.IssueDate.IsZero() && p.DueDate.Before(p.IssueDate) {
		add("dueDate", "Splatnosť nemôže byť pred dátumom vyhotovenia")
	}
	if !p.SupplyKind.Valid() {
		add("supplyKind", "Neznámy druh plnenia")
	} else if party.Foreign(s.Country) && p.SupplyKind == "" {
		add("supplyKind", "Pri zahraničnom dodávateľovi vyberte, či ide o tovar alebo službu")
	}
	if !p.PaymentMethod.Valid() {
		add("paymentMethod", "Neznámy spôsob úhrady")
	}
	if p.Foreign() {
		switch {
		case !fx.Known(p.Currency):
			add("currency", "Neznáma mena")
		case p.FXRate.Sign() <= 0:
			add("fxRate", "Chýba kurz ECB k dátumu dodania — aktualizujte kurzy alebo zadajte kurz ručne")
		}
	}
	if len(p.Rates) == 0 {
		add("rates", "Zadajte aspoň jeden riadok rekapitulácie DPH")
	}
	rateDate := p.DeliveryDate
	if rateDate.IsZero() {
		rateDate = p.IssueDate
	}
	hasVAT := false
	for i, r := range p.Rates {
		if r.Base.IsZero() {
			add(fmt.Sprintf("rates[%d].base", i), "Zadajte základ dane")
		}
		if !r.Rate.Known() || (!rateDate.IsZero() && !r.Rate.ValidOn(rateDate.String())) {
			add(fmt.Sprintf("rates[%d].vatRate", i), fmt.Sprintf("Sadzba %d %% neplatí k dátumu dodania %s", r.Rate, rateDate.Display()))
		}
		if r.Rate != vat.Rate0 {
			hasVAT = true
		}
	}
	neg, pos := false, false
	for _, r := range p.Rates {
		neg, pos = neg || r.Base.Sign() < 0, pos || r.Base.Sign() > 0
	}
	switch {
	case neg && pos:
		add("rates", "Dobropis má všetky sumy záporné, faktúra kladné — nemiešajte ich na jednom doklade")
	case neg && hasVAT && !p.NoDeduction && strings.TrimSpace(p.CorrectsNumber) == "":
		add("correctsNumber", "Pri dobropise zadajte číslo pôvodnej faktúry dodávateľa (kontrolný výkaz C.2)")
	}
	// pre kontrolný výkaz (B.2) a prenesenie je IČ DPH dodávateľa povinné
	if hasVAT && !p.NoDeduction && s.ICDPH == "" {
		add("supplier.icdph", "Na odpočet DPH zadajte IČ DPH dodávateľa (potrebné do kontrolného výkazu)")
	}
	if len(errs) > 0 {
		return &invoice.ValidationError{Fields: errs}
	}
	return nil
}

// ErrPaid — uhradená faktúra sa nedá meniť ani zmazať.
var ErrPaid = errors.New("uhradená prijatá faktúra sa nedá meniť — najprv zrušte úhradu")

// MarkPaid označí faktúru ako uhradenú.
func (p *Purchase) MarkPaid(on invoice.Date) error {
	if p.Status != StatusBooked {
		return invoice.ErrBadTransition
	}
	p.Status, p.PaidDate = StatusPaid, on
	return nil
}

// Unpay zruší označenie úhrady (napr. omylom označená).
func (p *Purchase) Unpay() error {
	if p.Status != StatusPaid {
		return invoice.ErrBadTransition
	}
	p.Status, p.PaidDate = StatusBooked, invoice.Date{}
	return nil
}

// Overdue vráti true, ak je neuhradená po splatnosti.
func (p *Purchase) Overdue(today invoice.Date) bool {
	return p.Status == StatusBooked && !p.DueDate.IsZero() && today.After(p.DueDate)
}

// SameDocument vráti true, ak ide o tú istú faktúru toho istého dodávateľa
// (ochrana pred dvojitým zaevidovaním).
func SameDocument(a, b Purchase) bool {
	if !strings.EqualFold(strings.TrimSpace(a.SupplierNumber), strings.TrimSpace(b.SupplierNumber)) {
		return false
	}
	switch {
	case a.ContactID != 0 && a.ContactID == b.ContactID:
		return true
	case a.Supplier.ICO != "" && a.Supplier.ICO == b.Supplier.ICO:
		return true
	case a.Supplier.ICO == "" && b.Supplier.ICO == "":
		return strings.EqualFold(strings.TrimSpace(a.Supplier.Name), strings.TrimSpace(b.Supplier.Name))
	}
	return false
}
