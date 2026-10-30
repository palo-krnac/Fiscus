package app

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

// issuePaidProforma vystaví zálohu a uhradí ju 10. 10. 2026.
func issuePaidProforma(t *testing.T, a *App, cid int64, d invoice.Invoice, amount string) invoice.Invoice {
	t.Helper()
	dr, err := a.SaveDraft(ctx, cid, d)
	if err != nil {
		t.Fatalf("koncept zálohy: %v", err)
	}
	p, err := a.Issue(ctx, cid, dr.ID)
	if err != nil {
		t.Fatalf("záloha: %v", err)
	}
	p, err = a.AddPayment(ctx, cid, p.ID, invoice.Payment{Date: invoice.MustDate("2026-10-10"), Amount: m.MustParse(amount), Source: invoice.PaymentManual}, false)
	if err != nil {
		t.Fatalf("úhrada: %v", err)
	}
	return p
}

func TestAdvanceEUGoods(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d := proformaDraft() // 10 × 40 € = 400 €
	d.Customer = invoice.Party{Name: "Kunde GmbH", Country: "DE", ICDPH: "DE123456789"}
	d.SupplyKind, d.ReverseCharge = invoice.SupplyGoods, true
	p := issuePaidProforma(t, a, c.ID, d, "150")
	if len(p.AllReceipts()) != 0 {
		t.Fatalf("tovar do EÚ: doklad k platbe nevzniká: %+v", p.Advance)
	}
	if _, err := a.IssueMissingReceipts(ctx, c.ID, p.ID); !fieldErr(err, "advance") {
		t.Fatalf("dodatočný doklad pri tovare do EÚ: %v", err)
	}
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil || f.Summary.Base.String() != "400.00" || f.Advance.Prepaid.String() != "150.00" {
		t.Fatalf("vyúčtovanie: %+v %v", f.Summary, err)
	}
	f, err = a.Issue(ctx, c.ID, f.ID)
	if err != nil || f.Status != invoice.StatusIssued || f.Remaining().String() != "250.00" || f.Payments[0].Source != invoice.PaymentAdvance {
		t.Fatalf("vystavené vyúčtovanie: %+v %v", f.Payments, err)
	}
	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil || ret.Rows["r13"].String() != "400.00" || ret.Rows["r14"].String() != "400.00" || len(ret.SV) != 1 || ret.SV[0].Value.String() != "400.00" {
		t.Fatalf("DPH: %+v SV %+v %v", ret.Rows, ret.SV, err)
	}
}

func TestAdvanceEUServices(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d := proformaDraft()
	d.Customer = invoice.Party{Name: "Kunde GmbH", Country: "AT", ICDPH: "ATU12345678"}
	d.SupplyKind, d.ReverseCharge = invoice.SupplyServices, true
	p := issuePaidProforma(t, a, c.ID, d, "100")
	r, err := a.Store.GetInvoice(ctx, c.ID, p.Advance.ReceiptID)
	if err != nil || !r.ReverseCharge || !r.Summary.VAT.IsZero() || r.Summary.Total.String() != "100.00" {
		t.Fatalf("doklad k platbe s prenesením: %+v %v", r.Summary, err)
	}
	f, _ := a.SettleAdvance(ctx, c.ID, p.ID)
	f.DeliveryDate = invoice.MustDate("2026-11-05")
	if f, err = a.SaveDraft(ctx, c.ID, f); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Issue(ctx, c.ID, f.ID); err != nil || f.Summary.Base.String() != "300.00" {
		t.Fatalf("vyúčtovanie: %+v %v", f.Summary, err)
	}
	// súhrnný výkaz: platba v októbri, zostatok v novembri
	oct, _, _ := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
	nov := vatreturn.Compute(vatreturn.Input{Period: vatreturn.Period{Year: 2026, Month: 11}, Invoices: invs})
	if len(oct.SV) != 1 || oct.SV[0].Value.String() != "100.00" || len(nov.SV) != 1 || nov.SV[0].Value.String() != "300.00" {
		t.Fatalf("SV: %+v / %+v", oct.SV, nov.SV)
	}
}

func TestAdvanceConstruction(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d := proformaDraft()
	d.Customer = invoice.Party{Name: "Stavby s.r.o.", Country: "SK", ICO: "31322832", ICDPH: "SK2020372640"}
	d.Lines[0].Reverse = invoice.RCConstruction // 400 € s prenesením
	p := issuePaidProforma(t, a, c.ID, d, "100")
	r, err := a.Store.GetInvoice(ctx, c.ID, p.Advance.ReceiptID)
	if err != nil || !r.DomesticReverse() || !r.Summary.VAT.IsZero() {
		t.Fatalf("doklad k platbe: %+v %v", r.Lines, err)
	}
	f, _ := a.SettleAdvance(ctx, c.ID, p.ID)
	if f, err = a.Issue(ctx, c.ID, f.ID); err != nil {
		t.Fatal(err)
	}
	// A.2: doklad k platbe 100 €, vyúčtovanie 300 € (nie celých 400 €)
	ret, _, _ := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	got := map[string]string{}
	for _, l := range ret.KV.A2 {
		got[l.Number] = l.Base.String()
	}
	if len(got) != 2 || got[r.Number] != "100.00" || got[f.Number] != "300.00" || !ret.Rows["r03"].IsZero() {
		t.Fatalf("A.2: %+v rows %+v", ret.KV.A2, ret.Rows)
	}
}

