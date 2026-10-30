package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/chart"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/ledger"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Účtovný denník sa skladá z dokladov roka: vydané a prijaté faktúry,
// bankové pohyby a pokladničné doklady sa zaúčtujú podľa predkontácie
// (Fiškus ju navrhne, používateľ ju môže zmeniť alebo zadať riadky ručne),
// interné doklady zadáva používateľ. Hlavná kniha je súčet zápisov
// s počiatočnými stavmi účtov.

// LedgerData sú uložené údaje denníka za rok.
type LedgerData struct {
	// Assign je zvolená predkontácia dokladu (kľúč → kód).
	Assign map[string]string `json:"assign,omitempty"`
	// Manual sú ručné riadky dokladu (majú prednosť pred predkontáciou).
	Manual   map[string]ManualPosting `json:"manual,omitempty"`
	Internal []InternalDoc            `json:"internal,omitempty"`
	// Opening sú počiatočné stavy účtov (kladný = MD, záporný = D).
	Opening map[string]money.Dec `json:"opening,omitempty"`
	NextID  int64                `json:"nextId,omitempty"`
	// Revalued sú sumy pohľadávok a záväzkov v cudzej mene v eurách
	// prepočítané kurzom k 31. 12. (kľúč sale:ID, purchase:ID).
	Revalued map[string]money.Dec `json:"revalued,omitempty"`
	// Dims sú stredisko a zákazka dokladu (náklady a výnosy).
	Dims map[string]ledger.Dims `json:"dims,omitempty"`
}

// ManualPosting sú ručné riadky dokladu.
type ManualPosting struct {
	Lines []ledger.Line `json:"lines"`
	Note  string        `json:"note,omitempty"`
}

// InternalDoc je interný doklad: buď predkontácia so sumou, alebo riadky.
type InternalDoc struct {
	ID       int64         `json:"id"`
	Number   string        `json:"number"`
	Date     invoice.Date  `json:"date"`
	Text     string        `json:"text"`
	Template string        `json:"template,omitempty"`
	Amount   money.Dec     `json:"amount"`
	Lines    []ledger.Line `json:"lines,omitempty"`
	ledger.Dims
	// FX: kurzové rozdiely k 31. 12. (vytvára Fiškus).
	FX bool `json:"fx,omitempty"`
}

// LedgerBook je účtovný denník a hlavná kniha za rok.
type LedgerBook struct {
	Year      int               `json:"year"`
	Postings  []ledger.Posting  `json:"postings"`
	Accounts  []ledger.Turnover `json:"accounts"`
	Synthetic []ledger.Turnover `json:"synthetic"`
	Totals    ledger.Totals     `json:"totals"`
	// Unposted je počet dokladov bez zápisu alebo s chybou.
	Unposted  int                  `json:"unposted"`
	Opening   map[string]money.Dec `json:"opening"`
	Templates []TemplateRef        `json:"templates"`
	Internal  []InternalDoc        `json:"internal"`
	// DimTurnovers sú náklady a výnosy po strediskách a zákazkách.
	DimTurnovers []ledger.DimTurnover `json:"dimTurnovers"`
	// MoneySources sú bankové účty a pokladnice s priradeným účtom.
	MoneySources []MoneySource `json:"moneySources"`
}

// MoneySource je bankový účet alebo pokladnica a jej analytický účet.
type MoneySource struct {
	Key     string `json:"key"` // bank:IBAN | cash:ID
	Label   string `json:"label"`
	Account string `json:"account"` // priradený účet ("" = syntetický z predkontácie)
}

// TemplateRef je predkontácia na výber.
type TemplateRef struct {
	Code string        `json:"code"`
	Name string        `json:"name"`
	Doc  chart.DocKind `json:"doc"`
}

func ledgerKey(companyID int64, year int) string { return fmt.Sprintf("ledger.%d.%d", companyID, year) }

func (a *App) loadLedger(ctx context.Context, companyID int64, year int) (LedgerData, error) {
	var d LedgerData
	raw, err := a.Store.GetSetting(ctx, ledgerKey(companyID, year))
	if errors.Is(err, store.ErrNotFound) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	return d, json.Unmarshal([]byte(raw), &d)
}

// editLedger upraví údaje denníka pod zámkom a zapíše audit.
func (a *App) editLedger(ctx context.Context, companyID int64, year int, edit func(*LedgerData, chart.Chart, []chart.Template) (string, error)) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	if year < 2000 || year > 2100 {
		return mfaFieldErr("year", "Neplatný rok")
	}
	a.ledgerMu.Lock()
	defer a.ledgerMu.Unlock()
	cb, err := a.loadChart(ctx, companyID)
	if err != nil {
		return err
	}
	d, err := a.loadLedger(ctx, companyID, year)
	if err != nil {
		return err
	}
	summary, err := edit(&d, chart.Chart{Accounts: cb.Accounts}, cb.Templates)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if err := a.Store.SetSetting(ctx, ledgerKey(companyID, year), string(raw)); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "posting", "ledger", 0, summary, nil, nil)
	return nil
}

