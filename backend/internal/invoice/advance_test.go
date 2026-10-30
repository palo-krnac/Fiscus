package invoice

import (
	"errors"
	"strings"
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
	r, err := NewAdvanceReceipt(p, p.Summary.Total, p.PaidDate, MustDate("2026-10-12"))
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
		want string
	}{
		{"bez dátumu platby", func(p *Invoice) { p.PaidDate = Date{} }, ErrAdvanceNotPaid.Error()},
		{"bežná faktúra", func(p *Invoice) { p.Kind = KindInvoice }, ErrNotAdvance.Error()},
		{"cudzia mena", func(p *Invoice) { p.Currency = "CZK" }, "cudzej mene"},
		{"tovar do EÚ", func(p *Invoice) { p.Customer.Country, p.SupplyKind = "DE", SupplyGoods }, "nevyhotovuje"},
		{"zahraničie bez druhu plnenia", func(p *Invoice) { p.Customer.Country = "DE" }, "tovar alebo službu"},
		{"prenesenie bez druhu", func(p *Invoice) { p.ReverseCharge = true }, "§ 69 ods. 12"},
	}
	for _, c := range cases {
		x := p
		c.edit(&x)
		if _, err := NewAdvanceReceipt(x, x.Summary.Total, x.PaidDate, MustDate("2026-10-12")); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := NewAdvanceReceipt(p, m.Zero, p.PaidDate, MustDate("2026-10-12")); !errors.Is(err, ErrAdvanceNotPaid) {
		t.Errorf("nulová platba: %v", err)
	}
}

func TestPartialAdvanceReceipts(t *testing.T) {
	// záloha 1 261,50 € (1 230 € v 23 % + 31,50 € v 5 %) zaplatená na dva razy
	p := paidProforma(t, ln("Rozvádzač", "1", "1000", 23), ln("Literatúra", "2", "15", 5))
	p.Status, p.PaidDate = StatusIssued, Date{}
	first, err := NewAdvanceReceipt(p, m.MustParse("500"), MustDate("2026-10-05"), MustDate("2026-10-06"))
	if err != nil {
		t.Fatal(err)
	}
	// 500 × 1230 / 1261,50 = 487,51 v 23 %, zvyšok 12,49 v 5 %
	if first.DeliveryDate.String() != "2026-10-05" || first.Summary.Total.String() != "500.00" || len(first.Summary.Rates) != 2 ||
		first.Summary.Rates[0].Total.String() != "487.51" || first.Summary.Rates[1].Total.String() != "12.49" ||
		first.Summary.Rates[0].VAT.String() != "91.16" {
		t.Fatalf("prvá platba: %+v", first.Summary)
	}
	second, _ := NewAdvanceReceipt(p, m.MustParse("761.50"), MustDate("2026-10-12"), MustDate("2026-10-12"))
	first.ID, first.Number, second.ID, second.Number = 11, "ZD20260001", 12, "ZD20260002"

	// vyúčtovanie po prvej platbe: odpočet iba prijatej časti, zvyšok na úhradu
	p.Payments = []Payment{{Date: MustDate("2026-10-05"), Amount: m.MustParse("500")}}
	f, err := NewFinalInvoice(p, []Invoice{first}, MustDate("2026-10-20"), 14)
	if err != nil || f.Summary.Total.String() != "761.50" || len(f.Advance.Deductions) != 1 {
		t.Fatalf("po prvej platbe: %+v %v", f.Summary, err)
	}
	// chýba doklad k druhej platbe
	p.Payments = append(p.Payments, Payment{Date: MustDate("2026-10-12"), Amount: m.MustParse("761.50")})
	p.Status = StatusPaid
	if _, err := NewFinalInvoice(p, []Invoice{first}, MustDate("2026-10-20"), 14); err == nil {
		t.Fatal("platba bez dokladu")
	}
	f, err = NewFinalInvoice(p, []Invoice{first, second}, MustDate("2026-10-20"), 14)
	if err != nil || !f.Summary.Total.IsZero() || len(f.Advance.Deductions) != 2 || f.Advance.Deductions[1].Number != "ZD20260002" {
		t.Fatalf("po dvoch platbách: %+v %v", f.Summary, err)
	}
	// nič nezaplatené → vyúčtovať sa nedá
	p.Payments, p.Status = nil, StatusIssued
	if _, err := NewFinalInvoice(p, nil, MustDate("2026-10-20"), 14); !errors.Is(err, ErrAdvanceNotPaid) {
		t.Fatalf("bez platby: %v", err)
	}
}

