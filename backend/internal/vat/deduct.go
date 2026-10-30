package vat

import "github.com/palo-krnac/fiscus/backend/internal/money"

// DeductPart vráti odpočítateľnú časť dane pri čiastočnom odpočte
// (percent 1–99, napr. 50 % pri osobnom vozidle podľa § 85n alebo
// koeficient). Percento 0 alebo 100 znamená plný odpočet.
func DeductPart(tax money.Dec, percent int) money.Dec {
	if percent <= 0 || percent >= 100 {
		return tax
	}
	return tax.MulRatio(money.FromInt(int64(percent)), money.FromInt(100)).RoundCents()
}

// DeductShare vráti odpočítateľnú časť dane pri čiastočnom odpočte
// (percent, napr. osobné vozidlo) a pomernom odpočítaní podľa § 50
// (coef = koeficient v percentách 0–100; 100 = celá daň, 0 = nič).
func DeductShare(tax money.Dec, percent, coef int) money.Dec {
	t := DeductPart(tax, percent)
	switch {
	case coef >= 100:
		return t
	case coef <= 0:
		return money.Zero
	}
	return t.MulRatio(money.FromInt(int64(coef)), money.FromInt(100)).RoundCents()
}

// CoefPercent vypočíta koeficient podľa § 50 ods. 2 v percentách:
// plnenia s nárokom na odpočet / všetky plnenia, zaokrúhlený na dve
// desatinné miesta nahor (0,3411 → 35 %). Bez plnení je koeficient 100 %.
func CoefPercent(withRight, exempt money.Dec) int {
	total := withRight.Add(exempt)
	if total.Sign() <= 0 || exempt.Sign() <= 0 {
		return 100
	}
	if withRight.Sign() <= 0 {
		return 0 // žiadne plnenia s nárokom na odpočet
	}
	// percentá na 4 desatinné miesta, potom nahor na celé
	r := withRight.MulRatio(money.FromInt(100), total)
	n := int(r.Raw() / money.FromInt(1).Raw())
	if money.FromInt(int64(n)).Cmp(r) < 0 {
		n++
	}
	return min(max(n, 0), 100)
}