// Ledger zostaví účtovný denník a hlavnú knihu za rok.
func (a *App) Ledger(ctx context.Context, companyID int64, year int) (LedgerBook, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return LedgerBook{}, err
	}
	if year == 0 {
		year = a.Today().Year()
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return LedgerBook{}, err
	}
	cb, err := a.loadChart(ctx, companyID)
	if err != nil {
		return LedgerBook{}, err
	}
	d, err := a.loadLedger(ctx, companyID, year)
	if err != nil {
		return LedgerBook{}, err
	}
	c := chart.Chart{Accounts: cb.Accounts}
	p := poster{tpl: map[string]chart.Template{}, vatPayer: company.VATPayer, moneyMap: cb.Money}
	for _, t := range cb.Templates {
		if t.Active {
			p.tpl[t.Code] = t
		}
	}
	ps, err := a.ledgerSources(ctx, companyID, year, &p)
	if err != nil {
		return LedgerBook{}, err
	}
	for i := range ps {
		post := &ps[i]
		post.Dims = d.Dims[post.Key]
		if m, ok := d.Manual[post.Key]; ok {
			post.Lines, post.Note, post.Manual, post.Template, post.Problem = append([]ledger.Line(nil), m.Lines...), m.Note, true, "", ""
		} else {
			if code, ok := d.Assign[post.Key]; ok {
				post.Template = code
			}
			p.post(post)
		}
		ledger.ApplyDims(post.Lines, post.Dims)
	}
	for _, doc := range d.Internal {
		post := ledger.Posting{Key: "internal:" + strconv.FormatInt(doc.ID, 10), Date: doc.Date, Doc: chart.DocInternal,
			Number: doc.Number, Text: doc.Text, Amount: doc.Amount, Template: doc.Template, Link: ""}
		if doc.Template == "" {
			post.Lines, post.Manual = append([]ledger.Line(nil), doc.Lines...), true
			_, post.Amount = ledger.Sums(doc.Lines)
		} else {
			p.apply(&post, chart.Amounts{Total: doc.Amount})
		}
		post.Dims = doc.Dims
		ledger.ApplyDims(post.Lines, post.Dims)
		ps = append(ps, post)
	}
	ledger.Sort(ps)
	unposted := 0
	for i := range ps {
		post := &ps[i]
		if post.Problem == "" && !ledger.Balanced(post.Lines) {
			post.Problem = "Zápis nie je vyrovnaný: MD sa nerovná D"
		}
		if post.Problem != "" {
			unposted++
		}
		for j := range post.Lines {
			if acc, ok := c.Find(post.Lines[j].Account); ok {
				post.Lines[j].Name = acc.Name
			}
		}
		if post.Lines == nil {
			post.Lines = []ledger.Line{}
		}
	}
	accs, syn, tot := ledger.Book(ps, d.Opening, c)
	refs := []TemplateRef{}
	for _, t := range cb.Templates {
		if t.Active {
			refs = append(refs, TemplateRef{t.Code, t.Name, t.Doc})
		}
	}
	opening := d.Opening
	if opening == nil {
		opening = map[string]money.Dec{}
	}
	internal := d.Internal
	if internal == nil {
		internal = []InternalDoc{}
	}
	names := map[string]string{}
	for _, x := range cb.Centers {
		names["center:"+x.Code] = x.Name
	}
	for _, x := range cb.Jobs {
		names["job:"+x.Code] = x.Name
	}
	return LedgerBook{Year: year, Postings: ps, Accounts: accs, Synthetic: syn, Totals: tot, Unposted: unposted,
		Opening: opening, Templates: refs, Internal: internal, DimTurnovers: ledger.DimBook(ps, names),
		MoneySources: a.moneySources(ctx, companyID, company.IBAN, cb.Money)}, nil
}

// poster zaúčtuje doklad podľa predkontácie.
type poster struct {
	tpl      map[string]chart.Template
	vatPayer bool
	// amounts sú sumy dokladov podľa kľúča; reverse = prenesenie daňovej
	// povinnosti (neodpočítaná časť DPH ide MD nákladový účet / D 343).
	amounts map[string]chart.Amounts
	reverse map[string]money.Dec
	// moneyDir je smer peňazí pri banke a pokladni (+1 príjem, −1 výdavok).
	moneyDir map[string]int
	// moneyMap sú analytické účty bánk a pokladníc (bank:IBAN, cash:ID),
	// money je zdroj peňazí dokladu (kľúč do moneyMap).
	moneyMap map[string]string
	money    map[string]string
	// fx je suma dokladu v eurách pri úhrade faktúry v cudzej mene
	// (rozdiel oproti úhrade je kurzový zisk 663 alebo strata 563).
	fx map[string]money.Dec
	// customs je DPH pri dovoze, ktorú vyrubí colný úrad (D 379).
	customs map[string]money.Dec
	// deduct sú odpočítané zálohy vyúčtovacej faktúry.
	deduct map[string][]invoice.Deduction
}

