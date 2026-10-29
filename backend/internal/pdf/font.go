// Package pdf je malý generátor PDF bez externých knižníc: strany A4, text
// s vloženým písmom TrueType (iba použité znaky), čiary, obdĺžniky a obrázky.
// Stačí na doklady (faktúra, ponuka, upomienka), ktoré sa posielajú e-mailom.
package pdf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// Font je písmo TrueType pripravené na meranie textu a vloženie do PDF.
type Font struct {
	Name       string // PostScript názov (bez predpony podmnožiny)
	data       []byte
	tables     map[string][]byte
	unitsPerEm int
	ascent     int
	descent    int
	capHeight  int
	bbox       [4]int
	numGlyphs  int
	advances   []uint16
	cmap       map[rune]uint16
	loca       []uint32
	used       map[uint16]rune // použité glyfy → znak (pre ToUnicode)
}

// ParseTTF načíta písmo TrueType (glyf/loca, nie CFF).
func ParseTTF(name string, data []byte) (*Font, error) {
	if len(data) < 12 {
		return nil, errors.New("pdf: písmo je príliš krátke")
	}
	if v := binary.BigEndian.Uint32(data); v != 0x00010000 && v != 0x74727565 {
		return nil, errors.New("pdf: nie je to písmo TrueType")
	}
	n := int(binary.BigEndian.Uint16(data[4:]))
	f := &Font{Name: name, data: data, tables: map[string][]byte{}, used: map[uint16]rune{}}
	for i := 0; i < n; i++ {
		rec := 12 + 16*i
		if rec+16 > len(data) {
			return nil, errors.New("pdf: poškodený adresár tabuliek")
		}
		tag := string(data[rec : rec+4])
		off := int(binary.BigEndian.Uint32(data[rec+8:]))
		ln := int(binary.BigEndian.Uint32(data[rec+12:]))
		if off < 0 || ln < 0 || off+ln > len(data) {
			return nil, fmt.Errorf("pdf: tabuľka %s je mimo súboru", tag)
		}
		f.tables[tag] = data[off : off+ln]
	}
	for _, t := range []string{"head", "hhea", "maxp", "hmtx", "loca", "glyf", "cmap"} {
		if f.tables[t] == nil {
			return nil, fmt.Errorf("pdf: chýba tabuľka %s", t)
		}
	}
	head := f.tables["head"]
	f.unitsPerEm = int(binary.BigEndian.Uint16(head[18:]))
	for i := 0; i < 4; i++ {
		f.bbox[i] = int(int16(binary.BigEndian.Uint16(head[36+2*i:])))
	}
	longLoca := binary.BigEndian.Uint16(head[50:]) == 1
	hhea := f.tables["hhea"]
	f.ascent = int(int16(binary.BigEndian.Uint16(hhea[4:])))
	f.descent = int(int16(binary.BigEndian.Uint16(hhea[6:])))
	numHMetrics := int(binary.BigEndian.Uint16(hhea[34:]))
	f.numGlyphs = int(binary.BigEndian.Uint16(f.tables["maxp"][4:]))
	f.capHeight = f.ascent
	if os2 := f.tables["OS/2"]; len(os2) >= 90 && binary.BigEndian.Uint16(os2) >= 2 {
		f.capHeight = int(int16(binary.BigEndian.Uint16(os2[88:])))
	}

	hmtx := f.tables["hmtx"]
	f.advances = make([]uint16, f.numGlyphs)
	var last uint16
	for g := 0; g < f.numGlyphs; g++ {
		if g < numHMetrics && 4*g+2 <= len(hmtx) {
			last = binary.BigEndian.Uint16(hmtx[4*g:])
		}
		f.advances[g] = last
	}

	loca := f.tables["loca"]
	f.loca = make([]uint32, f.numGlyphs+1)
	for g := 0; g <= f.numGlyphs; g++ {
		if longLoca {
			if 4*g+4 > len(loca) {
				return nil, errors.New("pdf: krátka tabuľka loca")
			}
			f.loca[g] = binary.BigEndian.Uint32(loca[4*g:])
		} else {
			if 2*g+2 > len(loca) {
				return nil, errors.New("pdf: krátka tabuľka loca")
			}
			f.loca[g] = uint32(binary.BigEndian.Uint16(loca[2*g:])) * 2
		}
	}
	var err error
	if f.cmap, err = parseCmap(f.tables["cmap"]); err != nil {
		return nil, err
	}
	return f, nil
}

