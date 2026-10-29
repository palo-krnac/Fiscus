// Package mcp je server Model Context Protocol pre AI asistentov (Claude a i.).
//
// Asistent sa pripája API kľúčom používateľa a má presne jeho práva vo firmách
// (kľúč môže byť navyše obmedzený len na čítanie). Každé volanie nástroja sa
// zapíše do auditu v kategórii „AI asistent (MCP)“; zmeny dokladov sa okrem
// toho zapisujú do svojich kategórií s kanálom „mcp“.
//
// Implementuje JSON-RPC 2.0 cez „Streamable HTTP“ (POST /mcp, odpoveď JSON).
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Podporované verzie protokolu, najnovšia prvá.
var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Server obsluhuje MCP požiadavky nad aplikáciou.
type Server struct {
	App     *app.App
	Version string
	tools   []Tool
}

// New vytvorí server so všetkými nástrojmi.
func New(a *app.App, version string) *Server {
	s := &Server{App: a, Version: version}
	s.tools = s.allTools()
	return s
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Kódy chýb JSON-RPC.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Handle spracuje telo požiadavky (jedna správa alebo dávka). Vráti odpoveď,
// alebo nil, ak išlo iba o notifikácie.
func (s *Server) Handle(ctx context.Context, body []byte) []byte {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) > 0 && body[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(body, &batch); err != nil {
			return mustJSON(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "neplatný JSON"}})
		}
		var out []response
		for _, raw := range batch {
			if r := s.handleOne(ctx, raw); r != nil {
				out = append(out, *r)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return mustJSON(out)
	}
	if r := s.handleOne(ctx, body); r != nil {
		return mustJSON(r)
	}
	return nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (s *Server) handleOne(ctx context.Context, raw []byte) *response {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "neplatný JSON"}}
	}
	notification := len(req.ID) == 0
	if req.JSONRPC != "2.0" || req.Method == "" {
		if notification {
			return nil
		}
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{codeInvalidRequest, "neplatná požiadavka JSON-RPC"}}
	}
	if notification {
		return nil // notifications/initialized, notifications/cancelled …
	}
	res, rerr := s.dispatch(ctx, req)
	if rerr != nil {
		return &response{JSONRPC: "2.0", ID: req.ID, Error: rerr}
	}
	return &response{JSONRPC: "2.0", ID: req.ID, Result: res}
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := protocolVersions[0]
		for _, v := range protocolVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "fiskus", "title": "Fiškus", "version": s.Version},
			"instructions": "Fiškus je slovenský účtovný program. Sumy sú v eurách ako reťazce s bodkou (\"1234.50\"), " +
				"dátumy vo formáte RRRR-MM-DD. Ak má používateľ viac firiem, najprv zavolaj list_companies a potom posielaj company_id. " +
				"Faktúru najprv vytvor ako koncept (create_invoice_draft); vystav ju (issue_invoice) iba na výslovný pokyn používateľa. " +
				"Všetky volania sa zapisujú do auditu.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		list := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			list = append(list, t.describe())
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{codeInvalidParams, "chýba názov nástroja"}
		}
		t := s.tool(p.Name)
		if t == nil {
			return nil, &rpcError{codeInvalidParams, "neznámy nástroj " + p.Name}
		}
		return s.call(ctx, t, p.Arguments), nil
	}
	return nil, &rpcError{codeMethodNotFound, "neznáma metóda " + req.Method}
}

func (s *Server) tool(name string) *Tool {
	for i := range s.tools {
		if s.tools[i].Name == name {
			return &s.tools[i]
		}
	}
	return nil
}

// call spustí nástroj, zapíše volanie do auditu a vráti výsledok MCP.
func (s *Server) call(ctx context.Context, t *Tool, args json.RawMessage) map[string]any {
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	var c callCtx
	c.ctx, c.s = ctx, s
	out, err := t.Run(&c, args)

	summary := "MCP " + t.Name
	if err != nil {
		summary += " – chyba: " + err.Error()
	}
	var argsLog any
	_ = json.Unmarshal(args, &argsLog)
	s.App.Audit(ctx, c.companyID, audit.CatAI, "tool_call", "mcp_tool", 0, summary, nil, map[string]any{"tool": t.Name, "arguments": argsLog, "ok": err == nil})

	if err != nil {
		return map[string]any{"content": []map[string]any{{"type": "text", "text": userError(err)}}, "isError": true}
	}
	text, _ := json.MarshalIndent(out, "", "  ")
	res := map[string]any{"content": []map[string]any{{"type": "text", "text": string(text)}}}
	if obj, ok := asObject(out); ok {
		res["structuredContent"] = obj
	}
	return res
}

func asObject(v any) (map[string]any, bool) {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || b[0] != '{' {
		return nil, false
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m) == nil
}

// userError preloží chybu na text pre asistenta (vrátane chýb polí).
func userError(err error) string {
	var ve *invoice.ValidationError
	if errors.As(err, &ve) {
		var b strings.Builder
		b.WriteString("Údaje obsahujú chyby:")
		for _, f := range ve.Fields {
			fmt.Fprintf(&b, "\n- %s: %s", f.Field, f.Message)
		}
		return b.String()
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		return "Záznam neexistuje alebo k nemu nemáte prístup."
	case errors.Is(err, app.ErrForbidden):
		return "Na túto akciu nemáte oprávnenie."
	}
	return err.Error()
}

// ActorFor je pomocník pre testy: kontext s actorom kanála MCP.
func ActorFor(ctx context.Context, a auth.Actor) context.Context {
	a.Channel = auth.ChannelMCP
	return auth.WithActor(ctx, a)
}
