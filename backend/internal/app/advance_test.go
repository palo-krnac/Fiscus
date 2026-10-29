package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

func proformaDraft() invoice.Invoice {
	d := validDraft() // 10 × 40 € (23 %) = 492 €
	d.Kind = invoice.KindProforma
	d.Customer.Country = "SK"
	d.IssueDate, d.DeliveryDate, d.DueDate = invoice.MustDate("2026-10-01"), invoice.MustDate("2026-10-01"), invoice.MustDate("2026-10-08")
	return d
}

func TestAdvanceFlow(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	dr, err := a.SaveDraft(ctx, c.ID, proformaDraft())
	if err != nil || dr.Kind != invoice.KindProforma {
		t.Fatalf("koncept zálohy: %+v %v", dr, err)
	}
	p, err := a.Issue(ctx, c.ID, dr.ID)
	if err != nil || p.Number != "ZF20260001" {
		t.Fatalf("záloha: %s %v", p.Number, err)
	}
	// záloha je v návrhoch párovania platieb
	cands, _ := a.candidates(ctx, c.ID)
	if len(cands) != 1 || cands[0].ID != p.ID || !cands[0].Amount.Equal(m.MustParse("492")) {
		t.Fatalf("kandidáti: %+v", cands)
	}

	// úhrada 10. 10. → daňový doklad k prijatej platbe
	p, err = a.MarkPaid(ctx, c.ID, p.ID, invoice.MustDate("2026-10-10"))
	if err != nil || p.Status != invoice.StatusPaid || p.Advance == nil || p.Advance.ReceiptNumber != "ZD20260001" {
		t.Fatalf("úhrada zálohy: %+v %v", p.Advance, err)
	}
	r, _ := a.Store.GetInvoice(ctx, c.ID, p.Advance.ReceiptID)
	if r.Kind != invoice.KindAdvanceReceipt || r.Status != invoice.StatusPaid || r.DeliveryDate.String() != "2026-10-10" ||
		r.Summary.VAT.String() != "92.00" || r.Advance.ProformaID != p.ID {
		t.Fatalf("doklad k platbe: %+v", r)
	}
	if _, err := a.Unpay(ctx, c.ID, p.ID); !errors.Is(err, ErrReceiptIssued) {
		t.Fatalf("zrušenie úhrady s dokladom: %v", err)
	}

	// priznanie za október: doklad k platbe áno, záloha nie
	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil || ret.Rows["r03"].String() != "400.00" || ret.Rows["r04"].String() != "92.00" {
		t.Fatalf("DPH 10/2026: %+v %v", ret.Rows, err)
	}

	// vyúčtovanie: k zálohe pribudne práca za 100 € bez DPH
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil || f.Kind != invoice.KindInvoice || f.Status != invoice.StatusDraft || f.Advance == nil || !f.Summary.Total.IsZero() {
		t.Fatalf("vyúčtovanie: %+v %v", f, err)
	}
	if _, err := a.SettleAdvance(ctx, c.ID, p.ID); !fieldErr(err, "advance") {
		t.Fatalf("druhé vyúčtovanie: %v", err)
	}
	f.Lines = append(f.Lines, invoice.Line{Description: "Práca navyše", Quantity: m.MustParse("1"), Unit: "hod", UnitPrice: m.MustParse("100"), Rate: vat.Rate23})
	f.Advance = nil // klient nevie odpočet zmeniť ani zrušiť
	if f, err = a.SaveDraft(ctx, c.ID, f); err != nil || f.Advance == nil || f.Summary.Total.String() != "123.00" {
		t.Fatalf("úprava vyúčtovania: %+v %v", f.Summary, err)
	}
	f, err = a.Issue(ctx, c.ID, f.ID)
	if err != nil || f.Status != invoice.StatusIssued {
		t.Fatalf("vystavenie vyúčtovania: %v", err)
	}
	if p, _ = a.Store.GetInvoice(ctx, c.ID, p.ID); p.Advance.FinalNumber != f.Number {
		t.Fatalf("väzba na vyúčtovanie: %+v", p.Advance)
	}
	// do DPH ide iba zostatok
	nov, _, _ := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if nov.Rows["r03"].String() != "500.00" || nov.Rows["r04"].String() != "115.00" {
		t.Fatalf("DPH po vyúčtovaní: %+v", nov.Rows)
	}
	var a1 []vatreturn.KVLine
	for _, l := range nov.KV.A1 {
		if l.Number == f.Number {
			a1 = append(a1, l)
		}
	}
	if len(a1) != 0 { // odberateľ bez IČ DPH → D.2, nie A.1
		t.Fatalf("A.1: %+v", a1)
	}
}

func TestAdvanceFullyPaid(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	pd := proformaDraft()
	pd.Customer.ICDPH = "SK2120461244" // platiteľ DPH → kontrolný výkaz A.1
	dr, _ := a.SaveDraft(ctx, c.ID, pd)
	p, _ := a.Issue(ctx, c.ID, dr.ID)
	p, _ = a.MarkPaid(ctx, c.ID, p.ID, invoice.MustDate("2026-10-10"))
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	f, err = a.Issue(ctx, c.ID, f.ID)
	if err != nil || f.Status != invoice.StatusPaid || len(f.Summary.Rates) != 0 {
		t.Fatalf("vyúčtovanie 100 %% zálohy: %+v %v", f, err)
	}
	// KV A.1: doklad k prijatej platbe áno, vyúčtovanie pri 100 % zálohe nie
	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil {
		t.Fatal(err)
	}
	var nums []string
	for _, l := range ret.KV.A1 {
		nums = append(nums, l.Number)
	}
	if len(nums) != 1 || nums[0] != "ZD20260001" || ret.Rows["r04"].String() != "92.00" {
		t.Fatalf("A.1: %v, r04 %s", nums, ret.Rows["r04"])
	}
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{Status: invoice.StatusIssued})
	if len(invs) != 0 {
		t.Fatalf("ostali neuhradené doklady: %d", len(invs))
	}
}

