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
		{"druh výdavku zásoby", func(p *Purchase) { p.Expense = ExpenseStock }, ""},
		{"neznámy druh výdavku", func(p *Purchase) { p.Expense = "x" }, "expense"},
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
		{"zahraničný dodávateľ bez druhu plnenia", func(p *Purchase) { p.Supplier.Country = "CZ" }, "supplyKind"},
		{"zahraničný dodávateľ – tovar", func(p *Purchase) { p.Supplier.Country = "CZ"; p.SupplyKind = invoice.SupplyGoods }, ""},
		{"neznáma mena", func(p *Purchase) { p.Currency, p.FXRate = "XYZ", m.MustParse("2") }, "currency"},
		{"cudzia mena bez kurzu", func(p *Purchase) { p.Currency = "CZK" }, "fxRate"},
		{"cudzia mena s kurzom", func(p *Purchase) { p.Currency, p.FXRate = "CZK", m.MustParse("24.5") }, ""},
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

func TestForeignCurrency(t *testing.T) {
	cases := []struct {
		name                  string
		currency, rate        string
		reverse               bool
		ok                    bool
		base, vat, total, ded string
	}{
		{"euro", "", "", false, true, "100.00", "23.00", "123.00", "23.00"},
		{"EUR sa vyčistí", "EUR", "1", false, true, "100.00", "23.00", "123.00", "23.00"},
		{"CZK bez kurzu", "CZK", "", false, false, "0", "0", "0", "0"},
		// 2450 / 24,5 = 100; 563,50 / 24,5 = 23
		{"CZK", "CZK", "24.5", false, true, "100.00", "23.00", "123.00", "23.00"},
		{"USD zaokrúhlenie", "USD", "1.0837", false, true, "2260.77", "519.98", "2780.75", "519.98"},
		{"prenesenie platí základ", "CZK", "24.5", true, true, "100.00", "23.00", "100.00", "23.00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := sample()
			p.Currency, p.ReverseCharge = c.currency, c.reverse
			if c.rate != "" {
				p.FXRate = m.MustParse(c.rate)
			}
			if c.currency != "" && c.currency != "EUR" {
				p.Rates[0].Base = m.MustParse("2450")
			}
			p.Normalize()
			if c.currency == "EUR" && (p.Currency != "" || !p.FXRate.IsZero()) {
				t.Fatal("pri eurách sa údaje o kurze vyčistia")
			}
			tax, ok := p.Tax()
			if ok != c.ok {
				t.Fatalf("ok %v", ok)
			}
			got := []string{tax.Summary.Base.String(), tax.Summary.VAT.String(), tax.Summary.Total.String(), p.Deductible().String()}
			want := []string{c.base, c.vat, c.total, c.ded}
			if !c.ok {
				got = []string{"0", "0", "0", p.Deductible().String()}
				if !p.Deductible().IsZero() || !p.TotalEUR().IsZero() {
					t.Fatal("bez kurzu nič v eurách")
				}
				return
			}
			for i := range got {
				if !m.MustParse(got[i]).Equal(m.MustParse(want[i])) {
					t.Fatalf("%d: %s, chcem %s (%+v)", i, got[i], want[i], tax.Summary)
				}
			}
		})
	}
}
