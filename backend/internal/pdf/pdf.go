package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // dekodéry pre logo
	_ "image/png"
	"math"
	"sort"
	"strings"
	"unicode/utf16"
)

// Rozmery A4 v bodoch (1 bod = 1/72 palca) a prevod z milimetrov.
const (
	PageWidth  = 595.28
	PageHeight = 841.89
)

// MM prevedie milimetre na body.
func MM(v float64) float64 { return v * 72 / 25.4 }

// Color je farba RGB 0–255.
type Color struct{ R, G, B uint8 }

func (c Color) rgb() string {
	return fmt.Sprintf("%s %s %s", num(float64(c.R)/255), num(float64(c.G)/255), num(float64(c.B)/255))
}

// Doc je dokument PDF.
type Doc struct {
	Title  string
	fonts  []*Font
	images []*Image
	pages  []*Page
}

// Page je strana; súradnice sú v bodoch od ľavého horného rohu.
type Page struct {
	doc   *Doc
	buf   bytes.Buffer
	texts []string
}

// Image je obrázok (logo, QR kód).
type Image struct {
	W, H  int
	data  []byte // stream
	jpeg  bool
	space string
	alpha []byte // maska priehľadnosti (DeviceGray)
	bits  int
}

// New vytvorí prázdny dokument.
func New(title string) *Doc { return &Doc{Title: title} }

// AddFont zaregistruje písmo v dokumente (každé písmo iba raz).
func (d *Doc) AddFont(f *Font) {
	for _, x := range d.fonts {
		if x == f {
			return
		}
	}
	d.fonts = append(d.fonts, f)
}

// AddPage pridá stranu A4.
func (d *Doc) AddPage() *Page {
	p := &Page{doc: d}
	d.pages = append(d.pages, p)
	return p
}

// PlainText vráti texty strán v poradí zápisu (riadky oddelené \n, strany \f) —
// na kontrolu obsahu v testoch.
func (d *Doc) PlainText() string {
	pages := make([]string, len(d.pages))
	for i, p := range d.pages {
		pages[i] = strings.Join(p.texts, "\n")
	}
	return strings.Join(pages, "\f")
}

// Pages vráti počet strán.
func (d *Doc) Pages() int { return len(d.pages) }

func (d *Doc) fontIndex(f *Font) int {
	for i, x := range d.fonts {
		if x == f {
			return i
		}
	}
	d.fonts = append(d.fonts, f)
	return len(d.fonts) - 1
}

// num zapíše číslo bez zbytočných núl.
func num(v float64) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", v), "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

// Text napíše text; y je účiaria (baseline) od horného okraja.
func (p *Page) Text(x, y float64, f *Font, size float64, c Color, s string) {
	p.texts = append(p.texts, s)
	idx := p.doc.fontIndex(f)
	var hex strings.Builder
	for _, r := range s {
		fmt.Fprintf(&hex, "%04X", f.glyph(r))
	}
	fmt.Fprintf(&p.buf, "BT /F%d %s Tf %s rg %s %s Td <%s> Tj ET\n", idx+1, num(size), c.rgb(), num(x), num(PageHeight-y), hex.String())
}

// TextRight napíše text zarovnaný doprava k x.
func (p *Page) TextRight(x, y float64, f *Font, size float64, c Color, s string) {
	p.Text(x-f.Width(s, size), y, f, size, c, s)
}

// Line nakreslí čiaru.
func (p *Page) Line(x1, y1, x2, y2, width float64, c Color) {
	fmt.Fprintf(&p.buf, "%s RG %s w %s %s m %s %s l S\n", c.rgb(), num(width), num(x1), num(PageHeight-y1), num(x2), num(PageHeight-y2))
}

// Rect vyplní obdĺžnik (x, y = ľavý horný roh).
func (p *Page) Rect(x, y, w, h float64, c Color) {
	fmt.Fprintf(&p.buf, "%s rg %s %s %s %s re f\n", c.rgb(), num(x), num(PageHeight-y-h), num(w), num(h))
}

// Image vloží obrázok do obdĺžnika (x, y = ľavý horný roh).
func (p *Page) Image(img *Image, x, y, w, h float64) {
	idx := -1
	for i, im := range p.doc.images {
		if im == img {
			idx = i
		}
	}
	if idx < 0 {
		p.doc.images = append(p.doc.images, img)
		idx = len(p.doc.images) - 1
	}
	fmt.Fprintf(&p.buf, "q %s 0 0 %s %s %s cm /Im%d Do Q\n", num(w), num(h), num(x), num(PageHeight-y-h), idx+1)
}

// NewImage pripraví obrázok PNG alebo JPEG na vloženie. JPEG sa vloží bez zmeny.
func NewImage(data []byte) (*Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pdf: obrázok: %w", err)
	}
	if format == "jpeg" {
		space := "DeviceRGB"
		switch cfg.ColorModel {
		case color.GrayModel:
			space = "DeviceGray"
		case color.CMYKModel:
			space = "DeviceCMYK"
		}
		return &Image{W: cfg.Width, H: cfg.Height, data: data, jpeg: true, space: space, bits: 8}, nil
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pdf: obrázok: %w", err)
	}
	return FromImage(src), nil
}

