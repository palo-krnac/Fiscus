package bank

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/table"
)

// Stĺpce výpisu v CSV/Excel. Banky ich pomenúvajú rôzne, preto sa hľadajú
// podľa známych názvov (bez diakritiky a veľkosti písmen).
const (
	tcDate = iota
	tcAmount
	tcCredit // samostatný stĺpec príjmov
	tcDebit  // samostatný stĺpec výdavkov
	tcCurrency
	tcVS
	tcSS
	tcKS
	tcName
	tcIBAN
	tcMessage
	tcRef
	tcAccount
)

var tableHeaders = map[string]int{
	"datum": tcDate, "datum zauctovania": tcDate, "datum pohybu": tcDate, "datum transakcie": tcDate,
	"datum splatnosti": tcDate, "datum realizacie": tcDate, "date": tcDate, "booking date": tcDate, "datum valuty": tcDate,
	"suma": tcAmount, "ciastka": tcAmount, "objem": tcAmount, "amount": tcAmount, "suma transakcie": tcAmount, "suma v mene uctu": tcAmount,
	"prijem": tcCredit, "kredit": tcCredit, "pripisane": tcCredit,
	"vydaj": tcDebit, "vydavok": tcDebit, "debet": tcDebit, "odpisane": tcDebit,
	"mena": tcCurrency, "currency": tcCurrency,
	"vs": tcVS, "variabilny symbol": tcVS, "variabilni symbol": tcVS,
	"ss": tcSS, "specificky symbol": tcSS, "specificky": tcSS,
	"ks": tcKS, "konstantny symbol": tcKS, "konstantni symbol": tcKS,
	"nazov protiuctu": tcName, "protistrana": tcName, "nazov protistrany": tcName, "prijemca/platitel": tcName,
	"platitel/prijemca": tcName, "meno": tcName, "nazov uctu protistrany": tcName, "counterparty": tcName,
	"iban protiuctu": tcIBAN, "protiucet": tcIBAN, "cislo protiuctu": tcIBAN, "ucet protistrany": tcIBAN, "iban protistrany": tcIBAN,
	"sprava": tcMessage, "sprava pre prijemcu": tcMessage, "poznamka": tcMessage, "popis": tcMessage, "popis transakcie": tcMessage,
	"informacia pre prijemcu": tcMessage, "detail": tcMessage, "ucel platby": tcMessage,
	"id pohybu": tcRef, "id transakcie": tcRef, "referencia": tcRef, "referencia banky": tcRef, "id": tcRef,
	"iban uctu": tcAccount, "cislo uctu": tcAccount, "ucet": tcAccount, "iban": tcAccount,
}

var hdrFold = strings.NewReplacer(
	"á", "a", "ä", "a", "č", "c", "ď", "d", "é", "e", "ě", "e", "í", "i", "ĺ", "l", "ľ", "l", "ň", "n",
	"ó", "o", "ô", "o", "ŕ", "r", "ř", "r", "š", "s", "ť", "t", "ú", "u", "ů", "u", "ý", "y", "ž", "z",
)

func hdrKey(s string) string {
	s = strings.Trim(strings.ToLower(strings.Join(strings.Fields(s), " ")), ":")
	return hdrFold.Replace(s)
}

// parseAmount prečíta sumu so znamienkom: „-1 234,50“, „1234.50“, „+12,00 EUR“.
func parseAmount(s string) (money.Dec, error) {
	s = strings.NewReplacer(" ", "", " ", "", " ", "", "€", "", "EUR", "", "+", "").Replace(strings.TrimSpace(s))
	// desatinný oddeľovač je posledná čiarka alebo bodka („1.234,50“ aj „1,234.50“)
	if c, d := strings.LastIndex(s, ","), strings.LastIndex(s, "."); c > d {
		s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	} else if c >= 0 {
		s = strings.ReplaceAll(s, ",", "")
	}
	if s == "" {
		return money.Zero, nil
	}
	return money.Parse(s)
}

// parseDay prečíta dátum v bežných zápisoch alebo ako sériové číslo Excelu.
func parseDay(s string) (invoice.Date, error) {
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 20000 && n < 80000 {
		t := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n))
		return invoice.NewDate(t.Year(), t.Month(), t.Day()), nil
	}
	if i := strings.IndexAny(s, " T"); i > 7 && strings.Count(s[:i], ".") != 2 { // „2026-10-29 10:15“
		s = s[:i]
	}
	s = strings.ReplaceAll(s, " ", "")
	for _, layout := range []string{"2006-01-02", "2.1.2006", "02.01.2006", "2/1/2006", "02/01/2006", "2.1.06", "20060102"} {
		if t, err := time.Parse(layout, s); err == nil {
			return invoice.NewDate(t.Year(), t.Month(), t.Day()), nil
		}
	}
	if len(s) > 10 { // „29.10.2026 10:15“
		return parseDay(s[:10])
	}
	return invoice.Date{}, fmt.Errorf("neznámy dátum %q", s)
}

