package pdf

import (
	"bytes"
	"compress/zlib"
	"image"
	"image/color"
	"image/png"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func fam(t *testing.T) Family {
	t.Helper()
	f, err := SourceSans()
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// streams vráti rozbalené prúdy dokumentu.
func streams(t *testing.T, b []byte) []string {
	t.Helper()
	var out []string
	re := regexp.MustCompile(`(?s)/Length (\d+)>>\nstream\n`)
	for _, m := range re.FindAllSubmatchIndex(b, -1) {
		n, _ := strconv.Atoi(string(b[m[2]:m[3]]))
		data := b[m[1] : m[1]+n]
		if r, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
			d, _ := io.ReadAll(r)
			out = append(out, string(d))
		} else {
			out = append(out, string(data))
		}
	}
	return out
}

func TestStructure(t *testing.T) {
	f := fam(t)
	d := New("Faktúra 2026001")
	p := d.AddPage()
	p.Text(MM(18), MM(20), f.Bold, 18, Color{31, 58, 92}, "Faktúra")
	p.Line(MM(18), MM(25), MM(192), MM(25), 0.75, Color{})
	d.AddPage().Text(MM(18), MM(20), f.Regular, 10, Color{}, "Strana 2")
	b := d.Bytes()

	if !bytes.HasPrefix(b, []byte("%PDF-1.7")) || !bytes.HasSuffix(b, []byte("%%EOF\n")) {
		t.Fatal("chýba hlavička alebo koniec")
	}
	// každý záznam xref ukazuje na začiatok svojho objektu
	i := bytes.LastIndex(b, []byte("startxref\n"))
	xref, _ := strconv.Atoi(strings.Fields(string(b[i+10:]))[0])
	lines := strings.Split(string(b[xref:]), "\n")
	count, _ := strconv.Atoi(strings.Fields(lines[1])[1])
	for id := 1; id < count; id++ {
		off, _ := strconv.Atoi(strings.Fields(lines[2+id])[0])
		if want := strconv.Itoa(id) + " 0 obj"; !bytes.HasPrefix(b[off:], []byte(want)) {
			t.Fatalf("xref %d ukazuje na %q", id, b[off:off+12])
		}
	}
	if !bytes.Contains(b, []byte("/Count 2")) {
		t.Error("dokument nemá 2 strany")
	}
	if !bytes.Contains(b, []byte("/Title <FEFF")) {
		t.Error("názov s diakritikou nie je v UTF-16")
	}
}

func TestToUnicodeAndSubset(t *testing.T) {
	f := fam(t)
	d := New("x")
	d.AddPage().Text(0, 100, f.Regular, 10, Color{}, "Žltučký kôň ťa")
	b := d.Bytes()

	var cmap string
	for _, s := range streams(t, b) {
		if strings.Contains(s, "beginbfchar") {
			cmap = s
		}
	}
	for _, r := range "Žltučkýôňťa " {
		want := "<" + strings.ToUpper(strconv.FormatInt(int64(r), 16))
		for len(want) < 5 {
			want = "<0" + want[1:]
		}
		g := f.Regular.cmap[r]
		line := "<" + strings.ToUpper(hex4(g)) + "> " + want + ">"
		if !strings.Contains(cmap, line) {
			t.Errorf("ToUnicode nemá %q (%s)", string(r), line)
		}
	}

	// podmnožina: použité glyfy majú obrys, nepoužité nie, a písmo je oveľa menšie
	sub := f.Regular.subset()
	if len(sub) > len(regularTTF)/3 {
		t.Errorf("podmnožina má %d B z %d B", len(sub), len(regularTTF))
	}
	pf, err := ParseTTF("sub", sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.glyphData(f.Regular.cmap['Ž'])) == 0 {
		t.Error("v podmnožine chýba Ž")
	}
	if len(pf.glyphData(f.Regular.cmap['Q'])) != 0 {
		t.Error("podmnožina obsahuje nepoužité Q")
	}
	// nový dokument začína s prázdnou podmnožinou
	if g := fam(t); len(g.Regular.used) != 0 {
		t.Error("Fresh nezačína prázdnou podmnožinou")
	}
}

func hex4(g uint16) string {
	s := strconv.FormatUint(uint64(g), 16)
	return strings.Repeat("0", 4-len(s)) + s
}

func TestWidth(t *testing.T) {
	f := fam(t)
	tests := []struct {
		a, b string
	}{
		{"0", "9"}, // tabuľkové číslice v Source Sans majú rovnakú šírku
		{"1 234,50", "9 876,10"},
	}
	for _, tt := range tests {
		if wa, wb := f.Regular.Width(tt.a, 10), f.Regular.Width(tt.b, 10); wa != wb {
			t.Errorf("šírka %q=%v, %q=%v", tt.a, wa, tt.b, wb)
		}
	}
	if w := f.Regular.Width("Faktúra", 10); w < 20 || w > 50 {
		t.Errorf("šírka slova Faktúra = %v", w)
	}
}

func TestImages(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	src.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, src) // zvyšok je priehľadný
	tests := []struct {
		name  string
		data  []byte
		err   bool
		smask bool
	}{
		{"png s priehľadnosťou", buf.Bytes(), false, true},
		{"nezmysel", []byte("nie je obrázok"), true, false},
	}
	for _, tt := range tests {
		img, err := NewImage(tt.data)
		if (err != nil) != tt.err {
			t.Fatalf("%s: chyba %v", tt.name, err)
		}
		if err != nil {
			continue
		}
		d := New("x")
		d.AddPage().Image(img, 10, 10, 40, 20)
		b := d.Bytes()
		if got := bytes.Contains(b, []byte("/SMask")); got != tt.smask {
			t.Errorf("%s: SMask=%v", tt.name, got)
		}
		if !bytes.Contains(b, []byte("/Im1 Do")) && !strings.Contains(strings.Join(streams(t, b), ""), "/Im1 Do") {
			t.Errorf("%s: obrázok nie je na strane", tt.name)
		}
	}
}

func TestWrap(t *testing.T) {
	f := fam(t).Regular
	w := f.Width("Dodanie tovaru", 10) + 0.01
	tests := []struct {
		name, text string
		want       []string
	}{
		{"krátky", "Dodanie", []string{"Dodanie"}},
		{"zalomenie po slovách", "Dodanie tovaru podľa zmluvy", []string{"Dodanie tovaru", "podľa zmluvy"}},
		{"koniec riadku", "Riadok 1\nRiadok 2", []string{"Riadok 1", "Riadok 2"}},
		{"prázdny riadok ostane", "A\n\nB", []string{"A", "", "B"}},
		{"dlhé slovo", "Nadpriemernevysokoškolsky", nil},
	}
	for _, tt := range tests {
		got := Wrap(f, 10, w, tt.text)
		for _, l := range got {
			if f.Width(l, 10) > w {
				t.Errorf("%s: riadok %q je širší ako stĺpec", tt.name, l)
			}
		}
		if tt.want == nil {
			if strings.Join(got, "") != tt.text || len(got) < 2 {
				t.Errorf("%s: %q", tt.name, got)
			}
			continue
		}
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%s: %q, chcem %q", tt.name, got, tt.want)
		}
	}
}