func (p *poster) note(m *map[string]string, key, v string) {
	if *m == nil {
		*m = map[string]string{}
	}
	(*m)[key] = v
}

func (p *poster) set(key string, a chart.Amounts) {
	if p.amounts == nil {
		p.amounts = map[string]chart.Amounts{}
	}
	p.amounts[key] = a
}

func (p *poster) setFX(key string, v money.Dec) {
	if p.fx == nil {
		p.fx = map[string]money.Dec{}
	}
	p.fx[key] = v
}

func (p *poster) dir(key string, d int) {
	if p.moneyDir == nil {
		p.moneyDir = map[string]int{}
	}
	p.moneyDir[key] = d
}

// post zaúčtuje doklad podľa post.Template (navrhnutej alebo zvolenej).
func (p *poster) post(post *ledger.Posting) {
	if post.Template == "" {
		if post.Problem == "" {
			post.Problem = "Vyberte predkontáciu"
		}
		return
	}
	a, ok := p.amounts[post.Key]
	if !ok {
		return // problém nastavil zdroj (chýba kurz a pod.)
	}
	post.Problem = ""
	p.apply(post, a)
}

func (p *poster) apply(post *ledger.Posting, a chart.Amounts) {
	t, ok := p.tpl[post.Template]
	if !ok {
		post.Problem = "Predkontácia " + post.Template + " neexistuje alebo sa nepoužíva — vyberte inú"
		return
	}
	if t.Doc != post.Doc {
		post.Problem = "Predkontácia " + t.Code + " je pre iný druh dokladu"
		return
	}
	if d, ok := p.moneyDir[post.Key]; ok {
		if side := ledger.MoneySide(t); side != 0 && side != d {
			a = chart.Amounts{Total: a.Total.Neg(), Base: a.Base.Neg(), VAT: a.VAT.Neg(), VATNon: a.VATNon.Neg(), Self: a.Self.Neg()}
		}
	}
	lines := ledger.FromEntries(t.Apply(a))
	if non, ok := p.reverse[post.Key]; ok && !non.IsZero() {
		// prenesenie s čiastočným odpočtom: neodpočítaná daň je náklad
		acc := ""
		for _, l := range t.Lines {
			if l.Side == chart.Debit && l.Part == chart.PartBase {
				acc = l.Account
				break
			}
		}
		if acc != "" {
			lines = append(lines, ledger.Line{Account: acc, Debit: non}, ledger.Line{Account: "343", Credit: non})
		}
	}
	if v, ok := p.customs[post.Key]; ok && !v.IsZero() {
		lines = append(lines, ledger.Line{Account: "379", Credit: v})
	}
	if deds := p.deduct[post.Key]; len(deds) > 0 {
		lines = append(lines, deductionLines(t, deds)...)
	}
	if docEUR, ok := p.fx[post.Key]; ok {
		lines = fxLines(lines, t, docEUR)
	}
	if acc := p.moneyMap[p.money[post.Key]]; acc != "" {
		for i := range lines {
			if ledger.IsMoney(lines[i].Account, t.Doc) && strings.HasPrefix(acc, lines[i].Account) {
				lines[i].Account = acc
			}
		}
	}
	post.Lines = lines
}

// deductionLines zúčtuje odpočítané zálohy vo vyúčtovacej faktúre: záloha
// bez DPH MD 324, DPH zo zálohy MD 343, spolu D účet odberateľa (311).
func deductionLines(t chart.Template, deds []invoice.Deduction) []ledger.Line {
	acc := "311"
	for _, l := range t.Lines {
		if l.Side == chart.Debit && l.Part == chart.PartTotal {
			acc = l.Account
			break
		}
	}
	var out []ledger.Line
	for _, d := range deds {
		var base, tax money.Dec
		for _, r := range d.Rates {
			base, tax = base.Add(r.Base), tax.Add(r.VAT)
		}
		if d.ReceiptID == 0 {
			base, tax = d.Total, money.Zero // bez dokladu k platbe (neplatiteľ): celá záloha
		}
		out = append(out, ledger.Line{Account: "324", Debit: base})
		if !tax.IsZero() {
			out = append(out, ledger.Line{Account: "343", Debit: tax})
		}
		out = append(out, ledger.Line{Account: acc, Credit: base.Add(tax)})
	}
	return out
}

