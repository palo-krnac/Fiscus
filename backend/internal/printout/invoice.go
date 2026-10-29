package printout

import (
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/pdf"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// view sú údaje spoločné pre faktúru a cenovú ponuku.
type view struct {
	inv        invoice.Invoice
	quote      bool
	validUntil invoice.Date
	stamp      string
}

// Invoice vytvorí PDF faktúry (aj dobropisu, zálohovej faktúry a dokladu k platbe).
func Invoice(inv invoice.Invoice, sup Supplier) ([]byte, error) {
	return bytesOf(invoiceSheet(inv, sup))
}

func invoiceSheet(inv invoice.Invoice, sup Supplier) (*sheet, error) {
	v := view{inv: inv}
	switch inv.Status {
	case invoice.StatusDraft:
		v.stamp = "Koncept"
	case invoice.StatusCancelled:
		v.stamp = "Stornovaná"
	}
	return render(v, sup)
}

// Quote vytvorí PDF cenovej ponuky.
func Quote(q quote.Quote, sup Supplier) ([]byte, error) { return bytesOf(quoteSheet(q, sup)) }

func quoteSheet(q quote.Quote, sup Supplier) (*sheet, error) {
	v := view{quote: true, validUntil: q.ValidUntil, inv: invoice.Invoice{
		Kind: invoice.KindInvoice, Status: invoice.StatusIssued, Number: q.Number, Customer: q.Customer,
		IssueDate: q.IssueDate, Currency: q.Currency, PriceMode: q.PriceMode, ReverseCharge: q.ReverseCharge,
		SupplyKind: q.SupplyKind, NonVATPayer: q.NonVATPayer, Note: q.Note, Lines: q.Lines, Summary: q.Summary,
	}}
	if q.Status == quote.StatusDraft {
		v.stamp = "Koncept"
		v.inv.Status = invoice.StatusDraft
	}
	return render(v, sup)
}

// FileName vráti názov súboru PDF dokladu, napr. „Faktura-2026001.pdf“.
func FileName(kind, number string) string {
	if number == "" {
		number = "koncept"
	}
	safe := make([]rune, 0, len(number))
	for _, r := range number {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '-' || r == '_' {
			safe = append(safe, r)
		} else {
			safe = append(safe, '-')
		}
	}
	return kind + "-" + string(safe) + ".pdf"
}

func (v view) title() string {
	if v.quote {
		return "Cenová ponuka"
	}
	return v.inv.Title()
}

func render(v view, sup Supplier) (*sheet, error) {
	inv := v.inv
	number := inv.Number
	if inv.Status == invoice.StatusDraft || number == "" {
		number = "Koncept"
	}
	label := v.title() + " " + number
	if v.quote {
		label = "Cenová ponuka " + number
	} else if inv.Kind == invoice.KindInvoice {
		label = "Faktúra " + number
	}
	s, err := newSheet(label, label)
	if err != nil {
		return nil, err
	}
	if v.stamp != "" {
		s.stamp(v.stamp)
	}
	receipt := inv.Kind == invoice.KindAdvanceReceipt
	var deductions []invoice.Deduction
	if inv.Advance != nil {
		deductions = inv.Advance.Deductions
	}

	s.head(v, number, sup.Logo)
	if c := inv.Correction; c != nil {
		s.panel([]field{
			{"Opravuje faktúru", c.OriginalNumber},
			{"Zo dňa", c.OriginalIssueDate.Display()},
			{"Dátum dodania pôvodného plnenia", c.OriginalDeliveryDate.Display()},
			{"Dôvod opravy", c.Reason},
		})
	}
	s.parties(v, sup)
	if a := inv.Advance; a != nil && (receipt || len(deductions) > 0) {
		fs := []field{{"Vyúčtovanie zálohovej faktúry", a.ProformaNumber}}
		if receipt {
			fs = []field{{"K zálohovej faktúre", a.ProformaNumber}, {"Daňová povinnosť vznikla dňom", inv.DeliveryDate.Display()}}
		}
		s.panel(fs)
		s.y += pdf.MM(4)
	}
	if !v.quote && !receipt {
		s.payment(inv, sup)
	}
	s.lines(inv)
	if len(deductions) > 0 {
		s.y += pdf.MM(4)
		s.deductions(inv, deductions)
	}
	s.totals(v, len(deductions) > 0, sup)
	s.notes(v)
	return s.finish(), nil
}

// head: logo, druh a číslo dokladu, vpravo dátumy; pod tým hrubá čiara.
func (s *sheet) head(v view, number string, logo []byte) {
	inv := v.inv
	var dates []field
	switch {
	case v.quote:
		dates = []field{{"Dátum ponuky", inv.IssueDate.Display()}, {"Platná do", v.validUntil.Display()}}
	case inv.Kind == invoice.KindAdvanceReceipt:
		dates = []field{{"Dátum vystavenia", inv.IssueDate.Display()}, {"Dátum prijatia platby", inv.DeliveryDate.Display()}}
	default:
		dates = []field{{"Dátum vystavenia", inv.IssueDate.Display()}, {"Dátum dodania", inv.DeliveryDate.Display()}, {"Dátum splatnosti", inv.DueDate.Display()}}
	}
	y := s.y
	if h := s.logo(logo, left, y, pdf.MM(70), pdf.MM(16)); h > 0 {
		y += h + pdf.MM(4)
	}
	s.text(left, y, s.f.Regular, 10, muted, v.title())
	y += lh(10) + pdf.MM(1)
	s.text(left, y, s.f.Bold, 24, ink, number)
	y += 24 * 1.15
	// dátumy zarovnané k spodku hlavičky, sprava doľava
	dy := y - lh(8) - lh(11)
	x := right
	for i := len(dates) - 1; i >= 0; i-- {
		d := dates[i]
		c := ink
		if i == len(dates)-1 && inv.Kind != invoice.KindAdvanceReceipt { // splatnosť / platnosť ponuky
			c = navy
		}
		s.textRight(x, dy, s.f.Regular, 8, muted, d.label)
		s.textRight(x, dy+lh(8), s.f.Semibold, 11, c, d.value)
		x -= s.fieldWidth(d, s.f.Semibold, 11) + pdf.MM(7)
	}
	s.y = y + pdf.MM(5)
	s.p.Line(left, s.y, right, s.y, 2.5, navy)
	s.y += pdf.MM(1)
}

// panel je sivý pruh s modrou čiarou vľavo (oprava, záloha). Posledné pole
// zaberie zvyšok šírky a zalamuje sa.
func (s *sheet) panel(fs []field) {
	pad := pdf.MM(3)
	x := left + pad
	widths := make([]float64, len(fs))
	for i, f := range fs {
		widths[i] = s.fieldWidth(f, s.f.Bold, 9.5)
	}
	last := right - pad - x
	for _, w := range widths[:len(widths)-1] {
		last -= w + pdf.MM(8)
	}
	lastLines := s.wrap(s.f.Bold, 9.5, max(last, pdf.MM(30)), fs[len(fs)-1].value)
	h := lh(8) + float64(len(lastLines))*lh(9.5) + pdf.MM(5)
	y := s.y + pdf.MM(5)
	s.p.Rect(left, y, width, h, panel)
	s.p.Rect(left, y, 2, h, navy)
	for i, f := range fs {
		s.text(x, y+pdf.MM(2.5), s.f.Regular, 8, muted, f.label)
		vals := []string{f.value}
		if i == len(fs)-1 {
			vals = lastLines
		}
		for j, l := range vals {
			s.text(x, y+pdf.MM(2.5)+lh(8)+float64(j)*lh(9.5), s.f.Bold, 9.5, ink, l)
		}
		x += widths[i] + pdf.MM(8)
	}
	s.y = y + h
}

// parties: dodávateľ a odberateľ v dvoch stĺpcoch.
func (s *sheet) parties(v view, sup Supplier) {
	co := sup.Company
	colW := (width - pdf.MM(10)) / 2
	type block struct {
		title   string
		name    string
		lines   []string
		ids     []field
		contact string
	}
	ids := func(ico, dic, icdph string) []field {
		var out []field
		for _, f := range []field{{"IČO", ico}, {"DIČ", dic}, {"IČ DPH", icdph}} {
			if f.value != "" {
				out = append(out, f)
			}
		}
		return out
	}
	c := v.inv.Customer
	custLines := addressLines(c.Street, c.Zip, c.City)
	if party.Foreign(c.Country) {
		custLines = append(custLines, party.CountryName(c.Country))
	}
	to := "Odberateľ"
	if v.quote {
		to = "Pre"
	}
	blocks := []block{
		{"Dodávateľ", co.Name, addressLines(co.Street, co.Zip, co.City), ids(co.ICO, co.DIC, co.ICDPH), joinNonEmpty(", ", co.Email, co.Phone)},
		{to, c.Name, custLines, ids(c.ICO, c.DIC, c.ICDPH), ""},
	}
	y0 := s.y + pdf.MM(7)
	end := y0
	for i, b := range blocks {
		x := left + float64(i)*(colW+pdf.MM(10))
		y := y0
		s.text(x, y, s.f.Semibold, 8, muted, b.title)
		y += lh(8) + pdf.MM(1.5)
		for _, l := range s.wrap(s.f.Bold, 11.5, colW, b.name) {
			s.text(x, y, s.f.Bold, 11.5, ink, l)
			y += lh(11.5)
		}
		y += pdf.MM(0.5)
		for _, l := range b.lines {
			s.text(x, y, s.f.Regular, 9.5, ink, l)
			y += lh(9.5)
		}
		if len(b.ids) > 0 {
			y += pdf.MM(2)
			var lw float64
			for _, f := range b.ids {
				lw = max(lw, s.f.Regular.Width(f.label, 9.5))
			}
			for _, f := range b.ids {
				s.text(x, y, s.f.Regular, 9.5, muted, f.label)
				s.text(x+lw+pdf.MM(4), y, s.f.Regular, 9.5, ink, f.value)
				y += lh(9.5)
			}
		}
		if b.contact != "" {
			y += pdf.MM(2)
			for _, l := range s.wrap(s.f.Regular, 9.5, colW, b.contact) {
				s.text(x, y, s.f.Regular, 9.5, muted, l)
				y += lh(9.5)
			}
		}
		end = max(end, y)
	}
	s.y = end + pdf.MM(6)
}

// payment: spôsob úhrady, IBAN, variabilný symbol medzi dvoma čiarami.
func (s *sheet) payment(inv invoice.Invoice, sup Supplier) {
	fs := []field{{"Spôsob úhrady", invoice.PaymentLabel(inv.PaymentMethod)}}
	if inv.PaymentMethod == "" || inv.PaymentMethod == invoice.PayTransfer {
		fs = append(fs, field{"IBAN", dash(sup.Company.IBAN)})
	} else {
		fs = append(fs, field{"", ""})
	}
	fs = append(fs, field{"Variabilný symbol", dash(inv.VariableSymbol)})
	w0 := s.fieldWidth(fs[0], s.f.Bold, 9.5) + pdf.MM(6)
	rest := width - w0
	ws := []float64{w0, rest * 1.6 / 2.6, rest / 2.6}
	h := lh(8) + lh(9.5) + pdf.MM(4)
	y := s.y
	s.p.Line(left, y, right, y, 0.75, rule2)
	x := left
	for i, f := range fs {
		tx := x
		if i > 0 {
			tx += pdf.MM(6)
			s.p.Line(x, y+pdf.MM(1), x, y+h-pdf.MM(1), 0.75, rule)
		}
		if f.label != "" {
			s.text(tx, y+pdf.MM(2), s.f.Regular, 8, muted, f.label)
			s.text(tx, y+pdf.MM(2)+lh(8), s.f.Bold, 9.5, ink, f.value)
		}
		x += ws[i]
	}
	s.y = y + h
	s.p.Line(left, s.y, right, s.y, 0.75, rule2)
	s.y += pdf.MM(6)
}

func dash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

// lines: tabuľka položiek; pri zalomení strany sa hlavička zopakuje.
func (s *sheet) lines(inv invoice.Invoice) {
	nonVat := inv.NonVATPayer
	price := "Cena za MJ"
	total := "Spolu"
	if !nonVat {
		if inv.PriceMode == vat.PriceGross {
			price, total = "Cena za MJ s DPH", "Spolu s DPH"
		} else {
			price, total = "Cena za MJ bez DPH", "Základ"
		}
	}
	cols := []column{{"Popis", 0, false}, {"Množstvo", pdf.MM(20), true}, {"MJ", pdf.MM(14), false}, {price, pdf.MM(30), true}}
	if !nonVat {
		cols = append(cols, column{"DPH", pdf.MM(14), true})
	}
	cols = append(cols, column{total, pdf.MM(26), true})
	var used float64
	for _, c := range cols {
		used += c.w
	}
	cols[0].w = width - used
	t := &table{s: s, cols: cols, x: left, size: 9.5}
	s.ensure(lh(8) + pdf.MM(3) + lh(9.5) + pdf.MM(3.6))
	t.head()
	s.header = t.head
	for _, l := range inv.Lines {
		cells := []cellText{
			{l.Description, s.f.Regular, ink},
			{quantity(l.Quantity), s.f.Regular, ink},
			{l.Unit, s.f.Regular, ink},
			{amount(l.UnitPrice), s.f.Regular, ink},
		}
		if !nonVat {
			r := fmt.Sprintf("%d %%", l.Rate)
			if inv.ReverseCharge {
				r = "—"
			} else if l.Reverse != "" {
				r = "PDP" // prenesenie daňovej povinnosti (vysvetlené v poznámke)
			}
			cells = append(cells, cellText{r, s.f.Regular, ink})
		}
		cells = append(cells, cellText{amount(l.Amount), s.f.Semibold, ink})
		t.row(cells, pdf.MM(1.8), true)
	}
	s.header = nil
}

// deductions: odpočet uhradených záloh na vyúčtovacej faktúre.
func (s *sheet) deductions(inv invoice.Invoice, ds []invoice.Deduction) {
	nonVat := inv.NonVATPayer
	cols := []column{{"Odpočet zálohy", 0, false}, {"Prijatá", pdf.MM(24), false}}
	if !nonVat {
		cols = append(cols, column{"Sadzba", pdf.MM(16), true}, column{"Základ", pdf.MM(24), true}, column{"DPH", pdf.MM(22), true})
	}
	cols = append(cols, column{"Spolu", pdf.MM(26), true})
	var used float64
	for _, c := range cols {
		used += c.w
	}
	cols[0].w = width - used
	t := &table{s: s, cols: cols, x: left, size: 9.5}
	s.ensure(lh(8) + pdf.MM(3) + lh(9.5) + pdf.MM(3.6))
	t.head()
	s.header = t.head
	neg := func(d money.Dec) string { return "−" + amount(d) }
	for _, d := range ds {
		for i, rs := range d.Rates {
			desc, date := "", ""
			if i == 0 {
				desc = "Uhradená záloha " + d.Number
				if d.ReceiptID != 0 {
					desc = "Daňový doklad k prijatej platbe " + d.Number
				}
				date = d.Date.Display()
			}
			cells := []cellText{{desc, s.f.Regular, ink}, {date, s.f.Regular, ink}}
			if !nonVat {
				cells = append(cells, cellText{fmt.Sprintf("%d %%", rs.Rate), s.f.Regular, ink},
					cellText{neg(rs.Base), s.f.Regular, ink}, cellText{neg(rs.VAT), s.f.Regular, ink})
			}
			cells = append(cells, cellText{neg(rs.Total), s.f.Regular, ink})
			t.row(cells, pdf.MM(1.8), true)
		}
	}
	s.header = nil
}

// totals: rekapitulácia DPH a suma na úhradu s dvojitým podčiarknutím.
func (s *sheet) totals(v view, deducted bool, sup Supplier) {
	inv := v.inv
	w := pdf.MM(92)
	x := right - w
	rows := 0
	if !inv.NonVATPayer {
		rows = len(inv.Summary.Rates) + 2
	}
	h := float64(rows)*(lh(9.5)+pdf.MM(2.4)) + pdf.MM(4) + lh(18) + pdf.MM(4)
	if deducted && inv.Advance != nil && inv.Advance.Supply != nil {
		h += lh(9.5) + pdf.MM(3)
	}
	code := ""
	if !v.quote {
		code = payCode(inv, sup.Company)
	}
	qrSize := pdf.MM(30)
	if code != "" {
		h = max(h, qrSize)
	}
	s.y += pdf.MM(7)
	s.ensure(h)
	top := s.y
	if code != "" && s.qr(code, left, top, qrSize) {
		tx := left + qrSize + pdf.MM(4)
		s.text(tx, top+qrSize/2-lh(8.5), s.f.Regular, 8.5, muted, "Zaplaťte mobilom")
		s.text(tx, top+qrSize/2, s.f.Bold, 8.5, ink, "PAY by square")
		defer func() { s.y = max(s.y, top+qrSize) }()
	}
	if !inv.NonVATPayer {
		first := "Sadzba DPH"
		if deducted {
			first = "Po odpočte zálohy"
		}
		t := &table{s: s, x: x, size: 9.5, cols: []column{
			{first, w - pdf.MM(72), false}, {"Základ", pdf.MM(26), true}, {"DPH", pdf.MM(22), true}, {"Spolu", pdf.MM(24), true},
		}}
		t.head()
		for _, r := range inv.Summary.Rates {
			t.row([]cellText{{rateLabel(r), s.f.Regular, ink}, {amount(r.Base), s.f.Regular, ink},
				{amount(r.VAT), s.f.Regular, ink}, {amount(r.Total), s.f.Regular, ink}}, pdf.MM(1.2), true)
		}
		s.p.Line(x, s.y, right, s.y, 0.75, ink)
		sm := inv.Summary
		t.row([]cellText{{"Spolu", s.f.Semibold, ink}, {amount(sm.Base), s.f.Semibold, ink},
			{amount(sm.VAT), s.f.Semibold, ink}, {amount(sm.Total), s.f.Semibold, ink}}, pdf.MM(1.2), false)
	}
	if deducted && inv.Advance != nil && inv.Advance.Supply != nil {
		s.y += pdf.MM(3)
		s.text(x, s.y, s.f.Regular, 9.5, muted, "Dodávka spolu")
		s.textRight(right, s.y, s.f.Regular, 9.5, muted, moneyText(inv.Advance.Supply.Total, inv.Currency))
		s.y += lh(9.5)
	}
	label := "K úhrade"
	switch {
	case v.quote:
		label = "Cena spolu"
	case inv.Kind == invoice.KindCreditNote:
		label = "Suma dobropisu"
	case inv.Kind == invoice.KindAdvanceReceipt:
		label = "Prijatá platba"
	}
	s.y += pdf.MM(4)
	total := moneyText(inv.Summary.Total, inv.Currency)
	s.text(x, s.y+pdf.MM(2.5), s.f.Regular, 10, muted, label)
	s.p.TextRight(right, s.y+18*0.95, s.f.Bold, 18, ink, total)
	// dvojité podčiarknutie ako v účtovnej knihe
	tw := s.f.Bold.Width(total, 18)
	uy := s.y + 18*1.15
	s.p.Line(right-tw, uy, right, uy, 0.75, ink)
	s.p.Line(right-tw, uy+2.25, right, uy+2.25, 0.75, ink)
	s.y = uy + 3
	s.localTotals(inv, x, w)
}

// rateText opíše kurz faktúry v cudzej mene.
func rateText(inv invoice.Invoice) string {
	t := "Kurz: 1 EUR = " + quantity(inv.FXRate) + " " + inv.Currency
	if inv.FXManual || inv.FXDate.IsZero() {
		return t + " (dohodnutý)"
	}
	return t + " (ECB " + inv.FXDate.Display() + ")"
}

// localTotals: pri cudzej mene kurz a rekapitulácia DPH v eurách (§ 26).
func (s *sheet) localTotals(inv invoice.Invoice, x, w float64) {
	if !inv.Foreign() || inv.Local == nil {
		return
	}
	l := inv.Local
	s.y += pdf.MM(5)
	s.ensure(lh(8.5) + float64(len(l.Rates)+2)*(lh(9)+pdf.MM(2.4)))
	s.text(x, s.y, s.f.Regular, 8.5, muted, rateText(inv))
	s.y += lh(8.5) + pdf.MM(1)
	if inv.NonVATPayer {
		s.text(x, s.y, s.f.Regular, 9, ink, "Spolu v eurách")
		s.textRight(right, s.y, s.f.Semibold, 9, ink, moneyText(l.Total, ""))
		s.y += lh(9)
		return
	}
	t := &table{s: s, x: x, size: 9, cols: []column{
		{"V eurách", w - pdf.MM(72), false}, {"Základ", pdf.MM(26), true}, {"DPH", pdf.MM(22), true}, {"Spolu", pdf.MM(24), true},
	}}
	t.head()
	for _, r := range l.Rates {
		t.row([]cellText{{rateLabel(r), s.f.Regular, ink}, {amount(r.Base), s.f.Regular, ink},
			{amount(r.VAT), s.f.Regular, ink}, {amount(r.Total), s.f.Regular, ink}}, pdf.MM(1), true)
	}
	t.row([]cellText{{"Spolu €", s.f.Semibold, ink}, {amount(l.Base), s.f.Semibold, ink},
		{amount(l.VAT), s.f.Semibold, ink}, {amount(l.Total), s.f.Semibold, ink}}, pdf.MM(1), false)
}

// notes: povinné texty a poznámka používateľa.
func (s *sheet) notes(v view) {
	inv := v.inv
	type note struct {
		text string
		c    pdf.Color
	}
	var ns []note
	if inv.NonVATPayer {
		ns = append(ns, note{"Dodávateľ nie je platiteľ DPH.", muted})
	} else if t := invoice.ExemptionNote(inv.ReverseCharge, inv.Customer, inv.SupplyKind); t != "" {
		ns = append(ns, note{t, muted})
	}
	if t := invoice.DomesticReverseNote(inv.Lines); t != "" && !inv.NonVATPayer {
		ns = append(ns, note{t, muted})
	}
	if inv.Note != "" {
		ns = append(ns, note{inv.Note, ink})
	}
	if v.quote {
		ns = append(ns, note{"Cenová ponuka nie je daňový doklad.", muted})
	}
	s.y += pdf.MM(8)
	for _, n := range ns {
		for _, l := range s.wrap(s.f.Regular, 9.5, width, n.text) {
			s.ensure(lh(9.5))
			s.text(left, s.y, s.f.Regular, 9.5, n.c, l)
			s.y += lh(9.5)
		}
		s.y += pdf.MM(1.5)
	}
}

// rateLabel je sadzba v rekapitulácii; riadok s prenesením daňovej povinnosti má „PDP“.
func rateLabel(r vat.RateSummary) string {
	if r.Reverse {
		return "PDP"
	}
	return fmt.Sprintf("%d %%", r.Rate)
}
