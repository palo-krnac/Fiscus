package payqr

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "prepísať testdata/golden.json")

// golden sú vzorky, ktoré frontend (lib/payqr.test.js) dekóduje knižnicami
// bysquare a jsQR — overenie, že banky QR prečítajú rovnako ako z frontendu.
type golden struct {
	Name    string   `json:"name"`
	ID      string   `json:"id"`
	Payment Payment  `json:"payment"`
	Text    string   `json:"text"`
	Version int      `json:"version"`
	Rows    []string `json:"rows"` // „1“ = tmavý modul
}

var samples = []struct {
	name string
	id   string
	p    Payment
}{
	{"faktúra", "FA20260039", Payment{Amount: "269.12", Currency: "EUR", DueDate: "20261112", VariableSymbol: "20260039",
		Note: "Faktura FA20260039", IBAN: "SK31 1200 0000 1987 4263 7541", Name: "Demo elektroservis", Street: "Námestie SNP 1", City: "974 01 Banská Bystrica"}},
	{"celé eurá a diakritika", "FA20260001", Payment{Amount: "100", Currency: "EUR", DueDate: "20261231", VariableSymbol: "1",
		Note: "Faktúra č. 1 – ďakujeme", IBAN: "sk3112000000198742637541", Name: "Pavol Krnáč – elektroservis", City: "Banská Bystrica"}},
	{"dlhá poznámka", "FA2026LONG", Payment{Amount: "123456.78", Currency: "EUR", DueDate: "20270101", VariableSymbol: "2026999999",
		Note: strings.Repeat("Servisný paušál a podpora ", 5), IBAN: "SK3112000000198742637541", Name: strings.Repeat("Veľmi dlhý názov firmy ", 3),
		Street: "Hlavná 1", City: "811 01 Bratislava"}},
}

func TestEncodeAndGolden(t *testing.T) {
	var out []golden
	for _, s := range samples {
		text, err := Encode(s.id, s.p)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		q, err := NewQR(text)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		rows := make([]string, q.Size)
		for y, r := range q.Modules {
			var b strings.Builder
			for _, d := range r {
				if d {
					b.WriteByte('1')
				} else {
					b.WriteByte('0')
				}
			}
			rows[y] = b.String()
		}
		out = append(out, golden{s.name, s.id, s.p, text, q.Version, rows})
	}
	data, _ := json.MarshalIndent(out, "", " ")
	data = append(data, '\n')
	if *update {
		if err := os.WriteFile("testdata/golden.json", data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, data) {
		t.Fatal("výstup sa líši od testdata/golden.json — over frontendovým testom a spusti go test -update")
	}
}

func TestEncodeErrors(t *testing.T) {
	ok := samples[0].p
	tests := []struct {
		name string
		mod  func(*Payment)
	}{
		{"bez IBAN", func(p *Payment) { p.IBAN = " " }},
		{"bez sumy", func(p *Payment) { p.Amount = "" }},
		{"bez príjemcu", func(p *Payment) { p.Name = "" }},
	}
	for _, tt := range tests {
		p := ok
		tt.mod(&p)
		if _, err := Encode("X", p); err == nil {
			t.Errorf("%s: prešlo", tt.name)
		}
	}
	if _, err := NewQR("malé písmená"); err == nil {
		t.Error("QR mimo alfanumerickej sady")
	}
	if _, err := NewQR(strings.Repeat("A", 4300)); err != ErrTooLong {
		t.Errorf("príliš dlhý text: %v", err)
	}
}

func TestPieces(t *testing.T) {
	if got := base32hex([]byte("foobar")); got != "CPNMUOJ1E8" { // RFC 4648 bez „=“
		t.Errorf("base32hex = %q", got)
	}
	if got := Deburr("Žltučký kôň úpel ďábelské ódy – Ŀ"); got != "Zltucky kon upel dabelske ody – L" {
		t.Errorf("Deburr = %q", got)
	}
	if got := serialize("FA1", Payment{Amount: "1.5", Currency: "EUR", Note: "a\tb", IBAN: "SK1", Name: "N"}); got !=
		"FA1\t1\t1\t1.5\tEUR\t\t\t\t\t\ta b\t1\tSK1\t\t0\t0\tN\t\t" {
		t.Errorf("serialize = %q", got)
	}
	// veľkosti QR podľa normy: verzia 1 má 21 modulov, 7 má aj informáciu o verzii
	for _, n := range []int{1, 100, 400} {
		q, err := NewQR(strings.Repeat("7", n))
		if err != nil || q.Size != q.Version*4+17 || !q.Modules[0][0] || q.Modules[7][7] {
			t.Errorf("QR %d znakov: %+v %v", n, q.Version, err)
		}
	}
}