// fxLines upraví úhradu faktúry v cudzej mene: účet odberateľa alebo
// dodávateľa sa vyrovná sumou faktúry v eurách, rozdiel je kurzový zisk
// (663) alebo strata (563).
func fxLines(lines []ledger.Line, t chart.Template, docEUR money.Dec) []ledger.Line {
	for i := range lines {
		l := &lines[i]
		if ledger.IsMoney(l.Account, t.Doc) {
			continue
		}
		if !l.Credit.IsZero() { // 311 D: prijatá úhrada
			delta := docEUR.Sub(l.Credit)
			l.Credit = docEUR
			if delta.Sign() > 0 {
				return append(lines, ledger.Line{Account: "563", Debit: delta})
			} else if delta.Sign() < 0 {
				return append(lines, ledger.Line{Account: "663", Credit: delta.Neg()})
			}
		} else { // 321 MD: úhrada dodávateľovi
			delta := docEUR.Sub(l.Debit)
			l.Debit = docEUR
			if delta.Sign() > 0 {
				return append(lines, ledger.Line{Account: "663", Credit: delta})
			} else if delta.Sign() < 0 {
				return append(lines, ledger.Line{Account: "563", Debit: delta.Neg()})
			}
		}
		return lines
	}
	return lines
}

// moneySources vráti bankové účty (firmy a z výpisov) a pokladnice.
func (a *App) moneySources(ctx context.Context, companyID int64, iban string, m map[string]string) []MoneySource {
	out := []MoneySource{}
	seen := map[string]bool{}
	addBank := func(i string) {
		i = normIBAN(i)
		if i == "" || seen[i] {
			return
		}
		seen[i] = true
		out = append(out, MoneySource{Key: "bank:" + i, Label: "Banka " + i, Account: m["bank:"+i]})
	}
	addBank(iban)
	if txs, err := a.Store.ListBankTransactions(ctx, companyID, store.BankFilter{}); err == nil {
		for _, t := range txs {
			addBank(t.Account)
		}
	}
	if regs, err := a.cashRegisters(ctx, companyID); err == nil {
		for _, r := range regs {
			k := "cash:" + strconv.FormatInt(r.ID, 10)
			label := r.Name
			if !strings.Contains(strings.ToLower(r.Name), "poklad") {
				label = "Pokladnica " + r.Name
			}
			if r.Currency != "" && r.Currency != "EUR" {
				label += " (" + r.Currency + ")"
			}
			out = append(out, MoneySource{Key: k, Label: label, Account: m[k]})
		}
	}
	return out
}

var (
	reFee      = regexp.MustCompile(`(?i)poplat|fee|vedenie [úu]čtu`)
	reInterest = regexp.MustCompile(`(?i)[úu]rok|interest`)
	// presun peňazí medzi bankou a pokladnicou (261 peniaze na ceste)
	reToCash   = regexp.MustCompile(`(?i)do pokladn|v[ýy]ber hotovost|v[ýy]ber z bankomatu|atm`)
	reFromBank = regexp.MustCompile(`(?i)z banky|z (bankov[ée]ho )?[úu][čc]tu|v[ýy]ber z`)
	rePHM      = regexp.MustCompile(`(?i)pohonn|\bphm\b|benz[íi]n|naft|tankov`)
)

