// Package vatreturn zostaví daňové priznanie k DPH (tlačivo DPHv25, platné od
// 1. 7. 2025) a kontrolný výkaz (KVDPHv25) z vydaných a prijatých faktúr.
//
// Pravidlá zaradenia:
//   - vydaná faktúra patrí do obdobia podľa dátumu dodania (vznik daňovej povinnosti),
//     dobropis podľa dátumu vyhotovenia (oprava podľa § 25 → riadky 24 a 25);
//   - prijatá faktúra patrí do obdobia, keď sú splnené obe podmienky odpočtu:
//     tovar/služba bola dodaná a faktúra je doručená (neskorší z dátumov);
//   - faktúry pre platiteľov DPH idú do KV jednotlivo (A.1, C.1), ostatné
//     súhrnne (D.2); prijaté s odpočtom do B.2, prenesenie daňovej povinnosti do B.1.
//
// Čo program zatiaľ nezaraďuje (dodanie do EÚ, export, tuzemské prenesenie
// pri dodaní, cudzia mena, dobropisy od dodávateľov), vráti ako upozornenie,
// aby to účtovník doplnil.
package vatreturn

import (
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Taxpayer sú údaje platiteľa do hlavičky priznania a výkazu.
type Taxpayer struct {
	Name, Street, City, Zip, Phone, Email string
	DIC, ICDPH                            string
	TaxOffice                             string // daňový úrad (môže byť prázdne)
}

// Warning je doklad, ktorý treba skontrolovať alebo doplniť ručne.
type Warning struct {
	Doc     string `json:"doc"`  // číslo dokladu
	Kind    string `json:"kind"` // "invoice" | "purchase"
	ID      int64  `json:"id"`   // ID dokladu
	Message string `json:"message"`
}

// KVLine je riadok A.1, B.1 alebo B.2.
type KVLine struct {
	Partner  string       `json:"partner"` // IČ DPH odberateľa / dodávateľa
	Name     string       `json:"name"`    // názov (iba na zobrazenie)
	Number   string       `json:"number"`  // číslo faktúry
	Date     invoice.Date `json:"date"`    // dátum dodania
	Base     money.Dec    `json:"base"`
	VAT      money.Dec    `json:"vat"`
	Rate     vat.Rate     `json:"rate"`
	Deducted money.Dec    `json:"deducted"` // odpočítaná daň (iba B.1, B.2)
	Kind     string       `json:"kind"`
	ID       int64        `json:"id"`
}

// KVCorrection je riadok C.1 (vydané opravné faktúry).
type KVCorrection struct {
	Partner  string    `json:"partner"`
	Name     string    `json:"name"`
	Number   string    `json:"number"`   // číslo opravnej faktúry (dobropisu)
	Original string    `json:"original"` // číslo pôvodnej faktúry
	BaseDiff money.Dec `json:"baseDiff"`
	VATDiff  money.Dec `json:"vatDiff"`
	Rate     vat.Rate  `json:"rate"`
	ID       int64     `json:"id"`
}

// KVSum je súhrnný riadok D.2: základná sadzba a znížené sadzby spolu.
type KVSum struct {
	Base        money.Dec `json:"base"`
	VAT         money.Dec `json:"vat"`
	BaseReduced money.Dec `json:"baseReduced"`
	VATReduced  money.Dec `json:"vatReduced"`
	Count       int       `json:"count"` // počet faktúr v súčte
}

// KV je kontrolný výkaz.
type KV struct {
	A1 []KVLine       `json:"a1"`
	B1 []KVLine       `json:"b1"`
	B2 []KVLine       `json:"b2"`
	C1 []KVCorrection `json:"c1"`
	D2 KVSum          `json:"d2"`
}

// Kódy plnenia v súhrnnom výkaze.
const (
	SVGoods      = 0 // dodanie tovaru do iného členského štátu (§ 43)
	SVTriangular = 1 // trojstranný obchod
	SVServices   = 2 // služby, pri ktorých daň platí príjemca v inom členskom štáte
)

// SVLine je riadok súhrnného výkazu (súčet za odberateľa a kód plnenia).
type SVLine struct {
	Country string    `json:"country"` // kód štátu z IČ DPH (EL pre Grécko)
	VATID   string    `json:"vatId"`   // IČ DPH bez kódu štátu
	Name    string    `json:"name"`
	Value   money.Dec `json:"value"`
	Code    int       `json:"code"`
	Count   int       `json:"count"` // počet dokladov
}

// Return je priznanie k DPH spolu s kontrolným výkazom.
type Return struct {
	Period Period               `json:"period"`
	Label  string               `json:"label"`
	Rows   map[string]money.Dec `json:"rows"` // r01 … r37 (iba nenulové)
	// Deduction je odpočet dane spolu (r. 18 + 18a + 19) na prehľad.
	Deduction money.Dec `json:"deduction"`
	KV        KV        `json:"kv"`
	SV        []SVLine  `json:"sv"` // súhrnný výkaz
	Warnings  []Warning `json:"warnings"`
	Invoices  int       `json:"invoices"`  // počet zahrnutých vydaných dokladov
	Purchase  int       `json:"purchases"` // počet zahrnutých prijatých faktúr
}

// Input sú podklady pre výpočet.
type Input struct {
	Period    Period
	Invoices  []invoice.Invoice
	Purchases []purchase.Purchase
	// ExcessCarried je nadmerný odpočet z predchádzajúceho obdobia (riadok 34).
	ExcessCarried money.Dec
}

// riadky základu a dane podľa sadzby (DPHv25)
type rowPair struct{ base, tax string }

var (
	outputRows      = map[vat.Rate]rowPair{19: {"r01", "r02"}, 5: {"r01a", "r02a"}, 23: {"r03", "r04"}}
	acquisitionRows = map[vat.Rate]rowPair{19: {"r05", "r06"}, 5: {"r05a", "r06a"}, 23: {"r07", "r08"}}
	reverseRows     = map[vat.Rate]rowPair{19: {"r09", "r10"}, 5: {"r09a", "r10a"}, 23: {"r09b", "r10b"}}
	deductRows      = map[vat.Rate]string{19: "r18", 5: "r18a", 23: "r19"}
	domesticRow     = map[vat.Rate]string{19: "r20", 5: "r20a", 23: "r21"}
	taxRows         = []string{"r02", "r02a", "r04", "r06", "r06a", "r08", "r10", "r10a", "r10b", "r12", "r12a", "r12b", "r12c", "r12d", "r12e", "r16"}
)

// Reduced vráti true pre zníženú sadzbu (v D.2 sa sčítavajú spolu).
func reduced(r vat.Rate) bool { return r != 23 && r != 0 }

func isSKPayer(icdph string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.ReplaceAll(icdph, " ", "")), "SK")
}

