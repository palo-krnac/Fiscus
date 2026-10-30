package ledger

import (
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Účtovná závierka mikro účtovnej jednotky v podvojnom účtovníctve
// (opatrenie MF SR č. MF/23377/2014-74, vzory Úč MÚJ 1-01 Súvaha
// a Úč MÚJ 2-01 Výkaz ziskov a strát). Riadky sa plnia zo zostatkov účtov
// hlavnej knihy; sumy sú v celých eurách, každý riadok sa zaokrúhli
// a súčtové riadky sú súčtom zaokrúhlených riadkov.
//
// Väzba riadkov na účty vychádza z rámcovej účtovej osnovy; opatrenie ju
// pre mikro účtovnú jednotku nepredpisuje, preto ju treba overiť
// s účtovníkom (najmä rozdelenie na dlhodobé a krátkodobé položky).

// StatementRow je riadok súvahy alebo výkazu ziskov a strát.
type StatementRow struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Sum   bool   `json:"sum,omitempty"`
	// Pri aktívach Brutto, Korekcia (oprávky, opravné položky) a Netto;
	// inde je suma v Netto.
	Brutto   money.Dec `json:"brutto"`
	Korekcia money.Dec `json:"korekcia"`
	Netto    money.Dec `json:"netto"`
	Previous money.Dec `json:"previous"` // netto predchádzajúceho obdobia
	Accounts string    `json:"accounts,omitempty"`
}

// rule je väzba riadku na účty. Položka je predpona účtu, voliteľne
// s príponou A (iba aktívny zostatok) alebo P (iba pasívny); predpona „-“
// je korekcia (oprávky a opravné položky k aktívam).
type rule struct {
	code, label string
	items       []string
	sum         []string // súčtový riadok: kódy sčítaných riadkov (predpona „-“ odpočíta)
}

var balanceAssets = []rule{
	{code: "01", label: "SPOLU MAJETOK", sum: []string{"02", "14"}},
	{code: "02", label: "Neobežný majetok", sum: []string{"03", "04", "09"}},
	{code: "03", label: "A.I. Dlhodobý nehmotný majetok", items: []string{"012", "013", "014", "015", "019", "041", "051", "-07", "-091", "-093"}},
	{code: "04", label: "A.II. Dlhodobý hmotný majetok", sum: []string{"05", "06", "07", "08"}},
	{code: "05", label: "A.II.1. Pozemky a stavby", items: []string{"021", "031", "-081"}},
	{code: "06", label: "A.II.2. Samostatné hnuteľné veci a súbory hnuteľných vecí", items: []string{"022", "-082"}},
	{code: "07", label: "A.II.3. Ostatný dlhodobý hmotný majetok", items: []string{"025", "026", "029", "032", "042", "052", "-085", "-086", "-089", "-092", "-094", "-095"}},
	{code: "08", label: "A.II.4. Opravná položka k nadobudnutému majetku", items: []string{"097", "098"}},
	{code: "09", label: "A.III. Dlhodobý finančný majetok", sum: []string{"10", "11", "12", "13"}},
	{code: "10", label: "A.III.1. Podielové cenné papiere", items: []string{"061", "062", "063"}},
	{code: "11", label: "A.III.2. Ostatný dlhodobý finančný majetok", items: []string{"043", "053", "065", "066", "067", "069", "-096"}},
	{code: "12", label: "A.III.3. Účty v bankách s dobou viazanosti dlhšou ako jeden rok"},
	{code: "13", label: "A.III.4. Ostatný dlhodobý finančný majetok so zostatkovou dobou splatnosti najviac jeden rok"},
	{code: "14", label: "B. Obežný majetok", sum: []string{"15", "16", "17", "21"}},
	{code: "15", label: "B.I. Zásoby", items: []string{"11", "12", "13", "15", "-19"}},
	{code: "16", label: "B.II. Dlhodobé pohľadávky"},
	{code: "17", label: "B.III. Krátkodobé pohľadávky", sum: []string{"18", "19", "20"}},
	{code: "18", label: "B.III.1. Pohľadávky z obchodného styku", items: []string{"311", "312", "313", "314", "315", "316", "-391"}},
	{code: "19", label: "B.III.2. Sociálne poistenie, daňové pohľadávky a dotácie", items: []string{"336A", "341A", "342A", "343A", "345A", "346A", "347A"}},
	{code: "20", label: "B.III.3. Ostatné pohľadávky", items: []string{"335", "351", "354A", "355A", "358A", "371", "373", "374", "375", "376", "377", "378", "381", "382", "385", "395A", "398A"}},
	{code: "21", label: "B.IV. Finančný majetok", sum: []string{"22", "23"}},
	{code: "22", label: "B.IV.1. Peniaze a účty v bankách", items: []string{"211", "213", "221A", "261"}},
	{code: "23", label: "B.IV.2. Ostatné finančné účty", items: []string{"251", "252", "253", "255", "256", "257", "259", "-291"}},
}

