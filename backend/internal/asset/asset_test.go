package asset

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func plan(t *testing.T, a Asset) map[int]Year {
	t.Helper()
	a.Normalize()
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	ys, err := a.Schedule()
	if err != nil {
		t.Fatal(err)
	}
	out := map[int]Year{}
	var sum money.Dec
	for _, y := range ys {
		out[y.Year] = y
		sum = sum.Add(y.Claimed).Add(y.Disposal)
	}
	if a.Share == 100 && !sum.Equal(a.Cost) {
		t.Fatalf("súčet odpisov %s ≠ VC %s", sum, a.Cost)
	}
	return out
}

func TestAccelerated(t *testing.T) {
	// príklad FR SR: VC 50 000 €, skupina 2, zaradenie v máji 2026
	p := plan(t, Asset{Name: "Stroj", Group: 2, Method: Accelerated, Acquired: invoice.MustDate("2026-05-15"), Cost: money.MustParse("50000")})
	want := map[int]string{2026: "5555.55", 2027: "13888.89", 2028: "11111.11", 2029: "8333.34", 2030: "5555.55", 2031: "2777.78", 2032: "2777.78"}
	if len(p) != len(want) {
		t.Fatalf("roky: %+v", p)
	}
	for y, w := range want {
		if got := p[y].Claimed.String(); got != w {
			t.Errorf("%d: %s, chcem %s", y, got, w)
		}
	}
	if p[2026].Residual.String() != "44444.45" || p[2031].Residual.String() != "2777.78" || !p[2032].Residual.IsZero() {
		t.Errorf("ZC: %s %s %s", p[2026].Residual, p[2031].Residual, p[2032].Residual)
	}
}

func TestStraight(t *testing.T) {
	tests := []struct {
		name     string
		a        Asset
		years    map[int]string
		residual map[int]string
	}{
		{"skupina 1 od januára", Asset{Name: "PC", Group: 1, Acquired: invoice.MustDate("2026-01-10"), Cost: money.MustParse("2000")},
			map[int]string{2026: "500.00", 2029: "500.00"}, map[int]string{2026: "1500.00", 2029: "0.00"}},
		{"skupina 2 od októbra", Asset{Name: "Auto", Group: 2, Acquired: invoice.MustDate("2026-10-01"), Cost: money.MustParse("24000")},
			map[int]string{2026: "1000.00", 2027: "4000.00", 2031: "4000.00", 2032: "3000.00"}, map[int]string{2026: "23000.00", 2031: "3000.00", 2032: "0.00"}},
		{"zaokrúhlenie", Asset{Name: "X", Group: 1, Acquired: invoice.MustDate("2026-01-01"), Cost: money.MustParse("1999.99")},
			map[int]string{2026: "500.00", 2029: "499.99"}, nil},
		{"podiel na podnikaní 60 %", Asset{Name: "Auto", Group: 1, Acquired: invoice.MustDate("2026-01-01"), Cost: money.MustParse("20000"), Share: 60},
			map[int]string{2026: "3000.00"}, map[int]string{2026: "15000.00"}},
		{"nehmotný 5 rokov", Asset{Name: "Softvér", Kind: Intangible, Years: 5, Acquired: invoice.MustDate("2026-07-01"), Cost: money.MustParse("6000")},
			map[int]string{2026: "600.00", 2027: "1200.00", 2031: "600.00"}, nil},
	}
	for _, tt := range tests {
		p := plan(t, tt.a)
		for y, w := range tt.years {
			if got := p[y].Claimed.String(); got != w {
				t.Errorf("%s %d: %s, chcem %s", tt.name, y, got, w)
			}
		}
		for y, w := range tt.residual {
			if got := p[y].Residual.String(); got != w {
				t.Errorf("%s ZC %d: %s, chcem %s", tt.name, y, got, w)
			}
		}
	}
}

func TestDisposal(t *testing.T) {
	a := Asset{Name: "Auto", Group: 1, Acquired: invoice.MustDate("2026-01-01"), Cost: money.MustParse("20000"),
		Disposed: invoice.MustDate("2028-06-30"), DisposalKind: DisposalSale}
	p := plan(t, a)
	// 2026 a 2027 odpisy, 2028 bez odpisu, ZC 10 000 € je výdavok
	if p[2027].Claimed.String() != "5000.00" || !p[2028].Claimed.IsZero() || p[2028].Disposal.String() != "10000.00" || len(p) != 3 {
		t.Fatalf("vyradenie: %+v", p)
	}
	a.Normalize()
	if !a.ResidualAt(2028).IsZero() || a.ResidualAt(2027).String() != "10000.00" || !a.ResidualAt(2025).IsZero() {
		t.Fatalf("ZC: %s %s", a.ResidualAt(2027), a.ResidualAt(2028))
	}
	// vyradenie v roku zaradenia: celá VC
	b := plan(t, Asset{Name: "X", Group: 1, Acquired: invoice.MustDate("2026-03-01"), Cost: money.MustParse("3000"),
		Disposed: invoice.MustDate("2026-11-01"), DisposalKind: DisposalLiquidation})
	if b[2026].Disposal.String() != "3000.00" || !b[2026].Claimed.IsZero() {
		t.Fatalf("v roku zaradenia: %+v", b)
	}
}

func TestValidate(t *testing.T) {
	ok := Asset{Name: "Stroj", Group: 2, Acquired: invoice.MustDate("2026-01-01"), Cost: money.MustParse("5000")}
	tests := []struct {
		name  string
		mod   func(*Asset)
		field string
	}{
		{"platný", func(*Asset) {}, ""},
		{"bez názvu", func(a *Asset) { a.Name = " " }, "name"},
		{"bez ceny", func(a *Asset) { a.Cost = money.Zero }, "cost"},
		{"pod hranicou", func(a *Asset) { a.Cost = money.MustParse("1700") }, "cost"},
		{"zlá skupina", func(a *Asset) { a.Group = 7 }, "group"},
		{"zrýchlené v skupine 1", func(a *Asset) { a.Group, a.Method = 1, Accelerated }, "method"},
		{"zrýchlené v skupine 3", func(a *Asset) { a.Group, a.Method = 3, Accelerated }, ""},
		{"nehmotný bez doby", func(a *Asset) { a.Kind = Intangible; a.Cost = money.MustParse("3000") }, "years"},
		{"nehmotný pod hranicou", func(a *Asset) { a.Kind, a.Years, a.Cost = Intangible, 3, money.MustParse("2400") }, "cost"},
		{"bez zaradenia", func(a *Asset) { a.Acquired = invoice.Date{} }, "acquired"},
		{"podiel 0 → 100", func(a *Asset) { a.Share = 0 }, ""},
		{"podiel 150", func(a *Asset) { a.Share = 150 }, "share"},
		{"vyradenie pred zaradením", func(a *Asset) { a.Disposed, a.DisposalKind = invoice.MustDate("2025-01-01"), DisposalSale }, "disposed"},
		{"vyradenie bez dôvodu", func(a *Asset) { a.Disposed = invoice.MustDate("2027-01-01") }, "disposal"},
	}
	for _, tt := range tests {
		a := ok
		tt.mod(&a)
		a.Normalize()
		err := a.Validate()
		got := ""
		if ve, ok := err.(*invoice.ValidationError); ok && ve != nil {
			got = ve.Fields[0].Field
		}
		if got != tt.field {
			t.Errorf("%s: %q (%v)", tt.name, got, err)
		}
	}
}
