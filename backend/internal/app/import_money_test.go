package app

import (
	"os"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestImportMoney(t *testing.T) {
	data, err := os.ReadFile("../moneys3/testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	a, c := newApp(t, edition.Desktop)
	prev, err := a.ImportMoney(ctx, c.ID, data, MigrateOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := ImportReport{Contacts: ImportCount{New: 2, Skipped: 1}, Items: ImportCount{New: 1}, Invoices: ImportCount{New: 3, Skipped: 2}, Purchases: ImportCount{New: 1}}
	if prev.Contacts != want.Contacts || prev.Items != want.Items || prev.Invoices != want.Invoices || prev.Purchases != want.Purchases ||
		prev.Other["ObjPrij"] != 1 || prev.Source != "Money S3" {
		t.Fatalf("náhľad: %+v", prev)
	}
	joined := strings.Join(prev.Warnings, "\n")
	for _, w := range []string{"mene USD", "Zálohová faktúra 2026005"} {
		if !strings.Contains(joined, w) {
			t.Errorf("chýba upozornenie %q v %s", w, joined)
		}
	}
	if res, err := a.ImportMoney(ctx, c.ID, data, MigrateOptions{}, true); err != nil || res.Purchases.New != 1 {
		t.Fatal(res.Warnings, err)
	}
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
	by := map[string]invoice.Invoice{}
	for _, inv := range invs {
		by[inv.Number] = inv
	}
	tests := []struct {
		number, total string
		kind          invoice.Kind
		lines         int
		status        invoice.Status
	}{
		{"2026001", "173.31", invoice.KindInvoice, 2, invoice.StatusIssued},
		{"2026002", "-14.98", invoice.KindCreditNote, 1, invoice.StatusIssued},
		{"2026003", "134.90", invoice.KindInvoice, 2, invoice.StatusIssued}, // z rekapitulácie
	}
	for _, tc := range tests {
		inv, ok := by[tc.number]
		if !ok || inv.Summary.Total.String() != tc.total || inv.Kind != tc.kind || len(inv.Lines) != tc.lines || inv.Status != tc.status {
			t.Errorf("%s: %v %s %s %d %s", tc.number, ok, inv.Summary.Total, inv.Kind, len(inv.Lines), inv.Status)
		}
	}
	fa := by["2026001"]
	if fa.Customer.ICDPH != "SK2020123457" || fa.Customer.DIC != "2020123457" || fa.Customer.Country != "SK" || fa.Lines[0].Unit != "h" || fa.Subject != "Montáž rozvádzača" || fa.Lines[1].Rate != vat.Rate23 {
		t.Errorf("odberateľ a položky: %+v %+v", fa.Customer, fa.Lines)
	}
	if db := by["2026002"]; db.Correction == nil || db.Correction.OriginalID != fa.ID {
		t.Errorf("dobropis bez väzby: %+v", db.Correction)
	}
	if r := by["2026003"].Lines; r[0].Rate != vat.Rate19 || r[0].UnitPrice.String() != "10.00" || r[1].Rate != vat.Rate23 {
		t.Errorf("rekapitulácia: %+v", r)
	}
	purs, _ := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	if len(purs) != 1 || purs[0].SupplierNumber != "2026001234" || purs[0].Summary.Total.String() != "134.90" ||
		purs[0].ReceivedDate.String() != "2026-03-07" || purs[0].Supplier.ICO != "35757442" {
		t.Errorf("prijatá: %+v", purs)
	}
	items, _ := a.Store.ListItems(ctx, c.ID)
	if len(items) != 1 || items[0].Code != "REL-24" || items[0].Price.String() != "12.18" {
		t.Errorf("cenník: %+v", items)
	}
	again, _ := a.ImportMoney(ctx, c.ID, data, MigrateOptions{}, false)
	if again.Invoices.Existing != 3 || again.Purchases.Existing != 1 || again.Contacts.Existing != 2 || again.Items.Existing != 1 {
		t.Errorf("opakovaný prenos: %+v", again)
	}
	if _, err := a.ImportMoney(ctx, c.ID, []byte(`<?xml version="1.0"?><dataPack/>`), MigrateOptions{}, false); !fieldErr(err, "file") {
		t.Errorf("súbor z POHODY: %v", err)
	}
}

func TestImportMoneyPaidUntil(t *testing.T) {
	data, err := os.ReadFile("../moneys3/testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	a, c := newApp(t, edition.Desktop)
	if _, err := a.ImportMoney(ctx, c.ID, data, MigrateOptions{PaidUntil: invoice.MustDate("2026-03-31")}, true); err != nil {
		t.Fatal(err)
	}
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
	for _, inv := range invs {
		paid := inv.Status == invoice.StatusPaid
		if want := inv.Number == "2026001"; paid != want {
			t.Errorf("%s (splatná %s): uhradená %v", inv.Number, inv.DueDate, paid)
		}
		if paid && inv.PaidDate.String() != "2026-03-16" {
			t.Errorf("%s: dátum úhrady %s", inv.Number, inv.PaidDate)
		}
	}
	purs, _ := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	if len(purs) != 1 || purs[0].Status != purchase.StatusPaid || purs[0].PaidDate.String() != "2026-03-19" {
		t.Errorf("prijatá: %+v", purs)
	}
}

func TestMigInvoiceBuild(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	line := func(price string) invoice.Line {
		return invoice.Line{Description: "Práca", Quantity: money.FromInt(1), Unit: "ks", UnitPrice: money.MustParse(price), Rate: vat.Rate23}
	}
	head := invoice.Invoice{Customer: invoice.Party{Name: "Odberateľ", Country: "SK"}, IssueDate: invoice.MustDate("2026-03-02"), PriceMode: vat.PriceNet}
	tests := []struct {
		name      string
		lines     []invoice.Line
		summary   []invoice.Line
		total     string
		credit    bool
		wantTotal string
		wantPrice string // cena prvého riadku
		wantNote  bool
		wantErr   bool
	}{
		{"položky sedia", []invoice.Line{line("100")}, []invoice.Line{line("100")}, "123", false, "123.00", "100.00", false, false},
		{"zľava → rekapitulácia", []invoice.Line{line("100")}, []invoice.Line{line("97.56")}, "120", false, "120.00", "97.56", false, false},
		{"bez rekapitulácie → poznámka", []invoice.Line{line("100")}, nil, "120", false, "123.00", "100.00", true, false},
		{"iba rekapitulácia", nil, []invoice.Line{line("50")}, "61.5", false, "61.50", "50.00", false, false},
		{"dobropis kladný → záporný", []invoice.Line{line("10")}, nil, "-12.3", true, "-12.30", "10.00", false, false},
		{"nič", nil, nil, "0", false, "", "", false, true},
	}
	numbers := map[string]int64{"1": 7}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := head
			inv.Lines = tc.lines
			mi := migInvoice{Number: "X", Inv: inv, Summary: tc.summary, Total: money.MustParse(tc.total), Credit: tc.credit, Original: "1"}
			var rep ImportReport
			got, err := a.migInvoiceBuild(ctx, c.ID, mi, numbers, &rep)
			if (err != nil) != tc.wantErr {
				t.Fatalf("chyba: %v", err)
			}
			if tc.wantErr {
				return
			}
			if got.Summary.Total.String() != tc.wantTotal || got.Lines[0].UnitPrice.String() != tc.wantPrice || (got.Note != "") != tc.wantNote {
				t.Errorf("suma %s, cena %s, poznámka %q", got.Summary.Total, got.Lines[0].UnitPrice, got.Note)
			}
		})
	}
}
