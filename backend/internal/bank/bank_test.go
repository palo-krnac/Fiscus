package bank

import (
	"errors"
	"os"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestParseSymbols(t *testing.T) {
	tests := []struct{ in, vs, ss, ks string }{
		{"/VS20260037/SS/KS0308", "20260037", "", "308"},
		{"/VS0001234/SS55/KS0558", "1234", "55", "558"},
		{"Faktura VS: 0202601200 pohonne hmoty", "202601200", "", ""},
		{"vs 123 ks 0308", "123", "", "308"},
		{"bez symbolu", "", "", ""},
		{"VSETKO OK 123", "", "", ""}, // „VS“ v slove nie je symbol
	}
	for _, tt := range tests {
		vs, ss, ks := ParseSymbols(tt.in)
		if vs != tt.vs || ss != tt.ss || ks != tt.ks {
			t.Errorf("%q → %q %q %q", tt.in, vs, ss, ks)
		}
	}
}

func load(t *testing.T, name string) []Transaction {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	txs, err := ParseCAMT053(b)
	if err != nil {
		t.Fatal(err)
	}
	return txs
}

func TestParseCAMT053(t *testing.T) {
	txs := load(t, "vypis.xml")
	type row struct {
		dir                 Direction
		amount, vs, date    string
		counter, iban, cref string
	}
	want := []row{
		{In, "2275.5", "20260037", "2026-10-20", "Stavmont Zvolen, a.s.", "SK8975000000000012345671", "REF-0001"},
		{Out, "100", "202601200", "2026-10-25", "Čerpacia stanica Hron s.r.o.", "SK0211000000002612345678", "REF-0002"},
		// blokácia PDNG sa vynechá; hromadná platba sa rozdelí
		{In, "120", "20260031", "2026-10-28", "Mlyn Brezno s.r.o.", "", "REF-0004#1"},
		{In, "180", "20260032", "2026-10-28", "Agro Liptov družstvo", "", "REF-0004#2"},
		{Out, "2.5", "", "2026-10-31", "", "", ""},
		{Out, "2.5", "", "2026-10-31", "", "", ""},
	}
	if len(txs) != len(want) {
		t.Fatalf("%d pohybov, chcem %d: %+v", len(txs), len(want), txs)
	}
	for i, w := range want {
		g := txs[i]
		if g.Direction != w.dir || !g.Amount.Equal(money.MustParse(w.amount)) || g.VS != w.vs || g.Date.String() != w.date ||
			g.CounterName != w.counter || g.CounterIBAN != w.iban || g.BankRef != w.cref || g.Account != "SK3112000000198742637541" || g.Currency != "EUR" {
			t.Errorf("pohyb %d: %+v", i, g)
		}
	}
	if txs[4].Message != "Poplatok za vedenie uctu" {
		t.Errorf("správa: %q", txs[4].Message)
	}
	// dva rovnaké poplatky majú rôzny odtlačok, opakovaný import rovnaké
	if txs[4].Fingerprint == txs[5].Fingerprint {
		t.Error("rovnaké pohyby v jednom výpise musia mať rôzny odtlačok")
	}
	again := load(t, "vypis.xml")
	for i := range txs {
		if txs[i].Fingerprint != again[i].Fingerprint {
			t.Errorf("odtlačok %d sa pri opakovanom importe zmenil", i)
		}
	}

	v8 := load(t, "vypis_v08.xml")
	if len(v8) != 1 || v8[0].CounterName != "Elektro Hron s.r.o." || v8[0].VS != "20260026" || v8[0].Date.String() != "2026-10-12" || v8[0].BankRef != "X-77" {
		t.Errorf("camt.053.001.08: %+v", v8)
	}
}

func TestParseRejectsOtherFiles(t *testing.T) {
	for _, in := range []string{"", "nie je xml", `<Document><CstmrCdtTrfInitn/></Document>`, `<BkToCstmrStmt><Stmt><Ntry><Amt>1</Amt><CdtDbtInd>XX</CdtDbtInd><BookgDt><Dt>2026-01-01</Dt></BookgDt></Ntry></Stmt></BkToCstmrStmt>`} {
		if _, err := ParseCAMT053([]byte(in)); !errors.Is(err, ErrFormat) {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestSuggestAndAutoMatch(t *testing.T) {
	d := invoice.MustDate
	tx := Transaction{Direction: In, Amount: money.MustParse("2275.50"), VS: "20260037", CounterName: "Stavmont Zvolen, a.s."}
	cands := []Candidate{
		{Kind: "invoice", ID: 1, Number: "FA20260037", VS: "20260037", Partner: "Stavmont Zvolen, a.s.", Amount: money.MustParse("2275.50"), Direction: In, Due: d("2026-10-30")},
		{Kind: "invoice", ID: 2, Number: "FA20260010", VS: "20260010", Partner: "Stavmont Zvolen a.s.", Amount: money.MustParse("2275.50"), Direction: In, Due: d("2026-07-01")},
		{Kind: "invoice", ID: 3, Number: "FA20260011", VS: "20260011", Partner: "Iný", Amount: money.MustParse("2275.50"), Direction: In, Due: d("2026-07-01")},
		{Kind: "purchase", ID: 4, Number: "PF1", VS: "20260037", Amount: money.MustParse("2275.50"), Direction: Out},
		{Kind: "invoice", ID: 5, Number: "FA20260099", VS: "99", Amount: money.MustParse("1"), Direction: In},
	}
	s := Suggest(tx, cands)
	if len(s) != 3 || s[0].ID != 1 || s[0].Score != ScoreExact || s[1].ID != 2 || s[1].Score != ScoreAmountWho || s[2].Score != ScoreAmount {
		t.Fatalf("návrhy: %+v", s)
	}
	if c, ok := AutoMatch(tx, cands); !ok || c.ID != 1 {
		t.Fatalf("automatické párovanie: %+v %v", c, ok)
	}

	tests := []struct {
		name string
		tx   Transaction
		ok   bool
	}{
		{"čiastočná platba", Transaction{Direction: In, Amount: money.MustParse("1000"), VS: "20260037"}, false},
		{"bez VS", Transaction{Direction: In, Amount: money.MustParse("2275.50")}, false},
		{"VS s nulami", Transaction{Direction: In, Amount: money.MustParse("2275.50"), VS: "0020260037"}, true},
		{"nejednoznačné", Transaction{Direction: In, Amount: money.MustParse("2275.50"), VS: "20260037"}, false},
	}
	for _, tt := range tests {
		cs := cands
		if tt.name == "nejednoznačné" {
			cs = append(append([]Candidate{}, cands...), Candidate{Kind: "invoice", ID: 9, VS: "20260037", Amount: money.MustParse("2275.50"), Direction: In})
		}
		if _, ok := AutoMatch(tt.tx, cs); ok != tt.ok {
			t.Errorf("%s: %v", tt.name, ok)
		}
	}
}
