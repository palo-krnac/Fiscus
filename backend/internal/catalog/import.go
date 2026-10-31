package catalog

import (
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Stĺpce importu: hlavičky sa porovnávajú bez diakritiky a veľkosti písmen.
// Rovnaké názvy ako export cenníka, aby sa dal exportovaný súbor naspäť načítať.
const (
	colCode = iota
	colName
	colUnit
	colNet
	colGross
	colRate
	colMode
	colArchived
	colNote
	colEAN
	colCategory
	colBrand
	colSupplier
	colSupplierCode
	colPurchase
	colWeight
	colParams
	colDesc
	colStock // iba v exporte (stav sa mení skladovými dokladmi), import ho ignoruje
)

var headers = map[string]int{
	"kod": colCode, "code": colCode, "cislo": colCode,
	"nazov": colName, "nazov polozky": colName, "polozka": colName, "name": colName,
	"mj": colUnit, "jednotka": colUnit, "merna jednotka": colUnit, "unit": colUnit,
	"cena": colNet, "cena bez dph": colNet, "jednotkova cena": colNet, "price": colNet,
	"cena s dph": colGross,
	"dph":        colRate, "dph (%)": colRate, "dph %": colRate, "sadzba": colRate, "sadzba dph": colRate, "vat": colRate,
	"zadana cena": colMode,
	"archivovana": colArchived,
	"poznamka":    colNote, "note": colNote,
	"ean": colEAN, "ciarovy kod": colEAN, "gtin": colEAN, "barcode": colEAN,
	"kategoria": colCategory, "category": colCategory,
	"znacka": colBrand, "vyrobca": colBrand, "brand": colBrand,
	"dodavatel": colSupplier, "supplier": colSupplier,
	"kod u dodavatela": colSupplierCode, "kod dodavatela": colSupplierCode,
	"nakupna cena": colPurchase, "purchase price": colPurchase,
	"hmotnost (kg)": colWeight, "hmotnost": colWeight, "weight": colWeight,
	"parametre": colParams, "params": colParams,
	"stav skladu": colStock,
	// „Popis“ je popis produktu; bez stĺpca Názov ho staršie programy používajú ako názov
	"popis": colDesc, "description": colDesc,
}

// ParseParams prečíta parametre v tvare z exportu: „Napätie: 24 V; Krytie: IP20“.
// Jednotka je posledné slovo hodnoty, ak hodnota začína číslom („24 V“).
func ParseParams(s string) []Param {
	var out []Param
	for _, part := range strings.Split(s, ";") {
		name, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		p := Param{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)}
		if f := strings.Fields(p.Value); len(f) == 2 && strings.ContainsAny(f[0][:1], "0123456789-") {
			p.Value, p.Unit = f[0], f[1]
		}
		out = append(out, p)
	}
	return out
}

var fold = strings.NewReplacer(
	"á", "a", "ä", "a", "č", "c", "ď", "d", "é", "e", "ě", "e", "í", "i", "ĺ", "l", "ľ", "l", "ň", "n",
	"ó", "o", "ô", "o", "ŕ", "r", "ř", "r", "š", "s", "ť", "t", "ú", "u", "ů", "u", "ý", "y", "ž", "z",
)

// Key je kľúč na porovnanie textov (hlavičky, názvy položiek).
func Key(s string) string { return fold.Replace(strings.ToLower(strings.Join(strings.Fields(s), " "))) }

// ImportRow je riadok importu: položka a chyby (riadok s chybou sa preskočí).
type ImportRow struct {
	Line   int                  `json:"line"` // číslo riadku v súbore
	Item   Item                 `json:"item"`
	Action string               `json:"action"` // create | update | skip
	Errors []invoice.FieldError `json:"errors"`
	// cols sú stĺpce v súbore; ostatné údaje sa pri zmene ponechajú z cenníka.
	cols map[int]bool
}

// parseNumber prečíta číslo v slovenskom aj anglickom zápise: „1 234,50“, „1234.5“, „23 %“, „12,50 €“.
func parseNumber(s string) (money.Dec, error) {
	s = strings.NewReplacer(" ", "", " ", "", " ", "", "%", "", "€", "", "EUR", "").Replace(strings.TrimSpace(s))
	// desatinný oddeľovač je posledná čiarka alebo bodka („1.234,50“ aj „1,234.50“)
	if c, d := strings.LastIndex(s, ","), strings.LastIndex(s, "."); c > d {
		s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	} else if c >= 0 {
		s = strings.ReplaceAll(s, ",", "")
	}
	return money.Parse(s)
}

