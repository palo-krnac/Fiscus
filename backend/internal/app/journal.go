package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
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
	docs := docCache{a: a, ctx: ctx, cid: companyID}
	for i := range entries {
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
	return JournalBook{Year: year, VATPayer: company.VATPayer, Columns: journal.Columns, Entries: entries, Totals: journal.Sum(entries)}, nil
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
	for _, d := range docs {
		if d.Cancelled {
			continue // stornovaný doklad: peniaze sa nepohli
		}
		e := journal.Entry{Key: "cash:" + strconv.FormatInt(d.ID, 10), Date: d.Date, Number: d.Number, Means: journal.Cash,
			Account: "Pokladnica", Amount: d.Signed(), Partner: d.Partner, Text: d.Purpose}
		if d.LinkKind != "" && d.LinkID != 0 {
			e.Doc = &journal.DocRef{Kind: d.LinkKind, ID: d.LinkID, Number: d.LinkNumber}
		}
		out = append(out, e)
	}
	return out, nil
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
		return nil
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
			Deduct: vatPayer && !p.NoDeduction})
	}
	return nil
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
		return journal.Entry{Key: key, Date: d.Date, Amount: d.Signed(), Means: journal.Cash}, nil
	}
	return journal.Entry{}, store.ErrNotFound
}

// ClassifyJournal uloží ručný rozpis pohybu (lines nil = vrátiť návrh Fiškusa).
func (a *App) ClassifyJournal(ctx context.Context, companyID int64, key string, lines []journal.Line, note string) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
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
