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

	"github.com/palo-krnac/fiscus/backend/internal/cash"
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

// KVCorrection je riadok C.1 (vydané opravné faktúry) alebo C.2 (prijaté
// opravné faktúry — tam aj oprava odpočítanej dane).
type KVCorrection struct {
	Partner      string    `json:"partner"`
	Name         string    `json:"name"`
	Number       string    `json:"number"`   // číslo opravnej faktúry (dobropisu)
	Original     string    `json:"original"` // číslo pôvodnej faktúry
	BaseDiff     money.Dec `json:"baseDiff"`
	VATDiff      money.Dec `json:"vatDiff"`
	Rate         vat.Rate  `json:"rate"`
	DeductedDiff money.Dec `json:"deductedDiff"` // iba C.2
	ID           int64     `json:"id"`
}

// KVSum je súhrnný riadok D.2: základná sadzba a znížené sadzby spolu.
type KVSum struct {
	Base        money.Dec `json:"base"`
	VAT         money.Dec `json:"vat"`
	BaseReduced money.Dec `json:"baseReduced"`
	VATReduced  money.Dec `json:"vatReduced"`
	Count       int       `json:"count"` // počet faktúr v súčte
}

// KVA2 je riadok A.2: dodanie s tuzemským prenesením daňovej povinnosti
// (§ 69 ods. 12 písm. f až j) — základ bez dane, pri tovare kód a množstvo.
type KVA2 struct {
	Partner  string       `json:"partner"`
	Name     string       `json:"name"`
	Number   string       `json:"number"`
	Date     invoice.Date `json:"date"`
	Base     money.Dec    `json:"base"`
	Code     string       `json:"code,omitempty"`  // 4-miestny kód colného sadzobníka (plodiny, kovy)
	Goods    string       `json:"goods,omitempty"` // druh tovaru: MT, IO
	Quantity *money.Dec   `json:"quantity,omitempty"`
	Unit     string       `json:"unit,omitempty"` // t, kg, m, ks
	Label    string       `json:"label"`          // druh plnenia (iba na zobrazenie)
	ID       int64        `json:"id"`
}

// KVB3 je riadok B.3 (prijaté zjednodušené faktúry — pokladničné bloky):
// v B.3.1 jeden súhrn bez IČ DPH, v B.3.2 súčet za každého dodávateľa.
type KVB3 struct {
	Partner  string    `json:"partner,omitempty"` // IČ DPH dodávateľa (iba B.3.2)
	Name     string    `json:"name,omitempty"`
	Base     money.Dec `json:"base"`
	VAT      money.Dec `json:"vat"`
	Deducted money.Dec `json:"deducted"`
	Count    int       `json:"count"` // počet dokladov
}

// B3Limit: od tejto sumy odpočítanej dane zo zjednodušených faktúr za
// obdobie sa B.3 vypĺňa po dodávateľoch (B.3.2), inak súhrnne (B.3.1).
var B3Limit = money.FromInt(3000)

// KV je kontrolný výkaz.
type KV struct {
	A1  []KVLine       `json:"a1"`
	A2  []KVA2         `json:"a2"`
	B1  []KVLine       `json:"b1"`
	B2  []KVLine       `json:"b2"`
	B31 *KVB3          `json:"b31,omitempty"` // súhrn (odpočet pod 3 000 €)
	B32 []KVB3         `json:"b32"`           // po dodávateľoch (odpočet od 3 000 €)
	C1  []KVCorrection `json:"c1"`
	C2  []KVCorrection `json:"c2"` // prijaté dobropisy od dodávateľov
	D1  KVSum          `json:"d1"` // tržby z e-kasy (ERP) súhrnne
	D2  KVSum          `json:"d2"`
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
	Cash      int       `json:"cash"`      // počet zahrnutých pokladničných dokladov s DPH
}