func foreign(country string) bool { return country != "" && country != "SK" }

func noSpaces(s string) string { return strings.Join(strings.Fields(s), "") }

// Compute zostaví priznanie a kontrolný výkaz.
func Compute(in Input) Return {
	ret := Return{Period: in.Period, Label: in.Period.Label(), Rows: map[string]money.Dec{}, KV: KV{
		A1: []KVLine{}, B1: []KVLine{}, B2: []KVLine{}, C1: []KVCorrection{},
	}, SV: []SVLine{}, Warnings: []Warning{}}
	add := func(row string, v money.Dec) { ret.Rows[row] = ret.Rows[row].Add(v) }
	warn := func(doc, kind string, id int64, msg string) {
		ret.Warnings = append(ret.Warnings, Warning{doc, kind, id, msg})
	}

	for _, inv := range in.Invoices {
		if inv.Status != invoice.StatusIssued && inv.Status != invoice.StatusPaid {
			continue // koncept, stornovaná
		}
		credit := inv.Kind == invoice.KindCreditNote
		when := inv.DeliveryDate
		if credit || when.IsZero() {
			when = inv.IssueDate
		}
		if !in.Period.Contains(when) {
			continue
		}
		c := inv.Customer
		// sumy v eurách (faktúra v cudzej mene je prepočítaná kurzom ECB)
		sum, ok := inv.TaxSummary()
		switch {
		case inv.Kind == invoice.KindProforma:
			continue
		case !ok:
			warn(inv.Number, "invoice", inv.ID, "Faktúra v mene "+inv.Currency+" nemá kurz — doplňte ho, inak nejde do priznania")
			continue
		case foreign(c.Country) && sum.VAT.IsZero():
			if msg := ret.addForeignSale(inv); msg != "" {
				warn(inv.Number, "invoice", inv.ID, msg)
			} else {
				ret.Invoices++
			}
			continue
		case inv.ReverseCharge:
			warn(inv.Number, "invoice", inv.ID, "Tuzemské prenesenie daňovej povinnosti (§ 69 ods. 12, A.2) zatiaľ nie je podporované — doplňte ručne")
			continue
		}
		payer := isSKPayer(c.ICDPH)
		ret.Invoices++
		for _, r := range sum.Rates {
			if r.Rate == 0 {
				// oslobodené plnenie: iba riadok 13, do KV sa neuvádza
				if !credit {
					add("r13", r.Base)
				}
				continue
			}
			if credit {
				// oprava základu dane podľa § 25 ods. 1 (dobropis)
				add("r24", r.Base)
				add("r25", r.VAT)
			} else if rows, ok := outputRows[r.Rate]; ok {
				add(rows.base, r.Base)
				add(rows.tax, r.VAT)
			} else {
				warn(inv.Number, "invoice", inv.ID, fmt.Sprintf("Sadzba %d %% nepatrí do tlačiva DPHv25", r.Rate))
				continue
			}
			switch {
			case payer && credit:
				orig := ""
				if inv.Correction != nil {
					orig = inv.Correction.OriginalNumber
				}
				if orig == "" {
					warn(inv.Number, "invoice", inv.ID, "Dobropis bez čísla pôvodnej faktúry — v C.1 ho doplňte ručne")
				}
				ret.KV.C1 = append(ret.KV.C1, KVCorrection{noSpaces(c.ICDPH), c.Name, noSpaces(inv.Number), noSpaces(orig), r.Base, r.VAT, r.Rate, inv.ID})
			case payer:
				ret.KV.A1 = append(ret.KV.A1, KVLine{Partner: noSpaces(c.ICDPH), Name: c.Name, Number: noSpaces(inv.Number), Date: when,
					Base: r.Base, VAT: r.VAT, Rate: r.Rate, Kind: "invoice", ID: inv.ID})
			default:
				if reduced(r.Rate) {
					ret.KV.D2.BaseReduced = ret.KV.D2.BaseReduced.Add(r.Base)
					ret.KV.D2.VATReduced = ret.KV.D2.VATReduced.Add(r.VAT)
				} else {
					ret.KV.D2.Base = ret.KV.D2.Base.Add(r.Base)
					ret.KV.D2.VAT = ret.KV.D2.VAT.Add(r.VAT)
				}
			}
		}
		if !payer {
			ret.KV.D2.Count++
		}
	}

	for _, p := range in.Purchases {
		when := p.ReceivedDate
		if p.DeliveryDate.After(when) {
			when = p.DeliveryDate
		}
		if !in.Period.Contains(when) {
			continue
		}
		doc := p.Number
		s := p.Supplier
		switch {
		case p.Summary.Base.Sign() < 0:
			warn(doc, "purchase", p.ID, "Dobropis od dodávateľa (C.2, r. 28) zatiaľ nie je podporovaný — doplňte ručne")
			continue
		case foreign(s.Country) && !p.ReverseCharge:
			warn(doc, "purchase", p.ID, "Zahraničná faktúra bez prenesenia daňovej povinnosti — zahraničnú DPH nemožno odpočítať v SR. Ak daň platíte vy, označte prenesenie.")
			continue
		case foreign(s.Country) && p.SupplyKind == "":
			warn(doc, "purchase", p.ID, "Pri zahraničnom dodávateľovi vyberte, či ide o tovar alebo službu")
			continue
		case foreign(s.Country) && p.SupplyKind == invoice.SupplyGoods && !party.IsEU(s.Country):
			warn(doc, "purchase", p.ID, "Dovoz tovaru z tretej krajiny (DPH platená colnému úradu, r. 22–23) zatiaľ nie je podporovaný — doplňte ručne")
			continue
		case p.NoDeduction && !p.ReverseCharge:
			continue // bez odpočtu: neuvádza sa ani v KV
		}
		included := false
		date := p.DeliveryDate
		if date.IsZero() {
			date = p.IssueDate
		}
		for _, r := range p.Rates {
			if r.Rate == 0 {
				continue
			}
			deducted := r.VAT
			if p.NoDeduction {
				deducted = money.Zero
			}
			if p.ReverseCharge {
				table := reverseRows // služby zo zahraničia, tuzemské prenesenie (§ 69)
				if foreign(s.Country) && p.SupplyKind == invoice.SupplyGoods {
					table = acquisitionRows // nadobudnutie tovaru z EÚ (§ 11)
				}
				rows, ok := table[r.Rate]
				if !ok {
					warn(doc, "purchase", p.ID, fmt.Sprintf("Sadzba %d %% nepatrí do tlačiva DPHv25", r.Rate))
					continue
				}
				add(rows.base, r.Base)
				add(rows.tax, r.VAT)
				if !deducted.IsZero() {
					add(deductRows[r.Rate], deducted)
				}
				partner := noSpaces(s.ICDPH)
				included = true
				ret.KV.B1 = append(ret.KV.B1, KVLine{Partner: partner, Name: s.Name, Number: noSpaces(p.SupplierNumber), Date: date,
					Base: r.Base, VAT: r.VAT, Rate: r.Rate, Deducted: deducted, Kind: "purchase", ID: p.ID})
				continue
			}
			row, ok := deductRows[r.Rate]
			if !ok {
				warn(doc, "purchase", p.ID, fmt.Sprintf("Sadzba %d %% nepatrí do tlačiva DPHv25", r.Rate))
				continue
			}
			if !isSKPayer(s.ICDPH) {
				warn(doc, "purchase", p.ID, "Dodávateľ nemá slovenské IČ DPH — odpočet bez neho nie je možný (B.2)")
				continue
			}
			included = true
			add(row, deducted)
			add(domesticRow[r.Rate], deducted)
			ret.KV.B2 = append(ret.KV.B2, KVLine{Partner: noSpaces(s.ICDPH), Name: s.Name, Number: noSpaces(p.SupplierNumber), Date: date,
				Base: r.Base, VAT: r.VAT, Rate: r.Rate, Deducted: deducted, Kind: "purchase", ID: p.ID})
		}
		if included {
			ret.Purchase++
		}
	}

	// súčty
	var r17 money.Dec
	for _, k := range taxRows {
		r17 = r17.Add(ret.Rows[k])
	}
	ret.Rows["r17"] = r17
	ret.Deduction = ret.Rows["r18"].Add(ret.Rows["r18a"]).Add(ret.Rows["r19"]).RoundCents()
	result := r17.Sub(ret.Rows["r18"]).Sub(ret.Rows["r18a"]).Sub(ret.Rows["r19"]).
		Add(ret.Rows["r25"]).Add(ret.Rows["r27"]).Add(ret.Rows["r28"]).Add(ret.Rows["r29"]).
		Sub(ret.Rows["r30"]).Sub(ret.Rows["r31"])
	if result.Sign() >= 0 {
		ret.Rows["r32"] = result
	} else {
		ret.Rows["r33"] = result.Neg()
	}
	// nadmerný odpočet z minulého obdobia sa započíta najviac do výšky vlastnej daňovej povinnosti
	if ex := in.ExcessCarried; ex.Sign() > 0 {
		if ex.Cmp(ret.Rows["r32"]) > 0 {
			ex = ret.Rows["r32"]
		}
		ret.Rows["r34"] = ex
	}
	if r35 := ret.Rows["r32"].Sub(ret.Rows["r34"]); r35.Sign() > 0 {
		ret.Rows["r35"] = r35
	}
	for k, v := range ret.Rows {
		v = v.RoundCents()
		if v.IsZero() {
			delete(ret.Rows, k)
		} else {
			ret.Rows[k] = v
		}
	}
	return ret
}

