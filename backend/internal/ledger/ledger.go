// Package ledger je podvojné účtovníctvo: účtovný denník (zápisy dokladov
// na strany MD a D) a hlavná kniha (počiatočné stavy, obraty a zostatky
// účtov). Zápisy vznikajú z dokladov podľa predkontácií (balík chart),
// ručne alebo z interných dokladov.
package ledger

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/chart"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Line je zápis na jeden účet: suma na strane MD alebo D.
type Line struct {
	Account string    `json:"account"`
	Name    string    `json:"name,omitempty"` // názov účtu na zobrazenie
	Debit   money.Dec `json:"debit"`
	Credit  money.Dec `json:"credit"`
}

// Posting je zápis v účtovnom denníku: jeden doklad a jeho riadky.
type Posting struct {
	Key     string        `json:"key"` // sale:ID, purchase:ID, bank:ID, cash:ID, internal:ID
	Date    invoice.Date  `json:"date"`
	Doc     chart.DocKind `json:"doc"`
	Number  string        `json:"number"`
	Partner string        `json:"partner,omitempty"`
	Text    string        `json:"text,omitempty"`
	Link    string        `json:"link,omitempty"` // cesta v rozhraní
	// Amount je suma dokladu (na zobrazenie a hľadanie).
	Amount money.Dec `json:"amount"`
	// Template je kód predkontácie; Manual = riadky zadal používateľ.
	Template string `json:"template,omitempty"`
	Manual   bool   `json:"manual,omitempty"`
	Note     string `json:"note,omitempty"`
	Lines    []Line `json:"lines"`
	// Problem: doklad nie je zaúčtovaný alebo zápis nie je v poriadku.
	Problem string `json:"problem,omitempty"`
}

// Sums vráti súčet strán MD a D.
func Sums(lines []Line) (debit, credit money.Dec) {
	for _, l := range lines {
		debit, credit = debit.Add(l.Debit), credit.Add(l.Credit)
	}
	return
}

// Balanced vráti true, ak MD = D.
func Balanced(lines []Line) bool {
	d, c := Sums(lines)
	return d.Equal(c)
}

// FromEntries prevedie zaúčtovanie predkontácie na riadky.
func FromEntries(es []chart.Entry) []Line {
	out := make([]Line, 0, len(es))
	for _, e := range es {
		out = append(out, Line{Account: e.Account, Debit: e.Debit, Credit: e.Credit})
	}
	return out
}

// Chyby ručných riadkov.
var (
	ErrNoLines    = errors.New("zápis nemá riadky")
	ErrUnbalanced = errors.New("strana MD sa nerovná strane D")
)

// CheckLines skontroluje ručné riadky voči rozvrhu: účet existuje a používa
// sa, suma v centoch je práve na jednej strane, MD = D. Čísla účtov
// normalizuje (221.001 → 221001).
func CheckLines(lines []Line, c chart.Chart) error {
	if len(lines) < 2 {
		return fmt.Errorf("%w: zadajte aspoň dva riadky (MD a D)", ErrNoLines)
	}
	for i := range lines {
		l := &lines[i]
		l.Account = chart.NormalizeNumber(l.Account)
		l.Name = ""
		a, ok := c.Find(l.Account)
		switch {
		case l.Account == "":
			return fmt.Errorf("riadok %d: vyberte účet", i+1)
		case !ok:
			return fmt.Errorf("riadok %d: účet %s nie je v rozvrhu", i+1, chart.Display(l.Account))
		case !a.Active:
			return fmt.Errorf("riadok %d: účet %s je skrytý", i+1, chart.Display(l.Account))
		case l.Debit.IsZero() == l.Credit.IsZero():
			return fmt.Errorf("riadok %d: zadajte sumu na strane MD alebo D", i+1)
		case !l.Debit.Equal(l.Debit.RoundCents()) || !l.Credit.Equal(l.Credit.RoundCents()):
			return fmt.Errorf("riadok %d: suma musí byť v centoch", i+1)
		}
	}
	if !Balanced(lines) {
		d, cr := Sums(lines)
		return fmt.Errorf("%w (MD %s, D %s)", ErrUnbalanced, d.Display(), cr.Display())
	}
	return nil
}

// MoneySide vráti +1, ak predkontácia účtuje peniaze na stranu MD (príjem),
// −1 pri strane D (výdavok) a 0, ak nemá peňažný účet. Peňažné účty sú
// pokladnica (21x), bankové účty (22x) a peniaze na ceste (261).
func MoneySide(t chart.Template) int {
	for _, l := range t.Lines {
		if !isMoney(l.Account, t.Doc) {
			continue
		}
		if l.Side == chart.Debit {
			return 1
		}
		return -1
	}
	return 0
}