func TestAdvanceWithin15(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // mesačný platiteľ
	d := proformaDraft()
	d.Within15 = true
	p := issuePaidProforma(t, a, c.ID, d, "492")
	if len(p.AllReceipts()) != 0 {
		t.Fatal("dodanie do 15 dní: doklad k platbe sa nevystavuje")
	}
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil || f.Summary.VAT.String() != "92.00" || !f.AmountDue().IsZero() {
		t.Fatalf("vyúčtovanie: %+v %v", f.Summary, err)
	}
	// dodanie po 15 dňoch od platby (10. 10.) sa nedá vystaviť
	cases := []struct {
		delivery, want string
	}{
		{"2026-10-26", "do 15 dní"},
		{"2026-11-02", "do 15 dní"},
	}
	for _, tc := range cases {
		f.DeliveryDate = invoice.MustDate(tc.delivery)
		f, _ = a.SaveDraft(ctx, c.ID, f)
		if _, err := a.Issue(ctx, c.ID, f.ID); !fieldErr(err, "deliveryDate") || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("dodanie %s: %v", tc.delivery, err)
		}
	}
	f.DeliveryDate = invoice.MustDate("2026-10-24")
	f, _ = a.SaveDraft(ctx, c.ID, f)
	f, err = a.Issue(ctx, c.ID, f.ID)
	if err != nil || f.Status != invoice.StatusPaid || f.Payments[0].Source != invoice.PaymentAdvance {
		t.Fatalf("vyúčtovanie v lehote: %+v %v", f, err)
	}
	ret, _, _ := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if ret.Rows["r03"].String() != "400.00" || ret.Rows["r04"].String() != "92.00" {
		t.Fatalf("DPH: %+v", ret.Rows)
	}
}

func TestAdvanceWithin15Late(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d := proformaDraft()
	d.Within15 = true
	d.Lines = append(d.Lines, invoice.Line{Description: "Materiál", Quantity: m.MustParse("1"), Unit: "ks", UnitPrice: m.MustParse("100"), Rate: vat.Rate23})
	p := issuePaidProforma(t, a, c.ID, d, "615")
	f, _ := a.SettleAdvance(ctx, c.ID, p.ID)
	f.DeliveryDate = invoice.MustDate("2026-11-03") // iné obdobie aj po lehote
	f, _ = a.SaveDraft(ctx, c.ID, f)
	if _, err := a.Issue(ctx, c.ID, f.ID); !fieldErr(err, "deliveryDate") {
		t.Fatalf("po lehote: %v", err)
	}
	// dodatočný doklad k platbe: rozpracované vyúčtovanie sa prepočíta
	p, err := a.IssueMissingReceipts(ctx, c.ID, p.ID)
	if err != nil || len(p.AllReceipts()) != 1 {
		t.Fatalf("dodatočný doklad: %+v %v", p.Advance, err)
	}
	if _, err := a.IssueMissingReceipts(ctx, c.ID, p.ID); !fieldErr(err, "advance") {
		t.Fatalf("druhýkrát: %v", err)
	}
	f, _ = a.Store.GetInvoice(ctx, c.ID, f.ID)
	if f.Advance.Deductions[0].Untaxed != "" || !f.Summary.Total.IsZero() || !f.Advance.Prepaid.IsZero() {
		t.Fatalf("prepočítané vyúčtovanie: %+v", f.Advance)
	}
	if f, err = a.Issue(ctx, c.ID, f.ID); err != nil {
		t.Fatalf("vyúčtovanie po doklade: %v", err)
	}
	oct, _, _ := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if oct.Rows["r04"].String() != "115.00" {
		t.Fatalf("daň z platby v októbri: %+v", oct.Rows)
	}
}

