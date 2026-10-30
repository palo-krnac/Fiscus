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
