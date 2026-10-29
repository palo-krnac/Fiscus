package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
)

func TestJournal(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	// vydaná faktúra 492 € (400 + 92 DPH), uhradená bankou
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, err := a.Issue(ctx, c.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	// prijatá faktúra za materiál 100 € (81.30 + 18.70), uhradená bankou
	pu := validPurchase()
	pu.Expense = purchase.ExpenseStock
	p, _, err := a.SavePurchase(ctx, c.ID, pu)
	if err != nil {
		t.Fatal(err)
	}
	tx := func(fp string, dir bank.Direction, amount, vs string) bank.Transaction {
		return bank.Transaction{Account: "SK3112000000198742637541", Date: invoice.MustDate("2026-10-20"), Amount: m.MustParse(amount),
			Currency: "EUR", Direction: dir, VS: vs, CounterName: "Partner", Fingerprint: fp, Status: bank.StatusOpen}
	}
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{
		tx("a", bank.In, "492", inv.VariableSymbol), tx("b", bank.Out, p.Summary.Total.String(), p.VariableSymbol), tx("c", bank.Out, "216.12", ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.MatchTransaction(ctx, c.ID, txs[0].ID, "invoice", inv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.MatchTransaction(ctx, c.ID, txs[1].ID, "purchase", p.ID); err != nil {
		t.Fatal(err)
	}
	// pokladnica: vklad 500 € (nerozpísaný) a stornovaný doklad
	in, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-10-02"), Amount: m.MustParse("500"), Purpose: "Vklad"})
	if err != nil {
		t.Fatal(err)
	}
	x, _ := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.Out, Date: invoice.MustDate("2026-10-03"), Amount: m.MustParse("10"), Purpose: "Omyl"})
	_, _ = a.CancelCash(ctx, c.ID, x.ID)

	book, err := a.Journal(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Entries) != 4 || book.Entries[0].Key != "cash:"+itoa(int(in.ID)) {
		t.Fatalf("pohyby: %+v", book.Entries)
	}
	byKey := map[string]journal.Entry{}
	for _, e := range book.Entries {
		byKey[e.Key] = e
	}
	sale := byKey["bank:"+itoa(int(txs[0].ID))]
	if !sale.Auto || len(sale.Lines) != 2 || sale.Lines[0].Column != journal.IncServices || sale.Lines[0].Amount.String() != "400.00" || sale.Lines[1].Amount.String() != "92.00" {
		t.Fatalf("úhrada faktúry: %+v", sale)
	}
	buy := byKey["bank:"+itoa(int(txs[1].ID))]
	if len(buy.Lines) != 2 || buy.Lines[0].Column != journal.ExpStock || buy.Lines[0].Amount.String() != "81.30" || buy.Lines[1].Column != journal.ExpVAT {
		t.Fatalf("úhrada nákupu: %+v", buy)
	}
	if book.Totals.Open != 2 || book.Totals.Result.String() != "318.70" {
		t.Fatalf("súčty: %+v", book.Totals)
	}

	// ručný rozpis: odvod poistného z banky
	fee := "bank:" + itoa(int(txs[2].ID))
	if err := a.ClassifyJournal(ctx, c.ID, fee, []journal.Line{{Column: journal.ExpInsur, Amount: m.MustParse("200")}}, ""); err == nil {
		t.Fatal("rozpis nesedí so sumou")
	}
	if err := a.ClassifyJournal(ctx, c.ID, fee, []journal.Line{{Column: journal.ExpInsur, Amount: m.MustParse("216.12")}}, "Sociálna poisťovňa 10/2026"); err != nil {
		t.Fatal(err)
	}
	// vklad do pokladnice z banky ako priebežná položka
	if err := a.ClassifyJournal(ctx, c.ID, "cash:"+itoa(int(in.ID)), []journal.Line{{Column: journal.TransitIn, Amount: m.MustParse("500")}}, ""); err != nil {
		t.Fatal(err)
	}
	if err := a.ClassifyJournal(ctx, c.ID, "cash:"+itoa(int(x.ID)), []journal.Line{{Column: journal.ExpOther, Amount: m.MustParse("10")}}, ""); err == nil {
		t.Fatal("stornovaný doklad")
	}
	book, _ = a.Journal(ctx, c.ID, 2026)
	if book.Totals.Open != 0 || book.Totals.Result.String() != "102.58" || book.Totals.Columns[journal.TransitIn].String() != "500.00" {
		t.Fatalf("po rozpise: %+v", book.Totals)
	}
	// aj spárovaný pohyb sa dá rozpísať ručne a vrátiť na návrh
	key := "bank:" + itoa(int(txs[0].ID))
	_ = a.ClassifyJournal(ctx, c.ID, key, []journal.Line{{Column: journal.IncGoods, Amount: m.MustParse("400")}, {Column: journal.IncVAT, Amount: m.MustParse("92")}}, "tovar")
	book, _ = a.Journal(ctx, c.ID, 2026)
	for _, e := range book.Entries {
		if e.Key == key && (e.Auto || e.Lines[0].Column != journal.IncGoods || e.Note != "tovar") {
			t.Fatalf("ručný rozpis: %+v", e)
		}
	}
	if err := a.ClassifyJournal(ctx, c.ID, key, nil, ""); err != nil {
		t.Fatal(err)
	}
	book, _ = a.Journal(ctx, c.ID, 2026)
	for _, e := range book.Entries {
		if e.Key == key && !e.Auto {
			t.Fatal("návrat na návrh")
		}
	}
	if err := a.ClassifyJournal(ctx, c.ID, "bank:999", nil, ""); err == nil {
		t.Fatal("neexistujúci pohyb")
	}
	if b, _ := a.Journal(ctx, c.ID, 2025); len(b.Entries) != 0 {
		t.Fatal("iný rok")
	}
}

func TestJournalNonVATPayerAndPerms(t *testing.T) {
	o := newOffice(t)
	admin := as(o.admin)
	// Beta nie je platiteľ DPH: úhrada ide celá do príjmov
	draft := validDraft()
	draft.NonVATPayer = true
	d, err := o.a.SaveDraft(admin, o.beta.ID, draft)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := o.a.Issue(admin, o.beta.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	txs, _, _ := o.a.Store.SaveBankTransactions(ctx, o.beta.ID, []bank.Transaction{{Account: "SK3112000000198742637541",
		Date: invoice.MustDate("2026-10-20"), Amount: inv.Summary.Total, Currency: "EUR", Direction: bank.In, Fingerprint: "x", Status: bank.StatusOpen}})
	if _, err := o.a.MatchTransaction(admin, o.beta.ID, txs[0].ID, "invoice", inv.ID); err != nil {
		t.Fatal(err)
	}
	book, err := o.a.Journal(admin, o.beta.ID, 2026)
	if err != nil || book.VATPayer || len(book.Entries) != 1 || len(book.Entries[0].Lines) != 1 ||
		!book.Entries[0].Lines[0].Amount.Equal(inv.Summary.Total) {
		t.Fatalf("neplatiteľ: %+v %v", book.Entries, err)
	}
	// klientka Beta vidí, nemení; účtovníčka Alfy do Bety nevidí
	if _, err := o.a.Journal(as(o.klient), o.beta.ID, 2026); err != nil {
		t.Fatalf("klient číta: %v", err)
	}
	if err := o.a.ClassifyJournal(as(o.klient), o.beta.ID, "bank:"+itoa(int(txs[0].ID)), nil, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("klient mení: %v", err)
	}
	if _, err := o.a.Journal(as(o.jana), o.beta.ID, 2026); err == nil {
		t.Fatal("cudzia firma")
	}
}

func TestJournalBalancesAndOpenItems(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	acc := "SK3112000000198742637541"
	// pokladnica: vklad v minulom roku (počiatočný stav) a výdavok v tomto
	if _, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2025-12-20"), Amount: m.MustParse("300"), Purpose: "Vklad"}); err != nil {
		t.Fatal(err)
	}
	_, _ = a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.Out, Date: invoice.MustDate("2026-01-10"), Amount: m.MustParse("20"), Purpose: "Známky"})
	_, _, _ = a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{{Account: acc, Date: invoice.MustDate("2026-02-01"),
		Amount: m.MustParse("50"), Currency: "EUR", Direction: bank.In, Fingerprint: "z", Status: bank.StatusOpen}})
	if err := a.SetJournalOpening(ctx, c.ID, 2026, map[string]m.Dec{"sk31 1200 0000 1987 4263 7541": m.MustParse("1000.10")}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetJournalOpening(ctx, c.ID, 2026, map[string]m.Dec{acc: m.MustParse("1.001")}); err == nil {
		t.Fatal("zlomky centov")
	}
	book, err := a.Journal(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	var cashB, bankB journal.Balance
	for _, b := range book.Balances {
		if b.Means == journal.Cash {
			cashB = b
		} else if b.Account == acc {
			bankB = b
		}
	}
	if cashB.Opening.String() != "300.00" || cashB.Closing.String() != "280.00" || bankB.Opening.String() != "1000.10" || bankB.Closing.String() != "1050.10" {
		t.Fatalf("stavy: %+v", book.Balances)
	}

	// kniha pohľadávok a záväzkov
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, _ := a.Issue(ctx, c.ID, d.ID) // 492 €
	pu, _, err := a.SavePurchase(ctx, c.ID, validPurchase())
	if err != nil {
		t.Fatal(err)
	}
	items, err := a.OpenItemsAt(ctx, c.ID, invoice.MustDate("2026-12-31"))
	if err != nil || len(items.Receivables) != 1 || items.Receivables[0].ID != inv.ID || items.ReceivablesTotal.String() != "492.00" ||
		len(items.Payables) != 1 || items.Payables[0].ID != pu.ID {
		t.Fatalf("%+v %v", items, err)
	}
	_, _ = a.MarkPaid(ctx, c.ID, inv.ID, invoice.MustDate("2026-11-05"))
	if items, _ = a.OpenItemsAt(ctx, c.ID, invoice.MustDate("2026-12-31")); len(items.Receivables) != 0 {
		t.Fatal("uhradená")
	}
	if items, _ = a.OpenItemsAt(ctx, c.ID, invoice.MustDate("2026-11-01")); len(items.Receivables) != 1 {
		t.Fatal("k dňu pred úhradou")
	}
}
