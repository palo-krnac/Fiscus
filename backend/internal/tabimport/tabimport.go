// Package tabimport číta adresár, vydané a prijaté faktúry z tabuľky
// (Excel alebo CSV z ľubovoľného programu): jeden riadok = jeden kontakt
// alebo jedna faktúra. Stĺpce sa nájdu podľa hlavičky (slovensky, česky,
// anglicky, bez ohľadu na diakritiku). Sumy faktúry môžu byť po sadzbách
// („Základ 23 %“, „DPH 23 %“), ako jeden základ so sadzbou, alebo iba
// suma spolu so sadzbou — základ a daň sa potom dopočítajú.
package tabimport

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Druhy tabuliek.
const (
	KindContacts  = "contacts"
	KindItems     = "items"
	KindInvoices  = "invoices"
	KindPurchases = "purchases"
)

// Party sú údaje partnera z riadku.
type Party struct {
	Name, Street, City, Zip, Country, ICO, DIC, ICDPH, Email, Phone, Note string
	Person                                                                string // meno a priezvisko kontaktnej osoby (Outlook, Google)
	IBAN                                                                  string
}

// Amount je základ a daň jednej sadzby.
type Amount struct {
	Rate vat.Rate
	Base money.Dec
	VAT  money.Dec
}

// Doc je faktúra z jedného riadku.
type Doc struct {
	Line           int
	Number         string
	SupplierNumber string
	VS             string
	Issue          invoice.Date
	Delivery       invoice.Date
	Due            invoice.Date
	Subject        string
	Party          Party
	Amounts        []Amount
	Total          money.Dec
	Paid           bool
	PaidDate       invoice.Date
	Credit         bool
	Original       string
	Err            string // dôvod, prečo sa riadok nedá preniesť
}

// Contact je kontakt z jedného riadku.
type Contact struct {
	Line  int
	Party Party
}

// Sheet je načítaná tabuľka.
type Sheet struct {
	Kind     string
	Contacts []Contact
	Docs     []Doc
	Columns  []string // rozpoznané stĺpce (na náhľad)
}

// stĺpce
const (
	cName = iota
	cStreet
	cCity
	cZip
	cCountry
	cICO
	cDIC
	cICDPH
	cEmail
	cPhone
	cNote
	cNumber
	cSupplierNumber
	cVS
	cIssue
	cDelivery
	cDue
	cSubject
	cBase
	cVAT
	cRate
	cTotal
	cPaid
	cPaidDate
	cType
	cOriginal
	cPrice // stĺpec ceny cenníka (rozpozná tabuľku cenníka)
	cFirst
	cLast
	cIBAN
)

var colNames = map[int]string{cName: "Názov", cStreet: "Ulica", cCity: "Obec", cZip: "PSČ", cCountry: "Štát", cICO: "IČO", cDIC: "DIČ",
	cICDPH: "IČ DPH", cEmail: "E-mail", cPhone: "Telefón", cNote: "Poznámka", cNumber: "Číslo", cSupplierNumber: "Číslo dodávateľa",
	cVS: "VS", cIssue: "Vystavená", cDelivery: "Dodanie", cDue: "Splatnosť", cSubject: "Predmet", cBase: "Základ", cVAT: "DPH",
	cRate: "Sadzba DPH", cTotal: "Spolu", cPaid: "Uhradená", cPaidDate: "Dátum úhrady", cType: "Druh", cOriginal: "Pôvodná faktúra"}

// aliases: hlavička (catalog.Key) → stĺpec.
var aliases = map[string]int{}

