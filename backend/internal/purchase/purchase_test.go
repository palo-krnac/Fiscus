package purchase

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func sample() Purchase {
	return Purchase{
		SupplierNumber: "2026100123",
		Supplier:       invoice.Party{Name: "Slovnaft, a.s.", ICO: "31322832", ICDPH: "SK2020372640", Country: "SK"},
		IssueDate:      invoice.MustDate("2026-10-05"),
		ReceivedDate:   invoice.MustDate("2026-10-07"),
		DueDate:        invoice.MustDate("2026-10-19"),
		Rates:          []RateRow{{Rate: vat.Rate23, Base: m.MustParse("100")}},
	}
}

func fields(err error) map[string]bool {
	out := map[string]bool{}
	var ve *invoice.ValidationError
	if errors.As(err, &ve) {
		for _, f := range ve.Fields {
			out[f.Field] = true
		}
	}
	return out
}

func TestNormalize(t *testing.T) {
	p := sample()
	p.Rates = append(p.Rates, RateRow{Rate: vat.Rate5, Base: m.MustParse("20"), VAT: m.MustParse("1.00")})
	warns := p.Normalize()
	if len(warns) != 0 {
		t.Fatalf("bez upozornení: %v", warns)
	}
	if p.Rates[0].VAT.String() != "23.00" { // dopočítaná
		t.Fatalf("DPH %s", p.Rates[0].VAT)
	}
	if p.Summary.Base.String() != "120.00" || p.Summary.VAT.String() != "24.00" || p.Summary.Total.String() != "144.00" {
		t.Fatalf("%+v", p.Summary)
	}
	if p.DeliveryDate != p.IssueDate || p.Status != StatusBooked || p.PaymentMethod != invoice.PayTransfer {
		t.Fatal("predvolené hodnoty")
	}
}

func TestSupplierRoundingIsKeptWithWarning(t *testing.T) {
	p := sample()
	p.Rates[0].VAT = m.MustParse("23.04") // v tolerancii — bez upozornenia, ostáva z dokladu
	if w := p.Normalize(); len(w) != 0 || p.Rates[0].VAT.String() != "23.04" {
		t.Fatalf("%v %s", w, p.Rates[0].VAT)
	}
	p.Rates[0].VAT = m.MustParse("25.00")
	w := p.Normalize()
	if len(w) != 1 || w[0].Field != "rates[0].vat" || p.Rates[0].VAT.String() != "25.00" {
		t.Fatalf("upozornenie: %v", w)
	}
}

func TestReverseChargeAndNoDeduction(t *testing.T) {
	p := sample()
	p.ReverseCharge = true
	p.Normalize()
	if p.Summary.Total.String() != "100.00" || p.Summary.VAT.String() != "23.00" {
		t.Fatalf("prenesenie: platí sa základ, DPH je samozdanenie: %+v", p.Summary)
	}
	p.NoDeduction = true
	if !p.Deductible().IsZero() {
		t.Fatal("bez odpočtu")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*Purchase)
		field  string
	}{
		{"platná", func(*Purchase) {}, ""},
		{"bez dodávateľa", func(p *Purchase) { p.Supplier.Name = "" }, "supplier.name"},
		{"bez čísla dokladu", func(p *Purchase) { p.SupplierNumber = " " }, "supplierNumber"},
		{"bez dátumu doručenia", func(p *Purchase) { p.ReceivedDate = invoice.Date{} }, "receivedDate"},
		{"doručená pred vystavením", func(p *Purchase) { p.ReceivedDate = invoice.MustDate("2026-10-01") }, "receivedDate"},
		{"bez riadkov", func(p *Purchase) { p.Rates = nil }, "rates"},
		{"nulový základ", func(p *Purchase) { p.Rates[0].Base = m.Zero }, "rates[0].base"},
		{"23 % v roku 2024", func(p *Purchase) {
			p.IssueDate, p.ReceivedDate, p.DueDate = invoice.MustDate("2024-12-10"), invoice.MustDate("2024-12-11"), invoice.MustDate("2024-12-24")
		}, "rates[0].vatRate"},
		{"odpočet bez IČ DPH", func(p *Purchase) { p.Supplier.ICDPH = "" }, "supplier.icdph"},
		{"bez odpočtu nepotrebuje IČ DPH", func(p *Purchase) { p.Supplier.ICDPH = ""; p.NoDeduction = true }, ""},
		{"neplatiteľ (0 %) nepotrebuje IČ DPH", func(p *Purchase) { p.Supplier.ICDPH = ""; p.Rates[0].Rate = 0 }, ""},
		{"zlý IBAN", func(p *Purchase) { p.SupplierIBAN = "SK00 1234" }, "supplierIban"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := sample()
			c.modify(&p)
			p.Normalize()
			err := p.Validate()
			if c.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !fields(err)[c.field] {
				t.Fatalf("chýba chyba %s: %v", c.field, err)
			}
		})
	}
}

func TestLifecycle(t *testing.T) {
	p := sample()
	p.Normalize()
	if !p.Overdue(invoice.MustDate("2026-10-20")) || p.Overdue(invoice.MustDate("2026-10-19")) {
		t.Fatal("po splatnosti")
	}
	if err := p.MarkPaid(invoice.MustDate("2026-10-15")); err != nil || p.Overdue(invoice.MustDate("2027-01-01")) {
		t.Fatal(err)
	}
	if err := p.MarkPaid(invoice.MustDate("2026-10-16")); err == nil {
		t.Fatal("dvakrát uhradená")
	}
	if err := p.Unpay(); err != nil || p.Status != StatusBooked || !p.PaidDate.IsZero() {
		t.Fatal("zrušenie úhrady")
	}
}

func TestSameDocument(t *testing.T) {
	a := sample()
	b := sample()
	b.SupplierNumber = " 2026100123 "
	if !SameDocument(a, b) {
		t.Fatal("rovnaké číslo a IČO")
	}
	b.Supplier.ICO = "35757442"
	if SameDocument(a, b) {
		t.Fatal("iný dodávateľ")
	}
	a.Supplier.ICO, b.Supplier.ICO = "", ""
	b.Supplier.Name = "slovnaft, a.s."
	if !SameDocument(a, b) {
		t.Fatal("bez IČO podľa názvu")
	}
	b.SupplierNumber = "X"
	if SameDocument(a, b) {
		t.Fatal("iné číslo")
	}
}