// FromImage prevedie obrázok na RGB (+ maska priehľadnosti).
func FromImage(src image.Image) *Image {
	b := src.Bounds()
	rgb := make([]byte, 0, b.Dx()*b.Dy()*3)
	alpha := make([]byte, 0, b.Dx()*b.Dy())
	opaque := true
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			rgb = append(rgb, c.R, c.G, c.B)
			alpha = append(alpha, c.A)
			opaque = opaque && c.A == 255
		}
	}
	img := &Image{W: b.Dx(), H: b.Dy(), data: rgb, space: "DeviceRGB", bits: 8}
	if !opaque {
		img.alpha = alpha
	}
	return img
}

func deflate(b []byte) []byte {
	var out bytes.Buffer
	w, _ := zlib.NewWriterLevel(&out, zlib.BestCompression)
	_, _ = w.Write(b)
	_ = w.Close()
	return out.Bytes()
}

// pdfString zapíše text ako reťazec PDF (UTF-16BE s BOM pre diakritiku).
func pdfString(s string) string {
	ascii := true
	for _, r := range s {
		if r > 126 || r < 32 || r == '(' || r == ')' || r == '\\' {
			ascii = false
		}
	}
	if ascii {
		return "(" + s + ")"
	}
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteString(">")
	return b.String()
}

// writer zostaví objekty PDF a tabuľku xref.
type writer struct {
	buf     bytes.Buffer
	offsets []int
}

func (w *writer) reserve() int {
	w.offsets = append(w.offsets, 0)
	return len(w.offsets)
}

func (w *writer) obj(id int, body string) {
	w.offsets[id-1] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n%s\nendobj\n", id, body)
}

func (w *writer) stream(id int, dict string, data []byte) {
	w.offsets[id-1] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n<<%s /Length %d>>\nstream\n", id, dict, len(data))
	w.buf.Write(data)
	w.buf.WriteString("\nendstream\nendobj\n")
}

