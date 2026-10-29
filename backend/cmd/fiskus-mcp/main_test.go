package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBridge(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if strings.Contains(string(body), "zlykaj") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"neplatný alebo zrušený API kľúč"}`))
			return
		}
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}` + "\n"))
	}))
	defer srv.Close()

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}
{"jsonrpc":"2.0","method":"notifications/initialized"}

{"jsonrpc":"2.0","id":5,"method":"zlykaj"}
`)
	var out bytes.Buffer
	if err := run(in, &out, srv.URL, "fk_test", srv.Client()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || lines[0] != `{"jsonrpc":"2.0","id":1,"result":{}}` || !strings.Contains(lines[1], `"id":5`) || !strings.Contains(lines[1], "zrušený") {
		t.Fatalf("výstup:\n%s", out.String())
	}
	if gotAuth != "Bearer fk_test" {
		t.Fatalf("hlavička: %q", gotAuth)
	}
	// server nebeží → chyba s id
	out.Reset()
	_ = run(strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"ping"}`+"\n"), &out, "http://127.0.0.1:1/mcp", "k", http.DefaultClient)
	if !strings.Contains(out.String(), "neodpovedá") {
		t.Fatalf("bez servera: %s", out.String())
	}
}
