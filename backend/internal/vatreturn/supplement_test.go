package vatreturn

import (
	"errors"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestApplySupplement(t *testing.T) {
	ptr := func(s string) *money.Dec { d := money.MustParse(s); return &d }
	p, _ := ParsePeriod("2026-09")
	if p.Deadline().String() != "2026-10-25" {
		t.Fatalf("lehota: %s", p.Deadline())
	}
	// faktúra s daňou 820 € (príklad z poučenia: riadne −60 €, dodatočné +760 €)
	base := func() Return {
		r := Compute(Input{Period: p, Invoices: []invoice.Invoice{inv(1, "FA1", "2026-09-10", "SK2020372640", map[vat.Rate]string{23: "3565.22"})}})
		r.Rows["r32"], r.Rows["r35"] = money.MustParse("760"), money.MustParse("760")
		return r
	}
	tests := []struct {
		name     string
		o        Options
		err      error
		r36, r37 string
	}{
		{"riadne", Options{}, nil, "0.00", "0.00"},
		{"opravné", Options{Kind: KindCorrective}, nil, "0.00", "0.00"},
		{"dodatočné", Options{Kind: KindSupplementary, Discovered: d("2026-11-03"), LastKnown: ptr("-60")}, nil, "820.00", "820.00"},
		{"dodatočné s inou r. 37", Options{Kind: KindSupplementary, Discovered: d("2026-11-03"), LastKnown: ptr("-60"), R37: ptr("760")}, nil, "820.00", "760.00"},
		{"zníženie povinnosti", Options{Kind: KindSupplementary, Discovered: d("2026-11-03"), LastKnown: ptr("1000")}, nil, "-240.00", "-240.00"},
		{"bez dátumu zistenia", Options{Kind: KindSupplementary, LastKnown: ptr("0")}, ErrNoDiscovery, "", ""},
		{"bez poslednej známej", Options{Kind: KindSupplementary, Discovered: d("2026-11-03")}, ErrNoLastKnown, "", ""},
		{"neznámy druh", Options{Kind: "x"}, ErrBadKind, "", ""},
	}
	for _, tt := range tests {
		r := base()
		err := r.Apply(tt.o)
		if !errors.Is(err, tt.err) {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if err != nil {
			continue
		}
		if r.Rows["r36"].RoundCents().String() != tt.r36 || r.Rows["r37"].RoundCents().String() != tt.r37 {
			t.Errorf("%s: r36=%s r37=%s", tt.name, r.Rows["r36"], r.Rows["r37"])
		}
	}

	tp := Taxpayer{Name: "Firma", DIC: "2020372640", ICDPH: "SK2020372640", City: "Zvolen"}
	r := base()
	_ = r.Apply(Options{Kind: KindSupplementary, Discovered: d("2026-11-03"), LastKnown: ptr("-60")})
	x := string(DPHXML(r, tp, d("2026-11-04")))
	for _, want := range []string{"<rdp>0</rdp><odp>0</odp><ddp>1</ddp><datumZisteniaDdp>03.11.2026</datumZisteniaDdp>", "<r36>820.00</r36>", "<r37>820.00</r37>"} {
		if !strings.Contains(x, want) {
			t.Errorf("DPH XML bez %s", want)
		}
	}
	r = base()
	_ = r.Apply(Options{Kind: KindCorrective})
	if x := string(DPHXML(r, tp, d("2026-10-20"))); !strings.Contains(x, "<rdp>0</rdp><odp>1</odp><ddp>0</ddp>") {
		t.Error("opravné v XML")
	}
	if kv := string(KVXML(r, tp)); !strings.Contains(kv, "<Druh>O</Druh>") {
		t.Error("opravný KV")
	}
	r = base()
	if kv := string(KVXML(r, tp)); !strings.Contains(kv, "<Druh>R</Druh>") || !strings.Contains(string(DPHXML(r, tp, d("2026-10-20"))), "<rdp>1</rdp>") {
		t.Error("riadne")
	}
}
