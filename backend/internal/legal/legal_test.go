package legal

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestDefaults(t *testing.T) {
	p := Defaults()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.Seed() {
		t.Fatal("predvolené hodnoty nemajú čo dopĺňať")
	}
	law := p.Law()
	if law.BusinessMargin.String() != "9.00" || law.ConsumerMargin.String() != "5.00" || law.FlatFee.String() != "40.00" ||
		law.ECB.Updated() != reminder.BuiltinECB().Updated() {
		t.Fatalf("parametre: %+v", law)
	}
	if !reflect.DeepEqual(p.VATPeriods, vat.DefaultPeriods) {
		t.Fatal("sadzby DPH")
	}
}

// Nová verzia programu prinesie nové riadky — doplnia sa, úpravy používateľa ostanú.
func TestSeedUpgrade(t *testing.T) {
	old := Defaults()
	last := old.ECB[len(old.ECB)-1]
	// stav „staršej verzie“: posledná sadzba ECB ani prirážka spotrebiteľa ešte neexistovali
	old.ECB = old.ECB[:len(old.ECB)-1]
	old.Seeded = slices.DeleteFunc(old.Seeded, func(k string) bool { return k == "ecb:"+last.From.String() || k == "param:consumerMargin" })
	old.ConsumerMargin = m.Zero
	// používateľ si medzitým upravil sadzbu a jednu zmazal a pridal vlastnú
	old.ECB[len(old.ECB)-1].Rate = m.MustParse("2.20")
	deleted := old.ECB[0]
	old.ECB = old.ECB[1:]
	old.ECB = append(old.ECB, reminder.Rate{From: invoice.MustDate("2027-03-01"), Rate: m.MustParse("3")})

	if !old.Seed() {
		t.Fatal("nič sa nedoplnilo")
	}
	has := func(d invoice.Date) bool {
		return slices.ContainsFunc(old.ECB, func(r reminder.Rate) bool { return r.From == d })
	}
	if !has(last.From) || has(deleted.From) || !has(invoice.MustDate("2027-03-01")) {
		t.Fatalf("doplnenie: %+v", old.ECB)
	}
	if old.ConsumerMargin.String() != "5.00" {
		t.Fatal("nová prirážka")
	}
	upd := old.ECB[slices.IndexFunc(old.ECB, func(r reminder.Rate) bool { return r.From == last.From })-1]
	if upd.Rate.String() != "2.20" {
		t.Fatalf("úprava používateľa sa prepísala: %+v", upd)
	}
	if !slices.IsSortedFunc(old.ECB, func(a, b reminder.Rate) int { return a.From.DaysUntil(b.From) * -1 }) {
		t.Fatal("zoradenie")
	}
	if old.Seed() {
		t.Fatal("druhé doplnenie")
	}
}

func TestSeedNewVATPeriod(t *testing.T) {
	p := Defaults()
	p.VATPeriods = p.VATPeriods[:1]
	p.VATPeriods[0].To = ""
	p.Seeded = slices.DeleteFunc(p.Seeded, func(k string) bool { return k == "vat:2025-01-01" })
	p.Seed()
	if len(p.VATPeriods) != 2 || p.VATPeriods[0].To != "2024-12-31" || p.VATPeriods[1].From != "2025-01-01" {
		t.Fatalf("nové obdobie DPH: %+v", p.VATPeriods)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidate(t *testing.T) {
	field := func(err error) []string {
		var ve *invoice.ValidationError
		if !errors.As(err, &ve) {
			return nil
		}
		var out []string
		for _, f := range ve.Fields {
			out = append(out, f.Field)
		}
		return out
	}
	cases := []struct {
		name string
		edit func(*Params)
		want string
	}{
		{"bez ECB", func(p *Params) { p.ECB = nil }, "ecb"},
		{"ECB bez dátumu", func(p *Params) { p.ECB[0].From = invoice.Date{} }, "ecb[0].from"},
		{"ECB mimo", func(p *Params) { p.ECB[1].Rate = m.MustParse("30") }, "ecb[1].rate"},
		{"ECB dvakrát", func(p *Params) { p.ECB[2].From = p.ECB[1].From }, "ecb[2].from"},
		{"prirážka", func(p *Params) { p.BusinessMargin = m.MustParse("-1") }, "businessMargin"},
		{"prirážka spotrebiteľ", func(p *Params) { p.ConsumerMargin = m.MustParse("51") }, "consumerMargin"},
		{"náhrada", func(p *Params) { p.FlatFee = m.MustParse("-5") }, "flatFee"},
		{"DPH", func(p *Params) { p.VATPeriods[0].To = "" }, "vatPeriods"},
	}
	for _, c := range cases {
		p := Defaults()
		c.edit(&p)
		if got := field(p.Validate()); !slices.Contains(got, c.want) {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

func TestNormalizeAndJSON(t *testing.T) {
	p := Defaults()
	p.ECB[0], p.ECB[1] = p.ECB[1], p.ECB[0]
	p.VATPeriods[1].Rates = []vat.Rate{0, 5, 23, 19}
	p.Normalize()
	if !p.ECB[0].From.Before(p.ECB[1].From) || !reflect.DeepEqual(p.VATPeriods[1].Rates, []vat.Rate{23, 19, 5, 0}) {
		t.Fatalf("normalizácia: %+v %+v", p.ECB[:2], p.VATPeriods[1])
	}
	b, _ := json.Marshal(p)
	var back Params
	if err := json.Unmarshal(b, &back); err != nil || !reflect.DeepEqual(back, p) {
		t.Fatalf("JSON:\n got %+v\nwant %+v", back, p)
	}
}
