// Package printout vytvára PDF dokladov (faktúra, ponuka, upomienka) na
// stiahnutie a do prílohy e-mailu. Vzhľad zodpovedá tlači v prehliadači
// (InvoicePrint.vue, ReminderPrint.vue); sumy berie hotové z dokladu.
package printout

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/pdf"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Farby tlače (rovnaké ako v CSS tlače).
var (
	ink    = pdf.Color{R: 0x1d, G: 0x26, B: 0x33}
	muted  = pdf.Color{R: 0x4e, G: 0x58, B: 0x66}
	faint  = pdf.Color{R: 0x6b, G: 0x76, B: 0x85}
	navy   = pdf.Color{R: 0x1f, G: 0x3a, B: 0x5c}
	red    = pdf.Color{R: 0xb0, G: 0x3a, B: 0x26}
	rule   = pdf.Color{R: 0xdc, G: 0xe1, B: 0xe8}
	rule2  = pdf.Color{R: 0xc5, G: 0xcc, B: 0xd6}
	panel  = pdf.Color{R: 0xf6, G: 0xf7, B: 0xf9}
	stampC = pdf.Color{R: 0xf5, G: 0xe5, B: 0xe3} // červený atrament 13 % na bielom
)

// Okraje strany A4.
var (
	left   = pdf.MM(16)
	right  = pdf.PageWidth - pdf.MM(16)
	width  = right - left
	top    = pdf.MM(16)
	bottom = pdf.PageHeight - pdf.MM(18)
)

const lineHeight = 1.38

// Supplier sú údaje dodávateľa (firma a jej logo).
type Supplier struct {
	Company store.Company
	Logo    []byte // PNG alebo JPEG; iné formáty sa vynechajú
}

// sheet je rozpracovaný doklad: aktuálna strana a zvislá pozícia.
type sheet struct {
	footLogo *pdf.Image // logo Fiškusa v päte
	doc      *pdf.Doc
	f        pdf.Family
	pages    []*pdf.Page
	p        *pdf.Page
	y        float64
	label    string // názov dokladu v päte strán
	out      []byte // hotové PDF
	// header nakreslí hlavičku tabuľky po zalomení strany
	header func()
}

func newSheet(title, label string) (*sheet, error) {
	f, err := pdf.SourceSans()
	if err != nil {
		return nil, err
	}
	s := &sheet{doc: pdf.New(title), f: f, label: label}
	s.newPage()
	return s, nil
}

func (s *sheet) newPage() {
	s.p = s.doc.AddPage()
	s.pages = append(s.pages, s.p)
	s.y = top
}

// ensure zalomí stranu, ak sa ďalší blok výšky h nezmestí.
func (s *sheet) ensure(h float64) {
	if s.y+h <= bottom {
		return
	}
	s.newPage()
	if s.header != nil {
		s.header()
	}
}

// text napíše riadok textu; y je horný okraj riadku.
func (s *sheet) text(x, y float64, f *pdf.Font, size float64, c pdf.Color, t string) {
	s.p.Text(x, baseline(y, size), f, size, c, t)
}

func (s *sheet) textRight(x, y float64, f *pdf.Font, size float64, c pdf.Color, t string) {
	s.p.TextRight(x, baseline(y, size), f, size, c, t)
}

// baseline vráti účiaru riadku s výškou lineHeight × veľkosť písma.
func baseline(y, size float64) float64 { return y + size*(lineHeight-1)/2 + size*0.9 }

func lh(size float64) float64 { return size * lineHeight }

// logo vloží logo firmy (max. výška a šírka v bodoch); vráti výšku alebo 0.
func (s *sheet) logo(data []byte, x, y, maxW, maxH float64) float64 {
	if len(data) == 0 {
		return 0
	}
	img, err := pdf.NewImage(data)
	if err != nil || img.W == 0 || img.H == 0 {
		return 0 // WebP a poškodené obrázky sa vynechajú
	}
	h := maxH
	w := h * float64(img.W) / float64(img.H)
	if w > maxW {
		w, h = maxW, maxW*float64(img.H)/float64(img.W)
	}
	s.p.Image(img, x, y, w, h)
	return h
}

// stamp nakreslí šikmú pečiatku (Koncept, Stornovaná) cez stred listu.
func (s *sheet) stamp(t string) {
	size := 54.0
	w := s.f.Bold.Width(t, size)
	cx, cy := pdf.PageWidth/2, pdf.MM(110)
	const sin, cos = 0.309, 0.951 // 18°
	s.p.TextRotated(cx-w/2*cos, cy+w/2*sin+size*0.35, s.f.Bold, size, stampC, 18, t)
}

// fiskusLogo je malé logo programu do päty každej strany.
//
//go:embed fiskus-logo.png
var fiskusLogo []byte

// Footer je text päty pod logom programu.
const Footer = "Vytvorené v programe Fiškus"

// footer nakreslí dole na stred malé logo a „Vytvorené v programe Fiškus“.
func (s *sheet) footer(p *pdf.Page) {
	size, gap := pdf.MM(3.2), pdf.MM(1.5)
	tw := s.f.Regular.Width(Footer, 7)
	x := (pdf.PageWidth - (size + gap + tw)) / 2
	y := pdf.PageHeight - pdf.MM(10)
	if s.footLogo == nil {
		s.footLogo, _ = pdf.NewImage(fiskusLogo) // jeden obrázok pre všetky strany
	}
	if s.footLogo != nil {
		p.Image(s.footLogo, x, y-size*0.78, size, size)
	}
	p.Text(x+size+gap, y, s.f.Regular, 7, faint, Footer)
}

