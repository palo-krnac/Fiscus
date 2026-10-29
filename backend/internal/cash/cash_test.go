package cash

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func doc(kind Kind, date, amount string) Document {
	return Document{Kind: kind, Date: invoice.MustDate(date), Amount: money.MustParse(amount), Purpose: "x"}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		d     Document
		field string
	}{
		{"platný", doc(In, "2026-10-01", "10"), ""},
		{"bez druhu", Document{Date: invoice.MustDate("2026-10-01"), Amount: money.MustParse("1"), Purpose: "x"}, "kind"},
		{"nulová suma", doc(In, "2026-10-01", "0"), "amount"},
		{"záporná suma", doc(Out, "2026-10-01", "-5"), "amount"},
		{"tri desatinné miesta", doc(Out, "2026-10-01", "1.005"), "amount"},
		{"bez účelu", Document{Kind: In, Date: invoice.MustDate("2026-10-01"), Amount: money.MustParse("1")}, "purpose"},
		{"bez dátumu", Document{Kind: In, Amount: money.MustParse("1"), Purpose: "x"}, "date"},
	}
	for _, tt := range tests {
		err := tt.d.Validate()
		var ve *invoice.ValidationError
		switch {
		case tt.field == "" && err != nil:
			t.Errorf("%s: %v", tt.name, err)
		case tt.field != "" && (!errors.As(err, &ve) || ve.Fields[0].Field != tt.field):
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

func TestBalance(t *testing.T) {
	cancelled := doc(Out, "2026-10-03", "1000")
	cancelled.Cancelled = true
	tests := []struct {
		name    string
		docs    []Document
		balance string
		ok      bool
	}{
		{"vklad a výdavok", []Document{doc(In, "2026-10-01", "100"), doc(Out, "2026-10-02", "40")}, "60", true},
		{"výdavok pred vkladom", []Document{doc(Out, "2026-10-01", "40"), doc(In, "2026-10-02", "100")}, "60", false},
		{"v ten istý deň najprv príjem", []Document{doc(Out, "2026-10-01", "40"), doc(In, "2026-10-01", "100")}, "60", true},
		{"stornovaný sa nepočíta", []Document{doc(In, "2026-10-01", "100"), cancelled}, "100", true},
		{"presne nula", []Document{doc(In, "2026-10-01", "50.50"), doc(Out, "2026-10-01", "50.50")}, "0", true},
	}
	for _, tt := range tests {
		if b := Balance(tt.docs); !b.Equal(money.MustParse(tt.balance)) {
			t.Errorf("%s: zostatok %s", tt.name, b)
		}
		err := CheckBalance(tt.docs)
		if (err == nil) != tt.ok || (err != nil && !errors.Is(err, ErrNegative)) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}
