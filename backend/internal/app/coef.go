package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

// Pomerné odpočítanie dane (§ 50 zákona o DPH): nákup, ktorý slúži plneniam
// s nárokom na odpočet aj oslobodeným bez nároku (napr. nájom bytu), sa
// počas roka odpočítava predbežným koeficientom (spravidla ročný koeficient
// predchádzajúceho roka). Po skončení roka sa odpočet prepočíta ročným
// koeficientom a rozdiel sa vysporiada v poslednom zdaňovacom období roka
// (§ 50 ods. 4) v riadkoch 18, 18a a 19.

// CoefYear je nastavenie koeficientu na rok (nil = počíta program).
type CoefYear struct {
	Provisional *int   `json:"provisional"` // predbežný koeficient (%)
	Final       *int   `json:"final"`       // ročný koeficient zadaný ručne (napr. po vylúčení predaja majetku)
	Note        string `json:"note"`
}

// CoefView je prehľad koeficientu na rok.
type CoefView struct {
	Year int `json:"year"`
	// Provisional je predbežný koeficient roka; 0 pri ProvisionalOK = false.
	Provisional       int    `json:"provisional"`
	ProvisionalOK     bool   `json:"provisionalOk"`
	ProvisionalSource string `json:"provisionalSource"` // zadaný / ročný koeficient predchádzajúceho roka
	// Computed je koeficient z plnení roka podľa Fiškusa (doteraz).
	Computed int                `json:"computed"`
	Turnover vatreturn.Turnover `json:"turnover"`
	// Final je ročný koeficient: zadaný ručne, inak Computed.
	Final       int      `json:"final"`
	FinalManual bool     `json:"finalManual"`
	Settings    CoefYear `json:"settings"`
	// Docs je počet dokladov s pomerným odpočítaním v roku.
	Docs int `json:"docs"`
	// Adjustment je vysporiadanie za rok (§ 50 ods. 4) podľa ročného koeficientu.
	Adjustment *vatreturn.CoefAdjustment `json:"adjustment,omitempty"`
	// LastPeriod je posledné zdaňovacie obdobie roka (kde sa vysporiada).
	LastPeriod string `json:"lastPeriod"`
}

func coefKey(companyID int64) string { return fmt.Sprintf("vat.coef.%d", companyID) }

func (a *App) coefSettings(ctx context.Context, companyID int64) (map[string]CoefYear, error) {
	out := map[string]CoefYear{}
	raw, err := a.Store.GetSetting(ctx, coefKey(companyID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal([]byte(raw), &out)
}

// yearData sú doklady potrebné na výpočty za rok.
type yearData struct {
	invoices  []invoice.Invoice
	purchases []purchase.Purchase
	cash      []cash.Document
}

func (a *App) loadYear(ctx context.Context, companyID int64, year int) (yearData, error) {
	var d yearData
	for _, y := range []int{year, year + 1} {
		invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{Year: y})
		if err != nil {
			return d, err
		}
		d.invoices = append(d.invoices, invs...)
		ps, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{Year: y})
		if err != nil {
			return d, err
		}
		d.purchases = append(d.purchases, ps...)
	}
	cd, err := a.Store.ListCashDocuments(ctx, companyID, year)
	d.cash = cd
	return d, err
}

// yearPeriods vráti zdaňovacie obdobia roka podľa firmy.
func yearPeriods(c store.Company, year int) []vatreturn.Period {
	var out []vatreturn.Period
	if c.VATPeriod == store.VATPeriodQuarter {
		for q := 1; q <= 4; q++ {
			out = append(out, vatreturn.Period{Year: year, Quarter: q})
		}
		return out
	}
	for m := 1; m <= 12; m++ {
		out = append(out, vatreturn.Period{Year: year, Month: m})
	}
	return out
}

// turnover sčíta plnenia roka pre koeficient (§ 50 ods. 2).
func turnover(c store.Company, d yearData, year int) vatreturn.Turnover {
	var t vatreturn.Turnover
	for _, p := range yearPeriods(c, year) {
		r := vatreturn.Compute(vatreturn.Input{Period: p, Invoices: d.invoices, Purchases: d.purchases, CashDocs: d.cash})
		t = t.Add(r.Turnover)
	}
	return t
}

// provisionalCoef určí predbežný koeficient roka: zadaný, inak ročný
// koeficient predchádzajúceho roka (zadaný alebo vypočítaný z plnení).
func (a *App) provisionalCoef(ctx context.Context, c store.Company, year int, set map[string]CoefYear) (int, string, bool, error) {
	if y, ok := set[strconv.Itoa(year)]; ok && y.Provisional != nil {
		return *y.Provisional, "zadaný", true, nil
	}
	if y, ok := set[strconv.Itoa(year-1)]; ok && y.Final != nil {
		return *y.Final, fmt.Sprintf("ročný koeficient za %d (zadaný)", year-1), true, nil
	}
	d, err := a.loadYear(ctx, c.ID, year-1)
	if err != nil {
		return 0, "", false, err
	}
	t := turnover(c, d, year-1)
	if t.WithRight.Add(t.Exempt).Sign() <= 0 {
		return 0, "", false, nil
	}
	return vat.CoefPercent(t.WithRight, t.Exempt), fmt.Sprintf("ročný koeficient za %d z plnení vo Fiškuse", year-1), true, nil
}

