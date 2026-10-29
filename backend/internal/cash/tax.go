package cash

import (
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// DPH na pokladničnom doklade bez väzby na faktúru (iba platiteľ DPH):
//
//   - výdavok podľa pokladničného bloku = prijatá zjednodušená faktúra
//     (§ 74 ods. 3): odpočet dane (r. 19, 21…) a kontrolný výkaz B.3;
//   - príjem = tržba evidovaná v e-kase (ERP): daň na výstupe (r. 03, 04…)
//     a kontrolný výkaz D.1.
//
// Zjednodušená faktúra a e-kasa uvádzajú ceny s DPH, preto sa zadáva suma
// s DPH v každej sadzbe a základ a daň dopočíta Fiškus.

// VATRow je suma dokladu v jednej sadzbe DPH.
type VATRow struct {
	Rate  vat.Rate  `json:"vatRate"`
	Gross money.Dec `json:"gross"` // s DPH (zadáva používateľ)
	Base  money.Dec `json:"base"`
	VAT   money.Dec `json:"vat"`
}

// Tax je DPH dokladu.
type Tax struct {
	Rows []VATRow `json:"rows"`
	// SupplierICDPH je IČ DPH predajcu z pokladničného bloku (výdavok) —
	// bez neho nemožno daň odpočítať.
	SupplierICDPH string `json:"supplierIcdph"`
	// NoDeduction: DPH z bloku sa neodpočítava (reprezentácia, súkromná spotreba).
	NoDeduction bool `json:"noDeduction"`
	// Expense je druh výdavku v peňažnom denníku (výdavok).
	Expense purchase.ExpenseKind `json:"expense"`
	// Goods: tržba za predaj tovaru (stĺpec d1), inak výrobky a služby (d2).
	Goods bool `json:"goods"`
}

// Totals vráti základ, daň a sumu s DPH spolu.
func (t *Tax) Totals() (base, tax, gross money.Dec) {
	for _, r := range t.Rows {
		base, tax, gross = base.Add(r.Base), tax.Add(r.VAT), gross.Add(r.Gross)
	}
	return
}

// Deductible vráti daň na odpočet (0 pri príjme alebo bez odpočtu).
func (d Document) Deductible() money.Dec {
	if d.Tax == nil || d.Kind != Out || d.Tax.NoDeduction {
		return money.Zero
	}
	_, tax, _ := d.Tax.Totals()
	return tax
}

// NormalizeTax zlúči riadky s rovnakou sadzbou, vynechá nulové, dopočíta
// základ a daň a zjednotí IČ DPH. Prázdny rozpis zmaže.
func (d *Document) NormalizeTax() {
	t := d.Tax
	if t == nil {
		return
	}
	sums := map[vat.Rate]money.Dec{}
	var order []vat.Rate
	for _, r := range t.Rows {
		if _, ok := sums[r.Rate]; !ok {
			order = append(order, r.Rate)
		}
		sums[r.Rate] = sums[r.Rate].Add(r.Gross)
	}
	t.Rows = t.Rows[:0]
	hundred := money.FromInt(100)
	for _, rate := range order {
		g := sums[rate]
		if g.IsZero() {
			continue
		}
		rr := money.FromInt(int64(rate))
		tax := g.MulRatio(rr, hundred.Add(rr)).RoundCents()
		t.Rows = append(t.Rows, VATRow{Rate: rate, Gross: g, Base: g.Sub(tax), VAT: tax})
	}
	t.SupplierICDPH = strings.ToUpper(strings.Join(strings.Fields(t.SupplierICDPH), ""))
	if d.Kind == In {
		t.SupplierICDPH, t.NoDeduction, t.Expense = "", false, ""
	} else {
		t.Goods = false
	}
	if len(t.Rows) == 0 && t.SupplierICDPH == "" {
		d.Tax = nil
	}
}

// ValidateTax skontroluje DPH dokladu (po NormalizeTax).
func (d *Document) ValidateTax(date string) []invoice.FieldError {
	t := d.Tax
	if t == nil {
		return nil
	}
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if d.LinkKind != "" {
		add("tax", "DPH sa uvádza na faktúre — doklad, ktorý uhrádza faktúru, ju nemá")
		return fe
	}
	if len(t.Rows) == 0 {
		add("tax.rows", "Zadajte sumu aspoň v jednej sadzbe DPH")
	}
	for i, r := range t.Rows {
		switch {
		case !r.Rate.Known() || (date != "" && !r.Rate.ValidOn(date)):
			add(fmt.Sprintf("tax.rows.%d.vatRate", i), fmt.Sprintf("Sadzba %d %% v tento deň neplatí", r.Rate))
		case !r.Gross.Equal(r.Gross.RoundCents()):
			add(fmt.Sprintf("tax.rows.%d.gross", i), "Suma môže mať najviac dve desatinné miesta")
		}
	}
	if _, _, gross := t.Totals(); len(t.Rows) > 0 && !gross.Equal(d.Amount) {
		add("tax.rows", fmt.Sprintf("Súčet súm s DPH (%s €) sa nerovná sume dokladu (%s €)", gross.Display(), d.Amount.Display()))
	}
	if d.Kind == Out {
		if !t.Expense.Valid() {
			add("tax.expense", "Neznámy druh výdavku")
		}
		if !t.NoDeduction && !strings.HasPrefix(t.SupplierICDPH, "SK") {
			add("tax.supplierIcdph", "Zadajte IČ DPH predajcu z bloku (SK…) — bez neho nemožno DPH odpočítať")
		}
	}
	return fe
}
