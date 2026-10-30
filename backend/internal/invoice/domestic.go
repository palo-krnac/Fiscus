package invoice

import (
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
)

// DomesticRC je druh tuzemského prenesenia daňovej povinnosti pri položke
// (§ 69 ods. 12 zákona o DPH) — daň z položky platí odberateľ (platiteľ DPH).
type DomesticRC string

const (
	RCScrap        DomesticRC = "scrap"        // a) kovový odpad a šrot
	RCEmissions    DomesticRC = "emissions"    // b) emisné kvóty
	RCProperty     DomesticRC = "property"     // c) nehnuteľnosť po 5 rokoch od kolaudácie
	RCForcedSale   DomesticRC = "forcedSale"   // d) nehnuteľnosť v nútenom predaji
	RCCollateral   DomesticRC = "collateral"   // e) tovar zo zabezpečenia pohľadávky
	RCCrops        DomesticRC = "crops"        // f) poľnohospodárske plodiny (kap. 10 a 12 SCS)
	RCMetals       DomesticRC = "metals"       // g) železo a oceľ (kap. 72, 7301, 7308, 7314)
	RCPhones       DomesticRC = "phones"       // h) mobilné telefóny
	RCChips        DomesticRC = "chips"        // i) integrované obvody
	RCConstruction DomesticRC = "construction" // j) stavebné práce (sekcia F)
)

// DomesticRCInfo je položka číselníka druhov prenesenia.
type DomesticRCInfo struct {
	Code   DomesticRC `json:"code"`
	Letter string     `json:"letter"`
	Label  string     `json:"label"`
}

// DomesticRCs je číselník druhov tuzemského prenesenia v poradí zákona.
var DomesticRCs = []DomesticRCInfo{
	{RCScrap, "a", "Kovový odpad a šrot"},
	{RCEmissions, "b", "Emisné kvóty"},
	{RCProperty, "c", "Nehnuteľnosť (po 5 rokoch od kolaudácie)"},
	{RCForcedSale, "d", "Nehnuteľnosť v nútenom predaji"},
	{RCCollateral, "e", "Tovar zo zabezpečenia pohľadávky"},
	{RCCrops, "f", "Poľnohospodárske plodiny"},
	{RCMetals, "g", "Železo a oceľ"},
	{RCPhones, "h", "Mobilné telefóny"},
	{RCChips, "i", "Integrované obvody"},
	{RCConstruction, "j", "Stavebné práce"},
}

// RCThreshold je hranica základu dane na faktúre pre písm. f) až i).
var RCThreshold = money.FromInt(5000)

func (k DomesticRC) info() (DomesticRCInfo, bool) {
	for _, i := range DomesticRCs {
		if i.Code == k {
			return i, true
		}
	}
	return DomesticRCInfo{}, false
}

// Valid vráti true pre prázdny (bežná DPH) alebo známy druh.
func (k DomesticRC) Valid() bool {
	_, ok := k.info()
	return k == "" || ok
}

// Letter je písmeno § 69 ods. 12.
func (k DomesticRC) Letter() string { i, _ := k.info(); return i.Letter }

// Label je názov druhu.
func (k DomesticRC) Label() string { i, _ := k.info(); return i.Label }

// Reported: dodávateľ ho uvádza v časti A.2 kontrolného výkazu (písm. f až j).
func (k DomesticRC) Reported() bool { return k.Letter() >= "f" }

// Goods: tovar s hranicou 5 000 €, množstvom a mernou jednotkou v KV (písm. f až i).
func (k DomesticRC) Goods() bool { l := k.Letter(); return l >= "f" && l <= "i" }

// NeedsCode: v KV sa uvádza 4-miestny kód Spoločného colného sadzobníka (písm. f, g).
func (k DomesticRC) NeedsCode() bool { return k == RCCrops || k == RCMetals }

// KVKind je skratka druhu tovaru v KV (MT, IO).
func (k DomesticRC) KVKind() string {
	switch k {
	case RCPhones:
		return "MT"
	case RCChips:
		return "IO"
	}
	return ""
}

// ValidCode overí kód colného sadzobníka pre plodiny a kovy.
func (k DomesticRC) ValidCode(code string) bool {
	c := CommodityCode(code)
	if len(c) != 4 {
		return false
	}
	switch k {
	case RCCrops:
		return strings.HasPrefix(c, "10") || strings.HasPrefix(c, "12")
	case RCMetals:
		return strings.HasPrefix(c, "72") || c == "7301" || c == "7308" || c == "7314"
	}
	return true
}

// CommodityCode vráti prvé 4 číslice kódu (napr. „7214 20 00“ → „7214“).
func CommodityCode(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() < 4 {
		return b.String()
	}
	return b.String()[:4]
}

