package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Peňažný denník sa skladá zo skutočných pohybov peňazí: bankové pohyby
// z výpisov a pokladničné doklady. Úhrady spárovaných faktúr rozpíše Fiškus
// sám (základ podľa druhu plnenia, DPH zvlášť); ostatné pohyby rozpíše
// používateľ. Ručný rozpis sa uloží ku zdroju a má prednosť pred návrhom.

// JournalBook je peňažný denník za rok.
type JournalBook struct {
	Year     int                  `json:"year"`
	VATPayer bool                 `json:"vatPayer"`
	Columns  []journal.ColumnInfo `json:"columns"`
	Entries  []journal.Entry      `json:"entries"`
	Totals   journal.Totals       `json:"totals"`
	// Balances sú stavy pokladnice a účtov: počiatočný stav, príjmy, výdavky
	// a konečný stav (§ 4 ods. 10 — záznamy nadväzujú na zostatky).
	Balances []journal.Balance `json:"balances"`
}

// Journal zostaví peňažný denník za rok.
func (a *App) Journal(ctx context.Context, companyID int64, year int) (JournalBook, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return JournalBook{}, err
	}
	if year == 0 {
		year = a.Today().Year()
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return JournalBook{}, err
	}
	entries, err := a.journalSources(ctx, companyID, year)
	if err != nil {
		return JournalBook{}, err
	}
	overrides, err := a.Store.ListJournalOverrides(ctx, companyID)
	if err != nil {
		return JournalBook{}, err
	}
	entries = append(entries, closingEntries(overrides, year)...)
	docs := docCache{a: a, ctx: ctx, cid: companyID}
	for i := range entries {
		if entries[i].Means == journal.Closing {
			continue
		}
		e := &entries[i]
		if o, ok := overrides[e.Key]; ok && journal.Check(e.Amount, o.Lines) == nil {
			e.Lines, e.Note = o.Lines, o.Note
			continue
		}
		e.Lines, e.Auto = docs.propose(*e, company.VATPayer), true
		if e.Lines == nil {
			e.Lines = []journal.Line{}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Date != entries[j].Date {
			return entries[i].Date.Before(entries[j].Date)
		}
		return entries[i].Key < entries[j].Key
	})
	opening, err := a.journalOpening(ctx, companyID, year)
	if err != nil {
		return JournalBook{}, err
	}
	regs, err := a.cashRegisters(ctx, companyID)
	if err != nil {
		return JournalBook{}, err
	}
	cashOpen, err := a.cashOpening(ctx, companyID, year, regs)
	if err != nil {
		return JournalBook{}, err
	}
	accounts := []string{}
	if iban := normIBAN(company.IBAN); iban != "" {
		accounts = append(accounts, iban)
	}
	return JournalBook{Year: year, VATPayer: company.VATPayer, Columns: journal.Columns, Entries: entries, Totals: journal.Sum(entries),
		Balances: journal.Balances(entries, cashOpen, opening, accounts)}, nil
}

func normIBAN(s string) string { return strings.ToUpper(strings.ReplaceAll(s, " ", "")) }

// journalOpeningKey je nastavenie s počiatočnými stavmi účtov firmy za rok.
func journalOpeningKey(companyID int64, year int) string {
	return fmt.Sprintf("journal.opening.%d.%d", companyID, year)
}