var balanceLiabilities = []rule{
	{code: "24", label: "SPOLU VLASTNÉ IMANIE A ZÁVÄZKY", sum: []string{"25", "34"}},
	{code: "25", label: "A. Vlastné imanie", sum: []string{"26", "29", "30", "31", "32", "33"}},
	{code: "26", label: "A.I. Základné imanie", sum: []string{"27", "28"}},
	{code: "27", label: "A.I.1. Základné imanie a zmeny základného imania", items: []string{"411", "419", "491"}},
	{code: "28", label: "A.I.2. Pohľadávky za upísané vlastné imanie", items: []string{"353"}},
	{code: "29", label: "A.II. Kapitálové fondy", items: []string{"412", "413", "417", "418"}},
	{code: "30", label: "A.III. Fondy zo zisku", items: []string{"421", "422", "423", "427"}},
	{code: "31", label: "A.IV. Oceňovacie rozdiely", items: []string{"414", "415", "416"}},
	{code: "32", label: "A.V. Nerozdelený zisk alebo neuhradená strata minulých rokov", items: []string{"428", "429", "431"}},
	{code: "33", label: "A.VI. Výsledok hospodárenia za účtovné obdobie po zdanení"}, // = VZaS r. 38
	{code: "34", label: "B. Záväzky", sum: []string{"35", "36", "37", "38", "43", "44", "45"}},
	{code: "35", label: "B.I. Dlhodobé záväzky okrem rezerv a úverov", items: []string{"471", "472", "473", "474", "475", "476", "478", "479", "481"}},
	{code: "36", label: "B.II. Dlhodobé rezervy", items: []string{"451", "459"}},
	{code: "37", label: "B.III. Dlhodobé bankové úvery", items: []string{"461"}},
	{code: "38", label: "B.IV. Krátkodobé záväzky okrem rezerv, úverov a výpomoci", sum: []string{"39", "40", "41", "42"}},
	{code: "39", label: "B.IV.1. Krátkodobé záväzky z obchodného styku", items: []string{"321", "322", "324", "325", "326"}},
	{code: "40", label: "B.IV.2. Záväzky voči zamestnancom a zo sociálneho poistenia", items: []string{"331", "333", "336P"}},
	{code: "41", label: "B.IV.3. Daňové záväzky a dotácie", items: []string{"341P", "342P", "343P", "345P", "346P", "347P"}},
	{code: "42", label: "B.IV.4. Ostatné krátkodobé záväzky", items: []string{"354P", "355P", "358P", "361", "364", "365", "367", "368", "372", "379", "383", "384", "395P", "398P"}},
	{code: "43", label: "B.V. Krátkodobé rezervy", items: []string{"323"}},
	{code: "44", label: "B.VI. Bežné bankové úvery", items: []string{"221P", "231", "232"}},
	{code: "45", label: "B.VII. Krátkodobé finančné výpomoci", items: []string{"241", "249"}},
}

