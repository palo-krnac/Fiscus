package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/chart"
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
	if err := a.SetPosting(ctx, c.ID, 2026, social, "", []ledger.Line{{Account: "336", Debit: m.MustParse("216.12")}, {Account: "221", Credit: m.MustParse("216")}}, "", ledger.Dims{}); !fieldErr(err, "lines") {
		t.Errorf("nevyrovnané: %v", err)
	}
	if err := a.SetPosting(ctx, c.ID, 2026, social, "", []ledger.Line{{Account: "336", Debit: m.MustParse("216.12")}, {Account: "221", Credit: m.MustParse("216.12")}}, "SP 10/2026", ledger.Dims{}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetPosting(ctx, c.ID, 2026, key("bank", txs[2].ID), "FV-TOVAR", nil, "", ledger.Dims{}); !fieldErr(err, "template") {
		t.Errorf("predkontácia iného dokladu: %v", err)
	}
	// výdavok s predkontáciou príjmu: suma sa otočí (vrátenie úroku)
	if err := a.SetPosting(ctx, c.ID, 2026, key("bank", txs[2].ID), "B-UROKY", nil, "", ledger.Dims{}); err != nil {
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
	if err := a.SetPosting(ctx, c.ID, 2026, social, "", nil, "", ledger.Dims{}); err != nil {
		t.Fatal(err)
	}
	if book, _ = a.Ledger(ctx, c.ID, 2026); book.Unposted != 1 {
		t.Errorf("po vrátení návrhu: %d", book.Unposted)
	}
	if err := a.DeleteInternal(ctx, c.ID, 2026, 1); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerStatements(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	if _, err := a.Issue(ctx, c.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	st, err := a.LedgerStatementsFor(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	row := func(rs []ledger.StatementRow, code string) string {
		for _, r := range rs {
			if r.Code == code {
				return r.Netto.String()
			}
		}
		return "?"
	}
	if row(st.Income, "03") != "400.00" || row(st.Income, "38") != "400.00" || row(st.Assets, "18") != "492.00" ||
		row(st.Liabilities, "41") != "92.00" || row(st.Liabilities, "33") != "400.00" || !st.Difference.IsZero() || !st.PreviousEmpty || !st.Balanced {
		t.Fatalf("závierka: %+v", st)
	}
	lines, err := a.OpeningProposal(ctx, c.ID, 2027)
	if err != nil {
		t.Fatal(err)
	}
	if lineStr(lines) != "311 MD 492.00, 343 D 92.00, 431 D 400.00" {
		t.Errorf("počiatočné stavy 2027: %s", lineStr(lines))
	}
	if err := a.SetLedgerOpening(ctx, c.ID, 2027, lines); err != nil {
		t.Fatal(err)
	}
	if st, _ = a.LedgerStatementsFor(ctx, c.ID, 2027); row(st.Liabilities, "32") != "400.00" || row(st.Assets, "18") != "492.00" || row(st.Assets, "01") != row(st.Liabilities, "24") {
		t.Errorf("2027: %+v", st)
	}
}

func TestLedgerAdvance(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	dr, _ := a.SaveDraft(ctx, c.ID, proformaDraft())
	p, err := a.Issue(ctx, c.ID, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p, err = a.MarkPaid(ctx, c.ID, p.ID, invoice.MustDate("2026-10-10")); err != nil {
		t.Fatal(err)
	}
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.Lines = append(f.Lines, invoice.Line{Description: "Práca navyše", Quantity: m.MustParse("1"), Unit: "hod", UnitPrice: m.MustParse("100"), Rate: vat.Rate23})
	if f, err = a.SaveDraft(ctx, c.ID, f); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Issue(ctx, c.ID, f.ID); err != nil {
		t.Fatal(err)
	}
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ps := range book.Postings {
		if ps.Problem != "" {
			t.Errorf("nezaúčtované: %+v", ps)
		}
		got[ps.Number] = ps.Template + " " + lineStr(ps.Lines)
	}
	want := map[string]string{
		"ZD20260001": "FV-ZALOHA 324 MD 92.00, 343 D 92.00",
		f.Number:     "FV-SLUZBY 311 MD 615.00, 602 D 500.00, 343 D 115.00, 324 MD 400.00, 343 MD 92.00, 311 D 492.00",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: %s", k, got[k])
		}
	}
	if len(got) != 2 {
		t.Errorf("zálohová faktúra sa neúčtuje: %v", got)
	}
}

func TestLedgerMoneyAndDims(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if _, err := a.SaveAccount(ctx, c.ID, "", chart.Account{Number: "221001", Name: "Tatra banka", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveCenter(ctx, c.ID, "", chart.Center{Code: "BB", Name: "Banská Bystrica", Active: true}); err != nil {
		t.Fatal(err)
	}
	iban := "SK3112000000198742637541"
	if err := a.SetMoneyAccounts(ctx, c.ID, map[string]string{"bank:" + iban: "211"}); !fieldErr(err, "money") {
		t.Errorf("pokladnica pre banku: %v", err)
	}
	if err := a.SetMoneyAccounts(ctx, c.ID, map[string]string{"bank:" + iban: "221.001"}); err != nil {
		t.Fatal(err)
	}
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{{Account: iban, Date: invoice.MustDate("2026-10-20"),
		Amount: m.MustParse("3.50"), Currency: "EUR", Direction: bank.Out, Message: "Poplatok", Fingerprint: "x", Status: bank.StatusOpen}})
	if err != nil {
		t.Fatal(err)
	}
	key := "bank:" + itoa(int(txs[0].ID))
	if err := a.SetPosting(ctx, c.ID, 2026, key, "", nil, "", ledger.Dims{Center: "XX"}); !fieldErr(err, "center") {
		t.Errorf("neznáme stredisko: %v", err)
	}
	if err := a.SetPosting(ctx, c.ID, 2026, key, "", nil, "", ledger.Dims{Center: "BB"}); err != nil {
		t.Fatal(err)
	}
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	ps := book.Postings[0]
	if lineStr(ps.Lines) != "568 MD 3.50, 221001 D 3.50" || ps.Lines[0].Center != "BB" || ps.Lines[1].Center != "" {
		t.Fatalf("zápis: %s %+v", lineStr(ps.Lines), ps.Lines)
	}
	if len(book.DimTurnovers) != 1 || book.DimTurnovers[0].Costs.String() != "3.50" || book.DimTurnovers[0].Result.String() != "-3.50" || book.DimTurnovers[0].Name != "Banská Bystrica" {
		t.Errorf("strediská: %+v", book.DimTurnovers)
	}
	if len(book.MoneySources) < 1 || book.MoneySources[0].Account != "221001" {
		t.Errorf("zdroje peňazí: %+v", book.MoneySources)
	}
}

func TestFXLines(t *testing.T) {
	tpl := map[string]chart.Template{}
	for _, x := range chart.DefaultTemplates {
		tpl[x.Code] = x
	}
	cases := []struct {
		name, tpl, paid, doc, want string
	}{
		{"odberateľ zaplatil viac — zisk", "B-ODBERATEL", "105", "100", "221 MD 105.00, 311 D 100.00, 663 D 5.00"},
		{"odberateľ zaplatil menej — strata", "B-ODBERATEL", "98", "100", "221 MD 98.00, 311 D 100.00, 563 MD 2.00"},
		{"dodávateľovi menej — zisk", "B-DODAVATEL", "97", "100", "321 MD 100.00, 221 D 97.00, 663 D 3.00"},
		{"dodávateľovi viac — strata", "B-DODAVATEL", "101", "100", "321 MD 100.00, 221 D 101.00, 563 MD 1.00"},
		{"bez rozdielu", "B-ODBERATEL", "100", "100", "221 MD 100.00, 311 D 100.00"},
	}
	for _, tc := range cases {
		x := tpl[tc.tpl]
		lines := ledger.FromEntries(x.Apply(chart.Amounts{Total: m.MustParse(tc.paid)}))
		lines = fxLines(lines, x, m.MustParse(tc.doc))
		if got := lineStr(lines); got != tc.want || !ledger.Balanced(lines) {
			t.Errorf("%s: %s", tc.name, got)
		}
	}
}