// ParseTable načíta pohyby z výpisu v CSV alebo Exceli (export z internet
// bankingu). Prvý riadok s rozpoznaným dátumom a sumou je hlavička.
func ParseTable(data []byte, ownAccount string) ([]Transaction, error) {
	rows, err := table.Read(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	head := -1
	cols := map[int]int{}
	for i, r := range rows {
		if i > 15 {
			break
		}
		c := map[int]int{}
		have := map[int]bool{}
		for j, h := range r {
			if k, ok := tableHeaders[hdrKey(h)]; ok && !have[k] {
				c[j], have[k] = k, true
			}
		}
		if have[tcDate] && (have[tcAmount] || have[tcCredit] || have[tcDebit]) {
			head, cols = i, c
			break
		}
	}
	if head < 0 {
		return nil, fmt.Errorf("%w: v súbore chýba hlavička so stĺpcami Dátum a Suma", ErrFormat)
	}
	var out []Transaction
	seen := map[string]int{}
	for li, r := range rows[head+1:] {
		line := head + li + 2
		cell := map[int]string{}
		for j, v := range r {
			if k, ok := cols[j]; ok && cell[k] == "" {
				cell[k] = v
			}
		}
		if cell[tcDate] == "" {
			continue // súčtové a prázdne riadky
		}
		date, err := parseDay(cell[tcDate])
		if err != nil {
			return nil, fmt.Errorf("%w: riadok %d: %v", ErrFormat, line, err)
		}
		amount, err := parseAmount(cell[tcAmount])
		if err != nil {
			return nil, fmt.Errorf("%w: riadok %d: suma %q", ErrFormat, line, cell[tcAmount])
		}
		if amount.IsZero() {
			cr, err1 := parseAmount(cell[tcCredit])
			db, err2 := parseAmount(cell[tcDebit])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("%w: riadok %d: suma", ErrFormat, line)
			}
			if db.Sign() > 0 {
				db = db.Neg() // výdavok v samostatnom stĺpci býva bez znamienka
			}
			amount = cr.Add(db)
		}
		if amount.IsZero() {
			continue
		}
		dir := In
		if amount.Sign() < 0 {
			dir, amount = Out, amount.Neg()
		}
		cur := strings.ToUpper(strings.TrimSpace(cell[tcCurrency]))
		if cur == "" {
			cur = "EUR"
		}
		account := strings.ToUpper(strings.ReplaceAll(cell[tcAccount], " ", ""))
		if account == "" {
			account = strings.ToUpper(strings.ReplaceAll(ownAccount, " ", ""))
		}
		t := Transaction{
			Account: account, Date: date, Amount: amount, Currency: cur, Direction: dir,
			VS: NormalizeVS(cell[tcVS]), SS: NormalizeVS(cell[tcSS]), KS: NormalizeVS(cell[tcKS]),
			CounterName: cell[tcName], CounterIBAN: strings.ToUpper(strings.ReplaceAll(cell[tcIBAN], " ", "")),
			Message: strings.Join(strings.Fields(cell[tcMessage]), " "), BankRef: cell[tcRef], Status: StatusOpen,
		}
		vs, ss, ks := ParseSymbols(t.Message) // symboly zo správy, ak nie sú v stĺpcoch
		t.VS, t.SS, t.KS = firstNonEmpty(t.VS, vs), firstNonEmpty(t.SS, ss), firstNonEmpty(t.KS, ks)
		key := fingerprint(t, 0)
		seen[key]++
		t.Fingerprint = fingerprint(t, seen[key]-1)
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: výpis neobsahuje žiadne pohyby", ErrFormat)
	}
	return out, nil
}

// Parse rozpozná formát výpisu (camt.053, GPC, CSV alebo Excel) a načíta pohyby.
// ownAccount (IBAN firmy) sa použije, ak účet nie je vo výpise.
func Parse(data []byte, ownAccount string) ([]Transaction, string, error) {
	switch {
	case strings.Contains(string(data[:min(len(data), 4096)]), "BkToCstmrStmt") || strings.HasPrefix(strings.TrimSpace(table.Text(data[:min(len(data), 64)])), "<"):
		txs, err := ParseCAMT053(data)
		return txs, "camt.053", err
	case IsGPC(data):
		txs, err := ParseGPC(data)
		return txs, "GPC", err
	}
	txs, err := ParseTable(data, ownAccount)
	return txs, "CSV/Excel", err
}