// ledgerSources načíta doklady roka a navrhne predkontácie.
func (a *App) ledgerSources(ctx context.Context, companyID int64, year int, p *poster) ([]ledger.Posting, error) {
	var out []ledger.Posting
	invs := map[int64]*invoice.Invoice{}
	// vydané faktúry: zápis k dátumu dodania (dobropis k dátumu vystavenia);
	// faktúry vystavené v nasledujúcom roku môžu patriť do tohto roka
	for _, y := range []int{year, year + 1} {
		list, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{Year: y})
		if err != nil {
			return nil, err
		}
		for i := range list {
			inv := &list[i]
			invs[inv.ID] = inv
			if inv.Status != invoice.StatusIssued && inv.Status != invoice.StatusPaid {
				continue
			}
			if inv.Kind == invoice.KindProforma {
				continue // zálohová faktúra nie je účtovný doklad
			}
			when := inv.DeliveryDate
			if inv.Kind == invoice.KindCreditNote || when.IsZero() {
				when = inv.IssueDate
			}
			if when.Year() != year {
				continue
			}
			key := "sale:" + strconv.FormatInt(inv.ID, 10)
			post := ledger.Posting{Key: key, Date: when, Doc: chart.DocSale, Number: inv.Number, Partner: inv.Customer.Name,
				Link: "/faktury/" + strconv.FormatInt(inv.ID, 10), Text: "Vydaná faktúra"}
			if inv.Kind == invoice.KindCreditNote {
				post.Text = "Dobropis"
			}
			sum, ok := inv.TaxSummary()
			switch {
			case !ok:
				post.Problem = "Faktúra v mene " + inv.Currency + " nemá kurz"
			case inv.Kind == invoice.KindAdvanceReceipt:
				// daňový doklad k prijatej platbe: DPH zo zálohy 324 / 343
				post.Amount, post.Text = sum.Total, "Daňový doklad k prijatej platbe"
				p.set(key, chart.Amounts{Total: sum.Total, Base: sum.Base, VAT: sum.VAT})
				post.Template = "FV-ZALOHA"
			case inv.Advance != nil && len(inv.Advance.Deductions) > 0:
				post.Amount, post.Text = sum.Total, "Vyúčtovacia faktúra"
				if inv.Foreign() || inv.Advance.Supply == nil {
					post.Problem = "Vyúčtovanie zálohy v cudzej mene — zaúčtujte ručne"
					break
				}
				full := *inv.Advance.Supply
				p.set(key, chart.Amounts{Total: full.Total, Base: full.Base, VAT: full.VAT})
				if p.deduct == nil {
					p.deduct = map[string][]invoice.Deduction{}
				}
				p.deduct[key] = inv.Advance.Deductions
				post.Template = "FV-SLUZBY"
				if inv.SupplyKind.Goods() {
					post.Template = "FV-TOVAR"
				}
			default:
				post.Amount = sum.Total
				p.set(key, chart.Amounts{Total: sum.Total, Base: sum.Base, VAT: sum.VAT})
				post.Template = "FV-SLUZBY"
				if inv.SupplyKind.Goods() {
					post.Template = "FV-TOVAR"
				}
			}
			out = append(out, post)
		}
	}
	purs := map[int64]*purchase.Purchase{}
	for _, y := range []int{year, year + 1} {
		list, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{Year: y})
		if err != nil {
			return nil, err
		}
		for i := range list {
			pu := &list[i]
			purs[pu.ID] = pu
			when := pu.DeliveryDate
			if when.IsZero() {
				when = pu.IssueDate
			}
			if when.IsZero() {
				when = pu.ReceivedDate
			}
			if when.Year() != year {
				continue
			}
			key := "purchase:" + strconv.FormatInt(pu.ID, 10)
			post := ledger.Posting{Key: key, Date: when, Doc: chart.DocPurchase, Number: pu.Number, Partner: pu.Supplier.Name,
				Link: "/prijate/" + strconv.FormatInt(pu.ID, 10), Text: strings.TrimSpace(pu.Description)}
			if post.Text == "" {
				post.Text = "Prijatá faktúra " + pu.SupplierNumber
			}
			t, ok := pu.Tax()
			if !ok {
				post.Problem = "Faktúra v mene " + pu.Currency + " nemá kurz"
				out = append(out, post)
				continue
			}
			s := t.Summary
			post.Amount = s.Total
			deduct := money.Zero
			if p.vatPayer {
				deduct = pu.DeductOf(s.VAT)
			}
			non := s.VAT.Sub(deduct)
			switch {
			case pu.Import() && !s.VAT.IsZero():
				// dovoz: dodávateľovi sa platí základ, DPH vyrubí colný úrad
				// (MD 343 odpočítateľná časť, neodpočítaná do nákladu / D 379)
				p.set(key, chart.Amounts{Total: s.Total, Base: s.Base, VAT: deduct, VATNon: non})
				if p.customs == nil {
					p.customs = map[string]money.Dec{}
				}
				p.customs[key] = s.VAT
			case pu.ReverseCharge || !s.Total.Equal(s.Base.Add(s.VAT)):
				p.set(key, chart.Amounts{Total: s.Total, Base: s.Base, Self: deduct})
				if p.reverse == nil {
					p.reverse = map[string]money.Dec{}
				}
				p.reverse[key] = non
			default:
				p.set(key, chart.Amounts{Total: s.Total, Base: s.Base, VAT: deduct, VATNon: non})
			}
			if post.Problem == "" {
				post.Template = purchaseTemplate(pu)
			}
			out = append(out, post)
		}
	}
	txs, err := a.Store.ListBankTransactions(ctx, companyID, store.BankFilter{Year: year})
	if err != nil {
		return nil, err
	}
	// doklady prepočítané kurzom k 31. 12. minulého roka
	var revalued map[string]money.Dec
	if prev, err := a.loadLedger(ctx, companyID, year-1); err == nil {
		revalued = prev.Revalued
	}
	// počet úhrad dokladu: kurzový rozdiel iba pri úhrade jednou platbou
	pays := map[string]int{}
	for _, t := range txs {
		if t.Status == bank.StatusMatched && t.Match != nil {
			pays[t.Match.Kind+":"+strconv.FormatInt(t.Match.ID, 10)]++
		}
	}
	getInv := func(id int64) *invoice.Invoice {
		if inv := invs[id]; inv != nil {
			return inv
		}
		if got, err := a.Store.GetInvoice(ctx, companyID, id); err == nil {
			invs[id] = &got
			return &got
		}
		return nil
	}
	getPur := func(id int64) *purchase.Purchase {
		if pu := purs[id]; pu != nil {
			return pu
		}
		if got, err := a.Store.GetPurchase(ctx, companyID, id); err == nil {
			purs[id] = &got
			return &got
		}
		return nil
	}
	for _, t := range txs {
		key := "bank:" + strconv.FormatInt(t.ID, 10)
		p.note(&p.money, key, "bank:"+normIBAN(t.Account))
		signed := t.Signed()
		post := ledger.Posting{Key: key, Date: t.Date, Doc: chart.DocBank, Number: t.BankRef, Partner: t.CounterName,
			Text: strings.TrimSpace(t.Message), Amount: signed, Link: "/banka"}
		if t.Currency != "" && t.Currency != "EUR" {
			post.Problem = "Pohyb v mene " + t.Currency + " — zaúčtujte ručne v eurách"
			out = append(out, post)
			continue
		}
		p.set(key, chart.Amounts{Total: abs(signed), Base: abs(signed)})
		p.dir(key, signed.Sign())
		if t.Status == bank.StatusMatched && t.Match != nil {
			post.Number = t.Match.Number
			single := pays[t.Match.Kind+":"+strconv.FormatInt(t.Match.ID, 10)] == 1
			switch t.Match.Kind {
			case "invoice":
				post.Template = "B-ODBERATEL"
				inv := getInv(t.Match.ID)
				switch {
				case inv == nil:
				case inv.Kind == invoice.KindProforma:
					post.Template = "B-ZALOHA"
				case inv.Foreign() && single && inv.Status == invoice.StatusPaid:
					if v, ok := revalued["sale:"+strconv.FormatInt(inv.ID, 10)]; ok {
						p.setFX(key, v) // prepočítané k 31. 12. minulého roka
					} else if s, ok := inv.TaxSummary(); ok {
						p.setFX(key, s.Total)
					}
				}
			case "purchase":
				post.Template = "B-DODAVATEL"
				if pu := getPur(t.Match.ID); pu != nil && pu.Foreign() && single && pu.Status == purchase.StatusPaid {
					if v, ok := revalued["purchase:"+strconv.FormatInt(pu.ID, 10)]; ok {
						p.setFX(key, v)
					} else if tx, ok := pu.Tax(); ok {
						p.setFX(key, tx.Summary.Total)
					}
				}
			}
		} else {
			text := t.Message + " " + t.CounterName + " " + t.Note
			switch {
			case reFee.MatchString(text) && signed.Sign() < 0:
				post.Template = "B-POPLATKY"
			case reInterest.MatchString(text) && signed.Sign() > 0:
				post.Template = "B-UROKY"
			case reToCash.MatchString(text) && signed.Sign() < 0:
				post.Template = "B-VKLAD"
			}
		}
		out = append(out, post)
	}
	docs, err := a.Store.ListCashDocuments(ctx, companyID, year)
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		if d.Cancelled {
			continue
		}
		key := "cash:" + strconv.FormatInt(d.ID, 10)
		p.note(&p.money, key, "cash:"+strconv.FormatInt(d.RegisterID, 10))
		post := ledger.Posting{Key: key, Date: d.Date, Doc: chart.DocCash, Number: d.Number, Partner: d.Partner,
			Text: d.Purpose, Amount: d.SignedEUR(), Link: "/pokladna"}
		dir := 1
		if d.Kind != cash.In {
			dir = -1
		}
		p.dir(key, dir)
		switch {
		case d.LinkKind == "invoice":
			p.set(key, chart.Amounts{Total: abs(d.SignedEUR()), Base: abs(d.SignedEUR())})
			post.Template = "P-ODBERATEL"
		case d.LinkKind == "purchase":
			p.set(key, chart.Amounts{Total: abs(d.SignedEUR()), Base: abs(d.SignedEUR())})
			post.Template = "P-DODAVATEL"
		case d.Tax != nil && len(d.Tax.Rows) > 0:
			if d.Foreign() {
				post.Problem = "Doklad v mene " + d.Currency + " s DPH — zaúčtujte ručne v eurách"
				break
			}
			base, tax, gross := d.Tax.Totals()
			if dir > 0 {
				p.set(key, chart.Amounts{Total: gross, Base: base, VAT: tax})
				post.Template = "P-TRZBA"
			} else {
				deduct := money.Zero
				if p.vatPayer {
					deduct = d.Tax.DeductOf(tax)
				}
				p.set(key, chart.Amounts{Total: gross, Base: base, VAT: deduct, VATNon: tax.Sub(deduct)})
				post.Template = "P-NAKUP"
				if d.Tax.VehicleID != 0 {
					post.Template = "P-PHM"
				}
			}
		default:
			p.set(key, chart.Amounts{Total: abs(d.SignedEUR()), Base: abs(d.SignedEUR())})
			switch {
			case dir > 0 && reFromBank.MatchString(d.Purpose):
				post.Template = "P-VKLAD"
			case dir < 0 && rePHM.MatchString(d.Purpose):
				post.Template = "P-PHM" // napr. tankovanie v zahraničí (bez DPH na odpočet)
			}
		}
		out = append(out, post)
	}
	return out, nil
}