func TestAdvanceWithin15Period(t *testing.T) {
	// platba 25. 10., dodanie 3. 11. — v lehote 15 dní; mesačný platiteľ má
	// platbu a dodanie v rôznych obdobiach, štvrťročný v tom istom
	for _, tc := range []struct {
		period string
		ok     bool
	}{{"month", false}, {"quarter", true}} {
		a, c := newApp(t, edition.Desktop)
		c.VATPeriod = tc.period
		if err := a.Store.UpdateCompany(ctx, c); err != nil {
			t.Fatal(err)
		}
		d := proformaDraft()
		d.Within15 = true
		dr, _ := a.SaveDraft(ctx, c.ID, d)
		p, _ := a.Issue(ctx, c.ID, dr.ID)
		if _, err := a.AddPayment(ctx, c.ID, p.ID, invoice.Payment{Date: invoice.MustDate("2026-10-25"), Source: invoice.PaymentManual}, true); err != nil {
			t.Fatal(err)
		}
		f, err := a.SettleAdvance(ctx, c.ID, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		f.DeliveryDate, f.IssueDate = invoice.MustDate("2026-11-03"), invoice.MustDate("2026-10-29")
		f, _ = a.SaveDraft(ctx, c.ID, f)
		_, err = a.Issue(ctx, c.ID, f.ID)
		if (err == nil) != tc.ok || (!tc.ok && !strings.Contains(err.Error(), "október 2026")) {
			t.Errorf("%s: %v", tc.period, err)
		}
	}
}

func TestAdvanceForeignCurrency(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if err := a.Store.SaveRates(ctx, []fx.Rate{
		{Currency: "CZK", Date: "2026-09-30", Rate: m.MustParse("25.1"), Source: fx.SourceECB},
		{Currency: "CZK", Date: "2026-10-09", Rate: m.MustParse("25"), Source: fx.SourceECB},
		{Currency: "CZK", Date: "2026-10-19", Rate: m.MustParse("24.6"), Source: fx.SourceECB},
	}); err != nil {
		t.Fatal(err)
	}
	d := proformaDraft() // 400 + 92 = 492 CZK
	d.Currency = "CZK"
	p := issuePaidProforma(t, a, c.ID, d, "492") // platba 10. 10., kurz 25
	r, err := a.Store.GetInvoice(ctx, c.ID, p.Advance.ReceiptID)
	if err != nil || r.Currency != "CZK" || !r.FXRate.Equal(m.MustParse("25")) || !r.Local.VAT.Equal(m.MustParse("3.68")) {
		t.Fatalf("doklad k platbe v CZK: %+v %v", r.Local, err)
	}
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.Lines[0].Quantity = m.MustParse("20") // dodávka 984 CZK
	f.DeliveryDate = invoice.MustDate("2026-10-20")
	if f, err = a.SaveDraft(ctx, c.ID, f); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Issue(ctx, c.ID, f.ID); err != nil {
		t.Fatalf("vyúčtovanie: %v", err)
	}
	// v eurách: 800/24,6 = 32,52; 184/24,6 = 7,48 → mínus záloha 16,00 + 3,68
	if !f.FXRate.Equal(m.MustParse("24.6")) || !f.Local.Base.Equal(m.MustParse("16.52")) || !f.Local.VAT.Equal(m.MustParse("3.8")) {
		t.Fatalf("vyúčtovanie v eurách: %s %+v", f.FXRate, f.Local)
	}
	ret, _, _ := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	// daň spolu: 3,68 (platba) + 3,80 (zostatok) = 7,48 € = celá dodávka kurzom vyúčtovania
	if !ret.Rows["r04"].Equal(m.MustParse("7.48")) || !ret.Rows["r03"].Equal(m.MustParse("32.52")) {
		t.Fatalf("DPH: %+v", ret.Rows)
	}
}

func TestAdvanceReverseGoods(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d := proformaDraft()
	d.Customer = invoice.Party{Name: "Mobil s.r.o.", Country: "SK", ICO: "31322832", ICDPH: "SK2020372640"}
	d.Lines = []invoice.Line{{Description: "Mobilné telefóny", Quantity: m.MustParse("50"), Unit: "ks", UnitPrice: m.MustParse("120"),
		Rate: vat.Rate23, Reverse: invoice.RCPhones}} // 6 000 € s prenesením
	p := issuePaidProforma(t, a, c.ID, d, "1500") // čiastočná záloha
	r, err := a.Store.GetInvoice(ctx, c.ID, p.Advance.ReceiptID)
	if err != nil || len(r.Lines) != 1 || r.Lines[0].A2 == nil || r.Lines[0].A2.Quantity.String() != "50.00" || r.Lines[0].A2.Unit != "ks" {
		t.Fatalf("doklad k platbe: %+v %v", r.Lines, err)
	}
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if f, err = a.Issue(ctx, c.ID, f.ID); err != nil {
		t.Fatal(err)
	}
	ret, _, _ := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	type row struct{ base, qty string }
	got := map[string]row{}
	for _, l := range ret.KV.A2 {
		q := ""
		if l.Quantity != nil {
			q = l.Quantity.String() + " " + l.Unit
		}
		got[l.Number] = row{l.Base.String(), q}
	}
	if len(got) != 2 || got[r.Number] != (row{"1500.00", "50.00 ks"}) || got[f.Number] != (row{"4500.00", ""}) {
		t.Fatalf("A.2: %+v", got)
	}
}
