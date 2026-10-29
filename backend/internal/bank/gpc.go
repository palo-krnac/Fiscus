package bank

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/table"
)

// numeric sú číselné kódy mien ISO 4217 vo formáte GPC.
var numeric = map[string]string{
	"0978": "EUR", "0203": "CZK", "0840": "USD", "0826": "GBP", "0348": "HUF", "0985": "PLN", "0756": "CHF",
}

// IsGPC vráti true pre výpis vo formáte GPC (ABO): prvý riadok je hlavička „074“.
func IsGPC(data []byte) bool {
	t := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return bytes.HasPrefix(t, []byte("074")) && len(t) >= 114
}

// gpcDate prečíta dátum DDMMRR.
func gpcDate(s string) (invoice.Date, error) {
	t, err := time.Parse("020106", s)
	if err != nil {
		return invoice.Date{}, err
	}
	return invoice.NewDate(t.Year(), t.Month(), t.Day()), nil
}

// field vráti pole na pozíciách from–to (číslované od 1 ako v popise formátu).
func field(line []rune, from, to int) string {
	if from > len(line) {
		return ""
	}
	return strings.TrimSpace(string(line[from-1 : min(to, len(line))]))
}

// ParseGPC načíta výpis vo formáte GPC (ABO), ktorý ponúkajú slovenské a české
// banky (SLSP, ČSOB, Fio…): hlavička 074, pohyby 075, doplnkové správy 076–079.
func ParseGPC(data []byte) ([]Transaction, error) {
	if !IsGPC(data) {
		return nil, ErrFormat
	}
	var out []Transaction
	seen := map[string]int{}
	account := ""
	var stmtDate invoice.Date
	for n, raw := range strings.Split(strings.ReplaceAll(table.Text(data), "\r", ""), "\n") {
		line := []rune(raw)
		if len(line) < 3 {
			continue
		}
		switch string(line[:3]) {
		case "074":
			account = strings.TrimLeft(field(line, 4, 19), "0")
			if d, err := gpcDate(field(line, 109, 114)); err == nil {
				stmtDate = d
			}
		case "075":
			if len(line) < 61 {
				return nil, fmt.Errorf("%w: krátky riadok %d", ErrFormat, n+1)
			}
			cents, err := strconv.ParseInt(field(line, 49, 60), 10, 64) // suma v centoch
			if err != nil || cents < 0 {
				return nil, fmt.Errorf("%w: suma v riadku %d", ErrFormat, n+1)
			}
			amount := money.FromCents(cents)
			dir, storno := In, false
			switch field(line, 61, 61) {
			case "1":
				dir = Out
			case "2":
			case "4": // storno výdaja = príjem
				storno = true
			case "5": // storno príjmu = výdaj
				dir, storno = Out, true
			default:
				return nil, fmt.Errorf("%w: neznámy kód účtovania v riadku %d", ErrFormat, n+1)
			}
			date, err := gpcDate(field(line, 123, 128))
			if err != nil {
				date = stmtDate
			}
			if date.IsZero() {
				return nil, fmt.Errorf("%w: pohyb bez dátumu v riadku %d", ErrFormat, n+1)
			}
			ks := field(line, 72, 81)
			if len(ks) == 10 { // 00 + kód banky (4) + konštantný symbol (4)
				ks = ks[6:]
			}
			cur := numeric[field(line, 119, 122)]
			if cur == "" {
				cur = "EUR"
			}
			t := Transaction{
				Account: account, Date: date, Amount: amount, Currency: cur, Direction: dir,
				VS: NormalizeVS(field(line, 62, 71)), KS: NormalizeVS(ks), SS: NormalizeVS(field(line, 82, 91)),
				CounterName: field(line, 98, 117), CounterIBAN: strings.TrimLeft(field(line, 20, 35), "0"),
				BankRef: strings.TrimLeft(field(line, 36, 48), "0"), Status: StatusOpen,
			}
			if storno {
				t.Message = "storno"
			}
			out = append(out, t)
		case "076", "078", "079":
			// doplnkové údaje: správa pre príjemcu, názov protiúčtu
			if len(out) > 0 {
				t := &out[len(out)-1]
				t.Message = strings.TrimSpace(strings.Join(strings.Fields(t.Message+" "+string(line[3:])), " "))
			}
		}
	}
	for i := range out {
		key := fingerprint(out[i], 0)
		seen[key]++
		out[i].Fingerprint = fingerprint(out[i], seen[key]-1)
	}
	if len(out) == 0 && account == "" {
		return nil, ErrFormat
	}
	return out, nil
}