func TestAdvanceNonVATPayer(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	co, _ := a.Store.GetCompany(ctx, c.ID)
	co.VATPayer, co.ICDPH = false, ""
	if _, err := a.UpdateCompany(ctx, co); err != nil {
		t.Fatal(err)
	}
	dr, _ := a.SaveDraft(ctx, c.ID, proformaDraft())
	p, _ := a.Issue(ctx, c.ID, dr.ID)
	p, err := a.MarkPaid(ctx, c.ID, p.ID, invoice.MustDate("2026-10-10"))
	if err != nil || (p.Advance != nil && p.Advance.ReceiptID != 0) {
		t.Fatalf("neplatiteľ nemá doklad k platbe: %+v %v", p.Advance, err)
	}
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil || !f.Summary.Total.IsZero() || f.Advance.Deductions[0].Number != p.Number {
		t.Fatalf("vyúčtovanie neplatiteľa: %+v %v", f.Summary, err)
	}
}

func TestAdvanceReceiptNotFromClient(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d := validDraft()
	d.Kind = invoice.KindAdvanceReceipt
	if _, err := a.SaveDraft(ctx, c.ID, d); !fieldErr(err, "kind") {
		t.Fatalf("doklad k platbe z klienta: %v", err)
	}
	d.Kind = invoice.KindInvoice
	d.Advance = &invoice.Advance{Deductions: []invoice.Deduction{{Number: "X", Total: m.MustParse("100")}}}
	if s, err := a.SaveDraft(ctx, c.ID, d); err != nil || s.Advance != nil {
		t.Fatalf("odpočet z klienta: %+v %v", s.Advance, err)
	}
}

func TestPartialAdvance(t *testing.T) {
	a, c := newApp(t, edition.Desktop)               // dnes 29. 10. 2026
	dr, _ := a.SaveDraft(ctx, c.ID, proformaDraft()) // 492 €
	p, err := a.Issue(ctx, c.ID, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	// prvá časť: daňový doklad k 246 € (daň 46 €), záloha ostáva neuhradená
	p, err = a.AddPayment(ctx, c.ID, p.ID, invoice.Payment{Date: invoice.MustDate("2026-10-05"), Amount: m.MustParse("246"), Source: invoice.PaymentManual}, false)
	if err != nil || p.Status != invoice.StatusIssued || len(p.AllReceipts()) != 1 || p.Advance.ReceiptNumber != "ZD20260001" {
		t.Fatalf("prvá platba: %+v %v", p.Advance, err)
	}
	r1, _ := a.Store.GetInvoice(ctx, c.ID, p.AllReceipts()[0].ID)
	if r1.Summary.Total.String() != "246.00" || r1.Summary.VAT.String() != "46.00" || r1.DeliveryDate.String() != "2026-10-05" || r1.Status != invoice.StatusPaid {
		t.Fatalf("doklad k prvej platbe: %+v", r1.Summary)
	}
	if _, err := a.RemovePayment(ctx, c.ID, p.ID, invoice.PaymentManual, 0); !errors.Is(err, ErrReceiptIssued) {
		t.Fatalf("zrušenie platby s dokladom: %v", err)
	}

	// vyúčtovanie po prvej platbe: odpočíta sa 246 €, doplatok 246 €
	f, err := a.SettleAdvance(ctx, c.ID, p.ID)
	if err != nil || f.Summary.Total.String() != "246.00" || len(f.Advance.Deductions) != 1 {
		t.Fatalf("vyúčtovanie: %+v %v", f.Summary, err)
	}
	// po koncepte vyúčtovania už ďalšia platba k zálohe nejde
	if _, err := a.MarkPaid(ctx, c.ID, p.ID, invoice.MustDate("2026-10-20")); !errors.Is(err, invoice.ErrAlreadySettled) {
		t.Fatalf("platba po vyúčtovaní: %v", err)
	}
	final, err := a.Issue(ctx, c.ID, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, _ = a.Store.GetInvoice(ctx, c.ID, p.ID)
	if !p.Settled() || p.Overdue(a.Today()) || p.Advance.FinalNumber != final.Number {
		t.Fatalf("vyúčtovaná záloha: %+v", p.Advance)
	}
	// vyúčtovaná záloha nie je v návrhoch párovania, vyúčtovanie áno
	cands, _ := a.candidates(ctx, c.ID)
	if len(cands) != 1 || cands[0].ID != final.ID {
		t.Fatalf("kandidáti: %+v", cands)
	}
	// priznanie: daň z prijatej časti zálohy a zo zvyšku na vyúčtovaní
	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil || ret.Rows["r04"].String() != "92.00" {
		t.Fatalf("DPH: %+v %v", ret.Rows, err)
	}
}
