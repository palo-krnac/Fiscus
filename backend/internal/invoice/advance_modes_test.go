package invoice

import (
	"strings"
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestAdvanceMode(t *testing.T) {
	base := func() Invoice {
		return Invoice{Kind: KindProforma, Customer: Party{Name: "Odberateľ", Country: "SK"},
			Lines: []Line{ln("Práca", "1", "100", 23)}}
	}
	cases := []struct {
		name string
		edit func(*Invoice)
		want AdvanceMode
	}{
		{"tuzemsko", func(*Invoice) {}, AdvanceReceipt},
		{"tuzemsko do 15 dní", func(i *Invoice) { i.Within15 = true }, AdvanceWithin15},
		{"neplatiteľ", func(i *Invoice) { i.NonVATPayer = true }, AdvanceNonVAT},
		{"cudzia mena", func(i *Invoice) { i.Currency = "USD" }, AdvanceReceipt},
		{"tovar do EÚ", func(i *Invoice) { i.Customer.Country, i.SupplyKind = "DE", SupplyGoods }, AdvanceUntaxed},
		{"vývoz", func(i *Invoice) { i.Customer.Country, i.SupplyKind = "US", SupplyGoods }, AdvanceUntaxed},
		{"služba mimo EÚ", func(i *Invoice) { i.Customer.Country, i.SupplyKind = "CH", SupplyServices }, AdvanceUntaxed},
		{"služba do EÚ", func(i *Invoice) { i.Customer.Country, i.SupplyKind = "AT", SupplyServices }, AdvanceReceipt},
		{"služba do EÚ do 15 dní", func(i *Invoice) { i.Customer.Country, i.SupplyKind, i.Within15 = "AT", SupplyServices, true }, AdvanceWithin15},
		{"tovar do EÚ do 15 dní: aj tak bez dokladu", func(i *Invoice) { i.Customer.Country, i.SupplyKind, i.Within15 = "DE", SupplyGoods, true }, AdvanceUntaxed},
		{"zahraničie bez druhu", func(i *Invoice) { i.Customer.Country = "DE" }, AdvanceManual},
		{"stavebné práce", func(i *Invoice) { i.Lines[0].Reverse = RCConstruction }, AdvanceReceipt},
		{"stavebné práce + materiál s DPH", func(i *Invoice) {
			i.Lines[0].Reverse = RCConstruction
			i.Lines = append(i.Lines, ln("Materiál", "1", "50", 23))
		}, AdvanceReceipt},
		{"prenesenie pri tovare", func(i *Invoice) { i.Lines[0].Reverse = RCPhones }, AdvanceReceipt},
		{"dva druhy prenesenia", func(i *Invoice) {
			i.Lines[0].Reverse = RCConstruction
			l := ln("Šrot", "1", "50", 23)
			l.Reverse = RCScrap
			i.Lines = append(i.Lines, l)
		}, AdvanceReceipt},
		{"prenesenie bez druhu v tuzemsku", func(i *Invoice) { i.ReverseCharge = true }, AdvanceManual},
	}
	for _, c := range cases {
		inv := base()
		c.edit(&inv)
		got, why := inv.AdvanceMode()
		if got != c.want || (got == AdvanceManual) != (why != "") {
			t.Errorf("%s: %s %q", c.name, got, why)
		}
	}
}

func TestUncoveredPayments(t *testing.T) {
	p := paidProforma(t, ln("Práca", "1", "1000", 23))
	p.Status, p.PaidDate = StatusIssued, Date{}
	p.Payments = []Payment{
		{Date: MustDate("2026-10-05"), Amount: m.MustParse("500")},
		{Date: MustDate("2026-10-08"), Amount: m.MustParse("500")},
		{Date: MustDate("2026-10-09"), Amount: m.MustParse("230")},
	}
	p.AddReceipt(ReceiptRef{ID: 1, Number: "ZD1", Date: MustDate("2026-10-08"), Amount: m.MustParse("500")})
	got := p.UncoveredPayments()
	if len(got) != 2 || got[0].Date.String() != "2026-10-05" || got[1].Amount.String() != "230.00" {
		t.Fatalf("nepokryté: %+v", got)
	}
	// stará úhrada bez evidencie platieb: celá suma k dátumu úhrady
	old := paidProforma(t, ln("Práca", "1", "100", 23))
	if u := old.UncoveredPayments(); len(u) != 1 || u[0].Amount.String() != "123.00" || u[0].Date.String() != "2026-10-10" {
		t.Fatalf("stará úhrada: %+v", u)
	}
}

func TestForeignGoodsAdvance(t *testing.T) {
	// tovar do Nemecka 2 000 € (oslobodené), záloha 800 € — platba nezakladá
	// daňovú povinnosť, vyúčtovanie má celý základ a odpočíta iba úhradu
	p := paidProforma(t, ln("Rozvádzač", "2", "1000", 0))
	p.Customer = Party{Name: "Kunde GmbH", Country: "DE", ICDPH: "DE123456789"}
	p.SupplyKind, p.ReverseCharge = SupplyGoods, true
	_ = p.Recalculate()
	p.Status, p.PaidDate = StatusIssued, Date{}
	p.Payments = []Payment{{Date: MustDate("2026-10-05"), Amount: m.MustParse("800")}}
	if p.NeedsReceipt() || p.CanIssueReceipt() {
		t.Fatal("tovar do EÚ nemá doklad k platbe")
	}
	f, err := NewFinalInvoice(p, nil, MustDate("2026-10-20"), 14)
	if err != nil {
		t.Fatal(err)
	}
	d := f.Advance.Deductions
	if len(d) != 1 || d[0].Untaxed != UntaxedForeign || d[0].Total.String() != "800.00" || len(d[0].Rates) != 0 {
		t.Fatalf("odpočet: %+v", d)
	}
	if f.Summary.Base.String() != "2000.00" || f.Advance.Prepaid.String() != "800.00" || f.AmountDue().String() != "1200.00" ||
		f.Advance.Supply != nil || !f.ReverseCharge || f.SupplyKind != SupplyGoods {
		t.Fatalf("vyúčtovanie: %+v prepaid %s due %s", f.Summary, f.Advance.Prepaid, f.AmountDue())
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("vyúčtovanie musí byť platné: %v", err)
	}
	// preplatok: záloha vyššia ako dodávka
	f.Lines[0].Quantity = m.MustParse("0.5")
	_ = f.Recalculate()
	if err := f.Validate(); err == nil || !strings.Contains(err.Error(), "preplatok") {
		t.Fatalf("preplatok: %v", err)
	}
}

func TestReverseChargeReceipts(t *testing.T) {
	// služba do Rakúska: doklad k platbe bez DPH s prenesením (súhrnný výkaz)
	p := paidProforma(t, ln("Programovanie PLC", "10", "50", 23))
	p.Customer = Party{Name: "Kunde GmbH", Country: "AT", ICDPH: "ATU12345678"}
	p.SupplyKind, p.ReverseCharge = SupplyServices, true
	_ = p.Recalculate()
	r, err := NewAdvanceReceipt(p, m.MustParse("300"), MustDate("2026-10-05"), MustDate("2026-10-06"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.ReverseCharge || r.SupplyKind != SupplyServices || !r.Summary.VAT.IsZero() || r.Summary.Base.String() != "300.00" {
		t.Fatalf("služba do EÚ: %+v", r.Summary)
	}

	// stavebné práce 10 000 € s prenesením + materiál 100 € s DPH: platba 6 150 €
	c := paidProforma(t, ln("Stavebné práce", "1", "10000", 23), ln("Materiál", "1", "100", 23))
	c.Lines[0].Reverse = RCConstruction
	c.Customer.ICDPH = "SK2020372640"
	_ = c.Recalculate()
	// 10 000 (prenesenie) + 123 (s DPH) = 10 123 €
	if c.Summary.Total.String() != "10123.00" {
		t.Fatalf("záloha: %+v", c.Summary)
	}
	r, err = NewAdvanceReceipt(c, c.Summary.Total, MustDate("2026-10-05"), MustDate("2026-10-06"))
	if err != nil {
		t.Fatal(err)
	}
	var rc, normal int
	for _, l := range r.Lines {
		if l.Reverse == RCConstruction {
			rc++
		} else {
			normal++
		}
	}
	if rc != 1 || normal != 1 || r.Summary.VAT.String() != "23.00" || !r.DomesticReverse() {
		t.Fatalf("stavebné práce: %+v %+v", r.Lines, r.Summary)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("doklad musí byť platný: %v", err)
	}
	// vyúčtovanie: všetko pokryté zálohou, zostatok 0
	c.Payments = []Payment{{Date: MustDate("2026-10-05"), Amount: c.Summary.Total}}
	r.ID, r.Number = 9, "ZD20260009"
	f, err := NewFinalInvoice(c, []Invoice{r}, MustDate("2026-10-20"), 14)
	if err != nil || !f.Summary.Total.IsZero() || len(f.Summary.Rates) != 0 {
		t.Fatalf("vyúčtovanie: %+v %v", f.Summary, err)
	}
}

func TestWithin15(t *testing.T) {
	p := paidProforma(t, ln("Montáž", "1", "1000", vat.Rate23))
	p.Within15 = true
	p.Status, p.PaidDate = StatusIssued, Date{}
	p.Payments = []Payment{{Date: MustDate("2026-10-05"), Amount: m.MustParse("1230")}}
	if p.NeedsReceipt() || !p.CanIssueReceipt() {
		t.Fatal("do 15 dní: bez dokladu, ale dá sa vystaviť dodatočne")
	}
	f, err := NewFinalInvoice(p, nil, MustDate("2026-10-12"), 14)
	if err != nil {
		t.Fatal(err)
	}
	if f.Advance.Deductions[0].Untaxed != UntaxedWithin15 || f.Summary.VAT.String() != "230.00" || !f.AmountDue().IsZero() {
		t.Fatalf("vyúčtovanie: %+v", f.Advance)
	}
	cases := []struct {
		delivery string
		ok       bool
	}{
		{"2026-10-05", true},
		{"2026-10-20", true},
		{"2026-10-21", false},
		{"2026-10-04", false},
	}
	for _, c := range cases {
		f.DeliveryDate = MustDate(c.delivery)
		if msg := f.Within15Error(); (msg == "") != c.ok {
			t.Errorf("dodanie %s: %q", c.delivery, msg)
		}
	}
	// dodatočný doklad k platbe: odpočet už podľa dokladu
	r, err := NewAdvanceReceipt(p, m.MustParse("1230"), MustDate("2026-10-05"), MustDate("2026-10-25"))
	if err != nil {
		t.Fatal(err)
	}
	r.ID, r.Number = 4, "ZD20260004"
	p.AddReceipt(ReceiptRef{ID: 4, Number: r.Number, Date: MustDate("2026-10-05"), Amount: m.MustParse("1230")})
	f, err = NewFinalInvoice(p, []Invoice{r}, MustDate("2026-11-02"), 14)
	if err != nil || len(f.Advance.Deductions) != 1 || f.Advance.Deductions[0].Untaxed != "" || !f.Summary.Total.IsZero() {
		t.Fatalf("po dodatočnom doklade: %+v %v", f.Advance.Deductions, err)
	}
}