// ErrNoCoef — chýba predbežný koeficient roka.
var ErrNoCoef = errors.New("chýba koeficient")

// stampCoef doplní do nákupu s pomerným odpočítaním predbežný koeficient roka.
func (a *App) stampCoef(ctx context.Context, c store.Company, field string, mixed bool, date invoice.Date, coef *int) error {
	if !mixed {
		*coef = 0
		return nil
	}
	set, err := a.coefSettings(ctx, c.ID)
	if err != nil {
		return err
	}
	pct, _, ok, err := a.provisionalCoef(ctx, c, date.Year(), set)
	if err != nil {
		return err
	}
	if !ok {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: field, Message: fmt.Sprintf(
			"Zadajte predbežný koeficient na rok %d (Priznanie DPH → Koeficient § 50) — v prvom roku kvalifikovaným odhadom", date.Year())}}}
	}
	*coef = pct
	return nil
}

// purchaseDate je deň, podľa ktorého nákup patrí do obdobia odpočtu.
func purchaseDate(p purchase.Purchase) invoice.Date {
	when := p.ReceivedDate
	if p.DeliveryDate.After(when) {
		when = p.DeliveryDate
	}
	return when
}

// coefAdjustment vypočíta vysporiadanie za rok: odpočet dokladov
// s pomerným odpočítaním ročným koeficientom mínus predbežný odpočet.
func coefAdjustment(d yearData, year, final int) *vatreturn.CoefAdjustment {
	adj := &vatreturn.CoefAdjustment{Year: year, Final: final, ByRate: map[vat.Rate]money.Dec{}}
	prov := -1
	note := func(c int) {
		switch {
		case prov == -1:
			prov = c
		case prov != c:
			prov = 0 // rôzne predbežné koeficienty
		}
	}
	for _, p := range d.purchases {
		if !p.Mixed || p.NoDeduction || purchaseDate(p).Year() != year {
			continue
		}
		tax, ok := p.Tax()
		if !ok {
			continue
		}
		adj.Docs++
		note(p.Coef)
		for _, r := range tax.Rates {
			if r.Rate == 0 || r.VAT.IsZero() {
				continue
			}
			diff := vat.DeductShare(r.VAT, p.DeductPercent, final).Sub(p.DeductOf(r.VAT))
			adj.ByRate[r.Rate] = adj.ByRate[r.Rate].Add(diff)
		}
	}
	for _, c := range d.cash {
		if c.Cancelled || c.Tax == nil || c.Kind != cash.Out || !c.Tax.Mixed || c.Tax.NoDeduction || c.LinkKind != "" || c.Date.Year() != year {
			continue
		}
		adj.Docs++
		note(c.Tax.Coef)
		for _, r := range c.Tax.Rows {
			if r.Rate == 0 || r.VAT.IsZero() {
				continue
			}
			diff := vat.DeductShare(r.VAT, c.Tax.DeductPercent, final).Sub(c.Tax.DeductOf(r.VAT))
			adj.ByRate[r.Rate] = adj.ByRate[r.Rate].Add(diff)
		}
	}
	for rate, v := range adj.ByRate {
		if v.IsZero() {
			delete(adj.ByRate, rate)
			continue
		}
		adj.Total = adj.Total.Add(v)
	}
	if prov > 0 {
		adj.Provisional = prov
	}
	return adj
}

// coefFor vráti vysporiadanie koeficientu, ak je obdobie posledné v roku.
func (a *App) coefFor(ctx context.Context, c store.Company, p vatreturn.Period) (*vatreturn.CoefAdjustment, error) {
	if p.End().Month() != 12 {
		return nil, nil
	}
	v, err := a.coefView(ctx, c, p.Year)
	if err != nil || v.Adjustment == nil || v.Adjustment.Total.IsZero() {
		return nil, err
	}
	return v.Adjustment, nil
}

func (a *App) coefView(ctx context.Context, c store.Company, year int) (CoefView, error) {
	set, err := a.coefSettings(ctx, c.ID)
	if err != nil {
		return CoefView{}, err
	}
	v := CoefView{Year: year, Settings: set[strconv.Itoa(year)]}
	periods := yearPeriods(c, year)
	v.LastPeriod = periods[len(periods)-1].String()
	if v.Provisional, v.ProvisionalSource, v.ProvisionalOK, err = a.provisionalCoef(ctx, c, year, set); err != nil {
		return v, err
	}
	d, err := a.loadYear(ctx, c.ID, year)
	if err != nil {
		return v, err
	}
	v.Turnover = turnover(c, d, year)
	v.Computed = vat.CoefPercent(v.Turnover.WithRight, v.Turnover.Exempt)
	v.Final = v.Computed
	if v.Settings.Final != nil {
		v.Final, v.FinalManual = *v.Settings.Final, true
	}
	adj := coefAdjustment(d, year, v.Final)
	v.Docs = adj.Docs
	if adj.Docs > 0 {
		v.Adjustment = adj
	}
	return v, nil
}

