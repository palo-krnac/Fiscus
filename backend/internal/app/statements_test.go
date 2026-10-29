package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestStatements(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	acc := "SK3112000000198742637541"
	// faktúra 492 € uhradená bankou, druhá 492 € neuhradená (pohľadávka)
	d1, _ := a.SaveDraft(ctx, c.ID, validDraft())
	i1, _ := a.Issue(ctx, c.ID, d1.ID)
	d2, _ := a.SaveDraft(ctx, c.ID, validDraft())
	_, _ = a.Issue(ctx, c.ID, d2.ID)
	txs, _, _ := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{
		{Account: acc, Date: invoice.MustDate("2026-10-20"), Amount: m.MustParse("492"), Currency: "EUR", Direction: bank.In, Fingerprint: "a", Status: bank.StatusOpen},
		{Account: acc, Date: invoice.MustDate("2026-10-21"), Amount: m.MustParse("216.12"), Currency: "EUR", Direction: bank.Out, Fingerprint: "b", Status: bank.StatusOpen},
	})
	_, _ = a.MatchTransaction(ctx, c.ID, txs[0].ID, "invoice", i1.ID)
	_ = a.ClassifyJournal(ctx, c.ID, "bank:"+itoa(int(txs[1].ID)), []journal.Line{{Column: journal.ExpInsur, Amount: m.MustParse("216.12")}}, "")
	_, _ = a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-01-05"), Amount: m.MustParse("100"), Purpose: "Vklad"})
	_ = a.SetJournalOpening(ctx, c.ID, 2026, map[string]m.Dec{acc: m.MustParse("1000")})
	if err := a.SetStatementInputs(ctx, c.ID, 2026, journal.AssetInputs{TangibleDM: m.MustParse("2500"), Loans: m.MustParse("800")}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetStatementInputs(ctx, c.ID, 2026, journal.AssetInputs{Loans: m.MustParse("0.001")}); err == nil {
		t.Fatal("zlomky centov")
	}
	// ručné vstupy sa pri chybe neprepíšu
	st, err := a.StatementsFor(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	inc, as := rowsByCode(st.Income), rowsByCode(st.Assets)
	if inc["02"].Current.String() != "400.00" || inc["08"].Current.String() != "216.00" || inc["12"].Current.String() != "184.00" {
		t.Fatalf("príjmy a výdavky: %+v", st.Income)
	}
	if st.Unclassified != 1 { // vklad do pokladnice
		t.Errorf("nerozpísané: %d", st.Unclassified)
	}
	// pohľadávka 492, pokladnica 100, banka 1000 + 492 − 216.12 = 1275.88 → 1276
	want := map[string]string{"02": "2500.00", "08": "492.00", "10": "100.00", "11": "1276.00", "15": "4368.00", "18": "800.00", "21": "3568.00"}
	for code, w := range want {
		if as[code].Current.String() != w {
			t.Errorf("r. %s: %s, chcem %s", code, as[code].Current, w)
		}
	}
	if !st.PrevEmpty {
		t.Error("prvá závierka: predchádzajúce obdobie je prázdne")
	}
}

func rowsByCode(rows []journal.Row) map[string]journal.Row {
	out := map[string]journal.Row{}
	for _, r := range rows {
		out[r.Code] = r
	}
	return out
}