// journalOpening vráti počiatočné stavy bankových účtov (IBAN → suma).
func (a *App) journalOpening(ctx context.Context, companyID int64, year int) (map[string]money.Dec, error) {
	out := map[string]money.Dec{}
	v, err := a.Store.GetSetting(ctx, journalOpeningKey(companyID, year))
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// cashOpening vráti počiatočné stavy pokladníc v eurách: súčet dokladov pred
// rokom (pokladňa v cudzej mene podľa kurzov dokladov).
func (a *App) cashOpening(ctx context.Context, companyID int64, year int, regs []cash.Register) ([]journal.CashOpening, error) {
	docs, err := a.Store.ListCashDocuments(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	sums := map[int64]money.Dec{}
	used := map[int64]bool{}
	for _, d := range docs {
		used[d.RegisterID] = true
		if !d.Cancelled && d.Date.Year() < year {
			sums[d.RegisterID] = sums[d.RegisterID].Add(d.SignedEUR())
		}
	}
	out := []journal.CashOpening{}
	for _, r := range regs {
		if r.ID == 0 || used[r.ID] {
			out = append(out, journal.CashOpening{Account: r.Name, Amount: sums[r.ID]})
		}
	}
	return out, nil
}

// SetJournalOpening uloží počiatočné stavy bankových účtov za rok.
func (a *App) SetJournalOpening(ctx context.Context, companyID int64, year int, bank map[string]money.Dec) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	if year < 2000 || year > 2100 {
		return mfaFieldErr("year", "Neplatný rok")
	}
	clean := map[string]money.Dec{}
	for k, v := range bank {
		k = normIBAN(k)
		if k == "" {
			continue
		}
		if !v.Equal(v.RoundCents()) {
			return mfaFieldErr("bank", "Stav účtu "+k+" musí byť v centoch")
		}
		clean[k] = v
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	if err := a.Store.SetSetting(ctx, journalOpeningKey(companyID, year), string(b)); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "opening", "journal", 0, fmt.Sprintf("Počiatočné stavy účtov k 1. 1. %d", year), nil, clean)
	return nil
}

// journalSources načíta pohyby peňazí roka (bez rozpisu).
func (a *App) journalSources(ctx context.Context, companyID int64, year int) ([]journal.Entry, error) {
	txs, err := a.Store.ListBankTransactions(ctx, companyID, store.BankFilter{Year: year})
	if err != nil {
		return nil, err
	}
	out := make([]journal.Entry, 0, len(txs))
	for _, t := range txs {
		e := journal.Entry{Key: "bank:" + strconv.FormatInt(t.ID, 10), Date: t.Date, Number: t.BankRef, Means: journal.Bank,
			Account: t.Account, Amount: t.Signed(), Partner: t.CounterName, Text: strings.TrimSpace(t.Message)}
		if t.Status == bank.StatusMatched && t.Match != nil {
			e.Doc = &journal.DocRef{Kind: t.Match.Kind, ID: t.Match.ID, Number: t.Match.Number}
		}
		out = append(out, e)
	}
	docs, err := a.Store.ListCashDocuments(ctx, companyID, year)
	if err != nil {
		return nil, err
	}
	regs, err := a.cashRegisters(ctx, companyID)
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	for _, r := range regs {
		names[r.ID] = r.Name
	}
	for _, d := range docs {
		if d.Cancelled {
			continue // stornovaný doklad: peniaze sa nepohli
		}
		text := d.Purpose
		if d.Foreign() {
			text += fmt.Sprintf(" (%s %s, kurz %s)", d.Amount.Display(), d.Currency, d.FXRate.String())
		}
		e := journal.Entry{Key: "cash:" + strconv.FormatInt(d.ID, 10), Date: d.Date, Number: d.Number, Means: journal.Cash,
			Account: names[d.RegisterID], Amount: d.SignedEUR(), Partner: d.Partner, Text: text}
		if d.LinkKind != "" && d.LinkID != 0 {
			e.Doc = &journal.DocRef{Kind: d.LinkKind, ID: d.LinkID, Number: d.LinkNumber}
		}
		out = append(out, e)
	}
	return out, nil
}

// closingPrefix je začiatok kľúča uzávierkovej operácie: close:ROK:N.
func closingPrefix(year int) string { return "close:" + strconv.Itoa(year) + ":" }

// closingEntries vráti uzávierkové operácie roka (k 31. 12.).
func closingEntries(overrides map[string]store.JournalOverride, year int) []journal.Entry {
	var out []journal.Entry
	end := invoice.MustDate(fmt.Sprintf("%d-12-31", year))
	for k, o := range overrides {
		if strings.HasPrefix(k, closingPrefix(year)) {
			out = append(out, journal.Entry{Key: k, Date: end, Means: journal.Closing, Account: "Uzávierka", Number: "UZ",
				Text: o.Note, Lines: o.Lines})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// AddClosing pridá uzávierkovú operáciu k 31. 12. roka (napr. odpisy).
func (a *App) AddClosing(ctx context.Context, companyID int64, year int, text string, lines []journal.Line) (string, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return "", err
	}
	if year < 2000 || year > 2100 {
		return "", mfaFieldErr("year", "Neplatný rok")
	}
	overrides, err := a.Store.ListJournalOverrides(ctx, companyID)
	if err != nil {
		return "", err
	}
	n := 1
	for k := range overrides {
		if rest, ok := strings.CutPrefix(k, closingPrefix(year)); ok {
			if i, err := strconv.Atoi(rest); err == nil && i >= n {
				n = i + 1
			}
		}
	}
	key := fmt.Sprintf("%s%03d", closingPrefix(year), n)
	return key, a.saveClosing(ctx, companyID, key, text, lines)
}

func (a *App) saveClosing(ctx context.Context, companyID int64, key, text string, lines []journal.Line) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return mfaFieldErr("text", "Zadajte popis operácie (napr. Odpisy dlhodobého majetku)")
	}
	if err := journal.CheckClosing(lines); err != nil {
		return mfaFieldErr("lines", err.Error())
	}
	if err := a.Store.SaveJournalOverride(ctx, companyID, store.JournalOverride{Key: key, Lines: lines, Note: text}); err != nil {
		return err
	}
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		parts = append(parts, fmt.Sprintf("%s %s €", l.Column, l.Amount.Display()))
	}
	a.record(ctx, companyID, audit.CatLedger, "closing", "journal", 0, "Uzávierková operácia "+key+": "+text+" → "+strings.Join(parts, ", "), nil, lines)
	return nil
}

