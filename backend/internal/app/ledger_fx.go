package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/chart"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/ledger"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Kurzové rozdiely k 31. 12.: neuhradené pohľadávky a záväzky v cudzej mene
// sa prepočítajú referenčným kurzom ECB ku dňu závierky (§ 24 ods. 2
// zákona o účtovníctve); rozdiel oproti sume v účtovníctve je kurzový zisk
// (663) alebo strata (563). Prepočítaná suma platí aj pri úhrade
// v nasledujúcom roku.

// FXItem je jedna prepočítaná pohľadávka alebo záväzok.
type FXItem struct {
	Key       string    `json:"key"`  // sale:ID | purchase:ID
	Kind      string    `json:"kind"` // receivable | payable
	Number    string    `json:"number"`
	Partner   string    `json:"partner"`
	Currency  string    `json:"currency"`
	Remaining money.Dec `json:"remaining"` // v cudzej mene
	Rate      money.Dec `json:"rate"`      // jednotiek meny za 1 EUR
	RateDate  string    `json:"rateDate"`
	BookEUR   money.Dec `json:"bookEur"` // v účtovníctve
	NewEUR    money.Dec `json:"newEur"`  // podľa kurzu k 31. 12.
	Diff      money.Dec `json:"diff"`    // + zisk, − strata
}

// FXRevaluation je návrh kurzových rozdielov k 31. 12.
type FXRevaluation struct {
	Year  int           `json:"year"`
	Items []FXItem      `json:"items"`
	Lines []ledger.Line `json:"lines"`
	// Missing sú meny bez kurzu k 31. 12. (treba aktualizovať kurzy).
	Missing []string `json:"missing"`
	// Posted je číslo interného dokladu, ak sú rozdiely zaúčtované.
	Posted string `json:"posted,omitempty"`
}

// FXRevaluationFor navrhne kurzové rozdiely k 31. 12. roka.
func (a *App) FXRevaluationFor(ctx context.Context, companyID int64, year int) (FXRevaluation, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return FXRevaluation{}, err
	}
	out := FXRevaluation{Year: year, Items: []FXItem{}, Lines: []ledger.Line{}, Missing: []string{}}
	if year < 2001 || year > 2100 {
		return out, mfaFieldErr("year", "Neplatný rok")
	}
	end := invoice.MustDate(fmt.Sprintf("%d-12-31", year))
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
	if err != nil {
		return out, err
	}
	purs, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
	if err != nil {
		return out, err
	}
	var prev map[string]money.Dec
	if d, err := a.loadLedger(ctx, companyID, year-1); err == nil {
		prev = d.Revalued
	}
	if d, err := a.loadLedger(ctx, companyID, year); err == nil {
		for _, doc := range d.Internal {
			if doc.FX {
				out.Posted = doc.Number
			}
		}
	}
	rates := map[string]FXItem{}
	missing := map[string]bool{}
	rateOf := func(cur string) (money.Dec, string, bool) {
		if r, ok := rates[cur]; ok {
			return r.Rate, r.RateDate, true
		}
		r, err := a.Store.RateBefore(ctx, cur, fmt.Sprintf("%d-01-01", year+1))
		if err != nil || r.Rate.Sign() <= 0 {
			missing[cur] = true
			return money.Zero, "", false
		}
		rates[cur] = FXItem{Rate: r.Rate, RateDate: r.Date}
		return r.Rate, r.Date, true
	}
	add := func(kind, key string, it journal.OpenItem) {
		if it.Currency == "EUR" || it.Remaining.IsZero() {
			return
		}
		rate, date, ok := rateOf(it.Currency)
		if !ok {
			return
		}
		book := it.RemainEUR
		if v, ok := prev[key]; ok {
			book = v
		}
		x := FXItem{Key: key, Kind: kind, Number: it.Number, Partner: it.Partner, Currency: it.Currency, Remaining: it.Remaining,
			Rate: rate, RateDate: date, BookEUR: book, NewEUR: it.Remaining.DivCents(rate)}
		x.Diff = x.NewEUR.Sub(x.BookEUR)
		if kind == "payable" {
			x.Diff = x.Diff.Neg() // vyšší záväzok je strata
		}
		if !x.Diff.IsZero() {
			out.Items = append(out.Items, x)
		}
	}
	var open []invoice.Invoice
	for _, inv := range invs {
		if inv.Kind != invoice.KindProforma && inv.Foreign() {
			open = append(open, inv)
		}
	}
	for _, it := range journal.Receivables(open, end) {
		add("receivable", "sale:"+strconv.FormatInt(it.ID, 10), it)
	}
	for _, it := range journal.Payables(purs, end) {
		add("payable", "purchase:"+strconv.FormatInt(it.ID, 10), it)
	}
	for c := range missing {
		out.Missing = append(out.Missing, c)
	}
	sort.Strings(out.Missing)
	out.Lines = fxRevaluationLines(out.Items)
	return out, nil
}