// Bytes zostaví celý dokument.
func (d *Doc) Bytes() []byte {
	w := &writer{}
	w.buf.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	catalog, pagesID, info := w.reserve(), w.reserve(), w.reserve()

	// písma: Type0 + CIDFontType2 s vloženou podmnožinou písma
	fontIDs := make([]int, len(d.fonts))
	for i, f := range d.fonts {
		fontIDs[i] = d.writeFont(w, f, i)
	}
	imgIDs := make([]int, len(d.images))
	for i, im := range d.images {
		imgIDs[i] = writeImage(w, im)
	}
	var res strings.Builder
	res.WriteString("<</Font <<")
	for i, id := range fontIDs {
		fmt.Fprintf(&res, " /F%d %d 0 R", i+1, id)
	}
	res.WriteString(" >> /XObject <<")
	for i, id := range imgIDs {
		fmt.Fprintf(&res, " /Im%d %d 0 R", i+1, id)
	}
	res.WriteString(" >> >>")
	resID := w.reserve()
	w.obj(resID, res.String())

	kids := make([]string, len(d.pages))
	for i, p := range d.pages {
		cid, pid := w.reserve(), w.reserve()
		w.stream(cid, " /Filter /FlateDecode", deflate(p.buf.Bytes()))
		w.obj(pid, fmt.Sprintf("<</Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources %d 0 R /Contents %d 0 R>>",
			pagesID, num(PageWidth), num(PageHeight), resID, cid))
		kids[i] = fmt.Sprintf("%d 0 R", pid)
	}
	w.obj(pagesID, fmt.Sprintf("<</Type /Pages /Kids [%s] /Count %d>>", strings.Join(kids, " "), len(kids)))
	w.obj(catalog, fmt.Sprintf("<</Type /Catalog /Pages %d 0 R /Lang (sk-SK)>>", pagesID))
	w.obj(info, fmt.Sprintf("<</Title %s /Producer (Fiskus) /Creator (Fiskus)>>", pdfString(d.Title)))

	xref := w.buf.Len()
	fmt.Fprintf(&w.buf, "xref\n0 %d\n0000000000 65535 f \n", len(w.offsets)+1)
	for _, off := range w.offsets {
		fmt.Fprintf(&w.buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&w.buf, "trailer\n<</Size %d /Root %d 0 R /Info %d 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(w.offsets)+1, catalog, info, xref)
	return w.buf.Bytes()
}

func (d *Doc) writeFont(w *writer, f *Font, i int) int {
	tag := subsetTag(i) + "+" + f.Name // predpona podmnožiny písma

	file := f.subset()
	fileID := w.reserve()
	w.stream(fileID, fmt.Sprintf(" /Filter /FlateDecode /Length1 %d", len(file)), deflate(file))

	descID := w.reserve()
	w.obj(descID, fmt.Sprintf("<</Type /FontDescriptor /FontName /%s /Flags 32 /FontBBox [%d %d %d %d] /ItalicAngle 0 /Ascent %d /Descent %d /CapHeight %d /StemV 80 /FontFile2 %d 0 R>>",
		tag, f.scale(f.bbox[0]), f.scale(f.bbox[1]), f.scale(f.bbox[2]), f.scale(f.bbox[3]),
		f.scale(f.ascent), f.scale(f.descent), f.scale(f.capHeight), fileID))

	glyphs := make([]int, 0, len(f.used))
	for g := range f.used {
		glyphs = append(glyphs, int(g))
	}
	sort.Ints(glyphs)
	var widths strings.Builder
	for _, g := range glyphs {
		fmt.Fprintf(&widths, "%d [%d] ", g, f.scale(int(f.advances[g])))
	}
	cidID := w.reserve()
	w.obj(cidID, fmt.Sprintf("<</Type /Font /Subtype /CIDFontType2 /BaseFont /%s /CIDSystemInfo <</Registry (Adobe) /Ordering (Identity) /Supplement 0>> /FontDescriptor %d 0 R /W [%s] /CIDToGIDMap /Identity>>",
		tag, descID, widths.String()))

	// ToUnicode: text v PDF sa dá hľadať a kopírovať
	var cmap strings.Builder
	cmap.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n/CIDSystemInfo <</Registry (Adobe) /Ordering (UCS) /Supplement 0>> def\n/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	for start := 0; start < len(glyphs); start += 100 {
		end := min(start+100, len(glyphs))
		fmt.Fprintf(&cmap, "%d beginbfchar\n", end-start)
		for _, g := range glyphs[start:end] {
			var u strings.Builder
			for _, x := range utf16.Encode([]rune{f.used[uint16(g)]}) {
				fmt.Fprintf(&u, "%04X", x)
			}
			fmt.Fprintf(&cmap, "<%04X> <%s>\n", g, u.String())
		}
		cmap.WriteString("endbfchar\n")
	}
	cmap.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	tuID := w.reserve()
	w.stream(tuID, " /Filter /FlateDecode", deflate([]byte(cmap.String())))

	id := w.reserve()
	w.obj(id, fmt.Sprintf("<</Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H /DescendantFonts [%d 0 R] /ToUnicode %d 0 R>>",
		tag, cidID, tuID))
	return id
}

// subsetTag vráti predponu podmnožiny písma (6 veľkých písmen).
func subsetTag(i int) string {
	b := []byte("FKSAAA")
	b[5] = 'A' + byte(i%26)
	b[4] = 'A' + byte(i/26%26)
	return string(b)
}

func writeImage(w *writer, im *Image) int {
	var smask string
	if im.alpha != nil {
		mid := w.reserve()
		w.stream(mid, fmt.Sprintf(" /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode",
			im.W, im.H), deflate(im.alpha))
		smask = fmt.Sprintf(" /SMask %d 0 R", mid)
	}
	id := w.reserve()
	filter, data := "/FlateDecode", deflate(im.data)
	if im.jpeg {
		filter, data = "/DCTDecode", im.data
	}
	w.stream(id, fmt.Sprintf(" /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /%s /BitsPerComponent %d /Filter %s%s",
		im.W, im.H, im.space, im.bits, filter, smask), data)
	return id
}

// TextRotated napíše text otočený o uhol (stupne, proti smeru hodinových
// ručičiek) okolo bodu x, y.
func (p *Page) TextRotated(x, y float64, f *Font, size float64, c Color, deg float64, s string) {
	p.texts = append(p.texts, s)
	idx := p.doc.fontIndex(f)
	var hex strings.Builder
	for _, r := range s {
		fmt.Fprintf(&hex, "%04X", f.glyph(r))
	}
	rad := deg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	fmt.Fprintf(&p.buf, "BT /F%d %s Tf %s rg %s %s %s %s %s %s Tm <%s> Tj ET\n", idx+1, num(size), c.rgb(),
		num(cos), num(sin), num(-sin), num(cos), num(x), num(PageHeight-y), hex.String())
}

// Wrap zalomí text do riadkov danej šírky (po slovách; príliš dlhé slovo
// sa zalomí po znakoch). Zachová konce riadkov z pôvodného textu.
func Wrap(f *Font, size, width float64, text string) []string {
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			try := word
			if line != "" {
				try = line + " " + word
			}
			if f.Width(try, size) <= width {
				line = try
				continue
			}
			if line != "" {
				out = append(out, line)
				line = ""
			}
			for f.Width(word, size) > width { // slovo dlhšie ako riadok
				rs := []rune(word)
				n := len(rs) - 1
				for n > 1 && f.Width(string(rs[:n]), size) > width {
					n--
				}
				out = append(out, string(rs[:n]))
				word = string(rs[n:])
			}
			line = word
		}
		out = append(out, line)
	}
	return out
}
