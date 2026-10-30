package chart

import (
	"fmt"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// DocKind je druh dokladu, pre ktorý sa predkontácia použije.
type DocKind string

const (
	DocSale     DocKind = "sale"     // vydaná faktúra
	DocPurchase DocKind = "purchase" // prijatá faktúra
	DocBank     DocKind = "bank"     // bankový výpis
	DocCash     DocKind = "cash"     // pokladničný doklad
	DocInternal DocKind = "internal" // interný doklad
)

// DocKinds sú druhy dokladov s popisom.
var DocKinds = []struct {
	Code  DocKind `json:"code"`
	Label string  `json:"label"`
}{
	{DocSale, "Vydaná faktúra"}, {DocPurchase, "Prijatá faktúra"}, {DocBank, "Banka"}, {DocCash, "Pokladňa"}, {DocInternal, "Interný doklad"},
}

func (k DocKind) valid() bool {
	for _, x := range DocKinds {
		if x.Code == k {
			return true
		}
	}
	return false
}

// Part je časť sumy dokladu, ktorú riadok predkontácie účtuje.
type Part string

const (
	PartTotal  Part = "total"  // suma dokladu (na úhradu)
	PartBase   Part = "base"   // základ dane (pri neplatiteľovi celá suma)
	PartVAT    Part = "vat"    // DPH na odpočet / na výstupe
	PartVATNon Part = "vatNon" // neodpočítaná DPH (súčasť nákladu)
	PartSelf   Part = "self"   // samozdanená DPH pri prenesení (MD aj D 343)
)

// Parts sú časti sumy s popisom.
var Parts = []struct {
	Code  Part   `json:"code"`
	Label string `json:"label"`
}{
	{PartTotal, "suma dokladu"}, {PartBase, "základ"}, {PartVAT, "DPH"}, {PartVATNon, "neodpočítaná DPH"}, {PartSelf, "samozdanenie DPH"},
}

func (p Part) valid() bool {
	for _, x := range Parts {
		if x.Code == p {
			return true
		}
	}
	return false
}

// Side je strana účtu.
type Side string

const (
	Debit  Side = "MD" // má dať
	Credit Side = "D"  // dal
)

// TemplateLine je riadok predkontácie: strana, účet a časť sumy.
type TemplateLine struct {
	Side    Side   `json:"side"`
	Account string `json:"account"`
	Part    Part   `json:"part"`
}

// Template je predkontácia — vzor zaúčtovania dokladu.
type Template struct {
	Code   string         `json:"code"`
	Name   string         `json:"name"`
	Doc    DocKind        `json:"doc"`
	Lines  []TemplateLine `json:"lines"`
	Active bool           `json:"active"`
	Note   string         `json:"note,omitempty"`
}

// Amounts sú sumy dokladu pre predkontáciu.
type Amounts struct {
	Total, Base, VAT, VATNon, Self money.Dec
}

func (a Amounts) part(p Part) money.Dec {
	switch p {
	case PartTotal:
		return a.Total
	case PartBase:
		return a.Base
	case PartVAT:
		return a.VAT
	case PartVATNon:
		return a.VATNon
	case PartSelf:
		return a.Self
	}
	return money.Zero
}

// Entry je zaúčtovanie na jeden účet.
type Entry struct {
	Account string    `json:"account"`
	Debit   money.Dec `json:"debit"`
	Credit  money.Dec `json:"credit"`
}

// Apply zaúčtuje sumy podľa predkontácie; nulové riadky vynechá, rovnaké
// účty na rovnakej strane zlúči.
//
// Ak predkontácia nemá riadok pre neodpočítanú DPH, pripočíta sa k základu
// (je súčasťou nákladu); samozdanenie bez riadkov sa neúčtuje.
func (t Template) Apply(a Amounts) []Entry {
	if !t.uses(PartVATNon) {
		a.Base, a.VATNon = a.Base.Add(a.VATNon), money.Zero
	}
	var out []Entry
	idx := map[string]int{}
	for _, l := range t.Lines {
		v := a.part(l.Part)
		if v.IsZero() {
			continue
		}
		key := string(l.Side) + "/" + l.Account
		i, ok := idx[key]
		if !ok {
			i = len(out)
			idx[key] = i
			out = append(out, Entry{Account: l.Account})
		}
		if l.Side == Debit {
			out[i].Debit = out[i].Debit.Add(v)
		} else {
			out[i].Credit = out[i].Credit.Add(v)
		}
	}
	return out
}

func (t Template) uses(p Part) bool {
	for _, l := range t.Lines {
		if l.Part == p {
			return true
		}
	}
	return false
}

// samples sú skúšobné sumy, pri ktorých musí byť predkontácia vyrovnaná:
// bežný doklad s čiastočným odpočtom, prenesenie a doklad bez DPH.
var samples = []Amounts{
	{Total: money.FromInt(123), Base: money.FromInt(100), VAT: money.FromInt(20), VATNon: money.FromInt(3)},
	{Total: money.FromInt(100), Base: money.FromInt(100), Self: money.FromInt(23)},
	{Total: money.FromInt(100), Base: money.FromInt(100)},
}

// Balanced vráti true, ak MD = D pre všetky skúšobné sumy.
func (t Template) Balanced() bool {
	for _, s := range samples {
		var d, c money.Dec
		for _, e := range t.Apply(s) {
			d, c = d.Add(e.Debit), c.Add(e.Credit)
		}
		if !d.Equal(c) {
			return false
		}
	}
	return true
}

// ValidateTemplate overí predkontáciu voči rozvrhu.
func (c Chart) ValidateTemplate(t *Template) error {
	var fe []invoice.FieldError
	bad := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	t.Code, t.Name, t.Note = normCode(t.Code), strings.TrimSpace(t.Name), strings.TrimSpace(t.Note)
	if m := codeErr(t.Code); m != "" {
		bad("code", m)
	}
	if t.Name == "" {
		bad("name", "Zadajte názov predkontácie")
	}
	if !t.Doc.valid() {
		bad("doc", "Vyberte druh dokladu")
	}
	if len(t.Lines) < 2 {
		bad("lines", "Predkontácia má aspoň dva riadky (MD a D)")
	}
	for i, l := range t.Lines {
		t.Lines[i].Account = NormalizeNumber(l.Account)
		f := func(s string) string { return fmt.Sprintf("lines[%d].%s", i, s) }
		if l.Side != Debit && l.Side != Credit {
			bad(f("side"), "Strana je MD alebo D")
		}
		if !l.Part.valid() {
			bad(f("part"), "Neznáma časť sumy")
		}
		acc, ok := c.Find(t.Lines[i].Account)
		switch {
		case !ok:
			bad(f("account"), "Účet "+Display(t.Lines[i].Account)+" nie je v rozvrhu")
		case !acc.Active:
			bad(f("account"), "Účet "+Display(acc.Number)+" je skrytý")
		}
	}
	if len(fe) == 0 && !t.Balanced() {
		bad("lines", "Predkontácia nie je vyrovnaná: strana MD sa musí rovnať strane D (suma = základ + DPH + neodpočítaná DPH; samozdanenie na oboch stranách)")
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// DefaultTemplates sú predvolené predkontácie (platiteľ DPH, rámcová osnova).
var DefaultTemplates = []Template{
	{Code: "FV-SLUZBY", Name: "Vydaná faktúra – služby", Doc: DocSale, Lines: []TemplateLine{
		{Debit, "311", PartTotal}, {Credit, "602", PartBase}, {Credit, "343", PartVAT}}},
	{Code: "FV-TOVAR", Name: "Vydaná faktúra – tovar", Doc: DocSale, Lines: []TemplateLine{
		{Debit, "311", PartTotal}, {Credit, "604", PartBase}, {Credit, "343", PartVAT}}},
	{Code: "FV-VYROBKY", Name: "Vydaná faktúra – vlastné výrobky", Doc: DocSale, Lines: []TemplateLine{
		{Debit, "311", PartTotal}, {Credit, "601", PartBase}, {Credit, "343", PartVAT}}},
	{Code: "FP-SLUZBY", Name: "Prijatá faktúra – služby", Doc: DocPurchase, Lines: []TemplateLine{
		{Debit, "518", PartBase}, {Debit, "518", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "321", PartTotal},
		{Debit, "343", PartSelf}, {Credit, "343", PartSelf}}},
	{Code: "FP-MATERIAL", Name: "Prijatá faktúra – materiál do spotreby", Doc: DocPurchase, Lines: []TemplateLine{
		{Debit, "501", PartBase}, {Debit, "501", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "321", PartTotal},
		{Debit, "343", PartSelf}, {Credit, "343", PartSelf}}},
	{Code: "FP-TOVAR", Name: "Prijatá faktúra – tovar na sklad", Doc: DocPurchase, Lines: []TemplateLine{
		{Debit, "131", PartBase}, {Debit, "131", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "321", PartTotal},
		{Debit, "343", PartSelf}, {Credit, "343", PartSelf}}},
	{Code: "FP-ENERGIA", Name: "Prijatá faktúra – energie", Doc: DocPurchase, Lines: []TemplateLine{
		{Debit, "502", PartBase}, {Debit, "502", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "321", PartTotal}}},
	{Code: "FP-OPRAVY", Name: "Prijatá faktúra – opravy a údržba", Doc: DocPurchase, Lines: []TemplateLine{
		{Debit, "511", PartBase}, {Debit, "511", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "321", PartTotal}}},
	{Code: "FP-MAJETOK", Name: "Prijatá faktúra – obstaranie dlhodobého hmotného majetku", Doc: DocPurchase, Lines: []TemplateLine{
		{Debit, "042", PartBase}, {Debit, "042", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "321", PartTotal}}},
	{Code: "FP-REPRE", Name: "Prijatá faktúra – reprezentácia", Doc: DocPurchase, Lines: []TemplateLine{
		{Debit, "513", PartBase}, {Debit, "513", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "321", PartTotal}}},
	{Code: "B-ODBERATEL", Name: "Úhrada od odberateľa", Doc: DocBank, Lines: []TemplateLine{
		{Debit, "221", PartTotal}, {Credit, "311", PartTotal}}},
	{Code: "B-DODAVATEL", Name: "Úhrada dodávateľovi", Doc: DocBank, Lines: []TemplateLine{
		{Debit, "321", PartTotal}, {Credit, "221", PartTotal}}},
	{Code: "B-POPLATKY", Name: "Bankové poplatky", Doc: DocBank, Lines: []TemplateLine{
		{Debit, "568", PartTotal}, {Credit, "221", PartTotal}}},
	{Code: "B-UROKY", Name: "Prijaté úroky", Doc: DocBank, Lines: []TemplateLine{
		{Debit, "221", PartTotal}, {Credit, "662", PartTotal}}},
	{Code: "B-DPH", Name: "Úhrada DPH daňovému úradu", Doc: DocBank, Lines: []TemplateLine{
		{Debit, "343", PartTotal}, {Credit, "221", PartTotal}}},
	{Code: "B-VKLAD", Name: "Výber z banky do pokladne", Doc: DocBank, Lines: []TemplateLine{
		{Debit, "261", PartTotal}, {Credit, "221", PartTotal}}},
	{Code: "P-TRZBA", Name: "Pokladňa – tržba (e-kasa)", Doc: DocCash, Lines: []TemplateLine{
		{Debit, "211", PartTotal}, {Credit, "602", PartBase}, {Credit, "343", PartVAT}}},
	{Code: "P-NAKUP", Name: "Pokladňa – nákup (blok)", Doc: DocCash, Lines: []TemplateLine{
		{Debit, "501", PartBase}, {Debit, "501", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "211", PartTotal}}},
	{Code: "P-PHM", Name: "Pokladňa – pohonné hmoty", Doc: DocCash, Lines: []TemplateLine{
		{Debit, "501", PartBase}, {Debit, "501", PartVATNon}, {Debit, "343", PartVAT}, {Credit, "211", PartTotal}}},
	{Code: "P-ODBERATEL", Name: "Pokladňa – úhrada faktúry v hotovosti", Doc: DocCash, Lines: []TemplateLine{
		{Debit, "211", PartTotal}, {Credit, "311", PartTotal}}},
	{Code: "P-VKLAD", Name: "Pokladňa – príjem z banky", Doc: DocCash, Lines: []TemplateLine{
		{Debit, "211", PartTotal}, {Credit, "261", PartTotal}}},
	{Code: "ID-ODPISY", Name: "Odpisy dlhodobého hmotného majetku", Doc: DocInternal, Lines: []TemplateLine{
		{Debit, "551", PartTotal}, {Credit, "082", PartTotal}}},
	{Code: "ID-ZARADENIE", Name: "Zaradenie majetku do používania", Doc: DocInternal, Lines: []TemplateLine{
		{Debit, "022", PartTotal}, {Credit, "042", PartTotal}}},
}

func init() {
	for i := range DefaultTemplates {
		DefaultTemplates[i].Active = true
	}
}

// SeedTemplates doplní predvolené predkontácie, ktoré chýbajú (podľa kódu)
// a ktoré nie sú v zozname zmazaných. Vráti počet doplnených.
func SeedTemplates(list *[]Template, deleted []string) int {
	have := map[string]bool{}
	for _, t := range *list {
		have[t.Code] = true
	}
	for _, d := range deleted {
		have[d] = true
	}
	n := 0
	for _, t := range DefaultTemplates {
		if !have[t.Code] {
			t.Lines = append([]TemplateLine(nil), t.Lines...)
			*list = append(*list, t)
			n++
		}
	}
	SortTemplates(*list)
	return n
}

// SortTemplates zoradí predkontácie podľa druhu dokladu a kódu.
func SortTemplates(list []Template) {
	order := map[DocKind]int{}
	for i, k := range DocKinds {
		order[k.Code] = i
	}
	sort.SliceStable(list, func(i, j int) bool {
		if order[list[i].Doc] != order[list[j].Doc] {
			return order[list[i].Doc] < order[list[j].Doc]
		}
		return list[i].Code < list[j].Code
	})
}
