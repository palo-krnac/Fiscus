// Package bank je import bankových výpisov a párovanie platieb s dokladmi.
//
// Výpis sa načíta vo formáte ISO 20022 camt.053 (ponúkajú ho slovenské banky:
// SLSP, VÚB, Tatra banka, ČSOB, …). Každý pohyb dostane odtlačok, takže
// opakovaný import toho istého výpisu nič nezdvojí. Platba sa automaticky
// spáruje iba vtedy, keď jednoznačne sedí variabilný symbol aj suma.
package bank

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Direction je smer platby.
type Direction string

const (
	In  Direction = "in"  // príjem (CRDT)
	Out Direction = "out" // výdavok (DBIT)
)

// Status je stav pohybu.
type Status string

const (
	StatusOpen    Status = "open"    // nespárovaný
	StatusMatched Status = "matched" // spárovaný s dokladom
	StatusIgnored Status = "ignored" // nepáruje sa (poplatok, prevod medzi účtami…)
)

// Match je väzba pohybu na doklad.
type Match struct {
	Kind   string    `json:"kind"`   // "invoice" | "purchase"
	ID     int64     `json:"id"`     // ID dokladu
	Number string    `json:"number"` // číslo dokladu (na zobrazenie)
	Auto   bool      `json:"auto"`   // spárované automaticky pri importe
	At     time.Time `json:"at"`
}

// Transaction je pohyb na bankovom účte.
type Transaction struct {
	ID          int64        `json:"id"`
	CompanyID   int64        `json:"companyId"`
	Account     string       `json:"account"` // IBAN nášho účtu
	Date        invoice.Date `json:"date"`    // dátum zaúčtovania
	Amount      money.Dec    `json:"amount"`  // vždy kladná
	Currency    string       `json:"currency"`
	Direction   Direction    `json:"direction"`
	VS          string       `json:"vs"`
	SS          string       `json:"ss"`
	KS          string       `json:"ks"`
	CounterName string       `json:"counterName"`
	CounterIBAN string       `json:"counterIban"`
	Message     string       `json:"message"`
	BankRef     string       `json:"bankRef"`
	Fingerprint string       `json:"-"`
	Status      Status       `json:"status"`
	Match       *Match       `json:"match,omitempty"`
	Note        string       `json:"note"`
}

// Signed vráti sumu so znamienkom (výdavok záporný).
func (t Transaction) Signed() money.Dec {
	if t.Direction == Out {
		return t.Amount.Neg()
	}
	return t.Amount
}

var (
	reSlash = regexp.MustCompile(`(?i)/(VS|SS|KS)\s*([0-9]{0,10})`)
	reWord  = regexp.MustCompile(`(?i)\b(VS|SS|KS)[\s.:-]*([0-9]{1,10})\b`)
)

// ParseSymbols vyberie variabilný, špecifický a konštantný symbol z textu
// (EndToEndId „/VS2026001/SS/KS0308“ alebo správa „VS: 2026001“).
func ParseSymbols(texts ...string) (vs, ss, ks string) {
	set := func(k, v string) {
		v = strings.TrimLeft(v, "0")
		switch strings.ToUpper(k) {
		case "VS":
			if vs == "" {
				vs = v
			}
		case "SS":
			if ss == "" {
				ss = v
			}
		case "KS":
			if ks == "" {
				ks = v
			}
		}
	}
	for _, t := range texts {
		for _, m := range reSlash.FindAllStringSubmatch(t, -1) {
			if m[2] != "" {
				set(m[1], m[2])
			}
		}
		for _, m := range reWord.FindAllStringSubmatch(t, -1) {
			set(m[1], m[2])
		}
	}
	return
}

// fingerprint je odtlačok pohybu na odhalenie opakovaného importu.
func fingerprint(t Transaction, occurrence int) string {
	parts := []string{t.Account}
	if t.BankRef != "" {
		parts = append(parts, "ref", t.BankRef)
	} else {
		parts = append(parts, t.Date.String(), t.Amount.String(), string(t.Direction), t.VS, t.CounterIBAN, t.Message, string(rune('0'+occurrence)))
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:16])
}

// NormalizeVS odstráni úvodné nuly a medzery (VS 0002026001 = 2026001).
func NormalizeVS(s string) string {
	return strings.TrimLeft(strings.Join(strings.Fields(s), ""), "0")
}
