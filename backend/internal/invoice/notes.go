package invoice

import "github.com/palo-krnac/fiscus/backend/internal/party"

// ExemptionNote vráti povinný text na faktúre pri plnení bez DPH (prenesenie
// daňovej povinnosti, oslobodenie). Rovnaké texty ako vatExemptionNote vo frontende.
func ExemptionNote(reverseCharge bool, customer Party, kind SupplyKind) string {
	if !reverseCharge {
		return ""
	}
	if !party.Foreign(customer.Country) {
		return "Prenesenie daňovej povinnosti podľa § 69 ods. 12 zákona o DPH. Daň odvedie odberateľ."
	}
	eu := party.IsEU(customer.Country)
	if kind == SupplyGoods {
		if eu {
			return "Dodanie tovaru do iného členského štátu oslobodené od dane podľa § 43 zákona o DPH (čl. 138 smernice 2006/112/ES)."
		}
		return "Vývoz tovaru oslobodený od dane podľa § 47 zákona o DPH."
	}
	if eu {
		return "Prenesenie daňovej povinnosti – daň platí príjemca služby (reverse charge, čl. 196 smernice 2006/112/ES)."
	}
	return "Miesto dodania služby je mimo územia SR (§ 15 ods. 1 zákona o DPH)."
}

// PaymentLabel vráti názov spôsobu úhrady.
func PaymentLabel(p PaymentMethod) string {
	for _, m := range PaymentMethods {
		if m.Code == p {
			return m.Label
		}
	}
	return PaymentMethods[0].Label
}

// Title vráti názov dokladu na tlači (napr. „Faktúra – daňový doklad“).
func (inv Invoice) Title() string {
	switch inv.Kind {
	case KindCreditNote:
		if inv.NonVATPayer {
			return "Dobropis – opravná faktúra"
		}
		return "Dobropis – opravný daňový doklad"
	case KindProforma:
		return "Zálohová faktúra – nie je daňový doklad"
	case KindAdvanceReceipt:
		return "Daňový doklad k prijatej platbe"
	}
	if inv.NonVATPayer {
		return "Faktúra"
	}
	return "Faktúra – daňový doklad"
}
