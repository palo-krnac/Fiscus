package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Chyby banky.
var (
	ErrAlreadyMatched = errors.New("pohyb je už spárovaný — najprv zrušte spárovanie")
	ErrDocumentPaid   = errors.New("doklad už je uhradený")
	ErrWrongDirection = errors.New("príjem sa páruje s vydanou faktúrou, výdavok s prijatou faktúrou alebo dobropisom")
)

// ImportResult je výsledok importu výpisu.
type ImportResult struct {
	Account     string   `json:"account"`
	Imported    int      `json:"imported"`    // nové pohyby
	Duplicates  int      `json:"duplicates"`  // už importované skôr
	AutoMatched int      `json:"autoMatched"` // automaticky spárované
	Open        int      `json:"open"`        // nové nespárované
	Warnings    []string `json:"warnings"`
}

// ListBank vráti bankové pohyby firmy.
func (a *App) ListBank(ctx context.Context, companyID int64, f store.BankFilter) ([]bank.Transaction, error) {
	if err := a.Require(ctx, companyID, auth.PermBankRead); err != nil {
		return nil, err
	}
	return a.Store.ListBankTransactions(ctx, companyID, f)
}

// candidates sú neuhradené doklady firmy, ku ktorým môže platba patriť.
func (a *App) candidates(ctx context.Context, companyID int64) ([]bank.Candidate, error) {
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{Status: invoice.StatusIssued})
	if err != nil {
		return nil, err
	}
	var out []bank.Candidate
	for _, inv := range invs {
		if inv.Status != invoice.StatusIssued || (inv.Currency != "" && inv.Currency != "EUR") {
			continue
		}
		c := bank.Candidate{Kind: "invoice", ID: inv.ID, Number: inv.Number, VS: inv.VariableSymbol, Partner: inv.Customer.Name,
			Amount: inv.Summary.Total, Direction: bank.In, Due: inv.DueDate}
		if inv.Kind == invoice.KindCreditNote {
			// dobropis vraciame odberateľovi
			c.Amount, c.Direction = inv.Summary.Total.Neg(), bank.Out
		} else if inv.Kind != invoice.KindInvoice {
			continue
		}
		out = append(out, c)
	}
	purs, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
	if err != nil {
		return nil, err
	}
	for _, p := range purs {
		if p.Status != purchase.StatusBooked {
			continue
		}
		out = append(out, bank.Candidate{Kind: "purchase", ID: p.ID, Number: p.Number, VS: p.VariableSymbol, Partner: p.Supplier.Name,
			IBAN: p.SupplierIBAN, Amount: p.Summary.Total, Direction: bank.Out, Due: p.DueDate})
	}
	return out, nil
}

// ImportStatement načíta výpis camt.053, uloží nové pohyby a jednoznačné spáruje.
func (a *App) ImportStatement(ctx context.Context, companyID int64, fileName string, data []byte) (ImportResult, error) {
	res := ImportResult{Warnings: []string{}}
	if err := a.Require(ctx, companyID, auth.PermBankWrite); err != nil {
		return res, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return res, err
	}
	txs, err := bank.ParseCAMT053(data)
	if err != nil {
		return res, err
	}
	if len(txs) > 0 {
		res.Account = txs[0].Account
	}
	own := strings.ReplaceAll(c.IBAN, " ", "")
	if own != "" && res.Account != "" && !strings.EqualFold(own, res.Account) {
		res.Warnings = append(res.Warnings, "Výpis je z účtu "+res.Account+", v nastaveniach firmy je iný IBAN. Skontrolujte, či ide o správnu firmu.")
	}
	saved, dup, err := a.Store.SaveBankTransactions(ctx, companyID, txs)
	if err != nil {
		return res, err
	}
	res.Imported, res.Duplicates = len(saved), dup
	a.record(ctx, companyID, audit.CatBank, "import", "statement", 0,
		fmt.Sprintf("Import výpisu %s: %d nových pohybov, %d už importovaných", fileName, len(saved), dup), nil,
		map[string]any{"file": fileName, "account": res.Account, "imported": len(saved), "duplicates": dup})

	cands, err := a.candidates(ctx, companyID)
	if err != nil {
		return res, err
	}
	for _, t := range saved {
		if t.Currency != "" && t.Currency != "EUR" {
			res.Open++
			continue
		}
		cand, ok := bank.AutoMatch(t, cands)
		if !ok {
			res.Open++
			continue
		}
		if err := a.pair(ctx, companyID, &t, cand, true); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", cand.Number, err))
			res.Open++
			continue
		}
		res.AutoMatched++
		// uhradený doklad už nie je kandidát
		for i := range cands {
			if cands[i].Kind == cand.Kind && cands[i].ID == cand.ID {
				cands = append(cands[:i], cands[i+1:]...)
				break
			}
		}
	}
	return res, nil
}