func init() {
	add := func(c int, names ...string) {
		for _, n := range names {
			aliases[n] = c
		}
	}
	add(cName, "nazov", "meno", "firma", "obchodne meno", "nazov firmy", "odberatel", "dodavatel", "partner", "zakaznik", "nazev",
		"odberatel nazov", "dodavatel nazov", "name", "company", "customer", "supplier", "nazov odberatela", "nazov dodavatela", "obchodni jmeno", "jmeno")
	add(cStreet, "ulica", "adresa", "ulice", "street", "address", "ulica a cislo")
	add(cCity, "obec", "mesto", "misto", "city", "town")
	add(cZip, "psc", "zip", "postal code", "postcode")
	add(cCountry, "stat", "krajina", "country", "kod statu", "kod krajiny")
	add(cICO, "ico", "ic", "company id", "reg. no.", "reg no")
	add(cDIC, "dic", "tax id")
	add(cICDPH, "ic dph", "icdph", "ic pre dph", "vat id", "vat number", "dic (ic dph)")
	add(cEmail, "email", "e-mail", "mail")
	add(cPhone, "telefon", "tel", "tel.", "phone", "mobil")
	add(cNote, "poznamka", "note", "notes", "pozn.")
	add(cNumber, "cislo", "cislo faktury", "cislo dokladu", "doklad", "faktura", "number", "invoice number", "invoice no", "c. faktury", "interne cislo")
	add(cSupplierNumber, "cislo faktury dodavatela", "cislo dodavatela", "externe cislo", "povodne cislo", "cislo u dodavatela", "prijaty doklad",
		"cislo dokladu dodavatela", "supplier invoice number", "externi cislo")
	add(cVS, "vs", "variabilny symbol", "variabilni symbol", "variable symbol")
	add(cIssue, "datum vystavenia", "vystavena", "vystavene", "vystaveno", "datum vystaveni", "datum", "issue date", "date", "issued", "vystavenie")
	add(cDelivery, "datum dodania", "dodanie", "duzp", "datum zdanitelneho plnenia", "zdanitelne plnenie", "datum plneni", "dodani", "delivery date", "tax date",
		"den dodania", "datum dodani")
	add(cDue, "splatnost", "datum splatnosti", "splatna", "splatno", "due date", "due")
	add(cSubject, "predmet", "popis", "text", "subject", "description", "predmet faktury", "polozka")
	add(cBase, "zaklad", "zaklad dph", "bez dph", "suma bez dph", "celkom bez dph", "spolu bez dph", "net", "net amount", "zaklad dane", "celkem bez dph")
	add(cVAT, "dph", "dan", "suma dph", "vat", "vat amount", "dph spolu", "celkom dph", "dph celkem")
	add(cRate, "sadzba", "sadzba dph", "dph %", "dph (%)", "sazba", "sazba dph", "vat rate", "vat %")
	add(cTotal, "spolu", "celkom", "suma", "suma spolu", "spolu s dph", "celkom s dph", "k uhrade", "celkem", "celkem s dph", "total", "amount", "gross",
		"celkova suma", "suma s dph")
	add(cPaid, "uhradena", "uhradene", "zaplatena", "zaplatene", "uhrazeno", "zaplaceno", "paid", "stav uhrady", "stav")
	add(cPaidDate, "datum uhrady", "uhradena dna", "datum zaplatenia", "datum uhrazeni", "paid date", "payment date", "zaplatene dna")
	add(cType, "druh", "typ", "typ dokladu", "druh dokladu", "type", "document type")
	add(cOriginal, "povodna faktura", "opravuje", "k fakture", "opravovana faktura", "puvodni doklad", "original invoice", "dobropis k")
	add(cFirst, "meno osoby", "krstne meno", "first name", "given name", "jmeno osoby", "krestni jmeno")
	add(cLast, "priezvisko", "last name", "family name", "surname", "prijmeni")
	add(cIBAN, "iban", "bankovy ucet", "cislo uctu", "ucet", "bank account", "bankovni ucet")
	// Outlook a Google Kontakty (anglicky aj slovensky)
	add(cName, "organization name", "organization 1 - name", "spolocnost", "company name", "display name", "zobrazovane meno")
	add(cEmail, "e-mail address", "e-mail 1 - value", "email address", "e-mailova adresa", "primary email")
	add(cPhone, "business phone", "mobile phone", "phone 1 - value", "primary phone", "home phone", "telefon do prace", "mobilny telefon",
		"telefon domov")
	add(cStreet, "business street", "home street", "address 1 - street", "ulica (praca)", "ulica (domov)")
	add(cCity, "business city", "home city", "address 1 - city", "mesto (praca)", "mesto (domov)")
	add(cZip, "business postal code", "home postal code", "address 1 - postal code", "psc (praca)", "psc (domov)")
	add(cCountry, "business country/region", "home country/region", "address 1 - country", "krajina/oblast (praca)", "krajina/oblast (domov)")
	add(cNote, "notes", "poznamky")
	add(cPrice, "cena", "cena bez dph", "cena s dph", "jednotkova cena", "price")
}

