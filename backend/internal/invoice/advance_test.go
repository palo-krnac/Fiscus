package invoice

import (
	"errors"
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func paidProforma(t *testing.T, lines ...Line) Invoice {
	t.Helper()
	p := Invoice{ID: 7, Kind: KindProforma, Number: "ZF20260003", VariableSymbol: "20260003", Status: StatusPaid,
		Customer: Party{Name: "Odberateľ s.r.o.", Country: "SK"}, PriceMode: vat.PriceNet,
		IssueDate: MustDate("2026-10-01"), DeliveryDate: MustDate("2026-10-01"), DueDate: MustDate("2026-10-15"),
		PaidDate: MustDate("2026-10-10"), Lines: lines}
	if err := p.Recalculate(); err != nil {
		t.Fatal(err)
	}
	return p
}

func ln(desc, qty, price string, rate vat.Rate) Line {
	return Line{Description: desc, Quantity: m.MustParse(qty), Unit: "ks", UnitPrice: m.MustParse(price), Rate: rate}
}

func TestAdvanceReceipt(t *testing.T) {
	p := paidProforma(t, ln("Rozvádzač", "1", "1000", 23), ln("Literatúra", "2", "15", 5))
	r, err := NewAdvanceReceipt(p, MustDate("2026-10-12"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != KindAdvanceReceipt || r.DeliveryDate.String() != "2026-10-10" || r.IssueDate.String() != "2026-10-12" ||
		r.Advance.ProformaID != 7 || r.Advance.ProformaNumber != "ZF20260003" || r.VariableSymbol != "20260003" {
		t.Fatalf("hlavička: %+v", r)
	}
	// daň z prijatej platby po sadzbách: 1230 € → 1000 + 230; 31,50 € → 30 + 1,50
	if !r.Summary.Total.Equal(p.Summary.Total) || len(r.Summary.Rates) != 2 ||
		r.Summary.Rates[0].Base.String() != "1000.00" || r.Summary.Rates[0].VAT.String() != "230.00" ||
		r.Summary.Rates[1].Base.String() != "30.00" || r.Summary.Rates[1].VAT.String() != "1.50" {
		t.Fatalf("rekapitulácia: %+v", r.Summary)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("doklad musí byť platný: %v", err)
	}

	cases := []struct {
		name string
		edit func(*Invoice)
		want error
	}{
		{"neuhradená", func(p *Invoice) { p.Status = StatusIssued }, ErrAdvanceNotPaid},
		{"bežná faktúra", func(p *Invoice) { p.Kind = KindInvoice }, ErrNotAdvance},
		{"zahraničie", func(p *Invoice) { p.Customer.Country = "DE" }, ErrForeignAdvance},
		{"prenesenie", func(p *Invoice) { p.ReverseCharge = true }, ErrForeignAdvance},
	}
	for _, c := range cases {
		x := p
		c.edit(&x)
		if _, err := NewAdvanceReceipt(x, MustDate("2026-10-12")); !errors.Is(err, c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestFinalInvoice(t *testing.T) {
	// ceny bez DPH, ktoré pri výpočte „zhora“ dávajú inú daň o cent
	p := paidProforma(t, ln("Montáž", "3", "33.33", 23))
	r, _ := NewAdvanceReceipt(p, MustDate("2026-10-10"))
	r.ID, r.Number = 9, "ZD20260001"

	cases := []struct {
		name      string
		extra     []Line
		total     string
		rates     int
		wantError bool
	}{
		{"rovnaká dodávka — nič na úhradu", nil, "0.00", 0, false},
		{"viac práce", []Line{ln("Práca navyše", "5", "40", 23)}, "246.00", 1, false},
		{"iná sadzba", []Line{ln("Kniha", "1", "20", 5)}, "21.00", 1, false},
	}
	for _, c := range cases {
		f, err := NewFinalInvoice(p, &r, MustDate("2026-10-20"), 14)
		if err != nil {
			t.Fatal(err)
		}
		f.Lines = append(f.Lines, c.extra...)
		if err := f.Recalculate(); err != nil {
			t.Fatal(err)
		}
		if f.Summary.Total.String() != c.total || len(f.Summary.Rates) != c.rates {
			t.Errorf("%s: %s %+v", c.name, f.Summary.Total, f.Summary.Rates)
		}
		if f.Advance.Supply == nil || !f.Advance.Supply.Total.Equal(f.Summary.Total.Add(r.Summary.Total)) {
			t.Errorf("%s: celá dodávka %+v", c.name, f.Advance.Supply)
		}
		if f.Advance.Deductions[0].Number != "ZD20260001" || f.Advance.Deductions[0].ReceiptID != 9 {
			t.Errorf("%s: odpočet %+v", c.name, f.Advance.Deductions)
		}
		if err := f.Validate(); (err != nil) != c.wantError {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// presný odpočet: zostatok sadzby 23 % je presne rozdiel dokladov, nie prepočet
	f, _ := NewFinalInvoice(p, &r, MustDate("2026-10-20"), 14)
	if len(f.Summary.Rates) != 0 || !f.Summary.VAT.IsZero() {
		t.Fatalf("100 %% záloha: %+v", f.Summary)
	}

	// menšia dodávka ako záloha → preplatok sa vracia dobropisom
	f.Lines[0].Quantity = m.MustParse("1")
	_ = f.Recalculate()
	var ve *ValidationError
	if err := f.Validate(); !errors.As(err, &ve) || ve.Fields[0].Field != "lines" {
		t.Fatalf("preplatok: %v", err)
	}

	// neplatiteľ DPH: odpočíta sa uhradená suma
	n := paidProforma(t, ln("Práca", "10", "20", 0))
	n.NonVATPayer = true
	_ = n.Recalculate()
	fn, err := NewFinalInvoice(n, nil, MustDate("2026-10-20"), 14)
	if err != nil || !fn.Summary.Total.IsZero() || fn.Advance.Deductions[0].Number != "ZF20260003" {
		t.Fatalf("neplatiteľ: %+v %v", fn.Summary, err)
	}
	// platiteľ bez dokladu k platbe
	if _, err := NewFinalInvoice(p, nil, MustDate("2026-10-20"), 14); err == nil {
		t.Fatal("platiteľ bez dokladu k prijatej platbe")
	}
	settled := p
	settled.Advance = &Advance{FinalID: 3}
	if _, err := NewFinalInvoice(settled, &r, MustDate("2026-10-20"), 14); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("druhé vyúčtovanie: %v", err)
	}
	foreign := p
	foreign.Customer.Country = "CZ"
	if _, err := NewFinalInvoice(foreign, &r, MustDate("2026-10-20"), 14); !errors.Is(err, ErrForeignAdvance) {
		t.Fatalf("zahraničie: %v", err)
	}
}