func parseCmap(t []byte) (map[rune]uint16, error) {
	if len(t) < 4 {
		return nil, errors.New("pdf: krátka tabuľka cmap")
	}
	n := int(binary.BigEndian.Uint16(t[2:]))
	best, bestScore := -1, 0
	for i := 0; i < n; i++ {
		rec := 4 + 8*i
		if rec+8 > len(t) {
			break
		}
		pid, eid := binary.BigEndian.Uint16(t[rec:]), binary.BigEndian.Uint16(t[rec+2:])
		off := int(binary.BigEndian.Uint32(t[rec+4:]))
		score := 0
		switch {
		case pid == 3 && eid == 10, pid == 0 && eid == 4:
			score = 3 // celý Unicode (formát 12)
		case pid == 3 && eid == 1, pid == 0:
			score = 2
		}
		if score > bestScore && off < len(t) {
			best, bestScore = off, score
		}
	}
	if best < 0 {
		return nil, errors.New("pdf: písmo nemá Unicode cmap")
	}
	sub := t[best:]
	m := map[rune]uint16{}
	switch binary.BigEndian.Uint16(sub) {
	case 4:
		segX2 := int(binary.BigEndian.Uint16(sub[6:]))
		ends, starts := sub[14:], sub[16+segX2:]
		deltas, ranges := sub[16+2*segX2:], sub[16+3*segX2:]
		for s := 0; s < segX2/2; s++ {
			end := binary.BigEndian.Uint16(ends[2*s:])
			start := binary.BigEndian.Uint16(starts[2*s:])
			delta := binary.BigEndian.Uint16(deltas[2*s:])
			ro := int(binary.BigEndian.Uint16(ranges[2*s:]))
			for c := uint32(start); c <= uint32(end) && c != 0xFFFF; c++ {
				var g uint16
				if ro == 0 {
					g = uint16(c) + delta
				} else {
					idx := 16 + 3*segX2 + 2*s + ro + 2*int(c-uint32(start))
					if idx+2 > len(sub) {
						continue
					}
					g = binary.BigEndian.Uint16(sub[idx:])
					if g != 0 {
						g += delta
					}
				}
				if g != 0 {
					m[rune(c)] = g
				}
			}
		}
	case 12:
		groups := int(binary.BigEndian.Uint32(sub[12:]))
		for i := 0; i < groups; i++ {
			rec := 16 + 12*i
			if rec+12 > len(sub) {
				break
			}
			start, end := binary.BigEndian.Uint32(sub[rec:]), binary.BigEndian.Uint32(sub[rec+4:])
			g := binary.BigEndian.Uint32(sub[rec+8:])
			for c := start; c <= end && end-start < 0x10000; c++ {
				m[rune(c)] = uint16(g + c - start)
			}
		}
	default:
		return nil, errors.New("pdf: nepodporovaný formát cmap")
	}
	return m, nil
}

// glyph vráti glyf pre znak (0 = chýbajúci) a zapamätá si ho ako použitý.
func (f *Font) glyph(r rune) uint16 {
	g := f.cmap[r]
	if _, seen := f.used[g]; !seen {
		f.used[g] = r
	}
	return g
}

// Width vráti šírku textu v bodoch pri danej veľkosti písma.
func (f *Font) Width(s string, size float64) float64 {
	var w int
	for _, r := range s {
		w += int(f.advances[f.cmap[r]])
	}
	return float64(w) * size / float64(f.unitsPerEm)
}

// scale prevedie jednotky písma na tisíciny (jednotky PDF).
func (f *Font) scale(v int) int { return v * 1000 / f.unitsPerEm }

// glyphData vráti obrys glyfu.
func (f *Font) glyphData(g uint16) []byte {
	if int(g) >= f.numGlyphs {
		return nil
	}
	start, end := f.loca[g], f.loca[g+1]
	glyf := f.tables["glyf"]
	if end <= start || int(end) > len(glyf) {
		return nil
	}
	return glyf[start:end]
}

