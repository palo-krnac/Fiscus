package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestPaymentOrder(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	save := func(mod func(*purchase.Purchase)) purchase.Purchase {
		t.Helper()
		p := validPurchase()
		mod(&p)
		p, _, err := a.SavePurchase(ctx, c.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	ok1 := save(func(p *purchase.Purchase) { p.DueDate = invoice.MustDate("2026-11-05") })
	ok2 := save(func(p *purchase.Purchase) { p.SupplierNumber = "FV-2"; p.DueDate = invoice.MustDate("2026-10-20") })
	noIBAN := save(func(p *purchase.Purchase) { p.SupplierNumber = "FV-3"; p.SupplierIBAN = "" })
	contact, err := a.SaveContact(ctx, c.ID, store.Contact{Name: "Z adresára s.r.o.", IBAN: "SK3112000000198742637541"})
	if err != nil {
		t.Fatal(err)
	}
	fromContact := save(func(p *purchase.Purchase) { p.SupplierNumber = "FV-4"; p.SupplierIBAN = ""; p.ContactID = contact.ID })
	cash := save(func(p *purchase.Purchase) { p.SupplierNumber = "FV-5"; p.PaymentMethod = invoice.PayCash })
	paid := save(func(p *purchase.Purchase) { p.SupplierNumber = "FV-6" })
	if _, err := a.MarkPurchasePaid(ctx, c.ID, paid.ID, invoice.Date{}); err != nil {
		t.Fatal(err)
	}

	cands, err := a.PaymentCandidates(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	problem := map[int64]string{}
	for _, x := range cands {
		problem[x.ID] = x.Problem
	}
	if len(cands) != 5 || cands[4].ID != ok1.ID || cands[4].Overdue || cands[3].ID != ok2.ID || !cands[3].Overdue {
		t.Fatalf("kandidáti: %+v", cands)
	}
	if problem[ok1.ID] != "" || problem[fromContact.ID] != "" || !strings.Contains(problem[noIBAN.ID], "IBAN") || !strings.Contains(problem[cash.ID], "hotovosti") {
		t.Fatalf("problémy: %v", problem)
	}
	if _, ok := problem[paid.ID]; ok {
		t.Fatal("uhradená medzi kandidátmi")
	}

	var poe *PaymentOrderError
	// firma bez IBAN
	if _, _, err := a.PaymentOrder(ctx, c.ID, []int64{ok1.ID}, invoice.Date{}); !errors.As(err, &poe) || !strings.Contains(poe.Msg, "IBAN firmy") {
		t.Fatalf("bez IBAN firmy: %v", err)
	}
	c.IBAN = "SK31 1200 0000 1987 4263 7541"
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		ids  []int64
		on   string
		want string
	}{
		{"nič nevybrané", nil, "", "aspoň jednu"},
		{"bez IBAN", []int64{ok1.ID, noIBAN.ID}, "", "Chýba IBAN"},
		{"uhradená", []int64{paid.ID}, "", "uhradená"},
		{"minulosť", []int64{ok1.ID}, "2026-10-01", "minulosti"},
	}
	for _, tt := range tests {
		var on invoice.Date
		if tt.on != "" {
			on = invoice.MustDate(tt.on)
		}
		if _, _, err := a.PaymentOrder(ctx, c.ID, tt.ids, on); !errors.As(err, &poe) || !strings.Contains(poe.Msg, tt.want) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
	name, data, err := a.PaymentOrder(ctx, c.ID, []int64{ok1.ID, ok2.ID, fromContact.ID, ok1.ID}, invoice.MustDate("2026-10-30"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if name != "prikaz-2026-10-30.xml" || strings.Count(s, "<CdtTrfTxInf>") != 3 || !strings.Contains(s, "<CtrlSum>300.00</CtrlSum>") ||
		!strings.Contains(s, "<EndToEndId>/VS20260815/SS/KS</EndToEndId>") || !strings.Contains(s, "<Ustrd>Faktura FV-2</Ustrd>") {
		t.Fatalf("%s\n%s", name, s)
	}
	_ = m.Zero
}
