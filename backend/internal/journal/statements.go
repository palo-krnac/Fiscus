package journal

import "github.com/palo-krnac/fiscus/backend/internal/money"

// Účtovná závierka v jednoduchom účtovníctve fyzickej osoby (opatrenie
// MF SR č. MF/27076/2007-74 v znení MF/15523/2014-74, vzory Úč FO 1-01
// a Úč FO 2-01). Sumy sú v celých eurách: každý riadok sa zaokrúhli
// matematicky, súčtové riadky sú súčtom zaokrúhlených riadkov.

// Row je riadok výkazu.
type Row struct {
	Code     string    `json:"code"` // „01“
	Label    string    `json:"label"`
	Current  money.Dec `json:"current"`
	Previous money.Dec `json:"previous"`
	Sum      bool      `json:"sum,omitempty"`   // súčtový riadok
	Input    string    `json:"input,omitempty"` // pole zadávané ručne (AssetInputs)
	Group    string    `json:"group,omitempty"` // príjmy/výdavky, majetok/záväzky
}

func euros(d money.Dec) money.Dec { return d.RoundTo(0) }

// IncomeStatement zostaví Výkaz o príjmoch a výdavkoch (Úč FO 1-01) zo
// súčtov peňažného denníka bežného a predchádzajúceho obdobia.
func IncomeStatement(cur, prev Totals) []Row {
	type def struct {
		code, label string
		col         Column
		group       string
	}
	defs := []def{
		{"01", "Predaj tovaru", IncGoods, "Príjmy"},
		{"02", "Predaj výrobkov a služieb", IncServices, "Príjmy"},
		{"03", "Ostatné príjmy", IncOther, "Príjmy"},
		{"05", "Zásoby", ExpStock, "Výdavky"},
		{"06", "Služby", ExpServices, "Výdavky"},
		{"07", "Mzdy", ExpWages, "Výdavky"},
		{"08", "Platby poistného a príspevkov", ExpInsur, "Výdavky"},
		{"09", "Tvorba sociálneho fondu", ExpSocial, "Výdavky"},
		{"10", "Ostatné výdavky", ExpOther, "Výdavky"},
	}
	var rows []Row
	var incC, incP, expC, expP money.Dec
	for _, d := range defs {
		c, p := euros(cur.Columns[d.col]), euros(prev.Columns[d.col])
		if d.group == "Príjmy" {
			incC, incP = incC.Add(c), incP.Add(p)
		} else {
			expC, expP = expC.Add(c), expP.Add(p)
		}
		rows = append(rows, Row{Code: d.code, Label: d.label, Current: c, Previous: p, Group: d.group})
		if d.code == "03" {
			rows = append(rows, Row{Code: "04", Label: "Príjmy celkom (r. 01 až 03)", Current: incC, Previous: incP, Sum: true, Group: "Príjmy"})
		}
	}
	rows = append(rows,
		Row{Code: "11", Label: "Výdavky celkom (r. 05 až 10)", Current: expC, Previous: expP, Sum: true, Group: "Výdavky"},
		Row{Code: "12", Label: "Rozdiel príjmov a výdavkov (r. 04 − r. 11)", Current: incC.Sub(expC), Previous: incP.Sub(expP), Sum: true})
	return rows
}

// AssetInputs sú riadky výkazu o majetku a záväzkoch, ktoré Fiškus zatiaľ
// nevie z dokladov (dlhodobý majetok, zásoby, úvery…); zadáva ich používateľ.
type AssetInputs struct {
	IntangibleDM money.Dec `json:"r01"` // dlhodobý nehmotný majetok
	TangibleDM   money.Dec `json:"r02"`
	FinancialDM  money.Dec `json:"r03"`
	Material     money.Dec `json:"r05"`
	Goods        money.Dec `json:"r06"`
	OtherStock   money.Dec `json:"r07"`
	OtherCashEq  money.Dec `json:"r12"` // ostatný krátkodobý finančný majetok
	AdjustAsset  money.Dec `json:"r14"` // opravná položka k nadobudnutému majetku (aktívna)
	Reserves     money.Dec `json:"r16"`
	OtherLiab    money.Dec `json:"r17x"` // záväzky mimo prijatých faktúr (DPH, mzdy, odvody, sociálny fond)
	Loans        money.Dec `json:"r18"`
	AdjustLiab   money.Dec `json:"r19"`
	OtherRecv    money.Dec `json:"r08x"` // pohľadávky mimo vydaných faktúr (nadmerný odpočet, preddavky…)
}

// AssetFacts sú hodnoty z dokladov k poslednému dňu obdobia.
type AssetFacts struct {
	Receivables money.Dec `json:"receivables"` // kniha pohľadávok
	Payables    money.Dec `json:"payables"`    // kniha záväzkov (aj dobropisy na vrátenie)
	Cash        money.Dec `json:"cash"`        // konečný stav pokladnice
	Bank        money.Dec `json:"bank"`        // konečné stavy účtov
	Transit     money.Dec `json:"transit"`     // priebežné položky (+/−): výdavok c − príjem c
}