func isMoney(acc string, doc chart.DocKind) bool {
	switch doc {
	case chart.DocCash:
		return strings.HasPrefix(acc, "21")
	case chart.DocBank:
		return strings.HasPrefix(acc, "22")
	}
	return false
}

// Turnover je riadok hlavnej knihy: počiatočný stav, obraty a konečný stav
// účtu. Stavy sú so znamienkom: kladný = zostatok na strane MD.
type Turnover struct {
	Account string     `json:"account"`
	Name    string     `json:"name"`
	Type    chart.Type `json:"type"`
	Opening money.Dec  `json:"opening"`
	Debit   money.Dec  `json:"debit"`
	Credit  money.Dec  `json:"credit"`
	Closing money.Dec  `json:"closing"`
}

// Totals sú súčty hlavnej knihy (obratová predvaha): počiatočné stavy
// a konečné stavy musia dať spolu nulu, obraty MD = D.
type Totals struct {
	OpeningDebit  money.Dec `json:"openingDebit"`
	OpeningCredit money.Dec `json:"openingCredit"`
	Debit         money.Dec `json:"debit"`
	Credit        money.Dec `json:"credit"`
	ClosingDebit  money.Dec `json:"closingDebit"`
	ClosingCredit money.Dec `json:"closingCredit"`
	Balanced      bool      `json:"balanced"`
}

// Book zostaví hlavnú knihu: obraty po účtoch (aj analytických) a súhrn
// za syntetické účty. Započítajú sa iba vyrovnané zápisy bez problému.
func Book(postings []Posting, opening map[string]money.Dec, c chart.Chart) (accounts, synthetic []Turnover, tot Totals) {
	acc := map[string]*Turnover{}
	get := func(n string) *Turnover {
		t, ok := acc[n]
		if !ok {
			t = &Turnover{Account: n}
			if a, found := c.Find(n); found {
				t.Name, t.Type = a.Name, a.Type
			}
			acc[n] = t
		}
		return t
	}
	for n, v := range opening {
		if !v.IsZero() {
			get(n).Opening = v
		}
	}
	for _, p := range postings {
		if p.Problem != "" || !Balanced(p.Lines) {
			continue
		}
		for _, l := range p.Lines {
			t := get(l.Account)
			t.Debit, t.Credit = t.Debit.Add(l.Debit), t.Credit.Add(l.Credit)
		}
	}
	syn := map[string]*Turnover{}
	for _, t := range acc {
		t.Closing = t.Opening.Add(t.Debit).Sub(t.Credit)
		accounts = append(accounts, *t)
		n := t.Account
		if len(n) > 3 {
			n = n[:3]
		}
		s, ok := syn[n]
		if !ok {
			s = &Turnover{Account: n}
			if a, found := c.Find(n); found {
				s.Name, s.Type = a.Name, a.Type
			}
			syn[n] = s
		}
		s.Opening, s.Debit, s.Credit, s.Closing = s.Opening.Add(t.Opening), s.Debit.Add(t.Debit), s.Credit.Add(t.Credit), s.Closing.Add(t.Closing)
		tot.Debit, tot.Credit = tot.Debit.Add(t.Debit), tot.Credit.Add(t.Credit)
		tot.OpeningDebit, tot.OpeningCredit = addSide(tot.OpeningDebit, tot.OpeningCredit, t.Opening)
		tot.ClosingDebit, tot.ClosingCredit = addSide(tot.ClosingDebit, tot.ClosingCredit, t.Closing)
	}
	for _, s := range syn {
		synthetic = append(synthetic, *s)
	}
	byAcc := func(l []Turnover) {
		sort.Slice(l, func(i, j int) bool { return l[i].Account < l[j].Account })
	}
	byAcc(accounts)
	byAcc(synthetic)
	if accounts == nil {
		accounts, synthetic = []Turnover{}, []Turnover{}
	}
	tot.Balanced = tot.Debit.Equal(tot.Credit) && tot.OpeningDebit.Equal(tot.OpeningCredit) && tot.ClosingDebit.Equal(tot.ClosingCredit)
	return accounts, synthetic, tot
}

func addSide(d, c, v money.Dec) (money.Dec, money.Dec) {
	if v.Sign() >= 0 {
		return d.Add(v), c
	}
	return d, c.Sub(v)
}

// Sort zoradí zápisy podľa dátumu, druhu dokladu a čísla.
func Sort(ps []Posting) {
	order := map[chart.DocKind]int{}
	for i, k := range chart.DocKinds {
		order[k.Code] = i
	}
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if a.Date != b.Date {
			return a.Date.Before(b.Date)
		}
		if order[a.Doc] != order[b.Doc] {
			return order[a.Doc] < order[b.Doc]
		}
		return a.Number < b.Number
	})
}
