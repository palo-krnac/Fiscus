package shop

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/license"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
)

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func TestVerify(t *testing.T) {
	body := []byte(`{"event_type":"transaction.completed"}`)
	good := Sign("tajne", body, now)
	ts := strings.Split(good, ";")[0]
	tests := []struct {
		name, header, secret string
		body                 []byte
		at                   time.Time
		want                 error
	}{
		{"platný", good, "tajne", body, now, nil},
		{"platný s druhým h1 (zmena tajomstva)", ts + ";h1=abcd;" + strings.Split(good, ";")[1], "tajne", body, now, nil},
		{"iné tajomstvo", good, "ine", body, now, ErrSignature},
		{"zmenené telo", good, "tajne", []byte(`{"event_type":"x"}`), now, ErrSignature},
		{"bez h1", ts, "tajne", body, now, ErrSignature},
		{"nečíselný čas", "ts=x;h1=00", "tajne", body, now, ErrSignature},
		{"prázdne tajomstvo", good, "", body, now, ErrSignature},
		{"starý podpis", good, "tajne", body, now.Add(10 * time.Minute), ErrStale},
		{"v tolerancii", good, "tajne", body, now.Add(4 * time.Minute), nil},
	}
	for _, tt := range tests {
		if err := Verify(tt.header, tt.body, tt.secret, tt.at); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, chcem %v", tt.name, err, tt.want)
		}
	}
}

type fakeMail struct {
	sent []mail.Message
	fail error
}

func (f *fakeMail) Send(_ context.Context, _ mail.Settings, m mail.Message) error {
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, m)
	return nil
}

type fakeLookup struct{ email, name string }

func (f fakeLookup) Customer(context.Context, string) (string, string, error) {
	return f.email, f.name, nil
}

func event(t *testing.T, typ, txn, price string, qty int, custom map[string]any) Event {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"id": txn, "status": "completed", "customer_id": "ctm_1", "custom_data": custom,
		"items": []map[string]any{{"quantity": qty, "price": map[string]any{"id": price}}}})
	return Event{EventID: "evt_" + txn, EventType: typ, Data: data}
}

func issuer(t *testing.T, m *fakeMail) (*Issuer, ed25519.PublicKey) {
	pub, priv, _ := license.GenerateKey()
	return &Issuer{Key: priv, PriceID: "pri_multi", Store: &FileStore{Path: filepath.Join(t.TempDir(), "licencie.jsonl")},
		Mail: m, SMTP: mail.Settings{Host: "smtp.example.sk", FromEmail: "predaj@fiskus.sk", FromName: "Fiškus"},
		Now: func() time.Time { return now }}, pub
}

func TestIssue(t *testing.T) {
	m := &fakeMail{}
	is, pub := issuer(t, m)
	ctx := context.Background()
	cust := map[string]any{"email": "kancelaria@example.sk", "company": "Kancelária s.r.o."}
	tests := []struct {
		name   string
		ev     Event
		action string
		expiry string
	}{
		{"iná udalosť", event(t, "subscription.created", "txn_0", "pri_multi", 1, cust), "ignored", ""},
		{"iný produkt", event(t, "transaction.completed", "txn_x", "pri_iny", 1, cust), "ignored", ""},
		{"nákup", event(t, "transaction.completed", "txn_1", "pri_multi", 1, cust), "issued", "2027-10-29"},
		{"opakované doručenie", event(t, "transaction.completed", "txn_1", "pri_multi", 1, cust), "duplicate", ""},
		{"obnova nadväzuje na koniec", event(t, "transaction.completed", "txn_2", "pri_multi", 1, cust), "issued", "2028-10-29"},
		{"dva roky naraz", event(t, "transaction.completed", "txn_3", "pri_multi", 2, map[string]any{"email": "iny@example.sk", "company": "Iná s.r.o."}), "issued", "2028-10-29"},
	}
	for _, tt := range tests {
		res, err := is.Handle(ctx, tt.ev)
		if err != nil || res.Action != tt.action {
			t.Fatalf("%s: %+v %v", tt.name, res, err)
		}
		if tt.expiry == "" {
			continue
		}
		var txn Transaction
		_ = json.Unmarshal(tt.ev.Data, &txn)
		r, _ := is.Store.Get(txn.ID)
		l, err := license.Parse(r.Key, []ed25519.PublicKey{pub})
		if err != nil || l.Expires.Format("2006-01-02") != tt.expiry || l.Plan != license.PlanMulti || l.ID != txn.ID || !r.Emailed {
			t.Fatalf("%s: licencia %+v %v (záznam %+v)", tt.name, l, err, r)
		}
	}
	if len(m.sent) != 3 || m.sent[0].To[0] != "kancelaria@example.sk" || !strings.Contains(m.sent[0].Body, "FISKUS1.") {
		t.Fatalf("e-maily: %+v", m.sent)
	}
}

