package printout

import (
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/pdf"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
)

// Reminder vytvorí PDF upomienky (list s adresným okienkom obálky).
func Reminder(r reminder.Reminder, sup Supplier) ([]byte, error) {
	return bytesOf(reminderSheet(r, sup))
}

func reminderSheet(r reminder.Reminder, sup Supplier) (*sheet, error) {
	co := sup.Company
	title := reminder.Title(r.Level)
	s, err := newSheet(title+" "+r.Number, title+" "+r.Number)
	if err != nil {
		return nil, err
	}

	// hlavička: vľavo odosielateľ, vpravo adresát v okienku obálky
	toX := right - pdf.MM(78)
	fromW := toX - pdf.MM(10) - left
	s.p.Line(left, s.y, left+fromW, s.y, 2.5, navy)
	y := s.y + pdf.MM(2)
	if h := s.logo(sup.Logo, left, y, pdf.MM(60), pdf.MM(14)); h > 0 {
		y += h + pdf.MM(3)
	}
	s.text(left, y, s.f.Bold, 11, ink, co.Name)
	y += lh(11)
	ids := joinNonEmpty(" · ", prefixed("IČO ", co.ICO), prefixed("DIČ ", co.DIC), prefixed("IČ DPH ", co.ICDPH))
	for _, l := range []string{joinNonEmpty(", ", addressLines(co.Street, co.Zip, co.City)...), ids, joinNonEmpty(", ", co.Email, co.Phone)} {
		for _, w := range s.wrap(s.f.Regular, 8.5, fromW, l) {
			if w == "" {
				continue
			}
			s.text(left, y, s.f.Regular, 8.5, muted, w)
			y += lh(8.5)
		}
	}
	c := r.Customer
	ty := s.y + pdf.MM(14)
	for _, l := range s.wrap(s.f.Bold, 11, pdf.MM(78), c.Name) {
		s.text(toX, ty, s.f.Bold, 11, ink, l)
		ty += lh(11)
	}
	lines := addressLines(c.Street, c.Zip, c.City)
	if party.Foreign(c.Country) {
		lines = append(lines, party.CountryName(c.Country))
	}
	for _, l := range lines {
		s.text(toX, ty, s.f.Regular, 10, ink, l)
		ty += lh(10)
	}
	if c.ICO != "" {
		ty += pdf.MM(1.5)
		s.text(toX, ty, s.f.Regular, 9, muted, "IČO "+c.ICO)
		ty += lh(9)
	}
	s.y = max(y, ty, s.y+pdf.MM(44)) + pdf.MM(8)

	// číslo, dátum, miesto
	meta := []field{{"Číslo", r.Number}, {"Dátum", r.Date.Display()}}
	if co.City != "" {
		meta = append(meta, field{"Miesto", co.City})
	}
	x := left
	for _, m := range meta {
		s.text(x, s.y, s.f.Regular, 8, muted, m.label)
		s.text(x, s.y+lh(8), s.f.Bold, 9, ink, m.value)
		x += s.fieldWidth(m, s.f.Bold, 9) + pdf.MM(10)
	}
	s.y += lh(8) + lh(9) + pdf.MM(4)

	hc := navy
	if r.Level >= reminder.MaxLevel {
		hc = red
	}
	s.text(left, s.y, s.f.Bold, 18, hc, title)
	s.y += lh(18) + pdf.MM(1)
	for _, l := range s.wrap(s.f.Regular, 10, pdf.MM(160), r.Intro()) {
		s.text(left, s.y, s.f.Regular, 10, ink, l)
		s.y += lh(10)
	}
	s.y += pdf.MM(6)

	// faktúry
	hasInterest := r.InterestTotal.Sign() > 0
	hasFees := r.Fees.Sign() > 0
	cols := []column{{"Faktúra", pdf.MM(24), false}, {"Variabilný symbol", 0, false}, {"Vystavená", pdf.MM(22), false},
		{"Splatná", pdf.MM(22), false}, {"Dní", pdf.MM(12), true}, {"Suma", pdf.MM(24), true}}
	if hasInterest {
		cols = append(cols, column{"Úrok", pdf.MM(28), true})
	}
	if hasFees {
		cols = append(cols, column{"Náhrada", pdf.MM(18), true})
	}
	var used float64
	for _, c := range cols {
		used += c.w
	}
	cols[1].w = width - used
	t := &table{s: s, cols: cols, x: left, size: 9.5}
	t.head()
	s.header = t.head
	for _, it := range r.Items {
		cells := []cellText{{it.Number, s.f.Regular, ink}, {it.VS, s.f.Regular, ink}, {it.IssueDate.Display(), s.f.Regular, ink},
			{it.DueDate.Display(), s.f.Regular, ink}, {strconv.Itoa(it.Days), s.f.Regular, ink}, {amount(it.Amount), s.f.Regular, ink}}
		if hasInterest {
			v := amount(it.Interest)
			if it.Rate.Sign() > 0 {
				v += " (" + amount(it.Rate) + " %)"
			}
			cells = append(cells, cellText{v, s.f.Regular, ink})
		}
		if hasFees {
			cells = append(cells, cellText{amount(it.Fee), s.f.Regular, ink})
		}
		t.row(cells, pdf.MM(1.6), true)
	}
	s.header = nil

	// súčty
	w := pdf.MM(92)
	tx := right - w
	s.y += pdf.MM(5)
	s.ensure(4*lh(10) + pdf.MM(12) + lh(16))
	if hasInterest || hasFees {
		rows := []field{{"Dlžná suma", moneyText(r.Principal, "")}}
		if hasInterest {
			rows = append(rows, field{"Úrok z omeškania", moneyText(r.InterestTotal, "")})
		}
		if hasFees {
			rows = append(rows, field{"Paušálna náhrada nákladov", moneyText(r.Fees, "")})
		}
		for _, f := range rows {
			s.text(tx, s.y, s.f.Regular, 10, muted, f.label)
			s.textRight(right, s.y, s.f.Regular, 10, ink, f.value)
			s.y += lh(10) + pdf.MM(1)
		}
	}
	s.y += pdf.MM(2)
	s.p.Line(tx, s.y, right, s.y, 0.75, ink)
	s.y += pdf.MM(2)
	total := moneyText(r.Total, "")
	s.text(tx, s.y+pdf.MM(1.5), s.f.Regular, 10, muted, "Spolu na úhradu do "+r.PayBy.Display())
	s.p.TextRight(right, s.y+16*0.95, s.f.Bold, 16, ink, total)
	tw := s.f.Bold.Width(total, 16)
	uy := s.y + 16*1.15
	s.p.Line(right-tw, uy, right, uy, 0.75, ink)
	s.p.Line(right-tw, uy+2.25, right, uy+2.25, 0.75, ink)
	s.y = uy + pdf.MM(7)

	// platobné údaje
	s.ensure(lh(8) + lh(10) + pdf.MM(4))
	s.p.Line(left, s.y, right, s.y, 0.75, rule2)
	x = left
	for _, f := range []field{{"IBAN", dash(co.IBAN)}, {"Variabilný symbol", "číslo príslušnej faktúry"}} {
		s.text(x, s.y+pdf.MM(2), s.f.Regular, 8, muted, f.label)
		s.text(x, s.y+pdf.MM(2)+lh(8), s.f.Bold, 10, ink, f.value)
		x += s.fieldWidth(f, s.f.Bold, 10) + pdf.MM(10)
	}
	s.y += lh(8) + lh(10) + pdf.MM(4)
	s.p.Line(left, s.y, right, s.y, 0.75, rule2)

	s.y += pdf.MM(5)
	if r.Note != "" {
		for _, l := range s.wrap(s.f.Regular, 10, width, r.Note) {
			s.ensure(lh(10))
			s.text(left, s.y, s.f.Regular, 10, ink, l)
			s.y += lh(10)
		}
		s.y += pdf.MM(5)
	}
	for _, n := range r.LegalNotes() {
		for _, l := range s.wrap(s.f.Regular, 8, width, n) {
			s.ensure(lh(8))
			s.text(left, s.y, s.f.Regular, 8, muted, l)
			s.y += lh(8)
		}
		s.y += pdf.MM(1)
	}
	s.y += pdf.MM(12)
	s.ensure(lh(10) * 2)
	s.text(left, s.y, s.f.Regular, 10, muted, "S pozdravom")
	s.text(left, s.y+lh(10), s.f.Bold, 10, ink, co.Name)
	return s.finish(), nil
}

func prefixed(p, v string) string {
	if v == "" {
		return ""
	}
	return p + v
}
