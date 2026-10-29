package vat

import (
	"testing"

	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func line(q, p string, r Rate) Line { return Line{m.MustParse(q), m.MustParse(p), r} }

func TestCalculate(t *testing.T) {
	cases := []struct {
		name             string
		lines            []Line
		mode             PriceMode
		reverse          bool
		base, vat, total string
		rateCount        int
	}{
		{
			name:  "jedna položka 23 %",
			lines: []Line{line("1", "100", Rate23)},
			mode:  PriceNet, base: "100.00", vat: "23.00", total: "123.00", rateCount: 1,
		},
		{
			name:  "zaokrúhlenie DPH na centy",
			lines: []Line{line("1", "10.01", Rate23)}, // 2.3023 → 2.30
			mode:  PriceNet, base: "10.01", vat: "2.30", total: "12.31", rateCount: 1,
		},
		{
			name: "DPH zo súčtu základov, nie z položiek",
			// 3 × 0.07 € pri 23 %: po položkách by bolo 3 × 0.02 = 0.06,
			// zo súčtu 0.21 × 0.23 = 0.0483 → 0.05
			lines: []Line{line("1", "0.07", Rate23), line("1", "0.07", Rate23), line("1", "0.07", Rate23)},
			mode:  PriceNet, base: "0.21", vat: "0.05", total: "0.26", rateCount: 1,
		},
		{
			name:  "viac sadzieb",
			lines: []Line{line("2", "50", Rate23), line("1", "10", Rate5), line("4", "2.5", Rate19)},
			mode:  PriceNet, base: "120.00", vat: "25.40", total: "145.40", rateCount: 3,
		},
		{
			name:  "ceny s DPH",
			lines: []Line{line("1", "123", Rate23)},
			mode:  PriceGross, base: "100.00", vat: "23.00", total: "123.00", rateCount: 1,
		},
		{
			name:  "ceny s DPH, nepárne",
			lines: []Line{line("1", "9.99", Rate23)}, // 9.99*23/123 = 1.868 → 1.87
			mode:  PriceGross, base: "8.12", vat: "1.87", total: "9.99", rateCount: 1,
		},
		{
			name:    "prenesenie daňovej povinnosti",
			lines:   []Line{line("1", "1000", Rate23), line("1", "100", Rate5)},
			mode:    PriceNet,
			reverse: true, base: "1100.00", vat: "0.00", total: "1100.00", rateCount: 1,
		},
		{
			name:  "dobropis (záporné množstvo)",
			lines: []Line{line("-1", "100", Rate23)},
			mode:  PriceNet, base: "-100.00", vat: "-23.00", total: "-123.00", rateCount: 1,
		},
		{
			name:  "prázdny doklad",
			lines: nil,
			mode:  PriceNet, base: "0.00", vat: "0.00", total: "0.00", rateCount: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := Calculate(c.lines, c.mode, c.reverse)
			if err != nil {
				t.Fatal(err)
			}
			if s.Base.String() != c.base || s.VAT.String() != c.vat || s.Total.String() != c.total {
				t.Errorf("základ %s DPH %s spolu %s; chcem %s / %s / %s",
					s.Base, s.VAT, s.Total, c.base, c.vat, c.total)
			}
			if len(s.Rates) != c.rateCount {
				t.Errorf("počet sadzieb %d, chcem %d", len(s.Rates), c.rateCount)
			}
			// Kontrola: spolu = základ + DPH v každom riadku rekapitulácie
			for _, r := range s.Rates {
				if !r.Base.Add(r.VAT).Equal(r.Total) {
					t.Errorf("sadzba %d: %s + %s != %s", r.Rate, r.Base, r.VAT, r.Total)
				}
			}
		})
	}
}

func TestRatesSortedDescending(t *testing.T) {
	s, _ := Calculate([]Line{line("1", "1", Rate5), line("1", "1", Rate23), line("1", "1", Rate19)}, PriceNet, false)
	if s.Rates[0].Rate != Rate23 || s.Rates[1].Rate != Rate19 || s.Rates[2].Rate != Rate5 {
		t.Fatalf("poradie: %+v", s.Rates)
	}
}

func TestUnknownRate(t *testing.T) {
	for _, r := range []Rate{21, 15, -5} {
		if _, err := Calculate([]Line{line("1", "1", r)}, PriceNet, false); err == nil {
			t.Fatalf("sadzba %d %% mala byť odmietnutá", r)
		}
	}
}

func TestRatesOn(t *testing.T) {
	cases := []struct {
		date  string
		valid []Rate
		not   []Rate
	}{
		{"2024-12-31", []Rate{20, 10, 5, 0}, []Rate{23, 19}},
		{"2025-01-01", []Rate{23, 19, 5, 0}, []Rate{20, 10}},
		{"2026-10-29", []Rate{23, 19, 5, 0}, []Rate{20, 10}},
		{"2020-06-01", []Rate{20, 10}, []Rate{23}}, // pred číselníkom → prvé obdobie
	}
	for _, c := range cases {
		for _, r := range c.valid {
			if !r.ValidOn(c.date) {
				t.Errorf("%s: %d %% má platiť", c.date, r)
			}
		}
		for _, r := range c.not {
			if r.ValidOn(c.date) {
				t.Errorf("%s: %d %% nemá platiť", c.date, r)
			}
		}
	}
}

func TestPeriodsAreContinuous(t *testing.T) {
	for i := 1; i < len(Periods); i++ {
		prev, cur := Periods[i-1], Periods[i]
		if prev.To == "" || prev.To >= cur.From {
			t.Fatalf("obdobia %d a %d sa prekrývajú alebo predchádzajúce nie je ukončené", i-1, i)
		}
	}
	if Periods[len(Periods)-1].To != "" {
		t.Fatal("posledné obdobie má platiť doteraz")
	}
}