// finish doplní pätu, čísla strán (pri viacerých stranách) a zostaví PDF.
func (s *sheet) finish() *sheet {
	n := len(s.pages)
	for i, p := range s.pages {
		s.footer(p)
		if n > 1 {
			t := fmt.Sprintf("%s · strana %d/%d", s.label, i+1, n)
			p.TextRight(right, pdf.PageHeight-pdf.MM(10), s.f.Regular, 7.5, faint, t)
		}
	}
	s.out = s.doc.Bytes()
	return s
}

func bytesOf(s *sheet, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	return s.out, nil
}

// wrap zalomí text do šírky stĺpca.
func (s *sheet) wrap(f *pdf.Font, size, w float64, t string) []string {
	return pdf.Wrap(f, size, w, t)
}

// amount zapíše sumu „1 234,50“.
func amount(d money.Dec) string { return d.Display() }

// moneyText zapíše sumu s menou „1 234,50 €“.
func moneyText(d money.Dec, currency string) string {
	if currency == "" || currency == "EUR" {
		return d.Display() + " €"
	}
	return d.Display() + " " + currency
}

// quantity zapíše množstvo bez zbytočných núl, najviac 4 desatinné miesta.
func quantity(d money.Dec) string {
	s := d.String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	frac = strings.TrimRight(frac, "0")
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteString("," + frac)
	}
	return b.String()
}

// addressLines vráti ulicu a „PSČ Mesto“.
func addressLines(street, zip, city string) []string {
	var out []string
	if strings.TrimSpace(street) != "" {
		out = append(out, street)
	}
	if zc := strings.TrimSpace(strings.TrimSpace(zip) + " " + strings.TrimSpace(city)); zc != "" {
		out = append(out, zc)
	}
	return out
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// column je stĺpec tabuľky.
type column struct {
	title string
	w     float64
	right bool
}

// table kreslí jednoduchú tabuľku s hlavičkou a riadkami.
type table struct {
	s    *sheet
	cols []column
	x    float64
	size float64
}

func (t *table) xs() []float64 {
	out := make([]float64, len(t.cols))
	x := t.x
	for i, c := range t.cols {
		out[i] = x
		x += c.w
	}
	return out
}

// pad je vnútorný okraj bunky; prvý a posledný stĺpec ho pri okraji nemajú.
func (t *table) cell(i int) (x0, x1 float64) {
	xs := t.xs()
	x0, x1 = xs[i], xs[i]+t.cols[i].w
	if i > 0 {
		x0 += pdf.MM(2)
	}
	if i < len(t.cols)-1 {
		x1 -= pdf.MM(2)
	}
	return
}

func (t *table) total() float64 {
	var w float64
	for _, c := range t.cols {
		w += c.w
	}
	return w
}

// head nakreslí hlavičku tabuľky.
func (t *table) head() {
	s := t.s
	h := lh(8) + pdf.MM(3)
	for i, c := range t.cols {
		x0, x1 := t.cell(i)
		if c.right {
			s.textRight(x1, s.y+pdf.MM(1.5), s.f.Semibold, 8, muted, c.title)
		} else {
			s.text(x0, s.y+pdf.MM(1.5), s.f.Semibold, 8, muted, c.title)
		}
	}
	s.y += h
	s.p.Line(t.x, s.y, t.x+t.total(), s.y, 0.75, ink)
}

// cellText je obsah bunky.
type cellText struct {
	text string
	font *pdf.Font
	c    pdf.Color
}

// row nakreslí riadok; text v stĺpcoch zarovnaných vľavo sa zalamuje.
func (t *table) row(cells []cellText, padY float64, line bool) {
	s := t.s
	lines := make([][]string, len(cells))
	n := 1
	for i, c := range cells {
		if t.cols[i].right {
			lines[i] = []string{c.text}
			continue
		}
		x0, x1 := t.cell(i)
		lines[i] = s.wrap(c.font, t.size, x1-x0, c.text)
		n = max(n, len(lines[i]))
	}
	h := float64(n)*lh(t.size) + 2*padY
	s.ensure(h)
	for i, c := range cells {
		x0, x1 := t.cell(i)
		col := c.c
		if col == (pdf.Color{}) {
			col = ink
		}
		for j, l := range lines[i] {
			y := s.y + padY + float64(j)*lh(t.size)
			if t.cols[i].right {
				s.textRight(x1, y, c.font, t.size, col, l)
			} else {
				s.text(x0, y, c.font, t.size, col, l)
			}
		}
	}
	s.y += h
	if line {
		s.p.Line(t.x, s.y, t.x+t.total(), s.y, 0.5, rule)
	}
}

// field je dvojica popis + hodnota (dátumy, platobné údaje).
type field struct{ label, value string }

// fieldWidth vráti šírku bloku popis/hodnota.
func (s *sheet) fieldWidth(f field, valueFont *pdf.Font, valueSize float64) float64 {
	return max(s.f.Regular.Width(f.label, 8), valueFont.Width(f.value, valueSize))
}