// fxRevaluationLines zaúčtuje rozdiely: zisk D 663, strata MD 563, proti
// účtu odberateľov (311) alebo dodávateľov (321). Rovnaké účty a strany zlúči.
func fxRevaluationLines(items []FXItem) []ledger.Line {
	type k struct {
		acc   string
		debit bool
	}
	sums := map[k]money.Dec{}
	var order []k
	put := func(acc string, debit bool, v money.Dec) {
		key := k{acc, debit}
		if _, ok := sums[key]; !ok {
			order = append(order, key)
		}
		sums[key] = sums[key].Add(v)
	}
	for _, it := range items {
		acc := "311"
		if it.Kind == "payable" {
			acc = "321"
		}
		gain := it.Diff.Sign() > 0
		v := it.Diff
		if !gain {
			v = v.Neg()
		}
		switch {
		case it.Kind == "receivable" && gain: // pohľadávka vzrástla
			put(acc, true, v)
			put("663", false, v)
		case it.Kind == "receivable":
			put("563", true, v)
			put(acc, false, v)
		case gain: // záväzok klesol
			put(acc, true, v)
			put("663", false, v)
		default:
			put("563", true, v)
			put(acc, false, v)
		}
	}
	out := []ledger.Line{}
	for _, key := range order {
		l := ledger.Line{Account: key.acc}
		if key.debit {
			l.Debit = sums[key]
		} else {
			l.Credit = sums[key]
		}
		out = append(out, l)
	}
	return out
}

// ErrNoFXDifferences — k 31. 12. nie sú kurzové rozdiely.
var ErrNoFXDifferences = errors.New("k 31. 12. nie sú žiadne kurzové rozdiely")

// PostFXRevaluation zaúčtuje kurzové rozdiely k 31. 12. ako interný doklad
// (predchádzajúci doklad s kurzovými rozdielmi toho roka nahradí).
func (a *App) PostFXRevaluation(ctx context.Context, companyID int64, year int) (InternalDoc, error) {
	rv, err := a.FXRevaluationFor(ctx, companyID, year)
	if err != nil {
		return InternalDoc{}, err
	}
	if len(rv.Missing) > 0 {
		return InternalDoc{}, mfaFieldErr("rates", "Chýba kurz ECB k 31. 12. pre "+fmt.Sprint(rv.Missing)+" — aktualizujte kurzy")
	}
	if len(rv.Items) == 0 {
		return InternalDoc{}, mfaFieldErr("items", ErrNoFXDifferences.Error())
	}
	var saved InternalDoc
	err = a.editLedger(ctx, companyID, year, func(d *LedgerData, c chart.Chart, _ []chart.Template) (string, error) {
		lines := append([]ledger.Line(nil), rv.Lines...)
		if err := ledger.CheckLines(lines, c); err != nil {
			return "", mfaFieldErr("lines", err.Error())
		}
		doc := InternalDoc{Date: invoice.MustDate(fmt.Sprintf("%d-12-31", year)), Text: fmt.Sprintf("Kurzové rozdiely k 31. 12. %d", year),
			Lines: lines, FX: true}
		for i, x := range d.Internal {
			if x.FX {
				doc.ID, doc.Number = x.ID, x.Number
				d.Internal[i] = doc
			}
		}
		if doc.ID == 0 {
			d.NextID++
			doc.ID = d.NextID
			doc.Number = fmt.Sprintf("ID%d%03d", year, doc.ID)
			d.Internal = append(d.Internal, doc)
		}
		d.Revalued = map[string]money.Dec{}
		for _, it := range rv.Items {
			d.Revalued[it.Key] = it.NewEUR
		}
		saved = doc
		return fmt.Sprintf("Kurzové rozdiely k 31. 12. %d (%d dokladov) %s", year, len(rv.Items), doc.Number), nil
	})
	return saved, err
}