func abs(d money.Dec) money.Dec {
	if d.Sign() < 0 {
		return d.Neg()
	}
	return d
}

// purchaseTemplate navrhne predkontáciu prijatej faktúry podľa druhu výdavku.
func purchaseTemplate(p *purchase.Purchase) string {
	switch p.Expense {
	case purchase.ExpenseStock:
		return "FP-MATERIAL"
	case purchase.ExpenseAsset:
		return "FP-MAJETOK"
	}
	return "FP-SLUZBY"
}

// postingDoc vráti druh dokladu podľa kľúča.
func postingDoc(key string) (chart.DocKind, bool) {
	kind, _, _ := strings.Cut(key, ":")
	switch kind {
	case "sale":
		return chart.DocSale, true
	case "purchase":
		return chart.DocPurchase, true
	case "bank":
		return chart.DocBank, true
	case "cash":
		return chart.DocCash, true
	}
	return "", false
}

// SetPosting zmení zaúčtovanie dokladu: zvolí predkontáciu (template),
// zadá riadky ručne (lines), alebo vráti návrh Fiškusa (oboje prázdne).
func (a *App) SetPosting(ctx context.Context, companyID int64, year int, key, template string, lines []ledger.Line, note string, dims ledger.Dims) error {
	doc, ok := postingDoc(key)
	if !ok {
		return store.ErrNotFound
	}
	if err := a.checkDims(ctx, companyID, &dims); err != nil {
		return err
	}
	return a.editLedger(ctx, companyID, year, func(d *LedgerData, c chart.Chart, tpls []chart.Template) (string, error) {
		delete(d.Assign, key)
		delete(d.Manual, key)
		delete(d.Dims, key)
		if !dims.Empty() {
			if d.Dims == nil {
				d.Dims = map[string]ledger.Dims{}
			}
			d.Dims[key] = dims
		}
		switch {
		case len(lines) > 0:
			if err := ledger.CheckLines(lines, c); err != nil {
				return "", mfaFieldErr("lines", err.Error())
			}
			if d.Manual == nil {
				d.Manual = map[string]ManualPosting{}
			}
			d.Manual[key] = ManualPosting{Lines: lines, Note: strings.TrimSpace(note)}
			return "Ručné zaúčtovanie dokladu " + key, nil
		case template != "":
			if err := checkTemplate(tpls, template, doc); err != nil {
				return "", err
			}
			if d.Assign == nil {
				d.Assign = map[string]string{}
			}
			d.Assign[key] = template
			return "Predkontácia " + template + " pre doklad " + key, nil
		}
		return "Návrh zaúčtovania pre doklad " + key, nil
	})
}