// Input sú podklady pre výpočet.
type Input struct {
	Period    Period
	Invoices  []invoice.Invoice
	Purchases []purchase.Purchase
	// CashDocs sú pokladničné doklady; do priznania idú iba platné doklady
	// s DPH bez väzby na faktúru (blok = B.3, tržba z e-kasy = D.1).
	CashDocs []cash.Document
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
		A1: []KVLine{}, A2: []KVA2{}, B1: []KVLine{}, B2: []KVLine{}, B32: []KVB3{}, C1: []KVCorrection{}, C2: []KVCorrection{},
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
			warn(inv.Number, "invoice", inv.ID, "Prenesenie pri odberateľovi v tuzemsku: vyberte na faktúre druh prenesenia pri položkách (§ 69 ods. 12) — doplňte ručne")
			continue
		}
		if inv.DomesticReverse() {
			if credit {
				warn(inv.Number, "invoice", inv.ID, "Dobropis k dodaniu s prenesením daňovej povinnosti (A.2, kód opravy) zatiaľ nie je podporovaný — doplňte ručne")
			} else {
				ret.addA2(inv, when, warn)
			}
		}
		payer := isSKPayer(c.ICDPH)
		ret.Invoices++
		for _, r := range sum.Rates {
			if r.Reverse {
				continue // tuzemské prenesenie: daň priznáva odberateľ, dodávateľ len A.2
			}
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
				ret.KV.C1 = append(ret.KV.C1, KVCorrection{Partner: noSpaces(c.ICDPH), Name: c.Name, Number: noSpaces(inv.Number), Original: noSpaces(orig),
					BaseDiff: r.Base, VATDiff: r.VAT, Rate: r.Rate, ID: inv.ID})
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
		tax, ok := p.Tax() // sumy v eurách (cudzia mena prepočítaná kurzom)
		switch {
		case !ok:
			warn(doc, "purchase", p.ID, "Prijatá faktúra v mene "+p.Currency+" nemá kurz — doplňte ho, inak nejde do priznania")
			continue
		case tax.Summary.Base.Sign() < 0:
			if msg := ret.addPurchaseCredit(p, tax); msg != "" {
				warn(doc, "purchase", p.ID, msg)
			} else {
				ret.Purchase++
			}
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
		for _, r := range tax.Rates {
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

	ret.addCash(in, add, warn)

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

// addA2 pridá položky faktúry s tuzemským prenesením do A.2: plodiny a kovy
// po 4-miestnych kódoch, telefóny a obvody po druhu, stavebné práce spolu.
// Písm. a) až e) sa v kontrolnom výkaze dodávateľa neuvádzajú.
func (ret *Return) addA2(inv invoice.Invoice, when invoice.Date, warn func(doc, kind string, id int64, msg string)) {
	type group struct {
		line  KVA2
		goods bool
		qty   money.Dec
	}
	var order []string
	groups := map[string]*group{}
	for _, l := range inv.Lines {
		k := l.Reverse
		if !k.Reported() {
			continue
		}
		code := ""
		if k.NeedsCode() {
			code = invoice.CommodityCode(l.Commodity)
		}
		key := string(k) + "/" + code
		g, ok := groups[key]
		if !ok {
			g = &group{goods: k.Goods(), line: KVA2{Partner: noSpaces(strings.ToUpper(inv.Customer.ICDPH)), Name: inv.Customer.Name,
				Number: noSpaces(inv.Number), Date: when, Code: code, Goods: k.KVKind(), Label: k.Label(), ID: inv.ID}}
			if g.goods {
				g.line.Unit = invoice.KVUnit(l.Unit)
			}
			groups[key] = g
			order = append(order, key)
		}
		g.line.Base = g.line.Base.Add(l.Amount)
		if g.goods {
			if u := invoice.KVUnit(l.Unit); u != g.line.Unit {
				warn(inv.Number, "invoice", inv.ID, fmt.Sprintf("%s: rôzne merné jednotky (%s, %s) — v A.2 prepočítajte množstvo na jednu", k.Label(), g.line.Unit, u))
				continue
			}
			g.qty = g.qty.Add(l.Quantity)
		}
	}
	for _, key := range order {
		g := groups[key]
		if inv.Foreign() {
			g.line.Base = g.line.Base.DivCents(inv.FXRate)
		}
		g.line.Base = g.line.Base.RoundCents()
		if g.goods {
			q := g.qty.RoundCents()
			g.line.Quantity = &q
		}
		ret.KV.A2 = append(ret.KV.A2, g.line)
	}
}

// addPurchaseCredit zaradí dobropis od tuzemského platiteľa: zníženie
// odpočítanej dane do r. 28 (§ 53 ods. 1, kladná suma zvyšuje daň na úhradu)
// a riadky C.2 so zápornými rozdielmi. Obdobie = doručenie dobropisu.
// Vráti text upozornenia, ak dobropis nemožno zaradiť.
func (ret *Return) addPurchaseCredit(p purchase.Purchase, tax purchase.Amounts) string {
	s := p.Supplier
	switch {
	case p.NoDeduction:
		return "" // odpočet sa neuplatnil — nie je čo opravovať
	case foreign(s.Country) || p.ReverseCharge:
		return "Dobropis od zahraničného dodávateľa alebo pri prenesení daňovej povinnosti zatiaľ nie je podporovaný — doplňte ručne"
	case !isSKPayer(s.ICDPH):
		return "Dobropis od dodávateľa bez slovenského IČ DPH — opravu odpočtu doplňte ručne"
	case strings.TrimSpace(p.CorrectsNumber) == "":
		return "Dobropis bez čísla pôvodnej faktúry dodávateľa — v C.2 ho doplňte ručne"
	}
	for _, r := range tax.Rates {
		if r.Rate == 0 {
			continue
		}
		if _, ok := deductRows[r.Rate]; !ok {
			return fmt.Sprintf("Sadzba %d %% nepatrí do tlačiva DPHv25", r.Rate)
		}
	}
	for _, r := range tax.Rates {
		if r.Rate == 0 || r.VAT.IsZero() && r.Base.IsZero() {
			continue
		}
		ret.Rows["r28"] = ret.Rows["r28"].Add(r.VAT.Neg())
		ret.KV.C2 = append(ret.KV.C2, KVCorrection{Partner: noSpaces(s.ICDPH), Name: s.Name, Number: noSpaces(p.SupplierNumber),
			Original: noSpaces(p.CorrectsNumber), BaseDiff: r.Base, VATDiff: r.VAT, Rate: r.Rate, DeductedDiff: r.VAT, ID: p.ID})
	}
	return ""
}

// addCash zaradí pokladničné doklady s DPH: tržby z e-kasy (daň na výstupe,
// D.1) a pokladničné bloky (odpočet, B.3.1 alebo B.3.2 podľa limitu).
func (ret *Return) addCash(in Input, add func(string, money.Dec), warn func(string, string, int64, string)) {
	b3 := map[string]*KVB3{}
	var order []string
	var total KVB3
	for _, d := range in.CashDocs {
		if d.Cancelled || d.Tax == nil || d.LinkKind != "" || !in.Period.Contains(d.Date) {
			continue
		}
		if d.Kind == cash.In {
			ret.Cash++
			for _, r := range d.Tax.Rows {
				if r.Rate == 0 {
					add("r13", r.Base)
					continue
				}
				rows, ok := outputRows[r.Rate]
				if !ok {
					warn(d.Number, "cash", d.ID, fmt.Sprintf("Sadzba %d %% nepatrí do tlačiva DPHv25", r.Rate))
					continue
				}
				add(rows.base, r.Base)
				add(rows.tax, r.VAT)
				if reduced(r.Rate) {
					ret.KV.D1.BaseReduced = ret.KV.D1.BaseReduced.Add(r.Base)
					ret.KV.D1.VATReduced = ret.KV.D1.VATReduced.Add(r.VAT)
				} else {
					ret.KV.D1.Base = ret.KV.D1.Base.Add(r.Base)
					ret.KV.D1.VAT = ret.KV.D1.VAT.Add(r.VAT)
				}
			}
			ret.KV.D1.Count++
			continue
		}
		if d.Tax.NoDeduction {
			continue // bez odpočtu: ani v KV
		}
		partner := d.Tax.SupplierICDPH
		if !isSKPayer(partner) {
			warn(d.Number, "cash", d.ID, "Pokladničný blok bez IČ DPH predajcu — DPH nemožno odpočítať")
			continue
		}
		var base, tax money.Dec
		for _, r := range d.Tax.Rows {
			if r.Rate == 0 {
				continue
			}
			row, ok := deductRows[r.Rate]
			if !ok {
				warn(d.Number, "cash", d.ID, fmt.Sprintf("Sadzba %d %% nepatrí do tlačiva DPHv25", r.Rate))
				continue
			}
			add(row, r.VAT)
			add(domesticRow[r.Rate], r.VAT)
			base, tax = base.Add(r.Base), tax.Add(r.VAT)
		}
		if tax.IsZero() {
			continue
		}
		ret.Cash++
		line := b3[partner]
		if line == nil {
			line = &KVB3{Partner: partner, Name: d.Partner}
			b3[partner] = line
			order = append(order, partner)
		}
		line.Base, line.VAT, line.Deducted, line.Count = line.Base.Add(base), line.VAT.Add(tax), line.Deducted.Add(tax), line.Count+1
		total.Base, total.VAT, total.Deducted, total.Count = total.Base.Add(base), total.VAT.Add(tax), total.Deducted.Add(tax), total.Count+1
	}
	if total.Count == 0 {
		return
	}
	// Štruktúru B.3 v XML zatiaľ nemáme overenú voči schéme KVDPHv25 —
	// súbor ju neobsahuje a účtovník ju doplní v eDane.
	part := "B.3.1"
	if total.Deducted.Cmp(B3Limit) < 0 {
		ret.KV.B31 = &total
	} else {
		part = "B.3.2"
		for _, p := range order {
			ret.KV.B32 = append(ret.KV.B32, *b3[p])
		}
	}
	warn("", "cash", 0, fmt.Sprintf("Pokladničné bloky (%d) patria do časti %s kontrolného výkazu, ktorú súbor XML zatiaľ neobsahuje — doplňte ju v eDane podľa prehľadu (základ %s €, daň %s €)",
		total.Count, part, total.Base.Display(), total.VAT.Display()))
}
