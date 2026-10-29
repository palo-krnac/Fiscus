package bank

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// gpcLine poskladá riadok GPC: pole = {od, do, hodnota}; zarovná na 128 znakov.
func gpcLine(fields ...[3]any) string {
	b := []rune(strings.Repeat(" ", 128))
	for _, f := range fields {
		from, to, v := f[0].(int), f[1].(int), []rune(f[2].(string))
		w := to - from + 1
		for len(v) < w {
			v = append(v, ' ')
		}
		copy(b[from-1:to], v[:w])
	}
	return string(b)
}

func gpc075(code, amount, vs, ks, ss, name, cur, date, ref, counter string) string {
	return gpcLine([3]any{1, 3, "075"}, [3]any{4, 19, "0000002900123456"}, [3]any{20, 35, counter}, [3]any{36, 48, ref},
		[3]any{49, 60, amount}, [3]any{61, 61, code}, [3]any{62, 71, vs}, [3]any{72, 81, ks}, [3]any{82, 91, ss},
		[3]any{92, 97, "000000"}, [3]any{98, 117, name}, [3]any{118, 118, "0"}, [3]any{119, 122, cur}, [3]any{123, 128, date})
}

func TestParseGPC(t *testing.T) {
	head := gpcLine([3]any{1, 3, "074"}, [3]any{4, 19, "0000002900123456"}, [3]any{20, 39, "Krnac"}, [3]any{40, 45, "011026"},
		[3]any{46, 59, "00000000100000"}, [3]any{60, 60, "+"}, [3]any{106, 108, "010"}, [3]any{109, 114, "291026"})
	lines := []string{
		head,
		gpc075("2", "000000049200", "0002026001", "0011000308", "0000000000", "Dlznik s.r.o.", "0978", "281026", "0000000000101", "0000000000123456"),
		gpcLine([3]any{1, 3, "078"}, [3]any{4, 38, "Faktúra 2026001"}),
		gpc075("1", "000000001250", "0000000000", "0000000000", "0000000077", "Poplatok", "0978", "291026", "0000000000102", ""),
		gpc075("5", "000000000500", "", "", "", "Vratka", "0203", "", "0000000000103", ""),
	}
	data := []byte(strings.Join(lines, "\r\n") + "\r\n")
	if !IsGPC(data) {
		t.Fatal("IsGPC")
	}
	txs, err := ParseGPC(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 3 {
		t.Fatalf("pohyby: %d", len(txs))
	}
	a, b, c := txs[0], txs[1], txs[2]
	if a.Direction != In || a.Amount.String() != "492.00" || a.VS != "2026001" || a.KS != "308" || a.Date.String() != "2026-10-28" ||
		a.CounterName != "Dlznik s.r.o." || a.Message != "Faktúra 2026001" || a.BankRef != "101" || a.Account != "2900123456" || a.Currency != "EUR" {
		t.Errorf("príjem: %+v", a)
	}
	if b.Direction != Out || b.Amount.String() != "12.50" || b.SS != "77" || b.VS != "" {
		t.Errorf("výdaj: %+v", b)
	}
	// storno príjmu = výdaj; bez dátumu pohybu dátum výpisu
	if c.Direction != Out || c.Message != "storno" || c.Currency != "CZK" || c.Date.String() != "2026-10-29" {
		t.Errorf("storno: %+v", c)
	}
	if a.Fingerprint == b.Fingerprint || a.Fingerprint == "" {
		t.Error("odtlačky")
	}
	bad := []string{
		"075 bez hlavičky",
		head + "\n" + gpc075("9", "000000000100", "", "", "", "", "0978", "281026", "1", ""),
		head + "\n" + gpc075("2", "00000000X100", "", "", "", "", "0978", "281026", "1", ""),
	}
	for _, x := range bad {
		if _, err := ParseGPC([]byte(x)); !errors.Is(err, ErrFormat) {
			t.Errorf("chybný GPC prešiel: %v", err)
		}
	}
}

func TestParseTable(t *testing.T) {
	csv := "Výpis z účtu SK31 1200 0000 1987 4263 7541\n\n" +
		"Dátum zaúčtovania;Suma;Mena;Variabilný symbol;Konštantný symbol;Názov protiúčtu;IBAN protiúčtu;Správa pre príjemcu;ID pohybu\n" +
		"28.10.2026;492,00;EUR;2026001;0308;Dlžník s.r.o.;SK89 0200 0000 0012 3456 7890;Faktúra;A1\n" +
		"29. 10. 2026;-12,50;EUR;;;Banka;;Poplatok za vedenie /VS99/SS77;A2\n" +
		";;;;;;;Spolu;\n"
	txs, err := ParseTable([]byte(csv), "SK3112000000198742637541")
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 2 {
		t.Fatalf("pohyby: %+v", txs)
	}
	a, b := txs[0], txs[1]
	if a.Direction != In || a.Amount.String() != "492.00" || a.VS != "2026001" || a.KS != "308" || a.CounterIBAN != "SK8902000000001234567890" ||
		a.CounterName != "Dlžník s.r.o." || a.Account != "SK3112000000198742637541" || a.BankRef != "A1" || a.Date.String() != "2026-10-28" {
		t.Errorf("príjem: %+v", a)
	}
	if b.Direction != Out || b.Amount.String() != "12.50" || b.VS != "99" || b.SS != "77" || b.Date.String() != "2026-10-29" {
		t.Errorf("výdaj: %+v", b)
	}
	// samostatné stĺpce príjem/výdaj, sériový dátum z Excelu, anglický zápis
	two := "Date,Credit,Debit,Counterparty\n46323,,\"1,234.50\",Dodavatel\n2026-10-29,100.00,,Odberatel\n"
	two = strings.Replace(two, "Credit", "Príjem", 1)
	two = strings.Replace(two, "Debit", "Výdaj", 1)
	txs, err = ParseTable([]byte(two), "")
	if err != nil {
		t.Fatal(err)
	}
	if txs[0].Direction != Out || txs[0].Amount.String() != "1234.50" || txs[0].Date.String() != "2026-10-28" || txs[1].Direction != In {
		t.Errorf("príjem/výdaj: %+v", txs)
	}
	for _, bad := range []string{"Meno;Adresa\nA;B\n", "Dátum;Suma\nzajtra;1\n", "Dátum;Suma\n1.1.2026;veľa\n", "Dátum;Suma\n"} {
		if _, err := ParseTable([]byte(bad), ""); !errors.Is(err, ErrFormat) {
			t.Errorf("chybný CSV %q: %v", bad, err)
		}
	}
}

func TestParseDetect(t *testing.T) {
	tests := []struct {
		name, data, format string
	}{
		{"camt", `<?xml version="1.0"?><Document><BkToCstmrStmt></BkToCstmrStmt></Document>`, "camt.053"},
		{"gpc", gpcLine([3]any{1, 3, "074"}, [3]any{109, 114, "291026"}), "GPC"},
		{"csv", "Dátum;Suma\n1.10.2026;5\n", "CSV/Excel"},
	}
	for _, tt := range tests {
		_, f, _ := Parse([]byte(tt.data), "")
		if f != tt.format {
			t.Errorf("%s: %s", tt.name, f)
		}
	}
	for _, s := range []string{"20.10.2026", "20. 10. 2026", "2026-10-20", "20/10/2026", "20.10.2026 14:05", "2026-10-20T08:00:00", "46315"} {
		d, err := parseDay(s)
		if err != nil || d.String() != "2026-10-20" {
			t.Errorf("parseDay(%q) = %s %v", s, d, err)
		}
	}
	_ = fmt.Sprint()
}
