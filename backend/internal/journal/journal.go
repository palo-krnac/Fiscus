// Package journal je peňažný denník jednoduchého účtovníctva podľa opatrenia
// MF SR č. MF/27076/2007-74 (postupy účtovania v sústave jednoduchého
// účtovníctva, znenie od 1. 1. 2025), § 4.
//
// Denník zaznamenáva skutočné pohyby peňazí (pokladnica a bankové účty).
// Každý pohyb sa rozpíše do stĺpcov § 4 ods. 6:
//
//	c   priebežné položky (prevody medzi pokladnicou a bankou)
//	d1  príjmy zahrnované do základu dane — predaj tovaru
//	d2  — predaj výrobkov a služieb
//	d3  — ostatné príjmy
//	e1  výdavky zahrnované do základu dane — zásoby
//	e2  — služby
//	e3  — mzdy
//	e4  — poistné a príspevky
//	e5  — tvorba sociálneho fondu
//	e6  — ostatné výdavky
//	f   príjmy, ktoré základ dane neovplyvňujú (f.dph = prijatá DPH)
//	g   výdavky, ktoré základ dane neovplyvňujú (g.dph = zaplatená DPH,
//	    g.dm = obstaranie dlhodobého majetku)
//
// Sumy riadkov sú so znamienkom podľa stĺpca: príjem kladný, storno príjmu
// (napr. vrátené peniaze k dobropisu) záporné.
package journal

