package invoice

import (
	"errors"
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func sample() Invoice {
	return Invoice{
		Kind:         KindInvoice,
		Customer:     Party{Name: "Testovacia firma s.r.o.", ICO: "31322832", ICDPH: "SK2020372640", Country: "SK"},
		IssueDate:    MustDate("2026-10-01"),
		DeliveryDate: MustDate("2026-10-01"),
		DueDate:      MustDate("2026-10-15"),
		Lines: []Line{
			{Description: "Programovanie PLC", Quantity: m.MustParse("8"), Unit: "hod", UnitPrice: m.MustParse("45"), Rate: vat.Rate23},
			{Description: "Kábel", Quantity: m.MustParse("2.5"), Unit: "m", UnitPrice: m.MustParse("1.99"), Rate: vat.Rate23},
		},
	}
}

func TestRecalculate(t *testing.T) {
	inv := sample()
	if err := inv.Recalculate(); err != nil {
		t.Fatal(err)
	}
	// 360 + 4.975→4.98 = 364.98; DPH 83.9454 → 83.95
	if got := inv.Lines[1].Amount.String(); got != "4.98" {
		t.Errorf("suma položky %s", got)
	}
	if inv.Summary.Base.String() != "364.98" || inv.Summary.VAT.String() != "83.95" || inv.Summary.Total.String() != "448.93" {
		t.Errorf("súhrn %+v", inv.Summary)
	}
	if inv.Currency != "EUR" || inv.PriceMode != vat.PriceNet {
		t.Errorf("predvolené hodnoty %s %s", inv.Currency, inv.PriceMode)
	}
}

func fields(err error) map[string]bool {
	var ve *ValidationError
	out := map[string]bool{}
	if errors.As(err, &ve) {
		for _, f := range ve.Fields {
			out[f.Field] = true
		}
	}
	return out
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*Invoice)
		field  string // očakávaná chyba; "" = platná
	}{
		{"platná", func(*Invoice) {}, ""},
		{"bez odberateľa", func(i *Invoice) { i.Customer.Name = " " }, "customer.name"},
		{"zlé IČO", func(i *Invoice) { i.Customer.ICO = "12345678" }, "customer.ico"},
		{"zlé IČ DPH", func(i *Invoice) { i.Customer.ICDPH = "SK2020372641" }, "customer.icdph"},
		{"zahraničné IČ DPH sa nekontroluje", func(i *Invoice) { i.Customer.ICDPH = "CZ12345678" }, ""},
		{"prenesenie bez IČ DPH", func(i *Invoice) { i.ReverseCharge = true; i.Customer.ICDPH = "" }, "customer.icdph"},
		{"bez položiek", func(i *Invoice) { i.Lines = nil }, "lines"},
		{"položka bez popisu", func(i *Invoice) { i.Lines[0].Description = "" }, "lines[0].description"},
		{"nulové množstvo", func(i *Invoice) { i.Lines[1].Quantity = m.Zero }, "lines[1].quantity"},
		{"zlá sadzba", func(i *Invoice) { i.Lines[0].Rate = 20 }, "lines[0].vatRate"},
		{"bez dátumu dodania", func(i *Invoice) { i.DeliveryDate = Date{} }, "deliveryDate"},
		{"splatnosť pred vystavením", func(i *Invoice) { i.DueDate = MustDate("2026-09-30") }, "dueDate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv := sample()
			c.modify(&inv)
			err := inv.Validate()
			if c.field == "" {
				if err != nil {
					t.Fatalf("neočakávaná chyba: %v", err)
				}
				return
			}
			if !fields(err)[c.field] {
				t.Fatalf("chýba chyba poľa %s, dostal som: %v", c.field, err)
			}
		})
	}
}

func TestLifecycle(t *testing.T) {
	inv := sample()
	if err := inv.Issue("FA20260001"); err != nil {
		t.Fatal(err)
	}
	if inv.Status != StatusIssued || inv.VariableSymbol != "20260001" {
		t.Fatalf("po vystavení: %s %s", inv.Status, inv.VariableSymbol)
	}
	if inv.Editable() {
		t.Fatal("vystavená faktúra nesmie byť upraviteľná")
	}
	if err := inv.Issue("FA20260002"); !errors.Is(err, ErrBadTransition) {
		t.Fatal("druhé vystavenie malo zlyhať")
	}
	if !inv.Overdue(MustDate("2026-10-16")) || inv.Overdue(MustDate("2026-10-15")) {
		t.Fatal("výpočet omeškania")
	}
	if err := inv.MarkPaid(MustDate("2026-10-10")); err != nil {
		t.Fatal(err)
	}
	if inv.Overdue(MustDate("2026-12-01")) {
		t.Fatal("uhradená faktúra nie je po splatnosti")
	}
	if err := inv.Cancel(); !errors.Is(err, ErrBadTransition) {
		t.Fatal("uhradenú faktúru nemožno stornovať")
	}
	if err := inv.Unpay(); err != nil || inv.Status != StatusIssued || !inv.PaidDate.IsZero() {
		t.Fatalf("zrušenie úhrady: %v %s", err, inv.Status)
	}
	if err := inv.Unpay(); !errors.Is(err, ErrBadTransition) {
		t.Fatal("neuhradenej faktúre nemožno zrušiť úhradu")
	}
}

func TestIssueRejectsInvalid(t *testing.T) {
	inv := sample()
	inv.Lines = nil
	if err := inv.Issue("FA20260001"); err == nil || inv.Status == StatusIssued {
		t.Fatal("neplatný koncept sa nesmie vystaviť")
	}
}

func TestNonVATPayerHasNoVAT(t *testing.T) {
	inv := sample()
	inv.NonVATPayer = true
	if err := inv.Recalculate(); err != nil {
		t.Fatal(err)
	}
	if !inv.Summary.VAT.IsZero() || inv.Summary.Total.String() != "364.98" || len(inv.Summary.Rates) != 1 || inv.Summary.Rates[0].Rate != vat.Rate0 {
		t.Fatalf("neplatiteľ DPH: %+v", inv.Summary)
	}
	// neplatiteľ nepotrebuje IČ DPH odberateľa (to platí iba pri prenesení)
	inv.Customer.ICDPH = ""
	if err := inv.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPaymentMethod(t *testing.T) {
	inv := sample()
	_ = inv.Recalculate()
	if inv.PaymentMethod != PayTransfer {
		t.Fatalf("predvolený spôsob úhrady: %q", inv.PaymentMethod)
	}
	inv.PaymentMethod = "bitcoin"
	if !fields(inv.Validate())["paymentMethod"] {
		t.Fatal("neznámy spôsob úhrady mal byť odmietnutý")
	}
	for _, m := range PaymentMethods {
		if !m.Code.Valid() || m.Label == "" {
			t.Errorf("číselník: %+v", m)
		}
	}
}

func TestDateDisplay(t *testing.T) {
	if MustDate("2024-12-05").Display() != "5. 12. 2024" || (Date{}).Display() != "" {
		t.Fatal(MustDate("2024-12-05").Display())
	}
}