// docCache načíta spárované doklady raz.
type docCache struct {
	a    *App
	ctx  context.Context
	cid  int64
	invs map[int64]*invoice.Invoice
	purs map[int64]*purchase.Purchase
}

// propose navrhne rozpis pohybu podľa spárovaného dokladu (nil = nevie).
func (c *docCache) propose(e journal.Entry, vatPayer bool) []journal.Line {
	if e.Doc == nil {
		return c.proposeCash(e, vatPayer)
	}
	switch e.Doc.Kind {
	case "invoice":
		inv := c.invoice(e.Doc.ID)
		if inv == nil {
			return nil
		}
		sum, ok := inv.TaxSummary()
		if !ok {
			return nil
		}
		return journal.SaleLines(e.Amount, journal.Sale{Total: sum.Total, VAT: sum.VAT,
			Goods: inv.SupplyKind == invoice.SupplyGoods, VATPayer: vatPayer && !inv.NonVATPayer})
	case "purchase":
		p := c.purchase(e.Doc.ID)
		if p == nil {
			return nil
		}
		t, ok := p.Tax()
		if !ok {
			return nil
		}
		paidVAT := t.Summary.VAT
		if p.ReverseCharge || !t.Summary.Total.Equal(t.Summary.Base.Add(t.Summary.VAT)) {
			paidVAT = t.Summary.Total.Sub(t.Summary.Base) // dodávateľovi sa platí iba základ (prenesenie)
		}
		return journal.BuyLines(e.Amount, journal.Buy{Total: t.Summary.Total, VAT: paidVAT, Kind: p.Expense,
			Deduct: vatPayer && !p.NoDeduction, Percent: p.DeductPercent})
	}
	return nil
}

// proposeCash rozpíše pokladničný doklad s DPH (blok, tržba z e-kasy).
func (c *docCache) proposeCash(e journal.Entry, vatPayer bool) []journal.Line {
	id, ok := strings.CutPrefix(e.Key, "cash:")
	if !ok {
		return nil
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil
	}
	d, err := c.a.Store.GetCashDocument(c.ctx, c.cid, n)
	if err != nil || d.Tax == nil || len(d.Tax.Rows) == 0 {
		return nil
	}
	_, tax, gross := d.Tax.Totals()
	if d.Kind == cash.In {
		return journal.SaleLines(e.Amount, journal.Sale{Total: gross, VAT: tax, Goods: d.Tax.Goods, VATPayer: vatPayer})
	}
	return journal.BuyLines(e.Amount, journal.Buy{Total: gross, VAT: tax, Kind: d.Tax.Expense, Deduct: vatPayer && !d.Tax.NoDeduction, Percent: d.Tax.DeductPercent})
}

func (c *docCache) invoice(id int64) *invoice.Invoice {
	if c.invs == nil {
		c.invs = map[int64]*invoice.Invoice{}
	}
	if v, ok := c.invs[id]; ok {
		return v
	}
	inv, err := c.a.Store.GetInvoice(c.ctx, c.cid, id)
	var p *invoice.Invoice
	if err == nil {
		p = &inv
	}
	c.invs[id] = p
	return p
}

func (c *docCache) purchase(id int64) *purchase.Purchase {
	if c.purs == nil {
		c.purs = map[int64]*purchase.Purchase{}
	}
	if v, ok := c.purs[id]; ok {
		return v
	}
	pu, err := c.a.Store.GetPurchase(c.ctx, c.cid, id)
	var p *purchase.Purchase
	if err == nil {
		p = &pu
	}
	c.purs[id] = p
	return p
}