// stĺpce súm podľa sadzby: „Základ 23 %“, „DPH 23%“, „základ 0“, „Bez DPH 19 %“
var (
	reRateBase = regexp.MustCompile(`^(zaklad|zaklad dph|zaklad dane|bez dph|net)\s*(\(?\s*(\d{1,2})\s*%?\s*\)?)$`)
	reRateVAT  = regexp.MustCompile(`^(dph|dan|vat)\s*(\(?\s*(\d{1,2})\s*%?\s*\)?)$`)
)

type layout struct {
	col      map[int][]int // stĺpec → indexy (prvá neprázdna hodnota)
	baseRate map[int]int   // sadzba → index stĺpca so základom
	vatRate  map[int]int
}

func (l layout) has(c int) bool { return len(l.col[c]) > 0 }

func readLayout(head []string) layout {
	l := layout{col: map[int][]int{}, baseRate: map[int]int{}, vatRate: map[int]int{}}
	keys := map[int]string{}
	for i, h := range head {
		k := strings.TrimSuffix(strings.TrimSpace(catalog.Key(h)), ":")
		k = strings.TrimSpace(strings.NewReplacer("€", "", "(eur)", "", "eur", "").Replace(k))
		if m := reRateBase.FindStringSubmatch(k); m != nil {
			r, _ := strconv.Atoi(m[3])
			l.baseRate[r] = i
			continue
		}
		if m := reRateVAT.FindStringSubmatch(k); m != nil {
			r, _ := strconv.Atoi(m[3])
			l.vatRate[r] = i
			continue
		}
		if c, ok := aliases[k]; ok {
			l.col[c] = append(l.col[c], i)
			keys[i] = k
		}
	}
	// Outlook po slovensky: „Meno“ vedľa „Priezvisko“ je krstné meno
	if l.has(cLast) {
		var rest []int
		for _, i := range l.col[cName] {
			if keys[i] == "meno" || keys[i] == "jmeno" {
				l.col[cFirst] = append(l.col[cFirst], i)
			} else {
				rest = append(rest, i)
			}
		}
		l.col[cName] = rest
	}
	// názov firmy má prednosť pred menom osoby
	sort.SliceStable(l.col[cName], func(a, b int) bool {
		return companyKeys[keys[l.col[cName][a]]] && !companyKeys[keys[l.col[cName][b]]]
	})
	return l
}

// companyKeys sú hlavičky s názvom firmy (prednosť pred menom osoby).
var companyKeys = map[string]bool{"firma": true, "nazov firmy": true, "obchodne meno": true, "company": true, "company name": true,
	"organization name": true, "organization 1 - name": true, "spolocnost": true, "obchodni jmeno": true, "nazov": true}

// Detect odhadne druh tabuľky z hlavičky: cenník, adresár; pri faktúrach
// nevie, či sú vydané alebo prijaté (vráti "").
func Detect(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	l := readLayout(rows[0])
	docs := l.has(cIssue) || l.has(cTotal) || l.has(cBase) || len(l.baseRate) > 0
	switch {
	case docs:
		return ""
	case l.has(cPrice):
		return KindItems
	case l.has(cName) || l.has(cFirst) || l.has(cLast):
		return KindContacts
	}
	return ""
}

// ErrNeedKind — pri faktúrach treba povedať, či sú vydané alebo prijaté.
var ErrNeedKind = fmt.Errorf("vyberte, či sú v tabuľke vydané alebo prijaté faktúry")

