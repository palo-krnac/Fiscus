package mcp

import (
	"context"
	"encoding/json"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

type rpcResp struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Structured map[string]any `json:"structuredContent"`
	IsError    bool           `json:"isError"`
}

func setup(t *testing.T) (*Server, context.Context, int64) {
	t.Helper()
	caps, _ := edition.For(edition.Desktop)
	a := app.New(memstore.New(), caps)
	a.Today = func() invoice.Date { return invoice.MustDate("2026-10-29") }
	c, err := a.CreateCompany(context.Background(), store.Company{Name: "Moja firma", VATPayer: true, ICDPH: "SK2020372640"})
	if err != nil {
		t.Fatal(err)
	}
	return New(a, "test"), ActorFor(context.Background(), auth.Local), c.ID
}

func rpc(t *testing.T, s *Server, ctx context.Context, method string, params any) rpcResp {
	t.Helper()
	p, _ := json.Marshal(params)
	body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":` + string(p) + `}`
	var r rpcResp
	if err := json.Unmarshal(s.Handle(ctx, []byte(body)), &r); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return r
}

func callTool(t *testing.T, s *Server, ctx context.Context, name string, args any) toolResult {
	t.Helper()
	r := rpc(t, s, ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if r.Error != nil {
		t.Fatalf("%s: %+v", name, r.Error)
	}
	var tr toolResult
	_ = json.Unmarshal(r.Result, &tr)
	return tr
}

func TestProtocol(t *testing.T) {
	s, ctx, _ := setup(t)
	r := rpc(t, s, ctx, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}})
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct{ Name string }
		Capabilities    map[string]any
	}
	_ = json.Unmarshal(r.Result, &init)
	if init.ProtocolVersion != "2025-03-26" || init.ServerInfo.Name != "fiskus" || init.Capabilities["tools"] == nil {
		t.Fatalf("initialize: %s", r.Result)
	}
	if r := rpc(t, s, ctx, "initialize", map[string]any{"protocolVersion": "1999-01-01"}); !strings.Contains(string(r.Result), protocolVersions[0]) {
		t.Fatal("neznáma verzia → najnovšia")
	}
	// notifikácia nemá odpoveď
	if out := s.Handle(ctx, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); out != nil {
		t.Fatalf("notifikácia: %s", out)
	}
	if r := rpc(t, s, ctx, "neznama", nil); r.Error == nil || r.Error.Code != codeMethodNotFound {
		t.Fatal("neznáma metóda")
	}
	if out := s.Handle(ctx, []byte(`{nie je json`)); !strings.Contains(string(out), "-32700") {
		t.Fatalf("zlý JSON: %s", out)
	}
	// dávka
	out := s.Handle(ctx, []byte(`[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/x"},{"jsonrpc":"2.0","id":2,"method":"ping"}]`))
	var batch []rpcResp
	if err := json.Unmarshal(out, &batch); err != nil || len(batch) != 2 {
		t.Fatalf("dávka: %s", out)
	}

	r = rpc(t, s, ctx, "tools/list", nil)
	var tl struct {
		Tools []struct {
			Name        string
			InputSchema map[string]any `json:"inputSchema"`
			Annotations struct {
				ReadOnly bool `json:"readOnlyHint"`
			}
		}
	}
	_ = json.Unmarshal(r.Result, &tl)
	names := map[string]bool{}
	for _, x := range tl.Tools {
		names[x.Name] = x.Annotations.ReadOnly
		if x.InputSchema["type"] != "object" {
			t.Errorf("%s: schéma", x.Name)
		}
	}
	for name, ro := range map[string]bool{"list_invoices": true, "list_quotes": true, "list_catalog_items": true, "list_recurring_invoices": true, "vat_return": true, "create_invoice_draft": false, "issue_invoice": false} {
		if got, ok := names[name]; !ok || got != ro {
			t.Errorf("%s: readOnly=%v (%v)", name, got, ok)
		}
	}
	if r := rpc(t, s, ctx, "tools/call", map[string]any{"name": "neexistuje"}); r.Error == nil {
		t.Fatal("neznámy nástroj")
	}
}