func TestIssueMailRetry(t *testing.T) {
	m := &fakeMail{fail: errors.New("SMTP nedostupné")}
	is, _ := issuer(t, m)
	ctx := context.Background()
	ev := event(t, "transaction.completed", "txn_9", "pri_multi", 1, map[string]any{"email": "a@b.sk"})
	if _, err := is.Handle(ctx, ev); err == nil {
		t.Fatal("chyba e-mailu → Paddle má zopakovať")
	}
	first, _ := is.Store.Get("txn_9")
	m.fail = nil
	if res, err := is.Handle(ctx, ev); err != nil || res.Action != "duplicate" || len(m.sent) != 1 {
		t.Fatalf("opakovanie: %+v %v", res, err)
	}
	again, _ := is.Store.Get("txn_9")
	if again.Key != first.Key || !again.Emailed || again.Customer != "a@b.sk" {
		t.Fatalf("rovnaký kľúč, iba odoslaný: %+v", again)
	}
	// ďalšie doručenie už nič neposiela
	if res, _ := is.Handle(ctx, ev); res.Action != "duplicate" || len(m.sent) != 1 {
		t.Fatal("poslané dvakrát")
	}
	// evidencia prežije reštart
	reloaded := &FileStore{Path: is.Store.(*FileStore).Path}
	if r, ok := reloaded.Get("txn_9"); !ok || !r.Emailed {
		t.Fatalf("po načítaní: %+v", r)
	}
}

func TestIssueCustomer(t *testing.T) {
	is, _ := issuer(t, &fakeMail{})
	ctx := context.Background()
	ev := event(t, "transaction.completed", "txn_5", "pri_multi", 1, nil)
	if _, err := is.Handle(ctx, ev); !errors.Is(err, ErrNoCustomer) {
		t.Fatalf("bez e-mailu: %v", err)
	}
	is.Lookup = fakeLookup{"z@api.sk", "Z API s.r.o."}
	if _, err := is.Handle(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if r, _ := is.Store.Get("txn_5"); r.Email != "z@api.sk" || r.Customer != "Z API s.r.o." {
		t.Fatalf("%+v", r)
	}
	// bez SMTP sa licencia vystaví a kľúč sa pošle ručne
	is.SMTP = mail.Settings{}
	res, err := is.Handle(ctx, event(t, "transaction.completed", "txn_6", "pri_multi", 1, map[string]any{"email": "q@q.sk"}))
	if err != nil || res.Action != "issued" || !strings.Contains(res.Detail, "ručne") {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestAPICustomer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer kluc" || r.URL.Path != "/customers/ctm_1" {
			w.WriteHeader(403)
			return
		}
		w.Write([]byte(`{"data":{"id":"ctm_1","email":"a@b.sk","name":"Firma"}}`))
	}))
	defer srv.Close()
	e, n, err := API{BaseURL: srv.URL, Key: "kluc"}.Customer(context.Background(), "ctm_1")
	if err != nil || e != "a@b.sk" || n != "Firma" {
		t.Fatalf("%s %s %v", e, n, err)
	}
	if _, _, err := (API{BaseURL: srv.URL, Key: "zly"}).Customer(context.Background(), "ctm_1"); err == nil {
		t.Fatal("zlý kľúč")
	}
}

func TestWebhook(t *testing.T) {
	is, _ := issuer(t, &fakeMail{})
	h := &Webhook{Secret: "tajne", Issuer: is, Now: func() time.Time { return now }, Log: log.New(io.Discard, "", 0)}
	ev := event(t, "transaction.completed", "txn_w", "pri_multi", 1, map[string]any{"email": "w@w.sk"})
	body, _ := json.Marshal(ev)
	tests := []struct {
		name, method, sig string
		status            int
	}{
		{"GET", http.MethodGet, "", 405},
		{"bez podpisu", http.MethodPost, "", 401},
		{"zlý podpis", http.MethodPost, Sign("ine", body, now), 401},
		{"platný", http.MethodPost, Sign("tajne", body, now), 200},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, "/paddle/webhook", bytes.NewReader(body))
		if tt.sig != "" {
			req.Header.Set("Paddle-Signature", tt.sig)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tt.status {
			t.Errorf("%s: %d %s", tt.name, rec.Code, rec.Body)
		}
	}
	if _, ok := is.Store.Get("txn_w"); !ok {
		t.Fatal("licencia nevystavená")
	}
}
