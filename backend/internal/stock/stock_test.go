package stock

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func m(s string) money.Dec { return money.MustParse(s) }

func doc(id int64, k Kind, date string, lines ...Line) Doc {
	d := Doc{ID: id, Number: k.Prefix() + string(rune('0'+id)), Kind: k, Date: invoice.MustDate(date), Warehouse: "HL", Lines: lines}
	if err := d.Validate(); err != nil {
		panic(err)
	}
	return d
}

func ln(q, cost string) Line {
	return Line{ItemID: 1, Name: "Kábel", Unit: "m", Quantity: m(q), UnitCost: m(cost)}
}

func TestValuate(t *testing.T) {
	docs := []Doc{
		doc(1, Receipt, "2026-01-05", ln("100", "1.00")),
		doc(2, Receipt, "2026-01-10", ln("50", "1.30")),
		// výdaj v ten istý deň ako príjem ide po príjme
		doc(3, Issue, "2026-01-10", ln("30", "0")),
		doc(4, Count, "2026-02-01", ln("-2", "0")),
		doc(5, Issue, "2026-02-10", ln("118", "0")), // posledné kusy: celý zostatok
	}
	cards, costed, probs := Valuate(docs, Average)
	c := cards[Key{"HL", 1}]
	if len(probs) != 0 || !c.Quantity.IsZero() || !c.Value.IsZero() {
		t.Fatalf("stav: %+v %v", c, probs)
	}
	byID := map[int64]Doc{}
	for _, d := range costed {
		byID[d.ID] = d
	}
	cases := []struct {
		id         int64
		total, avg string
	}{
		{3, "33.00", "1.10"}, // (100 + 65) / 150 = 1,10
		{4, "2.20", "1.10"},  // manko 2 × 1,10
		{5, "129.80", ""},    // 165 − 33 − 2,20
	}
	for _, tc := range cases {
		l := byID[tc.id].Lines[0]
		if l.Total.String() != tc.total || (tc.avg != "" && l.UnitCost.String() != tc.avg) {
			t.Errorf("doklad %d: %s %s", tc.id, l.Total, l.UnitCost)
		}
	}
	if len(c.Moves) != 5 || c.Moves[2].Balance.String() != "120.00" || c.Moves[2].Value.String() != "132.00" {
		t.Errorf("pohyby: %+v", c.Moves)
	}
	if docs[2].Lines[0].Total.String() != "0.00" {
		t.Error("vstupné doklady sa nemajú meniť")
	}

	// výdaj nad stav a vrátenie priemernou cenou
	ret := doc(7, Receipt, "2026-03-02", ln("5", "9.99"))
	ret.Return = true
	_ = ret.Validate()
	cards, costed, probs = Valuate([]Doc{
		doc(1, Receipt, "2026-03-01", ln("10", "2.00")),
		ret,
		doc(6, Issue, "2026-03-03", ln("20", "0")),
	}, Average)
	if len(probs) != 1 || costed[1].Lines[0].Total.String() != "10.00" || costed[2].Lines[0].Total.String() != "40.00" ||
		cards[Key{"HL", 1}].Quantity.String() != "-5.00" {
		t.Errorf("nad stav: %v %+v %+v", probs, costed, cards[Key{"HL", 1}])
	}
}

func TestValidateAndNumber(t *testing.T) {
	d := Doc{Kind: Issue, Date: invoice.MustDate("2026-01-01"), Warehouse: "HL", Lines: []Line{{ItemID: 1, Quantity: m("-1")}}}
	if err := d.Validate(); err == nil {
		t.Error("záporný výdaj")
	}
	d = Doc{Kind: "x"}
	if err := d.Validate(); err == nil {
		t.Error("neznámy druh")
	}
	docs := []Doc{{Number: "PR2026001"}, {Number: "PR2026007"}, {Number: "VY2026002"}, {Number: "PR2025009"}}
	if n := NextNumber(docs, Receipt, 2026); n != "PR2026008" {
		t.Errorf("číslo: %s", n)
	}
	if n := NextNumber(docs, Count, 2026); n != "IR2026001" {
		t.Errorf("číslo: %s", n)
	}
}

func TestFIFOAndTransfer(t *testing.T) {
	tr := Doc{ID: 9, Number: "PS1", Kind: Transfer, Date: invoice.MustDate("2026-01-20"), Warehouse: "HL", To: "ZV",
		Lines: []Line{{ItemID: 1, Name: "Kábel", Unit: "m", Quantity: m("30")}}}
	if err := tr.Validate(); err != nil {
		t.Fatal(err)
	}
	docs := []Doc{
		doc(1, Receipt, "2026-01-05", ln("100", "1.00")),
		doc(2, Receipt, "2026-01-10", ln("50", "1.30")),
		doc(3, Issue, "2026-01-15", ln("110", "0")), // FIFO: 100 × 1,00 + 10 × 1,30
		tr, // FIFO: 30 × 1,30
	}
	cases := []struct {
		m             Method
		issue, transf string
		hl, zv        string
	}{
		{FIFO, "113.00", "39.00", "13.00", "39.00"},
		{Average, "121.00", "33.00", "11.00", "33.00"},
	}
	for _, tc := range cases {
		cards, costed, probs := Valuate(docs, tc.m)
		by := map[int64]Doc{}
		for _, d := range costed {
			by[d.ID] = d
		}
		hl, zv := cards[Key{"HL", 1}], cards[Key{"ZV", 1}]
		if len(probs) != 0 || by[3].Total.String() != tc.issue || by[9].Total.String() != tc.transf ||
			hl.Quantity.String() != "10.00" || hl.Value.String() != tc.hl || zv.Quantity.String() != "30.00" || zv.Value.String() != tc.zv {
			t.Errorf("%s: výdaj %s presun %s HL %s/%s ZV %s/%s %v", tc.m, by[3].Total, by[9].Total, hl.Quantity, hl.Value, zv.Quantity, zv.Value, probs)
		}
	}
	bad := Doc{Kind: Transfer, Date: invoice.MustDate("2026-01-01"), Warehouse: "HL", To: "HL", Lines: []Line{{ItemID: 1, Quantity: m("1")}}}
	if err := bad.Validate(); err == nil {
		t.Error("presun do toho istého skladu")
	}
}

// Výdaj nad stav a neskorší príjem: obe metódy musia spolu oceniť výdaje
// celou obstarávacou cenou a hodnota karty pri nulovom stave je nula.
func TestShortageThenReceipt(t *testing.T) {
	docs := []Doc{
		doc(1, Issue, "2026-02-01", ln("2", "0")),
		doc(2, Receipt, "2026-02-03", ln("10", "1.00")),
		doc(3, Issue, "2026-02-05", ln("8", "0")),
	}
	for _, meth := range []Method{FIFO, Average} {
		cards, costed, probs := Valuate(docs, meth)
		sum := money.Zero
		for _, d := range costed {
			if d.Kind == Issue {
				sum = sum.Add(d.Total)
			}
		}
		c := cards[Key{"HL", 1}]
		if sum.String() != "10.00" || !c.Quantity.IsZero() || !c.Value.IsZero() || len(probs) != 1 {
			t.Errorf("%s: výdaje %s, stav %s/%s, problémy %v", meth, sum, c.Quantity, c.Value, probs)
		}
	}
}