var incomeRules = []rule{
	{code: "01", label: "Výnosy z hospodárskej činnosti spolu", sum: []string{"02", "03", "04", "05", "06", "07"}},
	{code: "02", label: "I. Tržby z predaja tovaru", items: []string{"604", "607"}},
	{code: "03", label: "II. Tržby z predaja vlastných výrobkov a služieb", items: []string{"601", "602"}},
	{code: "04", label: "III. Zmena stavu vnútroorganizačných zásob", items: []string{"61"}},
	{code: "05", label: "IV. Aktivácia", items: []string{"62"}},
	{code: "06", label: "V. Tržby z predaja dlhodobého nehmotného majetku, dlhodobého hmotného majetku a materiálu", items: []string{"641", "642"}},
	{code: "07", label: "VI. Ostatné výnosy z hospodárskej činnosti", items: []string{"644", "645", "646", "648"}},
	{code: "08", label: "Náklady na hospodársku činnosť spolu", sum: []string{"09", "10", "11", "12", "13", "14", "15", "16", "17"}},
	{code: "09", label: "A. Náklady vynaložené na obstaranie predaného tovaru", items: []string{"504", "507"}},
	{code: "10", label: "B. Spotreba materiálu, energie a ostatných neskladovateľných dodávok", items: []string{"501", "502", "503"}},
	{code: "11", label: "C. Služby", items: []string{"511", "512", "513", "518"}},
	{code: "12", label: "D. Osobné náklady", items: []string{"52"}},
	{code: "13", label: "E. Dane a poplatky", items: []string{"53"}},
	{code: "14", label: "F. Odpisy a opravné položky k dlhodobému nehmotnému majetku a dlhodobému hmotnému majetku", items: []string{"551", "553"}},
	{code: "15", label: "G. Zostatková cena predaného dlhodobého majetku a predaného materiálu", items: []string{"541", "542"}},
	{code: "16", label: "H. Opravné položky k pohľadávkam", items: []string{"547"}},
	{code: "17", label: "I. Ostatné náklady na hospodársku činnosť", items: []string{"505", "543", "544", "545", "546", "548", "549", "552", "554", "557", "558"}},
	{code: "18", label: "Výsledok hospodárenia z hospodárskej činnosti (+/-)", sum: []string{"01", "-08"}},
	{code: "19", label: "Pridaná hodnota", sum: []string{"02", "03", "04", "05", "-09", "-10", "-11"}},
	{code: "20", label: "Výnosy z finančnej činnosti spolu", sum: []string{"21", "22", "23", "24", "25", "26"}},
	{code: "21", label: "VII. Tržby z predaja cenných papierov a podielov", items: []string{"661"}},
	{code: "22", label: "VIII. Výnosy z dlhodobého finančného majetku", items: []string{"665"}},
	{code: "23", label: "IX. Výnosy z krátkodobého finančného majetku", items: []string{"666"}},
	{code: "24", label: "X. Výnosové úroky", items: []string{"662"}},
	{code: "25", label: "XI. Kurzové zisky", items: []string{"663"}},
	{code: "26", label: "XII. Ostatné výnosy z finančnej činnosti", items: []string{"664", "667", "668"}},
	{code: "27", label: "Náklady na finančnú činnosť spolu", sum: []string{"28", "29", "30", "31", "32", "33"}},
	{code: "28", label: "J. Predané cenné papiere a podiely", items: []string{"561"}},
	{code: "29", label: "K. Náklady na krátkodobý finančný majetok", items: []string{"566"}},
	{code: "30", label: "L. Opravné položky k finančnému majetku (+/-)", items: []string{"565"}},
	{code: "31", label: "M. Nákladové úroky", items: []string{"562"}},
	{code: "32", label: "N. Kurzové straty", items: []string{"563"}},
	{code: "33", label: "O. Ostatné náklady na finančnú činnosť", items: []string{"564", "567", "568", "569"}},
	{code: "34", label: "Výsledky hospodárenia z finančnej činnosti (+/-)", sum: []string{"20", "-27"}},
	{code: "35", label: "Výsledok hospodárenia za účtovné obdobie pred zdanením (+/-)", sum: []string{"18", "34"}},
	{code: "36", label: "P. Daň z príjmov", items: []string{"591", "592", "595"}},
	{code: "37", label: "Q. Prevod podielov na výsledku hospodárenia spoločníkom (+/-)", items: []string{"596"}},
	{code: "38", label: "Výsledok hospodárenia za účtovné obdobie po zdanení (+/-)", sum: []string{"35", "-36", "-37"}},
}

