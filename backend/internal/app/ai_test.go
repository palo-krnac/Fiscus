package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/extract"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func fakeClaude(t *testing.T, input string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","name":"zapis_fakturu","input":` + input + `}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestExtractPurchase(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	a.AIBaseURL = fakeClaude(t, `{"supplierName":"Slovnaft, a.s.","ico":"31322832","invoiceNumber":"FV-2026/0815",
		"issueDate":"2026-10-05","rates":[{"vatRate":23,"base":"81.30","vat":"18.70"}],"total":"100.00"}`)
	pdf := []byte("%PDF-1.7 faktúra")
	if _, err := a.ExtractPurchase(ctx, c.ID, "f.pdf", pdf); !errors.Is(err, extract.ErrOff) {
		t.Fatalf("bez kľúča: %v", err)
	}
	t.Setenv("FISKUS_ANTHROPIC_API_KEY", "sk-env")
	ct := store.Contact{Name: "Slovnaft, a.s.", ICO: "31322832", IBAN: "SK3112000000198742637541"}
	if err := a.Store.SaveContact(ctx, c.ID, &ct); err != nil {
		t.Fatal(err)
	}
	d, err := a.ExtractPurchase(ctx, c.ID, "f.pdf", pdf)
	if err != nil || d.Purchase.ContactID != ct.ID || d.Purchase.SupplierIBAN != ct.IBAN || len(d.Warnings) != 0 {
		t.Fatalf("koncept: %+v %v", d, err)
	}
	// rovnaká faktúra už je zaevidovaná
	if _, _, err := a.SavePurchase(ctx, c.ID, validPurchase()); err != nil {
		t.Fatal(err)
	}
	d, _ = a.ExtractPurchase(ctx, c.ID, "f.pdf", pdf)
	if len(d.Warnings) != 1 || !strings.Contains(d.Warnings[0], "už je zaevidovaná") {
		t.Fatalf("duplicita: %v", d.Warnings)
	}
}

func TestAISettingsPermissions(t *testing.T) {
	o := newOffice(t)
	if _, err := o.a.SetAISettings(as(o.jana), extract.Settings{APIKey: "x"}, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("účtovníčka nastavuje AI: %v", err)
	}
	s, err := o.a.SetAISettings(as(o.admin), extract.Settings{APIKey: "sk-ant-1", Model: "claude-x"}, false)
	if err != nil || !s.KeySet || s.APIKey != "" || s.Model != "claude-x" {
		t.Fatalf("nastavenie: %+v %v", s, err)
	}
	if s, _ := o.a.SetAISettings(as(o.admin), extract.Settings{Model: "claude-y"}, false); !s.KeySet {
		t.Fatal("kľúč sa stratil pri zmene modelu")
	}
	if s, _ := o.a.SetAISettings(as(o.admin), extract.Settings{}, true); s.KeySet || s.Model != extract.DefaultModel {
		t.Fatalf("zmazanie kľúča: %+v", s)
	}
	// klient (iba čítanie prijatých) doklady nečíta
	if _, err := o.a.ExtractPurchase(as(o.klient), o.beta.ID, "f.pdf", []byte("%PDF")); !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrReadOnly) {
		t.Fatalf("klient: %v", err)
	}
}
