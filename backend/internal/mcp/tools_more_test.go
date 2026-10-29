package mcp

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// issued vystaví faktúru s dvoma položkami (8 hod × 45 € a 2 ks × 20 €, 23 %).
func issued(t *testing.T, s *Server, cid int64, issue string) invoice.Invoice {
	t.Helper()
	ctx := ActorFor(t.Context(), auth.Local)
	d := invoice.MustDate(issue)
	inv, err := s.App.SaveDraft(ctx, cid, invoice.Invoice{
		Customer:  invoice.Party{Name: "Odberateľ s.r.o.", ICO: "31322832", Country: "SK"},
		IssueDate: d, DeliveryDate: d, DueDate: d.AddDays(14),
		Lines: []invoice.Line{
			{Description: "Programovanie", Quantity: money.MustParse("8"), Unit: "hod", UnitPrice: money.MustParse("45"), Rate: 23},
			{Description: "Kábel", Quantity: money.MustParse("2"), Unit: "ks", UnitPrice: money.MustParse("20"), Rate: 23},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	inv, err = s.App.Issue(ctx, cid, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestCreatePurchaseTool(t *testing.T) {
	s, ctx, _ := setup(t)
	args := map[string]any{
		"supplier":        map[string]any{"name": "Dodávateľ s.r.o.", "ico": "31322832", "icdph": "SK2020372640"},
		"supplier_number": "2026/0457",
		"issue_date":      "2026-10-20", "due_date": "2026-11-03",
		"rates":       []map[string]any{{"vat_rate": 23, "base": "100"}, {"vat_rate": 5, "base": "20", "vat": "1.10"}},
		"description": "Materiál",
	}
	r := callTool(t, s, ctx, "create_purchase", args)
	if r.IsError {
		t.Fatalf("%s", r.Content[0].Text)
	}
	for _, w := range []string{`"total": "144.10"`, `"number": "PF2026`, `"receivedDate": "2026-10-29"`, `"rates[1].vat"`} {
		if !strings.Contains(r.Content[0].Text, w) {
			t.Errorf("chýba %s v %s", w, r.Content[0].Text)
		}
	}
	id := r.Structured["id"].(float64)
	// rovnaká faktúra dodávateľa druhýkrát nie
	if d := callTool(t, s, ctx, "create_purchase", args); !d.IsError || !strings.Contains(d.Content[0].Text, "už zaevidovaná") {
		t.Fatalf("duplicita: %s", d.Content[0].Text)
	}
	if e := callTool(t, s, ctx, "create_purchase", map[string]any{"supplier_number": "X", "rates": []map[string]any{{"vat_rate": 23, "base": "1"}}}); !e.IsError {
		t.Fatal("bez dodávateľa")
	}
	p := callTool(t, s, ctx, "mark_purchase_paid", map[string]any{"id": id, "date": "2026-10-28"})
	if p.IsError || p.Structured["status"] != "paid" || p.Structured["paidDate"] != "2026-10-28" {
		t.Fatalf("úhrada: %+v", p.Structured)
	}
}

func TestBankMatchTools(t *testing.T) {
	s, ctx, cid := setup(t)
	inv := issued(t, s, cid, "2026-10-01")
	txs, _, err := s.App.Store.SaveBankTransactions(ctx, cid, []bank.Transaction{{
		Account: "SK3112000000198742637541", Date: invoice.MustDate("2026-10-10"), Amount: inv.Summary.Total, Currency: "EUR",
		Direction: bank.In, VS: inv.VariableSymbol, CounterName: "Odberateľ s.r.o.", Fingerprint: "a", Status: bank.StatusOpen,
	}})
	if err != nil {
		t.Fatal(err)
	}
	tx := txs[0].ID
	sug := callTool(t, s, ctx, "bank_match_suggestions", map[string]any{"id": tx, "all_open": true})
	if sug.IsError || !strings.Contains(sug.Content[0].Text, `"score": 100`) || !strings.Contains(sug.Content[0].Text, `"open"`) {
		t.Fatalf("návrhy: %s", sug.Content[0].Text)
	}
	if b := callTool(t, s, ctx, "match_bank_transaction", map[string]any{"id": tx, "kind": "cash", "doc_id": inv.ID}); !b.IsError {
		t.Fatal("zlý druh dokladu")
	}
	m := callTool(t, s, ctx, "match_bank_transaction", map[string]any{"id": tx, "kind": "invoice", "doc_id": inv.ID})
	if m.IsError || m.Structured["status"] != "matched" {
		t.Fatalf("párovanie: %s", m.Content[0].Text)
	}
	if got, _ := s.App.Store.GetInvoice(ctx, cid, inv.ID); got.Status != invoice.StatusPaid {
		t.Fatalf("faktúra po spárovaní: %s", got.Status)
	}
	if again := callTool(t, s, ctx, "match_bank_transaction", map[string]any{"id": tx, "kind": "invoice", "doc_id": inv.ID}); !again.IsError {
		t.Fatal("dvakrát spárovať")
	}
}

func TestCreditNoteTool(t *testing.T) {
	s, ctx, cid := setup(t)
	inv := issued(t, s, cid, "2026-10-01")
	cases := []struct {
		name  string
		lines []map[string]any
		total string // dobropis s DPH
		err   string
	}{
		{"celá faktúra", nil, "-492.00", ""},
		{"iba kábel", []map[string]any{{"line": 2}}, "-49.20", ""},
		{"3 hodiny", []map[string]any{{"line": 1, "quantity": "3"}}, "-166.05", ""},
		{"viac, ako bolo", []map[string]any{{"line": 1, "quantity": "9"}}, "", "od 0 do 8"},
		{"neexistujúca položka", []map[string]any{{"line": 5}}, "", "nemá položku 5"},
		{"dvakrát tá istá", []map[string]any{{"line": 1}, {"line": 1}}, "", "dvakrát"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before, _ := s.App.Store.ListInvoices(ctx, cid, store.InvoiceFilter{})
			args := map[string]any{"invoice_id": inv.ID, "reason": "Vrátenie tovaru"}
			if c.lines != nil {
				args["lines"] = c.lines
			}
			r := callTool(t, s, ctx, "create_credit_note", args)
			if c.err != "" {
				after, _ := s.App.Store.ListInvoices(ctx, cid, store.InvoiceFilter{})
				if !r.IsError || !strings.Contains(r.Content[0].Text, c.err) || len(after) != len(before) {
					t.Fatalf("%s (konceptov %d → %d)", r.Content[0].Text, len(before), len(after))
				}
				return
			}
			if r.IsError || !strings.Contains(r.Content[0].Text, `"total": "`+c.total+`"`) || r.Structured["reason"] != "Vrátenie tovaru" {
				t.Fatalf("%s", r.Content[0].Text)
			}
		})
	}
	if r := callTool(t, s, ctx, "create_credit_note", map[string]any{"invoice_id": inv.ID}); !r.IsError {
		t.Fatal("bez dôvodu")
	}
}

func TestReminderTools(t *testing.T) {
	s, ctx, cid := setup(t)
	inv := issued(t, s, cid, "2026-09-01") // splatná 15. 9., dnes 29. 10.
	o := callTool(t, s, ctx, "list_overdue", map[string]any{})
	if o.IsError || o.Structured["count"].(float64) != 1 || !strings.Contains(o.Content[0].Text, `"nextLevel": 1`) {
		t.Fatalf("po splatnosti: %s", o.Content[0].Text)
	}
	for level := 1; level <= 2; level++ {
		r := callTool(t, s, ctx, "create_reminder", map[string]any{"invoice_ids": []int64{inv.ID}, "charge_fee": true})
		if r.IsError || r.Structured["level"].(float64) != float64(level) || !strings.HasPrefix(r.Structured["number"].(string), "UP") {
			t.Fatalf("%d. upomienka: %s", level, r.Content[0].Text)
		}
		if level == 1 && !strings.Contains(r.Content[0].Text, `"principal": "492.00"`) {
			t.Errorf("istina: %s", r.Content[0].Text)
		}
	}
	if r := callTool(t, s, ctx, "create_reminder", map[string]any{"invoice_ids": []int64{}}); !r.IsError {
		t.Fatal("bez faktúr")
	}
}

func TestRequiredArguments(t *testing.T) {
	s, ctx, _ := setup(t)
	cases := []struct {
		tool string
		args map[string]any
		miss string
	}{
		{"create_credit_note", map[string]any{"invoice_id": 1, "reason": ""}, "reason"},
		{"create_reminder", map[string]any{"invoice_ids": []int{}}, "invoice_ids"},
		{"match_bank_transaction", map[string]any{"id": 1}, "kind, doc_id"},
		{"create_purchase", map[string]any{"supplier_number": "1"}, "rates"},
	}
	for _, c := range cases {
		r := callTool(t, s, ctx, c.tool, c.args)
		if !r.IsError || !strings.Contains(r.Content[0].Text, "chýbajú povinné argumenty: "+c.miss) {
			t.Errorf("%s: %s", c.tool, r.Content[0].Text)
		}
	}
}

func TestMoneyJournalTool(t *testing.T) {
	s, ctx, cid := setup(t)
	_, _, err := s.App.Store.SaveBankTransactions(ctx, cid, []bank.Transaction{{Account: "SK3112000000198742637541",
		Date: invoice.MustDate("2026-10-10"), Amount: money.MustParse("4.90"), Currency: "EUR", Direction: bank.Out, Fingerprint: "f", Status: bank.StatusOpen}})
	if err != nil {
		t.Fatal(err)
	}
	r := callTool(t, s, ctx, "money_journal", map[string]any{"year": 2026, "unclassified_only": true})
	if r.IsError || !strings.Contains(r.Content[0].Text, `"unclassified": 1`) || !strings.Contains(r.Content[0].Text, `"amount": "-4.90"`) {
		t.Fatalf("%s", r.Content[0].Text)
	}
}

func TestStatementsTool(t *testing.T) {
	s, ctx, _ := setup(t)
	r := callTool(t, s, ctx, "simple_accounting_statements", map[string]any{"year": 2026})
	if r.IsError || !strings.Contains(r.Content[0].Text, "Rozdiel príjmov a výdavkov") || !strings.Contains(r.Content[0].Text, "Rozdiel majetku a záväzkov") {
		t.Fatalf("%s", r.Content[0].Text)
	}
}