// AssetValues sú vstupy a fakty jedného obdobia.
type AssetValues struct {
	Inputs AssetInputs `json:"inputs"`
	Facts  AssetFacts  `json:"facts"`
}

// AssetStatement zostaví Výkaz o majetku a záväzkoch (Úč FO 2-01).
func AssetStatement(cur, prev AssetValues) []Row {
	type side struct{ c, p money.Dec }
	val := func(f func(AssetValues) money.Dec) side { return side{euros(f(cur)), euros(f(prev))} }
	add := func(xs ...side) side {
		var s side
		for _, x := range xs {
			s.c, s.p = s.c.Add(x.c), s.p.Add(x.p)
		}
		return s
	}
	r01 := val(func(v AssetValues) money.Dec { return v.Inputs.IntangibleDM })
	r02 := val(func(v AssetValues) money.Dec { return v.Inputs.TangibleDM })
	r03 := val(func(v AssetValues) money.Dec { return v.Inputs.FinancialDM })
	r05 := val(func(v AssetValues) money.Dec { return v.Inputs.Material })
	r06 := val(func(v AssetValues) money.Dec { return v.Inputs.Goods })
	r07 := val(func(v AssetValues) money.Dec { return v.Inputs.OtherStock })
	r04 := add(r05, r06, r07)
	r08 := val(func(v AssetValues) money.Dec { return v.Facts.Receivables.Add(v.Inputs.OtherRecv) })
	r10 := val(func(v AssetValues) money.Dec { return v.Facts.Cash })
	r11 := val(func(v AssetValues) money.Dec { return v.Facts.Bank })
	r12 := val(func(v AssetValues) money.Dec { return v.Inputs.OtherCashEq })
	r09 := add(r10, r11, r12)
	r13 := val(func(v AssetValues) money.Dec { return v.Facts.Transit })
	r14 := val(func(v AssetValues) money.Dec { return v.Inputs.AdjustAsset })
	r15 := add(r01, r02, r03, r04, r08, r09, r13, r14)
	r16 := val(func(v AssetValues) money.Dec { return v.Inputs.Reserves })
	r17 := val(func(v AssetValues) money.Dec { return v.Facts.Payables.Add(v.Inputs.OtherLiab) })
	r18 := val(func(v AssetValues) money.Dec { return v.Inputs.Loans })
	r19 := val(func(v AssetValues) money.Dec { return v.Inputs.AdjustLiab })
	r20 := add(r16, r17, r18, r19)
	r21 := side{r15.c.Sub(r20.c), r15.p.Sub(r20.p)}

	row := func(code, label string, s side, sum bool, input, group string) Row {
		return Row{Code: code, Label: label, Current: s.c, Previous: s.p, Sum: sum, Input: input, Group: group}
	}
	const a, l = "Majetok", "Záväzky"
	return []Row{
		row("01", "Dlhodobý nehmotný majetok", r01, false, "r01", a),
		row("02", "Dlhodobý hmotný majetok", r02, false, "r02", a),
		row("03", "Dlhodobý finančný majetok", r03, false, "r03", a),
		row("04", "Zásoby celkom (r. 05 až 07)", r04, true, "", a),
		row("05", "Materiál", r05, false, "r05", a),
		row("06", "Tovar", r06, false, "r06", a),
		row("07", "Nedokončená výroba, výrobky, zvieratá, ostatné", r07, false, "r07", a),
		row("08", "Pohľadávky", r08, false, "r08x", a),
		row("09", "Krátkodobý finančný majetok celkom (r. 10 až 12)", r09, true, "", a),
		row("10", "Peniaze a ceniny", r10, false, "", a),
		row("11", "Účty v bankách", r11, false, "", a),
		row("12", "Ostatný krátkodobý finančný majetok", r12, false, "r12", a),
		row("13", "Priebežné položky (+/−)", r13, false, "", a),
		row("14", "Opravná položka k nadobudnutému majetku (aktívna)", r14, false, "r14", a),
		row("15", "Majetok celkom (r. 01 + 02 + 03 + 04 + 08 + 09 +/− 13 + 14)", r15, true, "", a),
		row("16", "Rezervy", r16, false, "r16", l),
		row("17", "Záväzky", r17, false, "r17x", l),
		row("18", "Úvery", r18, false, "r18", l),
		row("19", "Opravná položka k nadobudnutému majetku (pasívna)", r19, false, "r19", l),
		row("20", "Záväzky celkom (r. 16 až 19)", r20, true, "", l),
		row("21", "Rozdiel majetku a záväzkov (r. 15 − r. 20)", r21, true, "", ""),
	}
}