func TestInvoiceTools(t *testing.T) {
	s, ctx, cid := setup(t)
	// bez company_id: jediná firma sa doplní
	res := callTool(t, s, ctx, "create_invoice_draft", map[string]any{
		"customer": map[string]any{"name": "Odberateľ s.r.o.", "ico": "31322832"},
		"lines":    []map[string]any{{"description": "Programovanie", "quantity": "10", "unit": "hod", "unit_price": "45.50", "vat_rate": 23}},
	})
	if res.IsError || res.Structured["summary"] == nil {
		t.Fatalf("koncept: %+v", res)
	}
	id := int64(res.Structured["id"].(float64))
	if !strings.Contains(res.Content[0].Text, "559.65") {
		t.Fatalf("súčet: %s", res.Content[0].Text)
	}
	// neplatné údaje → chyba nástroja s poľami
	bad := callTool(t, s, ctx, "create_invoice_draft", map[string]any{"customer": map[string]any{"name": "X"}, "lines": []map[string]any{{"description": "", "quantity": "1", "unit_price": "1", "vat_rate": 23}}})
	// koncept môže byť neúplný; chyby polí ukáže až vystavenie
	badID := int64(bad.Structured["id"].(float64))
	if r := callTool(t, s, ctx, "issue_invoice", map[string]any{"id": badID}); !r.IsError || !strings.Contains(r.Content[0].Text, "lines[0].description") {
		t.Fatalf("chyba polí: %+v", r)
	}
	if r := callTool(t, s, ctx, "create_invoice_draft", map[string]any{"neznamy": 1}); !r.IsError {
		t.Fatal("neznámy argument")
	}

	iss := callTool(t, s, ctx, "issue_invoice", map[string]any{"company_id": cid, "id": id})
	if iss.IsError || iss.Structured["number"] != "FA20260001" {
		t.Fatalf("vystavenie: %+v", iss)
	}
	list := callTool(t, s, ctx, "list_invoices", map[string]any{"status": "issued"})
	if list.IsError || list.Structured["count"].(float64) != 1 {
		t.Fatalf("zoznam: %+v", list)
	}
	ov := callTool(t, s, ctx, "company_overview", map[string]any{})
	if ov.IsError || !strings.Contains(ov.Content[0].Text, "559.65") {
		t.Fatalf("prehľad: %s", ov.Content[0].Text)
	}
	paid := callTool(t, s, ctx, "mark_invoice_paid", map[string]any{"id": id, "date": "2026-10-28"})
	if paid.IsError || paid.Structured["status"] != "paid" {
		t.Fatalf("úhrada: %+v", paid)
	}
	vr := callTool(t, s, ctx, "vat_return", map[string]any{"period": "2026-10"})
	if vr.IsError || !strings.Contains(vr.Content[0].Text, `"r04": "104.65"`) {
		t.Fatalf("DPH: %s", vr.Content[0].Text)
	}

	// audit: každé volanie s kanálom mcp
	entries, _, _ := s.App.Store.ListAudit(context.Background(), audit.Filter{Category: audit.CatAI})
	if len(entries) != 9 {
		t.Fatalf("audit volaní: %d", len(entries))
	}
	for _, e := range entries {
		if e.Channel != auth.ChannelMCP || !strings.HasPrefix(e.Summary, "MCP ") {
			t.Errorf("záznam: %+v", e)
		}
	}
	inv, _, _ := s.App.Store.ListAudit(context.Background(), audit.Filter{Category: audit.CatInvoices})
	if len(inv) == 0 || inv[0].Channel != auth.ChannelMCP {
		t.Fatal("zmena faktúry nemá kanál mcp")
	}
}