// journalEntry nájde pohyb podľa kľúča (bank:ID, cash:ID).
func (a *App) journalEntry(ctx context.Context, companyID int64, key string) (journal.Entry, error) {
	kind, idText, _ := strings.Cut(key, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return journal.Entry{}, store.ErrNotFound
	}
	switch kind {
	case "bank":
		t, err := a.Store.GetBankTransaction(ctx, companyID, id)
		if err != nil {
			return journal.Entry{}, err
		}
		return journal.Entry{Key: key, Date: t.Date, Amount: t.Signed(), Means: journal.Bank}, nil
	case "cash":
		d, err := a.Store.GetCashDocument(ctx, companyID, id)
		if err != nil {
			return journal.Entry{}, err
		}
		if d.Cancelled {
			return journal.Entry{}, store.ErrNotFound
		}
		return journal.Entry{Key: key, Date: d.Date, Amount: d.SignedEUR(), Means: journal.Cash}, nil
	}
	return journal.Entry{}, store.ErrNotFound
}

// ClassifyJournal uloží ručný rozpis pohybu (lines nil = vrátiť návrh Fiškusa).
func (a *App) ClassifyJournal(ctx context.Context, companyID int64, key string, lines []journal.Line, note string) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	if strings.HasPrefix(key, "close:") {
		overrides, err := a.Store.ListJournalOverrides(ctx, companyID)
		if err != nil {
			return err
		}
		if _, ok := overrides[key]; !ok {
			return store.ErrNotFound
		}
		if lines == nil {
			if err := a.Store.DeleteJournalOverride(ctx, companyID, key); err != nil {
				return err
			}
			a.record(ctx, companyID, audit.CatLedger, "closing", "journal", 0, "Zmazaná uzávierková operácia "+key, overrides[key], nil)
			return nil
		}
		return a.saveClosing(ctx, companyID, key, note, lines)
	}
	e, err := a.journalEntry(ctx, companyID, key)
	if err != nil {
		return err
	}
	if lines == nil {
		if err := a.Store.DeleteJournalOverride(ctx, companyID, key); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		a.record(ctx, companyID, audit.CatLedger, "classify", "journal", 0, "Peňažný denník: rozpis "+key+" vrátený na návrh Fiškusa", nil, nil)
		return nil
	}
	if err := journal.Check(e.Amount, lines); err != nil {
		return mfaFieldErr("lines", err.Error())
	}
	o := store.JournalOverride{Key: key, Lines: lines, Note: strings.TrimSpace(note)}
	if err := a.Store.SaveJournalOverride(ctx, companyID, o); err != nil {
		return err
	}
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		parts = append(parts, fmt.Sprintf("%s %s €", l.Column, l.Amount.Display()))
	}
	a.record(ctx, companyID, audit.CatLedger, "classify", "journal", 0,
		fmt.Sprintf("Peňažný denník: pohyb %s z %s (%s €) → %s", key, e.Date.Display(), e.Amount.Display(), strings.Join(parts, ", ")), nil, o)
	return nil
}

// OpenItems je kniha pohľadávok a záväzkov k dňu.
type OpenItems struct {
	Date             invoice.Date       `json:"date"`
	Receivables      []journal.OpenItem `json:"receivables"`
	Payables         []journal.OpenItem `json:"payables"`
	ReceivablesTotal money.Dec          `json:"receivablesTotal"` // v eurách
	PayablesTotal    money.Dec          `json:"payablesTotal"`
}

// OpenItemsAt vráti neuhradené pohľadávky a záväzky k dňu (prázdny = dnes).
func (a *App) OpenItemsAt(ctx context.Context, companyID int64, at invoice.Date) (OpenItems, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return OpenItems{}, err
	}
	if at.IsZero() {
		at = a.Today()
	}
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
	if err != nil {
		return OpenItems{}, err
	}
	purs, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
	if err != nil {
		return OpenItems{}, err
	}
	rec, pay := journal.Receivables(invs, at), journal.Payables(purs, at)
	sort.SliceStable(rec, func(i, j int) bool { return rec[i].DueDate.Before(rec[j].DueDate) })
	sort.SliceStable(pay, func(i, j int) bool { return pay[i].DueDate.Before(pay[j].DueDate) })
	return OpenItems{Date: at, Receivables: rec, Payables: pay, ReceivablesTotal: journal.SumEUR(rec), PayablesTotal: journal.SumEUR(pay)}, nil
}