// addForeignSale zaradí faktúru zahraničnému odberateľovi bez DPH:
// tovar do EÚ (r. 13, 14, SV kód 0), vývoz tovaru (r. 13, 15),
// služby do EÚ (iba SV kód 2), služby mimo EÚ (mimo priznania).
// Vráti text upozornenia, ak faktúru nemožno zaradiť.
func (ret *Return) addForeignSale(inv invoice.Invoice) string {
	c := inv.Customer
	eu := party.IsEU(c.Country)
	sum, _ := inv.TaxSummary()
	base := sum.Base
	add := func(row string) { ret.Rows[row] = ret.Rows[row].Add(base) }
	switch inv.SupplyKind {
	case "":
		return "Pri zahraničnom odberateľovi vyberte na faktúre, či ide o tovar alebo službu"
	case invoice.SupplyGoods:
		if !eu {
			add("r13")
			add("r15")
			return ""
		}
		if c.ICDPH == "" {
			return "Dodanie tovaru do EÚ je oslobodené iba pre odberateľa s IČ DPH — doplňte ho alebo fakturujte s DPH"
		}
		add("r13")
		add("r14")
		ret.addSV(c, SVGoods, base)
	case invoice.SupplyServices:
		if !eu {
			return "" // služba mimo EÚ: miesto dodania v zahraničí, do priznania sa neuvádza
		}
		if c.ICDPH == "" {
			return "Služba do EÚ s prenesením daňovej povinnosti vyžaduje IČ DPH odberateľa"
		}
		ret.addSV(c, SVServices, base)
	}
	return ""
}

func (ret *Return) addSV(c invoice.Party, code int, value money.Dec) {
	country := party.VATCountry(c.ICDPH, c.Country)
	id := strings.TrimPrefix(party.NormalizeID(c.ICDPH), country)
	for i := range ret.SV {
		if l := &ret.SV[i]; l.Country == country && l.VATID == id && l.Code == code {
			l.Value = l.Value.Add(value)
			l.Count++
			return
		}
	}
	ret.SV = append(ret.SV, SVLine{Country: country, VATID: id, Name: c.Name, Value: value, Code: code, Count: 1})
}