// matches vráti true, ak účet so zostatkom patrí pod položku.
func matches(item, account string, balance money.Dec) bool {
	side := ""
	if n := len(item); n > 0 && (item[n-1] == 'A' || item[n-1] == 'P') {
		item, side = item[:n-1], item[n-1:]
	}
	if !strings.HasPrefix(account, item) {
		return false
	}
	switch side {
	case "A":
		return balance.Sign() > 0
	case "P":
		return balance.Sign() < 0
	}
	return true
}

// side určuje znamienko sumy riadku: aktíva a náklady kladné pri zostatku
// MD, pasíva a výnosy pri zostatku D.
type side int

const (
	debitSide  side = 1
	creditSide side = -1
)

// fill vypočíta riadky; used zbiera použité účty (kontrola úplnosti).
func fill(rules []rule, bal map[string]money.Dec, s side, used map[string]bool) map[string]*StatementRow {
	out := map[string]*StatementRow{}
	for _, r := range rules {
		row := &StatementRow{Code: r.code, Label: r.label, Sum: r.sum != nil}
		var accs []string
		for _, it := range r.items {
			corr := strings.HasPrefix(it, "-")
			it = strings.TrimPrefix(it, "-")
			if corr {
				accs = append(accs, "−"+it)
			} else {
				accs = append(accs, it)
			}
			for acc, v := range bal {
				if !matches(it, acc, v) {
					continue
				}
				used[acc] = true
				if corr {
					row.Korekcia = row.Korekcia.Sub(v) // oprávky majú zostatok D
				} else if s == debitSide {
					row.Brutto = row.Brutto.Add(v)
				} else {
					row.Brutto = row.Brutto.Sub(v)
				}
			}
		}
		row.Accounts = strings.Join(accs, ", ")
		row.Brutto, row.Korekcia = row.Brutto.RoundTo(0), row.Korekcia.RoundTo(0)
		row.Netto = row.Brutto.Sub(row.Korekcia)
		out[r.code] = row
	}
	return out
}

// sums dopočíta súčtové riadky (v poradí od konca, aby boli podriadky hotové).
func sums(rules []rule, rows map[string]*StatementRow) {
	var calc func(code string) *StatementRow
	done := map[string]bool{}
	byCode := map[string]rule{}
	for _, r := range rules {
		byCode[r.code] = r
	}
	calc = func(code string) *StatementRow {
		row := rows[code]
		r := byCode[code]
		if r.sum == nil || done[code] {
			return row
		}
		done[code] = true
		row.Brutto, row.Korekcia, row.Netto = money.Zero, money.Zero, money.Zero
		for _, c := range r.sum {
			neg := strings.HasPrefix(c, "-")
			sub := calc(strings.TrimPrefix(c, "-"))
			if neg {
				row.Brutto, row.Korekcia, row.Netto = row.Brutto.Sub(sub.Brutto), row.Korekcia.Sub(sub.Korekcia), row.Netto.Sub(sub.Netto)
			} else {
				row.Brutto, row.Korekcia, row.Netto = row.Brutto.Add(sub.Brutto), row.Korekcia.Add(sub.Korekcia), row.Netto.Add(sub.Netto)
			}
		}
		return row
	}
	for _, r := range rules {
		calc(r.code)
	}
}

