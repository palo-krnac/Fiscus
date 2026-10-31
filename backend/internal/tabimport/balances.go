package tabimport

import (
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Balance je zostatok účtu z tabuľky (obratová predvaha, konečná súvaha
// z iného programu): na strane MD alebo D.
type Balance struct {
	Line    int
	Account string // ako v súbore (211, 221.001, 221 001)
	Name    string
	Debit   money.Dec
	Credit  money.Dec
	Err     string
}

// stĺpce zostatkov (vlastný slovník — „účet“ tu nie je bankový účet)
const (
	bAccount = iota
	bName
	bDebit
	bCredit
	bSigned
)

var balanceAliases = map[string]int{}

func init() {
	add := func(c int, names ...string) {
		for _, n := range names {
			balanceAliases[n] = c
		}
	}
	add(bAccount, "ucet", "cislo uctu", "account", "syntetika", "analytika", "ucet md/d", "ucet (analytika)")
	add(bName, "nazov", "nazov uctu", "nazev", "nazev uctu", "name", "account name", "text")
	add(bDebit, "md", "ma dat", "debet", "debit", "ps md", "pociatocny stav md", "zostatok md", "konecny stav md", "ks md", "zustatek md",
		"pocatecni stav md", "konecny zustatek md", "md (eur)")
	add(bCredit, "d", "dal", "dal (eur)", "kredit", "credit", "ps d", "ps dal", "pociatocny stav d", "pociatocny stav dal", "zostatok d", "zostatok dal",
		"konecny stav d", "konecny stav dal", "ks d", "ks dal", "zustatek d", "zustatek dal", "pocatecni stav d", "pocatecni stav dal")
	add(bSigned, "zostatok", "pociatocny stav", "konecny stav", "stav", "ps", "ks", "balance", "zustatek", "pocatecni stav", "suma")
}

// ParseBalances prečíta zostatky účtov (prvý riadok = hlavička). Suma je
// v stĺpcoch MD a D, alebo v jednom stĺpci (kladná = MD, záporná = D).
// Riadky bez sumy sa vynechajú.
func ParseBalances(rows [][]string) ([]Balance, error) {
	if len(rows) < 2 {
		return nil, fmt.Errorf("tabuľka je prázdna — v prvom riadku má byť hlavička (Účet; Názov; MD; D), v ďalších zostatky")
	}
	col := map[int]int{}
	for i, h := range rows[0] {
		k := strings.TrimSuffix(strings.TrimSpace(catalog.Key(h)), ":")
		if c, ok := balanceAliases[k]; ok {
			if _, dup := col[c]; !dup {
				col[c] = i
			}
		}
	}
	if _, ok := col[bAccount]; !ok {
		return nil, fmt.Errorf("v hlavičke chýba stĺpec „Účet“")
	}
	_, md := col[bDebit]
	_, d := col[bCredit]
	_, signed := col[bSigned]
	if !(md && d) && !signed {
		return nil, fmt.Errorf("v hlavičke chýbajú stĺpce „MD“ a „D“ (alebo jeden stĺpec „Zostatok“)")
	}
	get := func(row []string, c int) string {
		if i, ok := col[c]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var out []Balance
	for n, row := range rows[1:] {
		if blank(row) {
			continue
		}
		b := Balance{Line: n + 2, Account: get(row, bAccount), Name: get(row, bName)}
		amount := func(s string) money.Dec {
			if s == "" || b.Err != "" {
				return money.Zero
			}
			v, err := Number(s)
			if err != nil {
				b.Err = fmt.Sprintf("chybná suma „%s“", s)
				return money.Zero
			}
			return v
		}
		net := money.Zero
		if md && d {
			net = amount(get(row, bDebit)).Sub(amount(get(row, bCredit)))
		} else {
			net = amount(get(row, bSigned))
		}
		if b.Err == "" && net.IsZero() {
			continue // účet bez zostatku (súčtové riadky predvahy majú tiež účet prázdny)
		}
		if b.Err == "" && strings.Trim(b.Account, "0123456789. -") != "" {
			// „Spolu“, „Trieda 2“ a pod. — súčtový riadok
			continue
		}
		if b.Err == "" && b.Account == "" {
			if k := catalog.Key(b.Name); strings.Contains(k, "spolu") || strings.Contains(k, "celkom") || strings.Contains(k, "celkem") ||
				strings.Contains(k, "sucet") || strings.Contains(k, "total") {
				continue // súčtový riadok predvahy
			}
			b.Err = "chýba číslo účtu"
		}
		if net.Sign() > 0 {
			b.Debit = net.RoundCents()
		} else {
			b.Credit = net.Neg().RoundCents()
		}
		out = append(out, b)
	}
	return out, nil
}
