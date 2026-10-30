package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestCashRegisters(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if err := a.Store.SaveRates(ctx, []fx.Rate{{Currency: "CZK", Date: "2026-10-01", Rate: m.MustParse("25.000"), Source: fx.SourceECB}}); err != nil {
		t.Fatal(err)
	}
	regs, err := a.CashRegisters(ctx, c.ID)
	if err != nil || len(regs) != 1 || regs[0].Name != cash.MainName || regs[0].Currency != "EUR" {
		t.Fatalf("predvolená pokladňa: %+v %v", regs, err)
	}

	// chyby v zozname pokladní
	bad := []struct {
		name  string
		list  []cash.Register
		field string
	}{
		{"bez predpony", []cash.Register{{ID: 0}, {ID: -1, Name: "Prevádzka"}}, "registers[1].prefix"},
		{"rovnaká predpona", []cash.Register{{ID: 0}, {ID: -1, Name: "A", Prefix: "P2"}, {ID: -2, Name: "B", Prefix: "p2"}}, "registers[2].prefix"},
		{"neznáma mena", []cash.Register{{ID: 0}, {ID: -1, Name: "A", Prefix: "X", Currency: "XYZ"}}, "registers[1].currency"},
		{"rovnaký názov", []cash.Register{{ID: 0, Name: "Pokladňa"}, {ID: -1, Name: "pokladňa", Prefix: "P2"}}, "registers[1].name"},
		{"rezervovaná predpona", []cash.Register{{ID: 0}, {ID: -1, Name: "A", Prefix: "PPD"}}, "registers[1].prefix"},
	}
	for _, tc := range bad {
		if _, err := a.SaveCashRegisters(ctx, c.ID, tc.list); !fieldErr(err, tc.field) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	regs, err = a.SaveCashRegisters(ctx, c.ID, []cash.Register{{ID: 0, Name: "Hlavná pokladňa"}, {ID: -1, Name: "Pokladňa CZK", Currency: "czk", Prefix: "cz"}})
	if err != nil || len(regs) != 2 || regs[1].ID != 1 || regs[1].Prefix != "CZ" || regs[1].Currency != "CZK" || regs[0].Name != "Hlavná pokladňa" {
		t.Fatalf("pokladne: %+v %v", regs, err)
	}

	doc := func(reg int64, kind cash.Kind, date, amount string) cash.Document {
		return cash.Document{RegisterID: reg, Kind: kind, Date: invoice.MustDate(date), Amount: m.MustParse(amount), Purpose: "Pohyb"}
	}
	if _, err := a.CreateCash(ctx, c.ID, doc(0, cash.In, "2026-10-01", "1000")); err != nil {
		t.Fatal(err)
	}
	// každá pokladňa má vlastný zostatok: z prázdnej CZK pokladne nič nevydáte
	if _, err := a.CreateCash(ctx, c.ID, doc(1, cash.Out, "2026-10-02", "100")); !fieldErr(err, "amount") {
		t.Fatalf("zostatok CZK pokladne: %v", err)
	}
	in, err := a.CreateCash(ctx, c.ID, doc(1, cash.In, "2026-10-02", "2500"))
	if err != nil || in.Number != "CZPPD20260001" || in.Currency != "CZK" || !in.FXRate.Equal(m.MustParse("25")) || in.AmountEUR.String() != "100.00" {
		t.Fatalf("príjem CZK: %+v %v", in, err)
	}
	out, err := a.CreateCash(ctx, c.ID, doc(1, cash.Out, "2026-10-03", "500"))
	if err != nil || out.Number != "CZVPD20260001" || out.AmountEUR.String() != "20.00" {
		t.Fatalf("výdavok CZK: %+v %v", out, err)
	}
	// DPH na doklade v cudzej mene nie
	withTax := doc(1, cash.Out, "2026-10-03", "121")
	withTax.Tax = &cash.Tax{Rows: []cash.VATRow{{Rate: 21, Gross: m.MustParse("121")}}, Expense: "other"}
	if _, err := a.CreateCash(ctx, c.ID, withTax); !fieldErr(err, "tax") {
		t.Fatalf("DPH v cudzej mene: %v", err)
	}
	// faktúra v eurách sa z CZK pokladne neuhrádza
	dr, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, _ := a.Issue(ctx, c.ID, dr.ID)
	link := doc(1, cash.In, "2026-10-04", "100")
	link.LinkKind, link.LinkID = "invoice", inv.ID
	if _, err := a.CreateCash(ctx, c.ID, link); !fieldErr(err, "linkId") {
		t.Fatalf("úhrada EUR faktúry z CZK pokladne: %v", err)
	}

	// faktúra v CZK: kandidát CZK pokladne, uhradí sa v hotovosti v CZK
	czk := validDraft()
	czk.Currency = "CZK"
	dr2, err := a.SaveDraft(ctx, c.ID, czk)
	if err != nil {
		t.Fatal(err)
	}
	inv2, err := a.Issue(ctx, c.ID, dr2.ID)
	if err != nil {
		t.Fatal(err)
	}
	cands, _ := a.CashCandidates(ctx, c.ID, cash.In, 1)
	if len(cands) != 1 || cands[0].ID != inv2.ID {
		t.Fatalf("kandidáti CZK: %+v", cands)
	}
	if eur, _ := a.CashCandidates(ctx, c.ID, cash.In, 0); len(eur) != 1 || eur[0].ID != inv.ID {
		t.Fatalf("kandidáti EUR: %+v", eur)
	}
	pay := doc(1, cash.In, "2026-10-04", inv2.Remaining().String())
	pay.LinkKind, pay.LinkID = "invoice", inv2.ID
	if _, err := a.CreateCash(ctx, c.ID, pay); err != nil {
		t.Fatalf("úhrada CZK faktúry: %v", err)
	}
	if inv2, _ = a.Store.GetInvoice(ctx, c.ID, inv2.ID); inv2.Status != invoice.StatusPaid {
		t.Fatalf("CZK faktúra neuhradená: %s", inv2.Status)
	}

	book, err := a.CashBookOf(ctx, c.ID, 2026, 1)
	if err != nil || len(book.Documents) != 3 || book.Balance.String() != "2492.00" || book.Register.Currency != "CZK" {
		t.Fatalf("kniha CZK: %+v %v", book, err)
	}
	if main, _ := a.CashBook(ctx, c.ID, 2026); len(main.Documents) != 1 || main.Balance.String() != "1000.00" {
		t.Fatalf("hlavná kniha: %+v", main)
	}

	// peňažný denník: každá pokladnica zvlášť, v eurách
	jb, err := a.Journal(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, b := range jb.Balances {
		if b.Means == journal.Cash {
			got[b.Account] = b.Closing.String()
		}
	}
	if got["Hlavná pokladňa"] != "1000.00" || got["Pokladňa CZK"] != "99.68" {
		t.Fatalf("stavy pokladníc: %+v", got)
	}

	// pokladňu s dokladmi nemožno zmeniť na inú menu ani zmazať, iba archivovať
	if _, err := a.SaveCashRegisters(ctx, c.ID, []cash.Register{{ID: 0}, {ID: 1, Name: "Pokladňa CZK", Currency: "USD", Prefix: "CZ"}}); !fieldErr(err, "registers[1].currency") {
		t.Fatalf("zmena meny: %v", err)
	}
	if _, err := a.SaveCashRegisters(ctx, c.ID, []cash.Register{{ID: 0}}); !fieldErr(err, "registers") {
		t.Fatalf("zmazanie: %v", err)
	}
	if _, err := a.SaveCashRegisters(ctx, c.ID, []cash.Register{{ID: 0}, {ID: 1, Name: "Pokladňa CZK", Currency: "CZK", Prefix: "CZ", Archived: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCash(ctx, c.ID, doc(1, cash.In, "2026-10-05", "1")); !fieldErr(err, "registerId") {
		t.Fatalf("archivovaná pokladňa: %v", err)
	}
	if _, err := a.CreateCash(ctx, c.ID, doc(7, cash.In, "2026-10-05", "1")); !fieldErr(err, "registerId") {
		t.Fatalf("neexistujúca pokladňa: %v", err)
	}
}