func TestAdvanceReceiptsAndSettled(t *testing.T) {
	p := paidProforma(t, ln("Montáž", "1", "100", 23))
	p.Advance = &Advance{ReceiptID: 4, ReceiptNumber: "ZD1"}
	if got := p.AllReceipts(); len(got) != 1 || got[0].ID != 4 || !got[0].Amount.Equal(p.Summary.Total) {
		t.Fatalf("staré údaje: %+v", got)
	}
	p.AddReceipt(ReceiptRef{ID: 5, Number: "ZD2", Amount: m.MustParse("10")})
	if got := p.AllReceipts(); len(got) != 2 || p.Advance.ReceiptID != 4 {
		t.Fatalf("pridanie: %+v", got)
	}
	n := Invoice{Kind: KindProforma}
	n.AddReceipt(ReceiptRef{ID: 8, Number: "ZD8"})
	if n.Advance.ReceiptID != 8 || len(n.AllReceipts()) != 1 {
		t.Fatalf("prvý doklad: %+v", n.Advance)
	}
	// vyúčtovaná záloha nie je po splatnosti a ďalšiu platbu neprijme
	p.Status, p.Payments = StatusIssued, []Payment{{Date: MustDate("2026-10-10"), Amount: m.MustParse("50")}}
	p.Advance.FinalID, p.Advance.FinalNumber = 9, "FA20260009"
	if !p.Settled() || p.Overdue(MustDate("2026-12-31")) {
		t.Fatal("vyúčtovaná záloha po splatnosti")
	}
	if err := p.AddPayment(Payment{Date: MustDate("2026-10-11"), Amount: m.MustParse("10")}); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("platba po vyúčtovaní: %v", err)
	}
}

func TestFinalInvoice(t *testing.T) {
	// ceny bez DPH, ktoré pri výpočte „zhora“ dávajú inú daň o cent
	p := paidProforma(t, ln("Montáž", "3", "33.33", 23))
	r, _ := NewAdvanceReceipt(p, p.Summary.Total, p.PaidDate, MustDate("2026-10-10"))
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
		f, err := NewFinalInvoice(p, []Invoice{r}, MustDate("2026-10-20"), 14)
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
	f, _ := NewFinalInvoice(p, []Invoice{r}, MustDate("2026-10-20"), 14)
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
	if _, err := NewFinalInvoice(settled, []Invoice{r}, MustDate("2026-10-20"), 14); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("druhé vyúčtovanie: %v", err)
	}
	foreign := p
	foreign.Currency = "CZK"
	if _, err := NewFinalInvoice(foreign, []Invoice{r}, MustDate("2026-10-20"), 14); err == nil || err.Error() != ErrForeignAdvance.Error() {
		t.Fatalf("cudzia mena: %v", err)
	}
}

func TestAddRemoveAdvance(t *testing.T) {
	p1 := paidProforma(t, ln("Rozvádzač", "1", "1000", 23))
	p2 := paidProforma(t, ln("Montáž", "1", "200", 23))
	p2.ID, p2.Number = 8, "ZF20260004"
	r1, _ := NewAdvanceReceipt(p1, p1.Summary.Total, p1.PaidDate, p1.PaidDate)
	r2, _ := NewAdvanceReceipt(p2, p2.Summary.Total, p2.PaidDate, p2.PaidDate)
	r1.ID, r1.Number, r2.ID, r2.Number = 11, "ZD1", 12, "ZD2"
	f, err := NewFinalInvoice(p1, []Invoice{r1}, MustDate("2026-10-20"), 14)
	if err != nil {
		t.Fatal(err)
	}
	if f.Advance.Deductions[0].ProformaID != 7 {
		t.Fatalf("odpočet bez zálohy: %+v", f.Advance.Deductions)
	}
	other := p2
	other.Customer.Name = "Iný odberateľ"
	tests := []struct {
		name string
		p    Invoice
		want string
	}{
		{"iný odberateľ", other, "inému odberateľovi"},
		{"tá istá záloha", p1, "už je vo vyúčtovaní"},
	}
	for _, tt := range tests {
		x := f
		x.Advance = &Advance{ProformaID: f.Advance.ProformaID, Deductions: append([]Deduction(nil), f.Advance.Deductions...)}
		if err := x.AddAdvance(tt.p, []Invoice{r2}); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
	if err := f.AddAdvance(p2, []Invoice{r2}); err != nil {
		t.Fatal(err)
	}
	if ids := f.ProformaIDs(); len(ids) != 2 || ids[1] != 8 || len(f.Lines) != 2 || !f.Summary.Total.IsZero() ||
		!strings.Contains(f.Note, "ZF20260004") {
		t.Fatalf("po pridaní: %v %+v", ids, f.Summary)
	}
	if err := f.RemoveAdvance(7); err == nil {
		t.Fatal("pôvodnú zálohu nemožno odobrať")
	}
	if err := f.RemoveAdvance(8); err != nil || len(f.Advance.Deductions) != 1 || f.Summary.Total.String() != "246.00" {
		t.Fatalf("po odobraní: %v %+v", err, f.Summary)
	}
	if err := f.RemoveAdvance(8); err == nil {
		t.Fatal("dvakrát odobraná")
	}
	issued := f
	issued.Status = StatusIssued
	if err := issued.AddAdvance(p2, []Invoice{r2}); err == nil {
		t.Fatal("pridanie do vystavenej")
	}
	// IČO rozhoduje pred názvom
	a := Invoice{Customer: Party{ICO: "123", Name: "A"}}
	if !a.SameCustomer(Invoice{Customer: Party{ICO: "123", Name: "A s.r.o."}}) || a.SameCustomer(Invoice{Customer: Party{ICO: "999", Name: "A"}}) {
		t.Fatal("rovnaký odberateľ podľa IČO")
	}
}
