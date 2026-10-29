package postman

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		s     Settings
		field string
	}{
		{"vypnuté", Settings{}, ""},
		{"priečinky", Settings{Mode: ModeFolder, OutDir: "/a", InDir: "/b"}, ""},
		{"bez priečinka odoslaných", Settings{Mode: ModeFolder, InDir: "/b"}, "outDir"},
		{"rovnaké priečinky", Settings{Mode: ModeFolder, OutDir: "/a", InDir: "/a"}, "inDir"},
		{"HTTP", Settings{Mode: ModeHTTP, URL: "https://api.postar.sk/v1", Token: "x"}, ""},
		{"HTTP lokálne", Settings{Mode: ModeHTTP, URL: "http://127.0.0.1:9000", Token: "x"}, ""},
		{"HTTP bez šifrovania", Settings{Mode: ModeHTTP, URL: "http://postar.sk", Token: "x"}, "url"},
		{"HTTP bez tokenu", Settings{Mode: ModeHTTP, URL: "https://api.postar.sk"}, "token"},
		{"neznámy", Settings{Mode: "fax"}, "mode"},
	}
	for _, tt := range tests {
		tt.s.Normalize()
		err := tt.s.Validate()
		var se *SettingsError
		got := ""
		if errors.As(err, &se) {
			got = se.Problems[0].Field
		}
		if got != tt.field {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
	if _, err := New(Settings{}); !errors.Is(err, ErrOff) {
		t.Error("vypnuté")
	}
}

func TestPeppolID(t *testing.T) {
	for in, want := range map[string]string{"2120000003": "0245:2120000003", "SK 2120 000 003": "0245:2120000003", "": ""} {
		if got := PeppolID(in); got != want {
			t.Errorf("%q → %q", in, got)
		}
	}
}

func TestFolder(t *testing.T) {
	dir := t.TempDir()
	f := Folder{Out: filepath.Join(dir, "odoslane"), In: filepath.Join(dir, "prijate")}
	ctx := context.Background()
	if list, err := f.Inbox(ctx); err != nil || len(list) != 0 {
		t.Fatalf("prázdna schránka: %v %v", list, err)
	}
	s1, err := f.Send(ctx, Outgoing{FileName: "Faktura-FA1.xml", Data: []byte("<Invoice/>")})
	if err != nil || s1.MessageID != "Faktura-FA1.xml" {
		t.Fatal(s1, err)
	}
	s2, _ := f.Send(ctx, Outgoing{FileName: "Faktura-FA1.xml", Data: []byte("<Invoice v='2'/>")})
	if s2.MessageID == s1.MessageID {
		t.Fatal("opakované odoslanie prepísalo súbor")
	}
	if s3, _ := f.Send(ctx, Outgoing{FileName: "../../zle:meno.xml", Data: []byte("x")}); strings.ContainsAny(s3.MessageID, "/:\\") {
		t.Fatalf("nebezpečný názov %q", s3.MessageID)
	}
	entries, _ := os.ReadDir(f.Out)
	if len(entries) != 3 {
		t.Fatalf("súbory %v", entries)
	}

	_ = os.MkdirAll(f.In, 0o755)
	_ = os.WriteFile(filepath.Join(f.In, "dodavatel.xml"), []byte("<Invoice/>"), 0o644)
	_ = os.WriteFile(filepath.Join(f.In, "readme.txt"), []byte("x"), 0o644)
	list, err := f.Inbox(ctx)
	if err != nil || len(list) != 1 || list[0].ID != "dodavatel.xml" || string(list[0].Data) != "<Invoice/>" {
		t.Fatalf("schránka %+v %v", list, err)
	}
	if err := f.Ack(ctx, "../dodavatel.xml"); err == nil {
		t.Fatal("cesta mimo priečinka")
	}
	if err := f.Ack(ctx, "dodavatel.xml"); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.Inbox(ctx); len(list) != 0 {
		t.Fatal("prevzatá zostala v schránke")
	}
	if _, err := os.Stat(filepath.Join(f.In, DoneDir, "dodavatel.xml")); err != nil {
		t.Fatal("prevzatá sa má presunúť, nie zmazať")
	}
}

func TestHTTP(t *testing.T) {
	acked := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tajne" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":"zlý token"}`))
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/documents":
			b, _ := io.ReadAll(r.Body)
			if string(b) != "<Invoice/>" || r.Header.Get("X-Peppol-Receiver") != "0245:2020372640" {
				w.WriteHeader(400)
				return
			}
			json.NewEncoder(w).Encode(Sent{MessageID: "msg-1", Status: "prijaté poštárom"})
		case r.URL.Path == "/inbox":
			w.Write([]byte(`[{"id":"in 1","name":"FA9.xml","received":"2026-10-29T10:00:00Z"}]`))
		case r.URL.Path == "/inbox/in 1":
			w.Write([]byte("<Invoice id='9'/>"))
		case r.URL.Path == "/inbox/in 1/ack":
			acked = "in 1"
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	h := HTTP{BaseURL: srv.URL, Token: "tajne"}
	s, err := h.Send(ctx, Outgoing{Receiver: "0245:2020372640", FileName: "FA1.xml", Data: []byte("<Invoice/>")})
	if err != nil || s.MessageID != "msg-1" {
		t.Fatal(s, err)
	}
	list, err := h.Inbox(ctx)
	if err != nil || len(list) != 1 || string(list[0].Data) != "<Invoice id='9'/>" || list[0].Received.Year() != 2026 {
		t.Fatalf("%+v %v", list, err)
	}
	if err := h.Ack(ctx, "in 1"); err != nil || acked != "in 1" {
		t.Fatal(err)
	}
	if _, err := (HTTP{BaseURL: srv.URL, Token: "zle"}).Inbox(ctx); err == nil || !strings.Contains(err.Error(), "zlý token") {
		t.Fatal(err)
	}
}