// VATCoefficient vráti koeficient pomerného odpočítania na rok.
func (a *App) VATCoefficient(ctx context.Context, companyID int64, year int) (CoefView, error) {
	if err := a.Require(ctx, companyID, auth.PermVATRead); err != nil {
		return CoefView{}, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return CoefView{}, err
	}
	if !c.VATPayer {
		return CoefView{}, ErrNotVATPayer
	}
	if year < 2000 || year > a.Today().Year()+1 {
		return CoefView{}, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "year", Message: "Neplatný rok"}}}
	}
	return a.coefView(ctx, c, year)
}

// SetVATCoefficient uloží predbežný a ročný koeficient. Pri zmene
// predbežného koeficientu sa prepočítajú doklady s pomerným odpočítaním
// v obdobiach roka, za ktoré ešte nie je zaznamenané podanie priznania.
func (a *App) SetVATCoefficient(ctx context.Context, companyID int64, year int, y CoefYear) (CoefView, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return CoefView{}, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return CoefView{}, err
	}
	if !c.VATPayer {
		return CoefView{}, ErrNotVATPayer
	}
	var fe []invoice.FieldError
	for f, v := range map[string]*int{"provisional": y.Provisional, "final": y.Final} {
		if v != nil && (*v < 0 || *v > 100) {
			fe = append(fe, invoice.FieldError{Field: f, Message: "Koeficient zadajte v percentách 0 až 100"})
		}
	}
	if year < 2000 || year > a.Today().Year()+1 {
		fe = append(fe, invoice.FieldError{Field: "year", Message: "Neplatný rok"})
	}
	if len(fe) > 0 {
		return CoefView{}, &invoice.ValidationError{Fields: fe}
	}
	a.coefMu.Lock()
	defer a.coefMu.Unlock()
	set, err := a.coefSettings(ctx, companyID)
	if err != nil {
		return CoefView{}, err
	}
	key := strconv.Itoa(year)
	before := set[key]
	set[key] = y
	b, _ := json.Marshal(set)
	if err := a.Store.SetSetting(ctx, coefKey(companyID), string(b)); err != nil {
		return CoefView{}, err
	}
	n, err := a.restampCoef(ctx, c, year, set)
	if err != nil {
		return CoefView{}, err
	}
	a.record(ctx, companyID, audit.CatTaxes, "coef", "vat", 0, fmt.Sprintf("Koeficient § 50 na rok %d: predbežný %s, ročný %s; prepočítané doklady: %d",
		year, pctText(y.Provisional), pctText(y.Final), n), before, y)
	return a.coefView(ctx, c, year)
}

func pctText(v *int) string {
	if v == nil {
		return "podľa programu"
	}
	return strconv.Itoa(*v) + " %"
}

// restampCoef prepočíta predbežný koeficient na dokladoch roka v obdobiach
// bez zaznamenaného podania. Vráti počet zmenených dokladov.
func (a *App) restampCoef(ctx context.Context, c store.Company, year int, set map[string]CoefYear) (int, error) {
	pct, _, ok, err := a.provisionalCoef(ctx, c, year, set)
	if err != nil || !ok {
		return 0, err
	}
	filed := map[string]bool{}
	isFiled := func(d invoice.Date) bool {
		p := vatreturn.PeriodOf(d, c.VATPeriod == store.VATPeriodQuarter).String()
		v, seen := filed[p]
		if !seen {
			raw, err := a.Store.GetSetting(ctx, filingKey(c.ID, p))
			v = err == nil && raw != "" && raw != "[]"
			filed[p] = v
		}
		return v
	}
	n := 0
	ps, err := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	if err != nil {
		return 0, err
	}
	for _, p := range ps {
		d := purchaseDate(p)
		if !p.Mixed || p.Coef == pct || d.Year() != year || isFiled(d) {
			continue
		}
		p.Coef = pct
		p.Normalize()
		if err := a.Store.SavePurchase(ctx, c.ID, &p); err != nil {
			return n, err
		}
		n++
	}
	docs, err := a.Store.ListCashDocuments(ctx, c.ID, year)
	if err != nil {
		return n, err
	}
	for _, d := range docs {
		if d.Tax == nil || !d.Tax.Mixed || d.Tax.Coef == pct || isFiled(d.Date) {
			continue
		}
		d.Tax.Coef = pct
		if err := a.Store.SaveCashDocument(ctx, c.ID, &d); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
