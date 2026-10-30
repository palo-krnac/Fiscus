package journal

import (
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
)

// Návrh rozpisu úhrad dokladov. Pri čiastočnej úhrade sa DPH rozdelí
// pomerne (suma × DPH dokladu / suma dokladu); základ je zvyšok, aby rozpis
// vždy sedel na centy.

// Sale je vydaná faktúra pre rozpis jej úhrady (sumy v eurách).
type Sale struct {
	Total    money.Dec // suma dokladu s DPH (pri dobropise záporná)
	VAT      money.Dec // DPH dokladu
	Goods    bool      // predaj tovaru (d1), inak výrobky a služby (d2)
	VATPayer bool      // firma je platiteľ DPH
}

// SaleLines rozpíše prijatú platbu (amount > 0) alebo vrátené peniaze
// k dobropisu (amount < 0).
func SaleLines(amount money.Dec, s Sale) []Line {
	col := IncServices
	if s.Goods {
		col = IncGoods
	}
	vat := proportional(amount, s.VAT, s.Total)
	if !s.VATPayer || vat.IsZero() {
		return []Line{{col, amount}}
	}
	return []Line{{col, amount.Sub(vat)}, {IncVAT, vat}}
}

// Buy je prijatá faktúra pre rozpis jej úhrady (sumy v eurách).
type Buy struct {
	Total  money.Dec            // suma na úhradu dodávateľovi
	VAT    money.Dec            // DPH zaplatená dodávateľovi (pri prenesení 0)
	Kind   purchase.ExpenseKind // druh výdavku
	Deduct bool                 // DPH sa odpočítava (inak je súčasťou výdavku)
	// Percent je čiastočný odpočet (1–99 %); neodpočítaná časť DPH je výdavkom.
	Percent int
}

// ExpenseColumn vráti stĺpec pre druh výdavku.
func ExpenseColumn(k purchase.ExpenseKind) Column {
	switch k {
	case purchase.ExpenseStock:
		return ExpStock
	case purchase.ExpenseOther:
		return ExpOther
	case purchase.ExpenseAsset:
		return ExpAsset
	}
	return ExpServices
}

// BuyLines rozpíše úhradu dodávateľovi (amount < 0) alebo vrátené peniaze
// od dodávateľa (amount > 0, storno výdavku).
func BuyLines(amount money.Dec, b Buy) []Line {
	paid := amount.Neg() // kladná úhrada
	col := ExpenseColumn(b.Kind)
	vat := proportional(paid, b.VAT, b.Total)
	if b.Deduct && b.Percent > 0 && b.Percent < 100 {
		vat = vat.MulRatio(money.FromInt(int64(b.Percent)), money.FromInt(100)).RoundCents()
	}
	if !b.Deduct || vat.IsZero() {
		return []Line{{col, paid}}
	}
	return []Line{{col, paid.Sub(vat)}, {ExpVAT, vat}}
}

// proportional vráti amount × part / total zaokrúhlené na centy (0 pri nulovom total).
func proportional(amount, part, total money.Dec) money.Dec {
	if total.IsZero() || part.IsZero() {
		return money.Zero
	}
	return amount.MulRatio(part, total).RoundCents()
}
