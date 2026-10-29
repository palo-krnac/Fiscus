package reminder

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func d(s string) invoice.Date { return invoice.MustDate(s) }

func TestStatutoryRate(t *testing.T) {
	// hodnoty z prehľadu sadzieb úrokov z omeškania (najpravo.sk)
	cases := []struct {
		firstDay string
		business bool
		want     string
	}{
		{"2026-10-01", true, "11.65"},
		{"2026-10-01", false, "7.65"},
		{"2026-09-15", true, "11.40"},
		{"2026-06-17", true, "11.40"},
		{"2026-06-16", true, "11.15"},
		{"2025-04-23", false, "7.40"},
		{"2024-01-15", true, "13.50"},
		{"2022-07-26", true, "9.00"},
		{"2020-05-05", false, "5.00"},
	}
	for _, c := range cases {
		if got := StatutoryRate(d(c.firstDay), c.business).String(); got != c.want {
			t.Errorf("%s podnikateľ=%v: %s, chcel som %s", c.firstDay, c.business, got, c.want)
		}
	}
	if ECBUpdated.String() != "2026-09-16" {
		t.Fatal("dátum aktualizácie ECB")
	}
}

func TestInterest(t *testing.T) {
	cases := []struct {
		name           string
		amount, rate   string
		from, to, want string
	}{
		// príklad z podnikajte.sk: 5 000 € × 10,40 % × 30 / 365
		{"30 dní", "5000", "10.40", "2026-07-01", "2026-07-30", "42.74"},
		{"jeden deň", "1000", "11.65", "2026-10-01", "2026-10-01", "0.32"},
		{"cez nový rok do prestupného", "1000", "10", "2027-12-02", "2028-01-30", "16.42"},
		{"bez omeškania", "1000", "10", "2026-10-02", "2026-10-01", "0.00"},
		{"nulová sadzba", "1000", "0", "2026-01-01", "2026-12-31", "0.00"},
		{"celý rok", "1000", "10", "2026-01-01", "2026-12-31", "100.00"},
	}
	for _, c := range cases {
		if got := Interest(m.MustParse(c.amount), m.MustParse(c.rate), d(c.from), d(c.to)).String(); got != c.want {
			t.Errorf("%s: %s, chcel som %s", c.name, got, c.want)
		}
	}
}

func overdue(id int64, number, due, total string) invoice.Invoice {
	return invoice.Invoice{ID: id, Number: number, Kind: invoice.KindInvoice, Status: invoice.StatusIssued, ContactID: 7,
		Customer: invoice.Party{Name: "Dlžník s.r.o.", ICO: "31322832"}, VariableSymbol: strings.TrimPrefix(number, "FA"),
		IssueDate: d(due).AddDays(-14), DueDate: d(due)}
}