import (
	"errors"
	"fmt"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Column je stĺpec peňažného denníka.
type Column string

const (
	TransitIn   Column = "c.in"  // priebežná položka – príjem
	TransitOut  Column = "c.out" // priebežná položka – výdavok
	IncGoods    Column = "d1"
	IncServices Column = "d2"
	IncOther    Column = "d3"
	ExpStock    Column = "e1"
	ExpServices Column = "e2"
	ExpWages    Column = "e3"
	ExpInsur    Column = "e4"
	ExpSocial   Column = "e5"
	ExpOther    Column = "e6"
	IncVAT      Column = "f.dph"
	IncNonTax   Column = "f"
	ExpVAT      Column = "g.dph"
	ExpAsset    Column = "g.dm"
	ExpNonTax   Column = "g"
)

// Group je skupina stĺpcov (pre súčty a výsledok).
type Group string

const (
	GroupTransit  Group = "c"
	GroupTaxInc   Group = "d" // príjmy zahrnované do základu dane
	GroupTaxExp   Group = "e" // výdavky zahrnované do základu dane
	GroupOtherInc Group = "f"
	GroupOtherExp Group = "g"
)

// ColumnInfo opisuje stĺpec.
type ColumnInfo struct {
	Code   Column `json:"code"`
	Group  Group  `json:"group"`
	Label  string `json:"label"`
	Income bool   `json:"income"` // príjem (inak výdavok)
}

// Columns je číselník stĺpcov v poradí denníka.
var Columns = []ColumnInfo{
	{IncGoods, GroupTaxInc, "Predaj tovaru", true},
	{IncServices, GroupTaxInc, "Predaj výrobkov a služieb", true},
	{IncOther, GroupTaxInc, "Ostatné príjmy", true},
	{IncVAT, GroupOtherInc, "Prijatá DPH", true},
	{IncNonTax, GroupOtherInc, "Iné príjmy mimo základu dane", true},
	{TransitIn, GroupTransit, "Priebežná položka – príjem", true},
	{ExpStock, GroupTaxExp, "Zásoby", false},
	{ExpServices, GroupTaxExp, "Služby", false},
	{ExpWages, GroupTaxExp, "Mzdy", false},
	{ExpInsur, GroupTaxExp, "Poistné a príspevky", false},
	{ExpSocial, GroupTaxExp, "Tvorba sociálneho fondu", false},
	{ExpOther, GroupTaxExp, "Ostatné výdavky", false},
	{ExpVAT, GroupOtherExp, "Zaplatená DPH", false},
	{ExpAsset, GroupOtherExp, "Dlhodobý majetok", false},
	{ExpNonTax, GroupOtherExp, "Iné výdavky mimo základu dane", false},
	{TransitOut, GroupTransit, "Priebežná položka – výdavok", false},
}

// Info vráti opis stĺpca.
func (c Column) Info() (ColumnInfo, bool) {
	for _, x := range Columns {
		if x.Code == c {
			return x, true
		}
	}
	return ColumnInfo{}, false
}

// Means je platidlo: pokladnica alebo banka.
type Means string

const (
	Cash Means = "cash"
	Bank Means = "bank"
	// Closing je uzávierková operácia k 31. 12. (§ 8 ods. 4): úprava príjmov
	// a výdavkov bez pohybu peňazí, napr. odpisy dlhodobého majetku.
	Closing Means = "close"
)

// Line je časť pohybu v jednom stĺpci.
type Line struct {
	Column Column    `json:"column"`
	Amount money.Dec `json:"amount"`
}

// Entry je jeden pohyb peňazí v denníku.
type Entry struct {
	Key     string       `json:"key"` // zdroj: "bank:12", "cash:5"
	Date    invoice.Date `json:"date"`
	Number  string       `json:"number"`  // doklad o pohybe (PPD…, výpis)
	Means   Means        `json:"means"`   // pokladnica / banka
	Account string       `json:"account"` // IBAN alebo názov pokladnice
	Amount  money.Dec    `json:"amount"`  // so znamienkom: príjem +, výdavok −
	Partner string       `json:"partner"`
	Text    string       `json:"text"`
	Doc     *DocRef      `json:"doc,omitempty"` // spárovaný doklad
	Lines   []Line       `json:"lines"`
	Auto    bool         `json:"auto"` // rozpis navrhol Fiškus (nie ručne)
	Note    string       `json:"note,omitempty"`
}

// DocRef je doklad, ktorým sa pohyb uhrádza.
type DocRef struct {
	Kind   string `json:"kind"` // invoice | purchase
	ID     int64  `json:"id"`
	Number string `json:"number"`
}

// Classified vráti true, ak je pohyb rozpísaný do stĺpcov.
func (e Entry) Classified() bool { return len(e.Lines) > 0 }

// sign: +1 pre príjmové stĺpce, −1 pre výdavkové.
func sign(c Column) int64 {
	if i, _ := c.Info(); i.Income {
		return 1
	}
	return -1
}

// Signed vráti príspevok riadku k pohybu (výdavok záporný).
func (l Line) Signed() money.Dec {
	if sign(l.Column) < 0 {
		return l.Amount.Neg()
	}
	return l.Amount
}

// Chyby rozpisu.
var (
	ErrUnknownColumn = errors.New("neznámy stĺpec denníka")
	ErrSum           = errors.New("rozpis nesedí so sumou pohybu")
)

// Check overí rozpis: známe stĺpce, centy a súčet rovný sume pohybu.
func Check(amount money.Dec, lines []Line) error {
	sum := money.Zero
	seen := map[Column]bool{}
	for _, l := range lines {
		if _, ok := l.Column.Info(); !ok {
			return fmt.Errorf("%w: %q", ErrUnknownColumn, l.Column)
		}
		if seen[l.Column] {
			return fmt.Errorf("stĺpec %s je v rozpise dvakrát", l.Column)
		}
		seen[l.Column] = true
		if !l.Amount.Equal(l.Amount.RoundCents()) {
			return fmt.Errorf("suma v stĺpci %s musí byť v centoch", l.Column)
		}
		sum = sum.Add(l.Signed())
	}
	if !sum.Equal(amount) {
		return fmt.Errorf("%w: rozpis %s €, pohyb %s €", ErrSum, sum.Display(), amount.Display())
	}
	return nil
}

// CheckClosing overí uzávierkovú operáciu: aspoň jeden riadok, známe stĺpce
// (nie priebežné položky), centy, nenulové sumy. Súčet sa nekontroluje —
// peniaze sa nepohnú.
func CheckClosing(lines []Line) error {
	if len(lines) == 0 {
		return errors.New("uzávierková operácia musí mať aspoň jeden riadok")
	}
	seen := map[Column]bool{}
	for _, l := range lines {
		info, ok := l.Column.Info()
		if !ok {
			return fmt.Errorf("%w: %q", ErrUnknownColumn, l.Column)
		}
		if info.Group == GroupTransit {
			return errors.New("priebežné položky nie sú uzávierková operácia")
		}
		if seen[l.Column] {
			return fmt.Errorf("stĺpec %s je v rozpise dvakrát", l.Column)
		}
		seen[l.Column] = true
		if l.Amount.IsZero() || !l.Amount.Equal(l.Amount.RoundCents()) {
			return fmt.Errorf("suma v stĺpci %s musí byť nenulová a v centoch", l.Column)
		}
	}
	return nil
}

// Totals sú súčty denníka.
type Totals struct {
	Columns  map[Column]money.Dec `json:"columns"`
	CashIn   money.Dec            `json:"cashIn"`
	CashOut  money.Dec            `json:"cashOut"`
	BankIn   money.Dec            `json:"bankIn"`
	BankOut  money.Dec            `json:"bankOut"`
	TaxInc   money.Dec            `json:"taxIncome"`     // d spolu
	TaxExp   money.Dec            `json:"taxExpenses"`   // e spolu
	Result   money.Dec            `json:"result"`        // d − e (§ 8 ods. 4 písm. a) bod 3)
	OtherInc money.Dec            `json:"otherIncome"`   // f spolu
	OtherExp money.Dec            `json:"otherExpenses"` // g spolu
	Open     int                  `json:"unclassified"`  // pohyby bez rozpisu
}

// Sum spočíta denník.
func Sum(entries []Entry) Totals {
	t := Totals{Columns: map[Column]money.Dec{}}
	for _, e := range entries {
		switch {
		case e.Means == Closing:
			// bez pohybu peňazí
		case e.Means == Cash && e.Amount.Sign() >= 0:
			t.CashIn = t.CashIn.Add(e.Amount)
		case e.Means == Cash:
			t.CashOut = t.CashOut.Add(e.Amount.Neg())
		case e.Amount.Sign() >= 0:
			t.BankIn = t.BankIn.Add(e.Amount)
		default:
			t.BankOut = t.BankOut.Add(e.Amount.Neg())
		}
		if !e.Classified() {
			t.Open++
		}
		for _, l := range e.Lines {
			t.Columns[l.Column] = t.Columns[l.Column].Add(l.Amount)
			info, _ := l.Column.Info()
			switch info.Group {
			case GroupTaxInc:
				t.TaxInc = t.TaxInc.Add(l.Amount)
			case GroupTaxExp:
				t.TaxExp = t.TaxExp.Add(l.Amount)
			case GroupOtherInc:
				t.OtherInc = t.OtherInc.Add(l.Amount)
			case GroupOtherExp:
				t.OtherExp = t.OtherExp.Add(l.Amount)
			}
		}
	}
	t.Result = t.TaxInc.Sub(t.TaxExp)
	return t
}

// Balance je stav pokladnice alebo bankového účtu za obdobie.
type Balance struct {
	Means   Means     `json:"means"`
	Account string    `json:"account"`
	Opening money.Dec `json:"opening"`
	In      money.Dec `json:"in"`
	Out     money.Dec `json:"out"`
	Closing money.Dec `json:"closing"`
}

// Balances spočíta stavy: pokladnica (počiatočný stav cash) a každý bankový
// účet z pohybov alebo zo zoznamu accounts (počiatočné stavy bank).
func Balances(entries []Entry, cash money.Dec, bank map[string]money.Dec, accounts []string) []Balance {
	byAcc := map[string]*Balance{}
	var order []string
	get := func(means Means, acc string) *Balance {
		k := string(means) + "|" + acc
		if b, ok := byAcc[k]; ok {
			return b
		}
		b := &Balance{Means: means, Account: acc}
		if means == Cash {
			b.Opening = cash
		} else {
			b.Opening = bank[acc]
		}
		byAcc[k] = b
		order = append(order, k)
		return b
	}
	get(Cash, "Pokladnica")
	for _, a := range accounts {
		get(Bank, a)
	}
	keys := make([]string, 0, len(bank))
	for a := range bank {
		keys = append(keys, a)
	}
	sort.Strings(keys)
	for _, a := range keys {
		get(Bank, a)
	}
	for _, e := range entries {
		if e.Means == Closing {
			continue
		}
		acc := e.Account
		if e.Means == Bank {
			acc = normAcc(acc)
		}
		b := get(e.Means, acc)
		if e.Amount.Sign() >= 0 {
			b.In = b.In.Add(e.Amount)
		} else {
			b.Out = b.Out.Add(e.Amount.Neg())
		}
	}
	out := make([]Balance, 0, len(order))
	for _, k := range order {
		b := byAcc[k]
		b.Closing = b.Opening.Add(b.In).Sub(b.Out)
		out = append(out, *b)
	}
	return out
}

func normAcc(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r != ' ' {
			if r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			out = append(out, r)
		}
	}
	return string(out)
}