// ParseItems prevedie tabuľku (prvý riadok = hlavička) na položky cenníka.
// defaultRate je sadzba pre riadky bez stĺpca DPH.
func ParseItems(rows [][]string, defaultRate vat.Rate) ([]ImportRow, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("súbor je prázdny")
	}
	cols := map[int]int{}
	have := map[int]bool{}
	for i, h := range rows[0] {
		if c, ok := headers[Key(h)]; ok && !have[c] {
			cols[i], have[c] = c, true
		}
	}
	if !have[colName] && have[colDesc] {
		// „Popis“ bez „Názvu“ je názov položky (export starších programov)
		for i, c := range cols {
			if c == colDesc {
				cols[i] = colName
			}
		}
		have[colName], have[colDesc] = true, false
	}
	if !have[colName] {
		return nil, fmt.Errorf("v prvom riadku chýba stĺpec „Názov“ (hlavička: Kód; Názov; MJ; Cena bez DPH; DPH (%%); Cena s DPH; Poznámka)")
	}
	if !have[colNet] && !have[colGross] {
		return nil, fmt.Errorf("v prvom riadku chýba stĺpec „Cena bez DPH“ alebo „Cena s DPH“")
	}
	var out []ImportRow
	for li, r := range rows[1:] {
		row := ImportRow{Line: li + 2, Action: "create", cols: have}
		cell := map[int]string{}
		for i, v := range r {
			if c, ok := cols[i]; ok {
				cell[c] = v
			}
		}
		add := func(f, m string) { row.Errors = append(row.Errors, invoice.FieldError{Field: f, Message: m}) }
		it := Item{Code: cell[colCode], Name: cell[colName], Unit: cell[colUnit], Note: cell[colNote], Rate: defaultRate}
		if v := cell[colRate]; v != "" {
			if d, err := parseNumber(v); err != nil || !d.Equal(d.RoundTo(0)) {
				add("vatRate", "Neplatná sadzba DPH „"+v+"“")
			} else {
				it.Rate = vat.Rate(d.Raw() / 10000)
			}
		}
		net, gross := cell[colNet], cell[colGross]
		wantGross := strings.Contains(Key(cell[colMode]), "s dph") || (net == "" && gross != "")
		price := net
		it.PriceMode = vat.PriceNet
		if wantGross && gross != "" {
			price, it.PriceMode = gross, vat.PriceGross
		}
		if price == "" {
			add("price", "Chýba cena")
		} else if d, err := parseNumber(price); err != nil {
			add("price", "Neplatná cena „"+price+"“")
		} else if d.Sign() < 0 {
			add("price", "Cena nesmie byť záporná")
		} else {
			it.Price = d
		}
		it.Archived = Key(cell[colArchived]) == "ano"
		it.EAN, it.Category, it.Brand = cell[colEAN], cell[colCategory], cell[colBrand]
		it.Supplier, it.SupplierCode, it.Description = cell[colSupplier], cell[colSupplierCode], cell[colDesc]
		it.Params = ParseParams(cell[colParams])
		for _, n := range []struct {
			col   int
			field string
			label string
			dst   *money.Dec
		}{{colPurchase, "purchasePrice", "nákupná cena", &it.PurchasePrice}, {colWeight, "weight", "hmotnosť", &it.Weight}} {
			if v := cell[n.col]; v != "" {
				if d, err := parseNumber(v); err != nil {
					add(n.field, "Neplatná "+n.label+" „"+v+"“")
				} else {
					*n.dst = d
				}
			}
		}
		it.Normalize()
		if err := it.Validate(); err != nil {
			if ve, ok := err.(*invoice.ValidationError); ok {
				row.Errors = append(row.Errors, ve.Fields...)
			}
		}
		row.Item = it
		out = append(out, row)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("súbor nemá žiadne položky pod hlavičkou")
	}
	return out, nil
}

// Match priradí riadkom existujúce položky: podľa kódu, bez kódu podľa
// názvu. Rovnaký kód dvakrát v súbore je chyba. Riadky s chybou sa preskočia.
func Match(rows []ImportRow, existing []Item) {
	byCode := map[string]Item{}
	byName := map[string]Item{}
	for _, it := range existing {
		if it.Code != "" {
			byCode[Key(it.Code)] = it
		}
		byName[Key(it.Name)] = it
	}
	seen := map[string]int{}
	for i := range rows {
		r := &rows[i]
		if k := Key(r.Item.Code); k != "" {
			if line, dup := seen[k]; dup {
				r.Errors = append(r.Errors, invoice.FieldError{Field: "code", Message: fmt.Sprintf("Kód %s je už v riadku %d", r.Item.Code, line)})
			}
			seen[k] = r.Line
		}
		if len(r.Errors) > 0 {
			r.Action = "skip"
			continue
		}
		var old Item
		var ok bool
		if r.Item.Code != "" {
			old, ok = byCode[Key(r.Item.Code)]
		} else {
			old, ok = byName[Key(r.Item.Name)]
			ok = ok && old.Code == ""
		}
		if ok {
			r.Action, r.Item.ID = "update", old.ID
			r.keep(old)
		}
	}
}

// keep doplní údaje, ktorých stĺpec v súbore nie je, z existujúcej položky
// (import s kódom a cenou nezmaže EAN, parametre ani popis).
func (r *ImportRow) keep(old Item) {
	it := &r.Item
	for _, f := range []struct {
		col int
		do  func()
	}{
		{colUnit, func() { it.Unit = old.Unit }},
		{colRate, func() { it.Rate = old.Rate }},
		{colArchived, func() { it.Archived = old.Archived }},
		{colNote, func() { it.Note = old.Note }},
		{colEAN, func() { it.EAN = old.EAN }},
		{colCategory, func() { it.Category = old.Category }},
		{colBrand, func() { it.Brand = old.Brand }},
		{colSupplier, func() { it.Supplier = old.Supplier }},
		{colSupplierCode, func() { it.SupplierCode = old.SupplierCode }},
		{colPurchase, func() { it.PurchasePrice = old.PurchasePrice }},
		{colWeight, func() { it.Weight = old.Weight }},
		{colParams, func() { it.Params = append([]Param(nil), old.Params...) }},
		{colDesc, func() { it.Description = old.Description }},
	} {
		if !r.cols[f.col] {
			f.do()
		}
	}
}