func TestBuild(t *testing.T) {
	a := overdue(1, "FA20260010", "2026-09-30", "1000")
	a.Summary.Total = m.MustParse("1000")
	b := overdue(2, "FA20260012", "2026-10-14", "246")
	b.Summary.Total = m.MustParse("246")
	opt := Options{Date: d("2026-10-31"), PayBy: d("2026-11-07"), Interest: InterestStatutory, ChargeFee: true}

	r, err := Build([]invoice.Invoice{a, b}, 1, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Business || !r.ChargeFee || r.Level != 1 || len(r.Items) != 2 {
		t.Fatalf("hlavička: %+v", r)
	}
	// FA…10: 31 dní pri 11,65 % → 9,89; FA…12: 17 dní pri 11,65 % → 1,33
	if it := r.Items[0]; it.Days != 31 || it.Rate.String() != "11.65" || it.Interest.String() != "9.89" || it.Fee.String() != "40.00" {
		t.Fatalf("položka 1: %+v", it)
	}
	if it := r.Items[1]; it.Days != 17 || it.Interest.String() != "1.33" {
		t.Fatalf("položka 2: %+v", it)
	}
	if r.Principal.String() != "1246.00" || r.InterestTotal.String() != "11.22" || r.Fees.String() != "80.00" || r.Total.String() != "1337.22" {
		t.Fatalf("súčty: %s %s %s %s", r.Principal, r.InterestTotal, r.Fees, r.Total)
	}

	// spotrebiteľ: nižšia sadzba, bez paušálnej náhrady
	c := a
	c.Customer.ICO, c.ContactID = "", 0
	if r, _ := Build([]invoice.Invoice{c}, 2, opt); r.Business || r.ChargeFee || r.Fees.Sign() != 0 || r.Items[0].Rate.String() != "7.65" {
		t.Fatalf("spotrebiteľ: %+v", r)
	}
	// vlastná sadzba a bez úroku
	cust := opt
	cust.Interest, cust.CustomRate = InterestCustom, m.MustParse("18.25")
	if r, _ := Build([]invoice.Invoice{a}, 1, cust); r.Items[0].Rate.String() != "18.25" || r.Items[0].Interest.String() != "15.50" {
		t.Fatalf("vlastná sadzba: %+v", r.Items[0])
	}
	none := opt
	none.Interest = InterestNone
	if r, _ := Build([]invoice.Invoice{a}, 9, none); r.InterestTotal.Sign() != 0 || r.Level != MaxLevel {
		t.Fatalf("bez úroku / strop stupňa: %+v", r)
	}
}

func TestBuildErrors(t *testing.T) {
	a := overdue(1, "FA20260010", "2026-09-30", "")
	a.Summary.Total = m.MustParse("100")
	opt := Options{Date: d("2026-10-31"), PayBy: d("2026-11-07"), Interest: InterestStatutory}
	other := a
	other.ContactID, other.Customer.Name = 8, "Iný"
	paid := a
	paid.Status = invoice.StatusPaid
	notYet := a
	notYet.DueDate = d("2026-11-15")
	credit := a
	credit.Kind = invoice.KindCreditNote

	cases := []struct {
		name  string
		invs  []invoice.Invoice
		opt   func(*Options)
		check func(error) bool
	}{
		{"prázdna", nil, nil, func(e error) bool { return errors.Is(e, ErrNoInvoices) }},
		{"dvaja odberatelia", []invoice.Invoice{a, other}, nil, func(e error) bool { return errors.Is(e, ErrMixedCustomer) }},
		{"uhradená", []invoice.Invoice{paid}, nil, func(e error) bool {
			return errors.Is(e, ErrNotRemindable) && strings.Contains(e.Error(), "neuhradená")
		}},
		{"dobropis", []invoice.Invoice{credit}, nil, func(e error) bool { return e != nil }},
		{"ešte nie po splatnosti", []invoice.Invoice{notYet}, nil, func(e error) bool {
			return errors.Is(e, ErrNotRemindable) && strings.Contains(e.Error(), "po splatnosti")
		}},
		{"lehota pred dátumom", []invoice.Invoice{a}, func(o *Options) { o.PayBy = d("2026-10-01") }, fieldIs("payBy")},
		{"bez dátumu", []invoice.Invoice{a}, func(o *Options) { o.Date = invoice.Date{} }, fieldIs("date")},
		{"vlastná sadzba 0", []invoice.Invoice{a}, func(o *Options) { o.Interest = InterestCustom }, fieldIs("customRate")},
		{"neznámy úrok", []invoice.Invoice{a}, func(o *Options) { o.Interest = "x" }, fieldIs("interestMode")},
	}
	for _, c := range cases {
		o := opt
		if c.opt != nil {
			c.opt(&o)
		}
		if _, err := Build(c.invs, 1, o); !c.check(err) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func fieldIs(f string) func(error) bool {
	return func(err error) bool {
		var ve *invoice.ValidationError
		if !errors.As(err, &ve) {
			return false
		}
		for _, x := range ve.Fields {
			if x.Field == f {
				return true
			}
		}
		return false
	}
}

func TestCustomerKey(t *testing.T) {
	cases := []struct {
		inv  invoice.Invoice
		want string
	}{
		{invoice.Invoice{ContactID: 3, Customer: invoice.Party{ICO: "1"}}, "c3"},
		{invoice.Invoice{Customer: invoice.Party{ICO: "313 228 32"}}, "i31322832"},
		{invoice.Invoice{Customer: invoice.Party{Name: " Ján Novák "}}, "nján novák"},
	}
	for _, c := range cases {
		if got := CustomerKey(c.inv); got != c.want {
			t.Errorf("%q, chcel som %q", got, c.want)
		}
	}
}

// SQLite ukladá upomienku ako JSON — po prečítaní musí byť rovnaká.
func TestJSONRoundTrip(t *testing.T) {
	a := overdue(1, "FA20260010", "2026-09-30", "")
	a.Summary.Total = m.MustParse("1000")
	r, err := Build([]invoice.Invoice{a}, 2, Options{Date: d("2026-10-31"), PayBy: d("2026-11-07"),
		Interest: InterestCustom, CustomRate: m.MustParse("12"), ChargeFee: true, Note: "x"})
	if err != nil {
		t.Fatal(err)
	}
	r.ID, r.CompanyID, r.Number = 3, 1, "UP20260001"
	b, _ := json.Marshal(r)
	var back Reminder
	if err := json.Unmarshal(b, &back); err != nil || !reflect.DeepEqual(back, r) {
		t.Fatalf("JSON:\n got %+v\nwant %+v", back, r)
	}
}