// Parse prečíta adresár alebo faktúry (prvý riadok = hlavička).
// defRate je sadzba pre faktúry iba so sumou a bez stĺpca sadzby.
func Parse(rows [][]string, kind string, defRate vat.Rate) (Sheet, error) {
	if len(rows) < 2 {
		return Sheet{}, fmt.Errorf("tabuľka je prázdna — v prvom riadku má byť hlavička, v ďalších záznamy")
	}
	l := readLayout(rows[0])
	sh := Sheet{Kind: kind}
	for c, ii := range l.col {
		if n, ok := colNames[c]; ok {
			sh.Columns = append(sh.Columns, fmt.Sprintf("%d:%s", ii[0], n))
		}
	}
	for r, i := range l.baseRate {
		sh.Columns = append(sh.Columns, fmt.Sprintf("%d:Základ %d %%", i, r))
	}
	for r, i := range l.vatRate {
		sh.Columns = append(sh.Columns, fmt.Sprintf("%d:DPH %d %%", i, r))
	}
	sort.Slice(sh.Columns, func(a, b int) bool {
		x, _ := strconv.Atoi(strings.SplitN(sh.Columns[a], ":", 2)[0])
		y, _ := strconv.Atoi(strings.SplitN(sh.Columns[b], ":", 2)[0])
		return x < y
	})
	for i := range sh.Columns {
		sh.Columns[i] = strings.SplitN(sh.Columns[i], ":", 2)[1]
	}
	if !l.has(cName) && !l.has(cFirst) && !l.has(cLast) {
		return sh, fmt.Errorf("v hlavičke chýba stĺpec s názvom partnera (Názov, Odberateľ, Dodávateľ, Firma…)")
	}
	switch kind {
	case KindContacts:
		for n, row := range rows[1:] {
			if blank(row) {
				continue
			}
			sh.Contacts = append(sh.Contacts, Contact{Line: n + 2, Party: l.party(row)})
		}
		return sh, nil
	case KindInvoices, KindPurchases:
	case "":
		return sh, ErrNeedKind
	default:
		return sh, fmt.Errorf("neznámy druh tabuľky %q", kind)
	}
	if !l.has(cNumber) && !(kind == KindPurchases && l.has(cSupplierNumber)) {
		return sh, fmt.Errorf("v hlavičke chýba stĺpec s číslom faktúry (Číslo, Číslo faktúry, Doklad…)")
	}
	if !l.has(cTotal) && !l.has(cBase) && len(l.baseRate) == 0 {
		return sh, fmt.Errorf("v hlavičke chýba suma (Spolu, Základ alebo Základ 23 %%…)")
	}
	for n, row := range rows[1:] {
		if blank(row) {
			continue
		}
		sh.Docs = append(sh.Docs, l.doc(row, n+2, defRate))
	}
	return sh, nil
}

func blank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func (l layout) get(row []string, c int) string {
	for _, i := range l.col[c] {
		if i < len(row) && strings.TrimSpace(row[i]) != "" {
			return strings.TrimSpace(row[i])
		}
	}
	return ""
}

