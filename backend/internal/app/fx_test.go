package app

import (
	"context"
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func ecbStub(rates ...fx.Rate) func(context.Context, string) ([]fx.Rate, error) {
	return func(context.Context, string) ([]fx.Rate, error) { return rates, nil }
}

func rate(cur, date, v string) fx.Rate {
	return fx.Rate{Currency: cur, Date: date, Rate: m.MustParse(v), Source: fx.SourceECB}
}

func TestUpdateRates(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	if _, err := a.UpdateRates(ctx); !errors.Is(err, ErrRatesOff) {
		t.Fatalf("bez adresy: %v", err)
	}
	a.ECBURL = fx.ECB90
	a.FetchRates = ecbStub(rate("CZK", "2026-10-23", "25.123"), rate("CZK", "2026-10-22", "25.2"), rate("USD", "2026-10-23", "1.0823"))
	res, err := a.UpdateRates(ctx)
	if err != nil || res.Count != 3 || res.Latest != "2026-10-23" {
		t.Fatalf("aktualizácia: %+v %v", res, err)
	}
	if _, err := a.SetRate(ctx, fx.Rate{Currency: "CZK", Date: "2026-10-24", Rate: m.MustParse("0")}); err == nil {
		t.Fatal("nulový kurz")
	}
	if r, err := a.SetRate(ctx, fx.Rate{Currency: "CZK", Date: "2026-10-24", Rate: m.MustParse("25.05")}); err != nil || r.Source != fx.SourceManual {
		t.Fatalf("ručný kurz: %+v %v", r, err)
	}
	list, _ := a.Rates(ctx, fx.Filter{Currency: "CZK"})
	if len(list) != 3 || list[0].Source != fx.SourceManual {
		t.Fatalf("zoznam: %+v", list)
	}
	if err := a.DeleteRate(ctx, "CZK", "2026-10-24"); err != nil {
		t.Fatal(err)
	}
}

func TestRatePermissions(t *testing.T) {
	o := newOffice(t)
	o.a.ECBURL, o.a.FetchRates = fx.ECB90, ecbStub(rate("CZK", "2026-10-23", "25"))
	if _, err := o.a.UpdateRates(as(o.jana)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("účtovníčka aktualizuje: %v", err)
	}
	if _, err := o.a.SetRate(as(o.jana), fx.Rate{Currency: "CZK", Date: "2026-10-24", Rate: m.MustParse("25")}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("účtovníčka zadáva kurz: %v", err)
	}
	if _, err := o.a.UpdateRates(as(o.admin)); err != nil {
		t.Fatal(err)
	}
}

func TestForeignInvoice(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	if err := a.Store.SaveRates(ctx, []fx.Rate{rate("CZK", "2026-10-23", "25.123"), rate("CZK", "2026-10-22", "25.2")}); err != nil {
		t.Fatal(err)
	}
	d := validDraft()
	d.Currency = "CZK"
	d.IssueDate, d.DeliveryDate = invoice.MustDate("2026-10-26"), invoice.MustDate("2026-10-23")
	tests := []struct {
		name     string
		mod      func(*invoice.Invoice)
		rate     string
		date     string
		manual   bool
		issueErr bool
	}{
		{"kurz z predchádzajúceho dňa", nil, "25.20", "2026-10-22", false, false},
		{"víkend: posledný vyhlásený", func(i *invoice.Invoice) { i.DeliveryDate = invoice.MustDate("2026-10-25") }, "25.123", "2026-10-23", false, false},
		{"ručný kurz ostane", func(i *invoice.Invoice) { i.FXRate, i.FXManual = m.MustParse("24.5"), true }, "24.50", "", true, false},
		{"ručný bez čísla → ECB", func(i *invoice.Invoice) { i.FXManual = true }, "25.20", "2026-10-22", false, false},
		{"bez kurzu sa nevystaví", func(i *invoice.Invoice) { i.DeliveryDate = invoice.MustDate("2026-10-01") }, "0", "", false, true},
		{"klient nepodstrčí kurz ECB", func(i *invoice.Invoice) { i.FXRate = m.MustParse("1") }, "25.20", "2026-10-22", false, false},
	}
	for _, tt := range tests {
		inv := d
		if tt.mod != nil {
			tt.mod(&inv)
		}
		dr, err := a.SaveDraft(ctx, c.ID, inv)
		if err != nil {
			t.Fatal(err)
		}
		if dr.FXRate.String() != m.MustParse(tt.rate).String() || dr.FXDate.String() != tt.date || dr.FXManual != tt.manual {
			t.Errorf("%s: kurz %s z %s ručne=%v", tt.name, dr.FXRate, dr.FXDate, dr.FXManual)
		}
		if (dr.Local != nil) != (tt.rate != "0") {
			t.Errorf("%s: Local %+v", tt.name, dr.Local)
		}
		_, err = a.Issue(ctx, c.ID, dr.ID)
		if (err != nil) != tt.issueErr || (tt.issueErr && !fieldErr(err, "fxRate")) {
			t.Errorf("%s: vystavenie %v", tt.name, err)
		}
	}
	// náhľad počíta tiež s kurzom
	pv, err := a.Preview(ctx, c.ID, d)
	if err != nil || pv.Local == nil || pv.Local.Total.String() != pv.Summary.Total.DivCents(m.MustParse("25.2")).String() &&
		pv.Local.Total != pv.Local.Base.Add(pv.Local.VAT) {
		t.Fatalf("náhľad: %+v %v", pv.Local, err)
	}
	// eurá: bez kurzu
	e := validDraft()
	e.FXRate = m.MustParse("2")
	if dr, _ := a.SaveDraft(ctx, c.ID, e); !dr.FXRate.IsZero() || dr.Local != nil {
		t.Fatalf("eurová faktúra s kurzom: %+v", dr)
	}
}