// components vráti glyfy, z ktorých sa skladá zložený glyf.
func components(d []byte) []uint16 {
	if len(d) < 10 || int16(binary.BigEndian.Uint16(d)) >= 0 {
		return nil
	}
	var out []uint16
	p := 10
	for p+4 <= len(d) {
		flags := binary.BigEndian.Uint16(d[p:])
		out = append(out, binary.BigEndian.Uint16(d[p+2:]))
		p += 4
		if flags&0x0001 != 0 {
			p += 4
		} else {
			p += 2
		}
		switch {
		case flags&0x0008 != 0:
			p += 2
		case flags&0x0040 != 0:
			p += 4
		case flags&0x0080 != 0:
			p += 8
		}
		if flags&0x0020 == 0 {
			break
		}
	}
	return out
}

// subset vytvorí písmo, v ktorom majú obrys iba použité glyfy (vrátane
// zložiek zložených glyfov). Čísla glyfov ostanú rovnaké (CIDToGIDMap Identity).
func (f *Font) subset() []byte {
	keep := map[uint16]bool{0: true}
	var stack []uint16
	for g := range f.used {
		stack = append(stack, g)
	}
	for len(stack) > 0 {
		g := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if keep[g] && g != 0 {
			continue
		}
		keep[g] = true
		stack = append(stack, components(f.glyphData(g))...)
	}
	var glyf bytes.Buffer
	loca := make([]byte, 4*(f.numGlyphs+1))
	for g := 0; g < f.numGlyphs; g++ {
		binary.BigEndian.PutUint32(loca[4*g:], uint32(glyf.Len()))
		if keep[uint16(g)] {
			glyf.Write(f.glyphData(uint16(g)))
			for glyf.Len()%4 != 0 {
				glyf.WriteByte(0)
			}
		}
	}
	binary.BigEndian.PutUint32(loca[4*f.numGlyphs:], uint32(glyf.Len()))

	head := append([]byte(nil), f.tables["head"]...)
	binary.BigEndian.PutUint32(head[8:], 0)  // checkSumAdjustment
	binary.BigEndian.PutUint16(head[50:], 1) // dlhá loca
	tables := map[string][]byte{"head": head, "loca": loca, "glyf": glyf.Bytes()}
	for _, t := range []string{"hhea", "maxp", "hmtx", "cmap", "cvt ", "fpgm", "prep", "OS/2"} {
		if d := f.tables[t]; d != nil {
			tables[t] = d
		}
	}
	if post := f.tables["post"]; len(post) >= 32 {
		p := append([]byte(nil), post[:32]...)
		binary.BigEndian.PutUint32(p, 0x00030000) // bez názvov glyfov
		tables["post"] = p
	}
	return writeTTF(tables)
}

func writeTTF(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	n := len(tags)
	sr, es := 1, 0
	for sr*2 <= n {
		sr *= 2
		es++
	}
	var b bytes.Buffer
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint32(hdr, 0x00010000)
	binary.BigEndian.PutUint16(hdr[4:], uint16(n))
	binary.BigEndian.PutUint16(hdr[6:], uint16(sr*16))
	binary.BigEndian.PutUint16(hdr[8:], uint16(es))
	binary.BigEndian.PutUint16(hdr[10:], uint16(n*16-sr*16))
	b.Write(hdr)
	off := 12 + 16*n
	var body bytes.Buffer
	for _, t := range tags {
		d := tables[t]
		rec := make([]byte, 16)
		copy(rec, t)
		binary.BigEndian.PutUint32(rec[4:], checksum(d))
		binary.BigEndian.PutUint32(rec[8:], uint32(off+body.Len()))
		binary.BigEndian.PutUint32(rec[12:], uint32(len(d)))
		b.Write(rec)
		body.Write(d)
		for body.Len()%4 != 0 {
			body.WriteByte(0)
		}
	}
	b.Write(body.Bytes())
	return b.Bytes()
}

func checksum(d []byte) uint32 {
	var sum uint32
	for i := 0; i < len(d); i += 4 {
		var w [4]byte
		copy(w[:], d[i:min(i+4, len(d))])
		sum += binary.BigEndian.Uint32(w[:])
	}
	return sum
}

// Fresh vráti kópiu písma bez zapamätaných použitých znakov — každý
// dokument si vkladá vlastnú podmnožinu. Tabuľky sa zdieľajú (iba na čítanie).
func (f *Font) Fresh() *Font {
	c := *f
	c.used = map[uint16]rune{}
	return &c
}
