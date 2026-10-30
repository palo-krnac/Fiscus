package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/ledger"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// lineStr zapíše riadky ako „311 MD 492.00, 602 D 400.00“ na porovnanie.
func lineStr(ls []ledger.Line) string {
	s := ""
	for i, l := range ls {
		if i > 0 {
			s += ", "
		}
		if !l.Debit.IsZero() {
			s += l.Account + " MD " + l.Debit.String()
		} else {
			s += l.Account + " D " + l.Credit.String()
		}
	}
	return s
}

func TestLedger(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, err := a.Issue(ctx, c.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	pu := validPurchase()
	pu.Expense = purchase.ExpenseStock
	p, _, err := a.SavePurchase(ctx, c.ID, pu)
	if err != nil {
		t.Fatal(err)
	}
	// prenesenie daňovej povinnosti s 50 % odpočtom: 200 € + 46 € DPH
	rc := validPurchase()
	rc.SupplierNumber, rc.VariableSymbol = "RC-1", "777"
	rc.ReverseCharge, rc.DeductPercent = true, 50
	rc.Rates = []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("200")}}
	prc, _, err := a.SavePurchase(ctx, c.ID, rc)
	if err != nil {
		t.Fatal(err)
	}
	tx := func(fp string, dir bank.Direction, amount, msg string) bank.Transaction {
		return bank.Transaction{Account: "SK3112000000198742637541", Date: invoice.MustDate("2026-10-20"), Amount: m.MustParse(amount),
			Currency: "EUR", Direction: dir, Message: msg, CounterName: "Partner", Fingerprint: fp, Status: bank.StatusOpen}
	}
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{
		tx("a", bank.In, inv.Summary.Total.String(), ""), tx("b", bank.Out, p.Summary.Total.String(), ""),
		tx("c", bank.Out, "3.50", "Poplatok za vedenie účtu"), tx("d", bank.Out, "216.12", "Sociálna poisťovňa"),
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
	if _, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-10-02"), Amount: m.MustParse("500"), Purpose: "Výber z banky"}); err != nil {
		t.Fatal(err)
	}

	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ledger.Posting{}
	for _, ps := range book.Postings {
		by[ps.Key] = ps
	}
	key := func(kind string, id int64) string { return kind + ":" + itoa(int(id)) }
	cases := []struct{ key, tpl, lines string }{
		{key("sale", inv.ID), "FV-SLUZBY", "311 MD 492.00, 602 D 400.00, 343 D 92.00"},
		{key("purchase", p.ID), "FP-MATERIAL", "501 MD 81.30, 343 MD 18.70, 321 D 100.00"},
		{key("purchase", prc.ID), "FP-SLUZBY", "518 MD 200.00, 321 D 200.00, 343 MD 23.00, 343 D 23.00, 518 MD 23.00, 343 D 23.00"},
		{key("bank", txs[0].ID), "B-ODBERATEL", "221 MD 492.00, 311 D 492.00"},
		{key("bank", txs[1].ID), "B-DODAVATEL", "321 MD 100.00, 221 D 100.00"},
		{key("bank", txs[2].ID), "B-POPLATKY", "568 MD 3.50, 221 D 3.50"},
	}
	for _, tc := range cases {
		ps := by[tc.key]
		if ps.Template != tc.tpl || lineStr(ps.Lines) != tc.lines || ps.Problem != "" {
			t.Errorf("%s: %s [%s] %q", tc.key, ps.Template, lineStr(ps.Lines), ps.Problem)
		}
	}
	social := key("bank", txs[3].ID)
	if by[social].Problem == "" || book.Unposted != 1 {
		t.Fatalf("nezaúčtované: %d %+v", book.Unposted, by[social])
	}
	// pokladnica „Výber z banky“ je príjem → P-VKLAD
	for _, ps := range book.Postings {
		if ps.Doc == "cash" && (ps.Template != "P-VKLAD" || lineStr(ps.Lines) != "211 MD 500.00, 261 D 500.00") {
			t.Errorf("pokladnica: %+v", ps)
		}
	}

	// odvod poistného ručne, poplatok inou predkontáciou, chyby
	if err := a.SetPosting(ctx, c.ID, 2026, social, "", []ledger.Line{{Account: "336", Debit: m.MustParse("216.12")}, {Account: "221", Credit: m.MustParse("216")}}, ""); !fieldErr(err, "lines") {
		t.Errorf("nevyrovnané: %v", err)
	}
	if err := a.SetPosting(ctx, c.ID, 2026, social, "", []ledger.Line{{Account: "336", Debit: m.MustParse("216.12")}, {Account: "221", Credit: m.MustParse("216.12")}}, "SP 10/2026"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetPosting(ctx, c.ID, 2026, key("bank", txs[2].ID), "FV-TOVAR", nil, ""); !fieldErr(err, "template") {
		t.Errorf("predkontácia iného dokladu: %v", err)
	}
	// výdavok s predkontáciou príjmu: suma sa otočí (vrátenie úroku)
	if err := a.SetPosting(ctx, c.ID, 2026, key("bank", txs[2].ID), "B-UROKY", nil, ""); err != nil {
		t.Fatal(err)
	}
	// interný doklad: odpisy podľa predkontácie a ručný
	if _, err := a.SaveInternal(ctx, c.ID, 2026, InternalDoc{Date: invoice.MustDate("2026-12-31"), Text: "Odpisy", Template: "ID-ODPISY", Amount: m.MustParse("1200")}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveInternal(ctx, c.ID, 2026, InternalDoc{Date: invoice.MustDate("2025-12-31"), Text: "x", Template: "ID-ODPISY", Amount: m.MustParse("1")}); !fieldErr(err, "date") {
		t.Errorf("dátum mimo roka: %v", err)
	}
	if err := a.SetLedgerOpening(ctx, c.ID, 2026, []ledger.Line{{Account: "221", Debit: m.MustParse("1000")}, {Account: "411", Credit: m.MustParse("1000")}}); err != nil {
		t.Fatal(err)
	}

	book, err = a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	by = map[string]ledger.Posting{}
	for _, ps := range book.Postings {
		by[ps.Key] = ps
	}
	if ps := by[social]; !ps.Manual || ps.Note != "SP 10/2026" || ps.Problem != "" {
		t.Errorf("ručný zápis: %+v", ps)
	}
	if ps := by[key("bank", txs[2].ID)]; lineStr(ps.Lines) != "221 MD -3.50, 662 D -3.50" {
		t.Errorf("otočená suma: %s", lineStr(ps.Lines))
	}
	if ps := by["internal:1"]; ps.Number != "ID2026001" || lineStr(ps.Lines) != "551 MD 1200.00, 082 D 1200.00" {
		t.Errorf("interný doklad: %+v", ps)
	}
	if book.Unposted != 0 || !book.Totals.Balanced {
		t.Fatalf("predvaha: %d %+v", book.Unposted, book.Totals)
	}
	var b221 ledger.Turnover
	for _, s := range book.Synthetic {
		if s.Account == "221" {
			b221 = s
		}
	}
	// 1000 + 492 − 100 + 3.50 (vrátený poplatok ako záporný úrok: MD −3.50) − 216.12
	if b221.Opening.String() != "1000.00" || b221.Closing.String() != "1172.38" {
		t.Errorf("221: %+v", b221)
	}
	// späť na návrh Fiškusa
	if err := a.SetPosting(ctx, c.ID, 2026, social, "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if book, _ = a.Ledger(ctx, c.ID, 2026); book.Unposted != 1 {
		t.Errorf("po vrátení návrhu: %d", book.Unposted)
	}
	if err := a.DeleteInternal(ctx, c.ID, 2026, 1); err != nil {
		t.Fatal(err)
	}
}
