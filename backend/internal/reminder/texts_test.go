package reminder

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestTexts(t *testing.T) {
	by, _ := invoice.ParseDate("2026-11-10")
	tests := []struct {
		name  string
		r     Reminder
		title string
		intro string
		notes []string
	}{
		{"prvá, zákonný úrok podnikateľ",
			Reminder{Level: 1, PayBy: by, Business: true, Interest: InterestStatutory, Margin: money.MustParse("9"),
				InterestTotal: money.MustParse("1.2"), Fees: money.MustParse("40"), FeeEach: money.MustParse("40")},
			"Upomienka", "do 10. 11. 2026.", []string{"§ 369 Obchodného", "o 9 percentuálnych", "(40,00 € za každú", "plynie až do zaplatenia"}},
		{"druhá, spotrebiteľ",
			Reminder{Level: 2, PayBy: by, Interest: InterestStatutory, Margin: money.MustParse("5.5"), InterestTotal: money.MustParse("0.5")},
			"2. upomienka", "Napriek predchádzajúcej", []string{"§ 517 Občianskeho", "o 5,5 percentuálnych"}},
		{"posledná, dohodnutá sadzba",
			Reminder{Level: 3, PayBy: by, Interest: InterestCustom, CustomRate: money.MustParse("12.5"), InterestTotal: money.MustParse("3")},
			"Posledná upomienka pred vymáhaním", "súdnou cestou", []string{"12,5 % ročne"}},
		{"bez úroku a náhrady", Reminder{Level: 1, PayBy: by, Interest: InterestNone}, "Upomienka", "bezpredmetnú", nil},
	}
	for _, tt := range tests {
		if got := Title(tt.r.Level); got != tt.title {
			t.Errorf("%s: titul %q", tt.name, got)
		}
		if !strings.Contains(tt.r.Intro(), tt.intro) {
			t.Errorf("%s: úvod %q", tt.name, tt.r.Intro())
		}
		all := strings.Join(tt.r.LegalNotes(), "\n")
		for _, n := range tt.notes {
			if !strings.Contains(all, n) {
				t.Errorf("%s: chýba %q v %q", tt.name, n, all)
			}
		}
		if tt.notes == nil && all != "" {
			t.Errorf("%s: nečakané poznámky %q", tt.name, all)
		}
	}
}
