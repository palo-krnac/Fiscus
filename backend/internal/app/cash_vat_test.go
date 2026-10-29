package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestCashVAT(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // platiteľ DPH, dnes 29. 10. 2026
	if _, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-10-01"), Amount: m.MustParse("500"), Purpose: "Vklad"}); err != nil {
		t.Fatal(err)
	}
	fuel := cash.Document{Kind: cash.Out, Date: invoice.MustDate("2026-10-05"), Amount: m.MustParse("61.50"), Partner: "Čerpacia stanica", Purpose: "Pohonné hmoty",
		Tax: &cash.Tax{Rows: []cash.VATRow{{Rate: vat.Rate23, Gross: m.MustParse("61.50")}}, SupplierICDPH: "sk2020372640", Expense: purchase.ExpenseOther}}

	// náhľad dopočíta základ a daň, chyby vráti bez uloženia
	pv, err := a.PreviewCash(ctx, c.ID, fuel)
	if err != nil || len(pv.Errors) != 0 || pv.Tax.Rows[0].VAT.String() != "11.50" || pv.Tax.SupplierICDPH != "SK2020372640" {
		t.Fatalf("náhľad: %+v %v", pv, err)
	}
	bad := fuel
	bad.Amount = m.MustParse("60")
	if pv, _ := a.PreviewCash(ctx, c.ID, bad); len(pv.Errors) != 1 || pv.Errors[0].Field != "tax.rows" {
		t.Fatalf("nesedí súčet: %+v", pv.Errors)
	}

	saved, err := a.CreateCash(ctx, c.ID, fuel)
	if err != nil || saved.Tax == nil || saved.Tax.Rows[0].Base.String() != "50.00" {
		t.Fatalf("vystavenie: %+v %v", saved, err)
	}
	sales := cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-10-28"), Amount: m.MustParse("123"), Purpose: "Tržba z e-kasy 28. 10.",
		Tax: &cash.Tax{Rows: []cash.VATRow{{Rate: vat.Rate23, Gross: m.MustParse("123")}}, Goods: true}}
	if _, err := a.CreateCash(ctx, c.ID, sales); err != nil {
		t.Fatal(err)
	}

	// priznanie: odpočet z bloku, daň z tržby
	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil || ret.Rows["r19"].String() != "11.50" || ret.Rows["r04"].String() != "23.00" || ret.KV.B31 == nil || ret.KV.D1.Count != 1 {
		t.Fatalf("priznanie: %+v %+v %v", ret.Rows, ret.KV, err)
	}

	// peňažný denník: rozpis s DPH navrhne Fiškus
	book, err := a.Journal(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string][]journal.Line{}
	for _, e := range book.Entries {
		lines[e.Number] = e.Lines
	}
	if l := lines[saved.Number]; len(l) != 2 || l[0].Column != journal.ExpOther || l[0].Amount.String() != "50.00" || l[1].Column != journal.ExpVAT {
		t.Fatalf("blok v denníku: %+v", l)
	}
	if l := lines["PPD20260002"]; len(l) != 2 || l[0].Column != journal.IncGoods || l[1].Amount.String() != "23.00" {
		t.Fatalf("tržba v denníku: %+v %v", l, lines)
	}

	// neplatiteľ DPH na doklade DPH neuvádza
	c.VATPayer, c.ICDPH = false, ""
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	fuel.Date = invoice.MustDate("2026-10-06")
	if _, err := a.CreateCash(ctx, c.ID, fuel); !fieldErr(err, "tax") {
		t.Fatalf("neplatiteľ: %v", err)
	}
}