func cell(row []string, i int) string {
	if i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

func (l layout) party(row []string) Party {
	g := func(c int) string { return l.get(row, c) }
	name := g(cName)
	person := strings.Join(strings.Fields(g(cFirst)+" "+g(cLast)), " ")
	if name == "" {
		name = person
	}
	p := Party{Name: name, Person: person, IBAN: strings.ToUpper(strings.ReplaceAll(g(cIBAN), " ", "")), Street: g(cStreet), City: g(cCity), Zip: strings.ReplaceAll(g(cZip), " ", ""), Country: g(cCountry),
		ICO: strings.ReplaceAll(g(cICO), " ", ""), DIC: strings.ReplaceAll(g(cDIC), " ", ""), ICDPH: strings.ToUpper(strings.ReplaceAll(g(cICDPH), " ", "")),
		Email: g(cEmail), Phone: g(cPhone), Note: g(cNote)}
	// IČO z Excelu ako číslo stratí úvodné nuly
	if len(p.ICO) == 6 || len(p.ICO) == 7 {
		if _, err := strconv.Atoi(p.ICO); err == nil {
			p.ICO = strings.Repeat("0", 8-len(p.ICO)) + p.ICO
		}
	}
	// DIČ v tvare IČ DPH
	if len(p.DIC) > 2 && p.DIC[0] >= 'A' && p.DIC[0] <= 'Z' {
		if p.ICDPH == "" {
			p.ICDPH = strings.ToUpper(p.DIC)
		}
		p.DIC = ""
	}
	if p.DIC == "" && strings.HasPrefix(p.ICDPH, "SK") {
		p.DIC = p.ICDPH[2:]
	}
	return p
}

func (l layout) doc(row []string, line int, defRate vat.Rate) Doc {
	g := func(c int) string { return l.get(row, c) }
	d := Doc{Line: line, Number: g(cNumber), SupplierNumber: g(cSupplierNumber), VS: g(cVS), Subject: g(cSubject), Party: l.party(row),
		Original: g(cOriginal)}
	fail := func(format string, args ...any) Doc {
		if d.Err == "" {
			d.Err = fmt.Sprintf(format, args...)
		}
		return d
	}
	var ok bool
	if d.Issue, ok = Date(g(cIssue)); !ok {
		return fail("chybný dátum vystavenia „%s“", g(cIssue))
	}
	if d.Delivery, ok = Date(g(cDelivery)); !ok {
		return fail("chybný dátum dodania „%s“", g(cDelivery))
	}
	if d.Due, ok = Date(g(cDue)); !ok {
		return fail("chybná splatnosť „%s“", g(cDue))
	}
	if d.Issue.IsZero() {
		return fail("chýba dátum vystavenia")
	}
	if d.Delivery.IsZero() {
		d.Delivery = d.Issue
	}
	if d.Due.IsZero() {
		d.Due = d.Issue
	}
	if g(cName) == "" {
		return fail("chýba názov partnera")
	}

	// sumy
	num := func(s string) (money.Dec, bool) {
		if s == "" {
			return money.Zero, true
		}
		v, err := Number(s)
		return v, err == nil
	}
	total, okT := num(g(cTotal))
	if !okT {
		return fail("chybná suma „%s“", g(cTotal))
	}
	if len(l.baseRate) > 0 {
		rates := make([]int, 0, len(l.baseRate))
		for r := range l.baseRate {
			rates = append(rates, r)
		}
		sort.Ints(rates)
		for _, r := range rates {
			b, okB := num(cell(row, l.baseRate[r]))
			if !okB {
				return fail("chybný základ %d %% „%s“", r, cell(row, l.baseRate[r]))
			}
			if b.IsZero() {
				continue
			}
			a := Amount{Rate: vat.Rate(r), Base: b}
			if i, has := l.vatRate[r]; has {
				v, okV := num(cell(row, i))
				if !okV {
					return fail("chybná DPH %d %% „%s“", r, cell(row, i))
				}
				a.VAT = v
			} else {
				a.VAT = b.MulRatio(money.FromInt(int64(r)), money.FromInt(100)).RoundCents()
			}
			d.Amounts = append(d.Amounts, a)
		}
	} else {
		rate := defRate
		if s := g(cRate); s != "" {
			v, err := Number(s)
			if err != nil || !v.Equal(v.RoundTo(0)) {
				return fail("chybná sadzba DPH „%s“", s)
			}
			rate = vat.Rate(v.Raw() / 10000)
		}
		base, okB := num(g(cBase))
		tax, okV := num(g(cVAT))
		if !okB || !okV {
			return fail("chybný základ alebo DPH")
		}
		hasVAT := g(cVAT) != ""
		switch {
		case !base.IsZero() && !hasVAT && !total.IsZero():
			tax = total.Sub(base)
		case !base.IsZero() && !hasVAT:
			tax = base.MulRatio(money.FromInt(int64(rate)), money.FromInt(100)).RoundCents()
		case base.IsZero() && !total.IsZero() && hasVAT:
			base = total.Sub(tax)
		case base.IsZero() && !total.IsZero():
			base = total.MulRatio(money.FromInt(100), money.FromInt(int64(100+rate))).RoundCents()
			tax = total.Sub(base)
		}
		// bez stĺpca sadzby: sadzba z pomeru dane a základu
		if !l.has(cRate) && (hasVAT || !total.IsZero()) {
			if r, ok := GuessRate(base, tax); ok {
				rate = r
			}
		}
		if !base.IsZero() {
			d.Amounts = append(d.Amounts, Amount{Rate: rate, Base: base, VAT: tax})
		}
	}
	if len(d.Amounts) == 0 {
		return fail("faktúra nemá sumu")
	}
	sum := money.Zero
	for _, a := range d.Amounts {
		sum = sum.Add(a.Base).Add(a.VAT)
	}
	d.Total = total
	if d.Total.IsZero() {
		d.Total = sum
	}

	// druh a úhrada
	t := catalog.Key(g(cType))
	d.Credit = strings.Contains(t, "dobropis") || strings.Contains(t, "credit") || strings.Contains(t, "opravn") || d.Total.Sign() < 0
	if strings.Contains(t, "zaloh") || strings.Contains(t, "proforma") || strings.Contains(t, "advance") {
		return fail("zálohová faktúra sa neprenáša (zálohy zadajte vo Fiškuse)")
	}
	if pd := g(cPaidDate); pd != "" {
		if d.PaidDate, ok = Date(pd); !ok {
			return fail("chybný dátum úhrady „%s“", pd)
		}
		d.Paid = !d.PaidDate.IsZero()
	}
	if p := g(cPaid); p != "" {
		if dt, ok := Date(p); ok && !dt.IsZero() {
			d.Paid, d.PaidDate = true, dt
		} else {
			d.Paid = Yes(p)
		}
	}
	return d
}

// GuessRate odhadne sadzbu z pomeru dane a základu (23, 20, 19, 10, 5, 0 %).
func GuessRate(base, tax money.Dec) (vat.Rate, bool) {
	if base.IsZero() {
		return 0, false
	}
	for _, r := range []vat.Rate{vat.Rate23, vat.Rate20, vat.Rate19, vat.Rate10, vat.Rate5, vat.Rate0} {
		want := base.MulRatio(money.FromInt(int64(r)), money.FromInt(100)).RoundCents()
		diff := want.Sub(tax)
		if diff.Sign() < 0 {
			diff = diff.Neg()
		}
		if diff.Cmp(money.MustParse("0.02")) <= 0 {
			return r, true
		}
	}
	return 0, false
}

// Yes: áno, ano, yes, 1, x, true, uhradená, zaplatené, paid.
func Yes(s string) bool {
	k := catalog.Key(s)
	switch k {
	case "ano", "a", "yes", "y", "1", "x", "true", "pravda", "uhradena", "uhradene", "uhradeno", "zaplatena", "zaplatene", "zaplaceno",
		"uhrazeno", "uhrazena", "paid":
		return true
	}
	return false
}

// Number prečíta číslo: „1 234,50“, „1.234,50“, „1,234.50“, „-12,5 €“.
func Number(s string) (money.Dec, error) {
	s = strings.NewReplacer(" ", "", " ", "", " ", "", "%", "", "€", "", "EUR", "", "eur", "").Replace(strings.TrimSpace(s))
	if c, d := strings.LastIndex(s, ","), strings.LastIndex(s, "."); c > d {
		s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	} else if c >= 0 {
		s = strings.ReplaceAll(s, ",", "")
	}
	return money.Parse(s)
}

var reDate = regexp.MustCompile(`^(\d{1,2})\s*[./,-]\s*(\d{1,2})\s*[./,-]\s*(\d{4})$`)

// Date prečíta dátum: 2026-03-02, 2.3.2026, 02. 03. 2026, 2/3/2026
// (deň/mesiac) aj poradové číslo dňa z Excelu. Prázdny reťazec je nulový
// dátum (ok); ok=false pri nečitateľnom dátume.
func Date(s string) (invoice.Date, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return invoice.Date{}, true
	}
	if len(s) >= 10 && s[4] == '-' {
		d, err := invoice.ParseDate(s[:10])
		return d, err == nil
	}
	if m := reDate.FindStringSubmatch(s); m != nil {
		d, err := invoice.ParseDate(fmt.Sprintf("%s-%02s-%02s", m[3], m[2], m[1]))
		return d, err == nil
	}
	// Excel: dni od 30. 12. 1899 (aj s časťou dňa)
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 20000 && f < 80000 {
		t := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(f))
		d, err := invoice.ParseDate(t.Format("2006-01-02"))
		return d, err == nil
	}
	return invoice.Date{}, false
}