// pair označí doklad ako uhradený dátumom platby a spáruje pohyb.
func (a *App) pair(ctx context.Context, companyID int64, t *bank.Transaction, c bank.Candidate, auto bool) error {
	var number string
	switch c.Kind {
	case "invoice":
		inv, err := a.Store.GetInvoice(ctx, companyID, c.ID)
		if err != nil {
			return err
		}
		if (inv.Kind == invoice.KindCreditNote) != (t.Direction == bank.Out) {
			return ErrWrongDirection
		}
		if inv.Status != invoice.StatusIssued {
			return ErrDocumentPaid
		}
		if _, err := a.MarkPaid(ctx, companyID, c.ID, t.Date); err != nil {
			return err
		}
		number = inv.Number
	case "purchase":
		p, err := a.Store.GetPurchase(ctx, companyID, c.ID)
		if err != nil {
			return err
		}
		if t.Direction != bank.Out {
			return ErrWrongDirection
		}
		if p.Status != purchase.StatusBooked {
			return ErrDocumentPaid
		}
		if _, err := a.MarkPurchasePaid(ctx, companyID, c.ID, t.Date); err != nil {
			return err
		}
		number = p.Number
	default:
		return fmt.Errorf("neznámy druh dokladu %q", c.Kind)
	}
	t.Status = bank.StatusMatched
	t.Match = &bank.Match{Kind: c.Kind, ID: c.ID, Number: number, Auto: auto, At: time.Now().UTC().Truncate(time.Second)}
	if err := a.Store.UpdateBankTransaction(ctx, companyID, *t); err != nil {
		return err
	}
	how := "Ručne spárovaná"
	if auto {
		how = "Automaticky spárovaná"
	}
	a.record(ctx, companyID, audit.CatBank, "match", "bank_tx", t.ID,
		fmt.Sprintf("%s platba %s € z %s s dokladom %s", how, t.Signed().Display(), t.Date.Display(), number), nil, t.Match)
	return nil
}

func (a *App) bankTx(ctx context.Context, companyID, id int64, perm auth.Perm) (bank.Transaction, error) {
	if err := a.Require(ctx, companyID, perm); err != nil {
		return bank.Transaction{}, err
	}
	return a.Store.GetBankTransaction(ctx, companyID, id)
}

// Suggestions vráti návrhy dokladov pre pohyb.
func (a *App) Suggestions(ctx context.Context, companyID, id int64) ([]bank.Suggestion, error) {
	t, err := a.bankTx(ctx, companyID, id, auth.PermBankRead)
	if err != nil {
		return nil, err
	}
	cands, err := a.candidates(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := bank.Suggest(t, cands)
	if out == nil {
		out = []bank.Suggestion{}
	}
	return out, nil
}

// OpenDocuments vráti neuhradené doklady pre ručné párovanie (podľa smeru platby).
func (a *App) OpenDocuments(ctx context.Context, companyID, id int64) ([]bank.Candidate, error) {
	t, err := a.bankTx(ctx, companyID, id, auth.PermBankRead)
	if err != nil {
		return nil, err
	}
	cands, err := a.candidates(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := []bank.Candidate{}
	for _, c := range cands {
		if c.Direction == t.Direction {
			out = append(out, c)
		}
	}
	return out, nil
}

// MatchTransaction ručne spáruje pohyb s dokladom.
func (a *App) MatchTransaction(ctx context.Context, companyID, id int64, kind string, docID int64) (bank.Transaction, error) {
	t, err := a.bankTx(ctx, companyID, id, auth.PermBankWrite)
	if err != nil {
		return t, err
	}
	if t.Status == bank.StatusMatched {
		return t, ErrAlreadyMatched
	}
	err = a.pair(ctx, companyID, &t, bank.Candidate{Kind: kind, ID: docID}, false)
	return t, err
}

// UnmatchTransaction zruší spárovanie a úhradu dokladu.
func (a *App) UnmatchTransaction(ctx context.Context, companyID, id int64) (bank.Transaction, error) {
	t, err := a.bankTx(ctx, companyID, id, auth.PermBankWrite)
	if err != nil {
		return t, err
	}
	if t.Status != bank.StatusMatched || t.Match == nil {
		return t, invoice.ErrBadTransition
	}
	m := *t.Match
	switch m.Kind {
	case "invoice":
		_, err = a.Unpay(ctx, companyID, m.ID)
	case "purchase":
		_, err = a.UnpayPurchase(ctx, companyID, m.ID)
	}
	// doklad mohol byť medzitým upravený alebo zmazaný — spárovanie zrušíme aj tak
	if err != nil && !errors.Is(err, invoice.ErrBadTransition) && !errors.Is(err, store.ErrNotFound) {
		return t, err
	}
	t.Status, t.Match = bank.StatusOpen, nil
	if err := a.Store.UpdateBankTransaction(ctx, companyID, t); err != nil {
		return t, err
	}
	a.record(ctx, companyID, audit.CatBank, "unmatch", "bank_tx", t.ID,
		fmt.Sprintf("Zrušené spárovanie platby %s € s dokladom %s", t.Signed().Display(), m.Number), m, nil)
	return t, nil
}

// IgnoreTransaction označí pohyb, ktorý sa nepáruje (poplatok, prevod), alebo ho vráti medzi nespárované.
func (a *App) IgnoreTransaction(ctx context.Context, companyID, id int64, ignore bool, note string) (bank.Transaction, error) {
	t, err := a.bankTx(ctx, companyID, id, auth.PermBankWrite)
	if err != nil {
		return t, err
	}
	if t.Status == bank.StatusMatched {
		return t, ErrAlreadyMatched
	}
	before := t.Status
	if ignore {
		t.Status = bank.StatusIgnored
	} else {
		t.Status = bank.StatusOpen
	}
	t.Note = strings.TrimSpace(note)
	if err := a.Store.UpdateBankTransaction(ctx, companyID, t); err != nil {
		return t, err
	}
	what := "Platba vrátená medzi nespárované"
	if ignore {
		what = "Platba označená ako nepárovaná"
	}
	a.record(ctx, companyID, audit.CatBank, "ignore", "bank_tx", t.ID,
		fmt.Sprintf("%s: %s € z %s %s", what, t.Signed().Display(), t.Date.Display(), t.Note), before, t.Status)
	return t, nil
}