// KVUnit prevedie mernú jednotku z faktúry na jednotku KV (t, kg, m, ks);
// m² a m³ sa uvádzajú ako „m“. Prázdny reťazec = jednotka sa nedá použiť.
func KVUnit(u string) string {
	switch strings.ToLower(strings.TrimSuffix(strings.TrimSpace(u), ".")) {
	case "t", "tona", "ton", "tony":
		return "t"
	case "kg":
		return "kg"
	case "m", "bm", "m2", "m²", "m3", "m³":
		return "m"
	case "ks", "kus", "kusy", "kusov":
		return "ks"
	}
	return ""
}

// DomesticReverse vráti true, ak má faktúra položky s tuzemským prenesením.
func (inv *Invoice) DomesticReverse() bool {
	for _, l := range inv.Lines {
		if l.Reverse != "" {
			return true
		}
	}
	return false
}

// AnyReverse: aspoň časť faktúry je bez DPH s prenesením na odberateľa.
func (inv *Invoice) AnyReverse() bool { return inv.ReverseCharge || inv.DomesticReverse() }

// validateDomesticRC skontroluje položky s tuzemským prenesením.
func (inv *Invoice) validateDomesticRC() []FieldError {
	var errs []FieldError
	add := func(f, m string) { errs = append(errs, FieldError{f, m}) }
	c := inv.Customer
	if inv.ReverseCharge && !party.Foreign(c.Country) {
		add("reverseCharge", "Pri odberateľovi v tuzemsku vyberte prenesenie pri položkách (§ 69 ods. 12)")
	}
	if !inv.DomesticReverse() {
		for i, l := range inv.Lines {
			if !l.Reverse.Valid() {
				add(fmt.Sprintf("lines[%d].reverse", i), "Neznámy druh prenesenia")
			}
		}
		return errs
	}
	switch {
	case party.Foreign(c.Country):
		add("customer.country", "Tuzemské prenesenie daňovej povinnosti sa uplatní len voči odberateľovi v tuzemsku")
	case inv.NonVATPayer:
		add("lines", "Neplatiteľ DPH neuplatňuje prenesenie daňovej povinnosti")
	case !strings.HasPrefix(party.NormalizeID(c.ICDPH), "SK"):
		add("customer.icdph", "Prenesenie daňovej povinnosti je len voči platiteľovi DPH — zadajte IČ DPH odberateľa")
	}
	base := map[DomesticRC]money.Dec{}
	first := map[DomesticRC]int{}
	for i, l := range inv.Lines {
		f := func(s string) string { return fmt.Sprintf("lines[%d].%s", i, s) }
		switch {
		case !l.Reverse.Valid():
			add(f("reverse"), "Neznámy druh prenesenia")
			continue
		case l.Reverse == "":
			continue
		}
		if l.Reverse.NeedsCode() && !l.Reverse.ValidCode(l.Commodity) {
			if l.Reverse == RCCrops {
				add(f("commodity"), "Zadajte kód colného sadzobníka plodiny (kapitola 10 alebo 12, napr. 1001)")
			} else {
				add(f("commodity"), "Zadajte kód colného sadzobníka (kapitola 72 alebo 7301, 7308, 7314)")
			}
		}
		if l.Reverse.Goods() && KVUnit(l.Unit) == "" && inv.Kind != KindAdvanceReceipt {
			add(f("unit"), "Pre kontrolný výkaz použite jednotku t, kg, m alebo ks")
		}
		if _, ok := first[l.Reverse]; !ok {
			first[l.Reverse] = i
		}
		base[l.Reverse] = base[l.Reverse].Add(l.Quantity.Mul(l.UnitPrice).RoundCents())
	}
	if inv.Kind == KindCreditNote || inv.Kind == KindAdvanceReceipt {
		// dobropis opravuje pôvodnú faktúru, doklad k platbe je časť celej
		// dodávky — hranica 5 000 € sa posudzuje na celom plnení (záloha)
		return errs
	}
	for _, info := range DomesticRCs {
		k := info.Code
		b, ok := base[k]
		if !ok || !k.Goods() {
			continue
		}
		if inv.Foreign() {
			if inv.FXRate.Sign() <= 0 {
				continue // bez kurzu nevieme posúdiť (chyba kurzu sa ukáže zvlášť)
			}
			b = b.DivCents(inv.FXRate)
		}
		if b.Cmp(RCThreshold) < 0 {
			add(fmt.Sprintf("lines[%d].reverse", first[k]), fmt.Sprintf(
				"%s: prenesenie platí, len ak je ich základ na faktúre aspoň 5 000 € (teraz %s €) — inak fakturujte s DPH", k.Label(), b.Display()))
		}
	}
	return errs
}