func TestReadOnlyAndPermissions(t *testing.T) {
	s, _, cid := setup(t)
	ro := auth.Local
	ro.ReadOnly = true
	roCtx := ActorFor(context.Background(), ro)
	if r := callTool(t, s, roCtx, "list_invoices", map[string]any{}); r.IsError {
		t.Fatalf("čítanie: %+v", r)
	}
	r := callTool(t, s, roCtx, "create_contact", map[string]any{"contact": map[string]any{"name": "X"}})
	if !r.IsError || !strings.Contains(r.Content[0].Text, "čítanie") {
		t.Fatalf("zápis kľúčom na čítanie: %+v", r)
	}
	// používateľ bez prístupu do firmy
	stranger := ActorFor(context.Background(), auth.Actor{UserID: 99, Name: "Cudzí"})
	if r := callTool(t, s, stranger, "list_invoices", map[string]any{"company_id": cid}); !r.IsError || !strings.Contains(r.Content[0].Text, "neexistuje") {
		t.Fatalf("cudzia firma: %+v", r)
	}
}

func TestQuoteTool(t *testing.T) {
	s, ctx, cid := setup(t)
	q := validQuoteForMCP()
	q, err := s.App.SaveQuote(ctx, cid, q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.App.IssueQuote(ctx, cid, q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.App.SaveQuote(ctx, cid, validQuoteForMCP()); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args  map[string]any
		count float64
	}{
		{map[string]any{}, 2},
		{map[string]any{"status": "issued"}, 1},
		{map[string]any{"status": "draft"}, 1},
		{map[string]any{"status": "expired"}, 0},
		{map[string]any{"query": "cp2026"}, 1},
		{map[string]any{"query": "zakaznik"}, 2},
	}
	for _, c := range cases {
		r := callTool(t, s, ctx, "list_quotes", c.args)
		if r.IsError || r.Structured["count"].(float64) != c.count {
			t.Errorf("%v: %+v", c.args, r.Structured)
		}
	}
}

func validQuoteForMCP() quote.Quote {
	return quote.Quote{Customer: invoice.Party{Name: "Zákazník a.s.", Country: "SK"},
		Lines: []invoice.Line{{Description: "Práca", Quantity: money.MustParse("1"), Unit: "hod", UnitPrice: money.MustParse("50"), Rate: 23}}}
}

func TestRecurringTool(t *testing.T) {
	s, ctx, cid := setup(t)
	r := recurring.Template{Name: "Paušál", Active: true, Interval: recurring.Monthly, NextDate: invoice.MustDate("2026-11-01"),
		Customer: invoice.Party{Name: "Zákazník a.s.", Country: "SK"},
		Lines:    []invoice.Line{{Description: "Paušál", Quantity: money.MustParse("1"), Unit: "mes", UnitPrice: money.MustParse("100"), Rate: 23}}}
	if _, err := s.App.SaveRecurring(ctx, cid, r); err != nil {
		t.Fatal(err)
	}
	res := callTool(t, s, ctx, "list_recurring_invoices", map[string]any{})
	if res.IsError || res.Structured["count"].(float64) != 1 || !strings.Contains(res.Content[0].Text, `"next_date": "2026-11-01"`) {
		t.Fatalf("%+v", res)
	}
}

func TestCatalogTool(t *testing.T) {
	s, ctx, cid := setup(t)
	for _, it := range []catalog.Item{
		{Code: "PLC", Name: "Programovanie PLC", Unit: "hod", Price: money.MustParse("48"), Rate: 23},
		{Name: "Starý výjazd", Unit: "km", Price: money.MustParse("0.40"), Rate: 23, Archived: true},
	} {
		if _, err := s.App.SaveItem(ctx, cid, it); err != nil {
			t.Fatal(err)
		}
	}
	r := callTool(t, s, ctx, "list_catalog_items", map[string]any{"query": "plc"})
	if r.IsError || r.Structured["count"].(float64) != 1 || !strings.Contains(r.Content[0].Text, `"price_gross": "59.04"`) {
		t.Fatalf("%+v", r)
	}
	if r := callTool(t, s, ctx, "list_catalog_items", map[string]any{}); r.Structured["count"].(float64) != 1 {
		t.Fatal("archivovaná položka sa nemá ponúkať")
	}
}