// checkDims overí stredisko a zákazku voči číselníkom firmy.
func (a *App) checkDims(ctx context.Context, companyID int64, d *ledger.Dims) error {
	d.Center, d.Job = strings.TrimSpace(d.Center), strings.TrimSpace(d.Job)
	if d.Empty() {
		return nil
	}
	cb, err := a.loadChart(ctx, companyID)
	if err != nil {
		return err
	}
	if d.Center != "" {
		ok := false
		for _, c := range cb.Centers {
			ok = ok || (c.Code == d.Center && c.Active)
		}
		if !ok {
			return mfaFieldErr("center", "Stredisko "+d.Center+" neexistuje alebo nie je aktívne")
		}
	}
	if d.Job != "" {
		ok := false
		for _, j := range cb.Jobs {
			ok = ok || (j.Code == d.Job && j.Active)
		}
		if !ok {
			return mfaFieldErr("job", "Zákazka "+d.Job+" neexistuje alebo nie je aktívna")
		}
	}
	return nil
}

func checkTemplate(tpls []chart.Template, code string, doc chart.DocKind) error {
	for _, t := range tpls {
		if t.Code == code {
			if !t.Active {
				return mfaFieldErr("template", "Predkontácia "+code+" sa nepoužíva")
			}
			if t.Doc != doc {
				return mfaFieldErr("template", "Predkontácia "+code+" je pre iný druh dokladu")
			}
			return nil
		}
	}
	return mfaFieldErr("template", "Predkontácia "+code+" neexistuje")
}

