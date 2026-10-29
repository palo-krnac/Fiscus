package journal

import (
	"errors"
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
)

func lines(ls ...any) []Line {
	var out []Line
	for i := 0; i < len(ls); i += 2 {
		out = append(out, Line{ls[i].(Column), m.MustParse(ls[i+1].(string))})
	}
	return out
}

func eq(a, b []Line) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Column != b[i].Column || !a[i].Amount.Equal(b[i].Amount) {
			return false
		}
	}
	return true
}

func TestSaleLines(t *testing.T) {
	sale := Sale{Total: m.MustParse("492"), VAT: m.MustParse("92"), VATPayer: true}
	tests := []struct {
		name   string
		amount string
		s      Sale
		want   []Line
	}{
		{"celá úhrada služieb", "492", sale, lines(IncServices, "400.00", IncVAT, "92.00")},
		{"čiastočná úhrada", "100", sale, lines(IncServices, "81.30", IncVAT, "18.70")},
		{"tovar", "492", Sale{Total: sale.Total, VAT: sale.VAT, VATPayer: true, Goods: true}, lines(IncGoods, "400.00", IncVAT, "92.00")},
		{"neplatiteľ DPH", "400", Sale{Total: m.MustParse("400"), VATPayer: false}, lines(IncServices, "400")},
		{"vrátené k dobropisu", "-123", Sale{Total: m.MustParse("-123"), VAT: m.MustParse("-23"), VATPayer: true}, lines(IncServices, "-100.00", IncVAT, "-23.00")},
		{"oslobodené plnenie", "500", Sale{Total: m.MustParse("500"), VATPayer: true}, lines(IncServices, "500")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SaleLines(m.MustParse(tt.amount), tt.s)
			if !eq(got, tt.want) {
				t.Fatalf("%v", got)
			}
			if err := Check(m.MustParse(tt.amount), got); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuyLines(t *testing.T) {
	tests := []struct {
		name   string
		amount string
		b      Buy
		want   []Line
	}{
		{"služba s odpočtom", "-123", Buy{Total: m.MustParse("123"), VAT: m.MustParse("23"), Deduct: true}, lines(ExpServices, "100.00", ExpVAT, "23.00")},
		{"zásoby, čiastočne", "-61.50", Buy{Total: m.MustParse("123"), VAT: m.MustParse("23"), Deduct: true, Kind: purchase.ExpenseStock}, lines(ExpStock, "50.00", ExpVAT, "11.50")},
		{"bez odpočtu: DPH je výdavok", "-123", Buy{Total: m.MustParse("123"), VAT: m.MustParse("23"), Deduct: false}, lines(ExpServices, "123")},
		{"dlhodobý majetok", "-1230", Buy{Total: m.MustParse("1230"), VAT: m.MustParse("230"), Deduct: true, Kind: purchase.ExpenseAsset}, lines(ExpAsset, "1000.00", ExpVAT, "230.00")},
		{"ostatné, prenesenie (DPH sa neplatí)", "-500", Buy{Total: m.MustParse("500"), Deduct: true, Kind: purchase.ExpenseOther}, lines(ExpOther, "500")},
		{"vrátené od dodávateľa", "24.60", Buy{Total: m.MustParse("123"), VAT: m.MustParse("23"), Deduct: true}, lines(ExpServices, "-20.00", ExpVAT, "-4.60")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuyLines(m.MustParse(tt.amount), tt.b)
			if !eq(got, tt.want) {
				t.Fatalf("%v", got)
			}
			if err := Check(m.MustParse(tt.amount), got); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		amount string
		ls     []Line
		err    error
	}{
		{"príjem", "123", lines(IncServices, "100", IncVAT, "23"), nil},
		{"výdavok", "-50", lines(ExpInsur, "50"), nil},
		{"prevod do pokladnice (banka)", "-200", lines(TransitOut, "200"), nil},
		{"nesedí", "123", lines(IncServices, "100"), ErrSum},
		{"zlý smer", "50", lines(ExpServices, "50"), ErrSum},
		{"neznámy stĺpec", "1", lines(Column("x"), "1"), ErrUnknownColumn},
	}
	for _, tt := range tests {
		err := Check(m.MustParse(tt.amount), tt.ls)
		if (tt.err == nil) != (err == nil) || (tt.err != nil && !errors.Is(err, tt.err)) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
	if Check(m.MustParse("2"), lines(IncOther, "1", IncOther, "1")) == nil {
		t.Error("stĺpec dvakrát")
	}
	if Check(m.MustParse("1.005"), lines(IncOther, "1.005")) == nil {
		t.Error("zlomky centov")
	}
}

func TestSum(t *testing.T) {
	es := []Entry{
		{Means: Bank, Amount: m.MustParse("492"), Lines: lines(IncServices, "400", IncVAT, "92")},
		{Means: Bank, Amount: m.MustParse("-123"), Lines: lines(ExpServices, "100", ExpVAT, "23")},
		{Means: Cash, Amount: m.MustParse("-30"), Lines: lines(ExpStock, "30")},
		{Means: Cash, Amount: m.MustParse("200")},                                   // nerozpísaný
		{Means: Bank, Amount: m.MustParse("-50"), Lines: lines(IncServices, "-50")}, // vrátené k dobropisu
	}
	got := Sum(es)
	if got.TaxInc.String() != "350.00" || got.TaxExp.String() != "130.00" || got.Result.String() != "220.00" ||
		got.OtherInc.String() != "92.00" || got.OtherExp.String() != "23.00" || got.Open != 1 ||
		got.BankIn.String() != "492.00" || got.BankOut.String() != "173.00" || got.CashIn.String() != "200.00" || got.CashOut.String() != "30.00" {
		t.Fatalf("%+v", got)
	}
}