// Statements je súvaha a výkaz ziskov a strát.
type Statements struct {
	Assets      []StatementRow `json:"assets"`
	Liabilities []StatementRow `json:"liabilities"`
	Income      []StatementRow `json:"income"`
	// Unmapped sú účty so zostatkom, ktoré nepatria do žiadneho riadku.
	Unmapped []Turnover `json:"unmapped"`
	// Difference = aktíva − pasíva (po zaokrúhlení býva pár eur).
	Difference money.Dec `json:"difference"`
}

// statement zostaví výkazy z konečných stavov účtov jedného obdobia.
func statement(accounts []Turnover) (assets, liab, income map[string]*StatementRow, unmapped []Turnover) {
	bal := map[string]money.Dec{}
	for _, t := range accounts {
		if !t.Closing.IsZero() {
			bal[t.Account] = t.Closing
		}
	}
	used := map[string]bool{}
	income = fill(incomeRules, bal, creditSide, used)
	// náklady (riadky 09 – 17, 28 – 33, 36, 37) sú kladné pri zostatku MD
	for _, r := range incomeRules {
		if len(r.items) > 0 && (r.items[0][0] == '5') {
			row := income[r.code]
			row.Brutto, row.Netto = row.Brutto.Neg(), row.Netto.Neg()
		}
	}
	sums(incomeRules, income)
	assets = fill(balanceAssets, bal, debitSide, used)
	sums(balanceAssets, assets)
	liab = fill(balanceLiabilities, bal, creditSide, used)
	vh := income["38"].Netto
	liab["33"].Brutto, liab["33"].Netto, liab["33"].Accounts = vh, vh, "VZaS r. 38"
	sums(balanceLiabilities, liab)
	for _, t := range accounts {
		if _, ok := bal[t.Account]; ok && !used[t.Account] && t.Account[0] <= '6' {
			unmapped = append(unmapped, t)
		}
	}
	return
}

// BuildStatements zostaví súvahu a výkaz ziskov a strát z konečných stavov
// účtov bežného a predchádzajúceho obdobia.
func BuildStatements(current, previous []Turnover) Statements {
	a, l, i, un := statement(current)
	pa, pl, pi, _ := statement(previous)
	rows := func(rules []rule, cur, prev map[string]*StatementRow) []StatementRow {
		out := make([]StatementRow, 0, len(rules))
		for _, r := range rules {
			row := *cur[r.code]
			row.Previous = prev[r.code].Netto
			out = append(out, row)
		}
		return out
	}
	if un == nil {
		un = []Turnover{}
	}
	sort.Slice(un, func(x, y int) bool { return un[x].Account < un[y].Account })
	return Statements{
		Assets:      rows(balanceAssets, a, pa),
		Liabilities: rows(balanceLiabilities, l, pl),
		Income:      rows(incomeRules, i, pi),
		Unmapped:    un,
		Difference:  a["01"].Netto.Sub(l["24"].Netto),
	}
}

// OpeningFromClosing navrhne počiatočné stavy nasledujúceho roka: zostatky
// súvahových účtov (triedy 0 – 4) a výsledok hospodárenia (triedy 5 a 6)
// na účet 431 Výsledok hospodárenia v schvaľovaní.
func OpeningFromClosing(accounts []Turnover) []Line {
	var out []Line
	result := money.Zero
	add := func(acc string, v money.Dec) {
		if v.Sign() > 0 {
			out = append(out, Line{Account: acc, Debit: v})
		} else if v.Sign() < 0 {
			out = append(out, Line{Account: acc, Credit: v.Neg()})
		}
	}
	extra431 := money.Zero
	for _, t := range accounts {
		switch {
		case t.Account[0] == '5' || t.Account[0] == '6':
			result = result.Add(t.Closing)
		case t.Account == "431":
			extra431 = extra431.Add(t.Closing)
		case t.Account[0] <= '4':
			add(t.Account, t.Closing)
		}
	}
	add("431", extra431.Add(result))
	if out == nil {
		out = []Line{}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Account < out[j].Account })
	return out
}