// SaveInternal pridá (doc.ID = 0) alebo upraví interný doklad.
func (a *App) SaveInternal(ctx context.Context, companyID int64, year int, doc InternalDoc) (InternalDoc, error) {
	if err := a.checkDims(ctx, companyID, &doc.Dims); err != nil {
		return doc, err
	}
	err := a.editLedger(ctx, companyID, year, func(d *LedgerData, c chart.Chart, tpls []chart.Template) (string, error) {
		doc.Text = strings.TrimSpace(doc.Text)
		switch {
		case doc.Date.IsZero() || doc.Date.Year() != year:
			return "", mfaFieldErr("date", fmt.Sprintf("Zadajte dátum v roku %d", year))
		case doc.Text == "":
			return "", mfaFieldErr("text", "Zadajte popis (napr. Odpisy za rok)")
		}
		if doc.Template != "" {
			if err := checkTemplate(tpls, doc.Template, chart.DocInternal); err != nil {
				return "", err
			}
			if doc.Amount.Sign() <= 0 || !doc.Amount.Equal(doc.Amount.RoundCents()) {
				return "", mfaFieldErr("amount", "Zadajte kladnú sumu v centoch")
			}
			doc.Lines = nil
		} else {
			if err := ledger.CheckLines(doc.Lines, c); err != nil {
				return "", mfaFieldErr("lines", err.Error())
			}
			doc.Amount = money.Zero
		}
		if doc.ID == 0 {
			d.NextID++
			doc.ID = d.NextID
			if doc.Number == "" {
				doc.Number = fmt.Sprintf("ID%d%03d", year, doc.ID)
			}
			d.Internal = append(d.Internal, doc)
			return "Interný doklad " + doc.Number + ": " + doc.Text, nil
		}
		for i := range d.Internal {
			if d.Internal[i].ID == doc.ID {
				if strings.TrimSpace(doc.Number) == "" {
					doc.Number = d.Internal[i].Number
				}
				doc.FX = d.Internal[i].FX
				d.Internal[i] = doc
				return "Upravený interný doklad " + doc.Number + ": " + doc.Text, nil
			}
		}
		return "", store.ErrNotFound
	})
	return doc, err
}

// DeleteInternal zmaže interný doklad.
func (a *App) DeleteInternal(ctx context.Context, companyID int64, year int, id int64) error {
	return a.editLedger(ctx, companyID, year, func(d *LedgerData, _ chart.Chart, _ []chart.Template) (string, error) {
		for i, doc := range d.Internal {
			if doc.ID == id {
				d.Internal = append(d.Internal[:i], d.Internal[i+1:]...)
				if doc.FX {
					d.Revalued = nil
				}
				return "Zmazaný interný doklad " + doc.Number, nil
			}
		}
		return "", store.ErrNotFound
	})
}

// SetLedgerOpening uloží počiatočné stavy účtov k 1. 1. roka (riadky MD/D).
// Strany sa musia rovnať (súvaha je vyrovnaná).
func (a *App) SetLedgerOpening(ctx context.Context, companyID int64, year int, lines []ledger.Line) error {
	return a.editLedger(ctx, companyID, year, func(d *LedgerData, c chart.Chart, _ []chart.Template) (string, error) {
		out := map[string]money.Dec{}
		if len(lines) > 0 {
			if err := ledger.CheckLines(lines, c); err != nil {
				return "", mfaFieldErr("lines", err.Error())
			}
		}
		for _, l := range lines {
			out[l.Account] = out[l.Account].Add(l.Debit).Sub(l.Credit)
		}
		for k, v := range out {
			if v.IsZero() {
				delete(out, k)
			}
		}
		d.Opening = out
		return fmt.Sprintf("Počiatočné stavy účtov k 1. 1. %d (%d účtov)", year, len(out)), nil
	})
}

// LedgerStatements je účtovná závierka mikro účtovnej jednotky (súvaha
// a výkaz ziskov a strát) z hlavnej knihy.
type LedgerStatements struct {
	Year int `json:"year"`
	ledger.Statements
	// PreviousEmpty: predchádzajúce obdobie nemá v programe zápisy.
	PreviousEmpty bool `json:"previousEmpty"`
	// Unposted a Balanced sú z hlavnej knihy bežného roka (závierka je
	// spoľahlivá iba pri úplne zaúčtovanom a vyrovnanom denníku).
	Unposted int  `json:"unposted"`
	Balanced bool `json:"balanced"`
}

// LedgerStatementsFor zostaví súvahu a výkaz ziskov a strát za rok.
func (a *App) LedgerStatementsFor(ctx context.Context, companyID int64, year int) (LedgerStatements, error) {
	cur, err := a.Ledger(ctx, companyID, year)
	if err != nil {
		return LedgerStatements{}, err
	}
	prev, err := a.Ledger(ctx, companyID, cur.Year-1)
	if err != nil {
		return LedgerStatements{}, err
	}
	return LedgerStatements{Year: cur.Year, Statements: ledger.BuildStatements(cur.Accounts, prev.Accounts),
		PreviousEmpty: len(prev.Accounts) == 0, Unposted: cur.Unposted, Balanced: cur.Totals.Balanced}, nil
}

// OpeningProposal navrhne počiatočné stavy roka z konečných stavov
// predchádzajúceho roka (výsledok hospodárenia na účet 431).
func (a *App) OpeningProposal(ctx context.Context, companyID int64, year int) ([]ledger.Line, error) {
	if year < 2001 || year > 2100 {
		return nil, mfaFieldErr("year", "Neplatný rok")
	}
	prev, err := a.Ledger(ctx, companyID, year-1)
	if err != nil {
		return nil, err
	}
	return ledger.OpeningFromClosing(prev.Accounts), nil
}
