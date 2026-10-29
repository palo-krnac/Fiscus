package bank

import (
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Candidate je otvorený doklad, ku ktorému môže platba patriť.
type Candidate struct {
	Kind      string       `json:"kind"` // "invoice" | "purchase"
	ID        int64        `json:"id"`
	Number    string       `json:"number"`
	VS        string       `json:"vs"`
	Partner   string       `json:"partner"`
	IBAN      string       `json:"iban"` // IBAN partnera (dodávateľa), ak je známy
	Amount    money.Dec    `json:"amount"`
	Direction Direction    `json:"direction"` // smer platby, ktorou sa doklad uhrádza
	Due       invoice.Date `json:"due"`
}

// Suggestion je návrh spárovania so skóre a dôvodom.
type Suggestion struct {
	Candidate
	Score  int    `json:"score"` // 0–100
	Reason string `json:"reason"`
}

// Skóre návrhov.
const (
	ScoreExact     = 100 // VS aj suma
	ScoreVS        = 70  // VS sedí, suma nie (čiastočná platba, preplatok)
	ScoreAmountWho = 60  // suma a protistrana (IBAN alebo meno)
	ScoreAmount    = 30  // iba suma
)

func sameName(a, b string) bool {
	norm := func(s string) string {
		s = strings.ToLower(s)
		for _, x := range []string{"s.r.o.", "a.s.", ",", ".", "spol", "sro"} {
			s = strings.ReplaceAll(s, x, " ")
		}
		return strings.Join(strings.Fields(s), " ")
	}
	na, nb := norm(a), norm(b)
	return na != "" && nb != "" && (strings.Contains(na, nb) || strings.Contains(nb, na))
}

// Suggest zoradí kandidátov podľa toho, ako dobre sedia na platbu.
func Suggest(t Transaction, cands []Candidate) []Suggestion {
	var out []Suggestion
	vs := NormalizeVS(t.VS)
	for _, c := range cands {
		if c.Direction != t.Direction {
			continue
		}
		amountOK := c.Amount.Equal(t.Amount)
		vsOK := vs != "" && NormalizeVS(c.VS) == vs
		whoOK := (c.IBAN != "" && strings.EqualFold(strings.ReplaceAll(c.IBAN, " ", ""), t.CounterIBAN)) || sameName(c.Partner, t.CounterName)
		s := Suggestion{Candidate: c}
		switch {
		case vsOK && amountOK:
			s.Score, s.Reason = ScoreExact, "sedí variabilný symbol aj suma"
		case vsOK && t.Amount.Cmp(c.Amount) < 0:
			s.Score, s.Reason = ScoreVS, "sedí variabilný symbol — čiastočná úhrada, zostane "+c.Amount.Sub(t.Amount).Display()+" €"
		case vsOK:
			s.Score, s.Reason = ScoreVS, "sedí variabilný symbol, suma je vyššia (preplatok)"
		case amountOK && whoOK:
			s.Score, s.Reason = ScoreAmountWho, "sedí suma aj protistrana"
		case amountOK:
			s.Score, s.Reason = ScoreAmount, "sedí suma"
		default:
			continue
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Due.Before(out[j].Due)
	})
	return out
}

// AutoMatch vráti doklad, ak je spárovanie jednoznačné: práve jeden
// kandidát so zhodným VS aj sumou.
func AutoMatch(t Transaction, cands []Candidate) (Candidate, bool) {
	var hit []Candidate
	for _, s := range Suggest(t, cands) {
		if s.Score == ScoreExact {
			hit = append(hit, s.Candidate)
		}
	}
	if len(hit) != 1 {
		return Candidate{}, false
	}
	return hit[0], true
}
