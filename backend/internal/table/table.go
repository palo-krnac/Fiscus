// Package table číta tabuľky zo súborov CSV a Excel (.xlsx) na import
// (cenník, bankové výpisy). Vracia riadky textových buniek.
package table

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrFormat — súbor nie je CSV ani XLSX.
var ErrFormat = errors.New("súbor nie je tabuľka CSV ani Excel (.xlsx)")

// Read prečíta tabuľku; formát určí podľa obsahu (XLSX je ZIP).
func Read(data []byte) ([][]string, error) {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return readXLSX(data)
	}
	if bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0}) {
		return nil, errors.New("starý formát Excel (.xls) — uložte súbor ako .xlsx alebo CSV")
	}
	return readCSV(data)
}

// readCSV zvládne UTF-8 (aj s BOM) aj Windows-1250 (CSV z Excelu v slovenských
// Windows) a oddeľovač ; , alebo tabulátor.
func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) {
		data = []byte(decode1250(data))
	}
	text := string(data)
	if strings.ContainsRune(text, 0) {
		return nil, ErrFormat
	}
	first, _, _ := strings.Cut(text, "\n")
	sep := ';'
	best := strings.Count(first, ";")
	for _, c := range []rune{',', '\t'} {
		if n := strings.Count(first, string(c)); n > best {
			sep, best = c, n
		}
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV: %w", err)
	}
	return trimRows(rows), nil
}

// trimRows oreže medzery a vynechá prázdne riadky.
func trimRows(rows [][]string) [][]string {
	out := rows[:0]
	for _, r := range rows {
		empty := true
		for i := range r {
			r[i] = strings.TrimSpace(r[i])
			empty = empty && r[i] == ""
		}
		if !empty {
			out = append(out, r)
		}
	}
	return out
}

// cp1250 sú znaky Windows-1250 pre bajty 0x80–0xFF.
var cp1250 = []rune("€�‚�„…†‡�‰Š‹ŚŤŽŹ�‘’“”•–—�™š›śťžź ˇ˘Ł¤Ą¦§¨©Ş«¬­®Ż°±˛ł´µ¶·¸ąş»Ľ˝ľżŔÁÂĂÄĹĆÇČÉĘËĚÍÎĎĐŃŇÓÔŐÖ×ŘŮÚŰÜÝŢßŕáâăäĺćçčéęëěíîďđńňóôőö÷řůúűüýţ˙")

func decode1250(b []byte) string {
	var s strings.Builder
	for _, c := range b {
		if c < 0x80 {
			s.WriteByte(c)
		} else {
			s.WriteRune(cp1250[c-0x80])
		}
	}
	return s.String()
}

// readXLSX prečíta prvý hárok zošita.
func readXLSX(data []byte) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrFormat
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	read := func(name string, v any) error {
		f := files[name]
		if f == nil {
			return fmt.Errorf("xlsx: chýba %s", name)
		}
		if f.UncompressedSize64 > 64<<20 {
			return errors.New("xlsx: hárok je príliš veľký")
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return xml.NewDecoder(io.LimitReader(rc, 64<<20)).Decode(v)
	}

	// prvý hárok podľa zošita a jeho vzťahov
	sheet := "xl/worksheets/sheet1.xml"
	var wb struct {
		Sheets []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if read("xl/workbook.xml", &wb) == nil && len(wb.Sheets) > 0 && read("xl/_rels/workbook.xml.rels", &rels) == nil {
		for _, r := range rels.Rels {
			if r.ID == wb.Sheets[0].RID {
				t := strings.TrimPrefix(r.Target, "/")
				if !strings.HasPrefix(t, "xl/") {
					t = path.Join("xl", t)
				}
				sheet = t
			}
		}
	}

	var shared []string
	if files["xl/sharedStrings.xml"] != nil {
		var sst struct {
			SI []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := read("xl/sharedStrings.xml", &sst); err != nil {
			return nil, err
		}
		for _, si := range sst.SI {
			if len(si.R) > 0 {
				var b strings.Builder
				for _, r := range si.R {
					b.WriteString(r.T)
				}
				shared = append(shared, b.String())
			} else {
				shared = append(shared, si.T)
			}
		}
	}

	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref string `xml:"r,attr"`
				T   string `xml:"t,attr"`
				V   string `xml:"v"`
				IS  struct {
					T string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := read(sheet, &ws); err != nil {
		return nil, err
	}
	var rows [][]string
	for _, r := range ws.Rows {
		var row []string
		for i, c := range r.Cells {
			col := i
			if c.Ref != "" {
				col = colIndex(c.Ref)
			}
			v := c.V
			switch c.T {
			case "s":
				n, err := strconv.Atoi(c.V)
				if err != nil || n < 0 || n >= len(shared) {
					return nil, errors.New("xlsx: chybný odkaz na text")
				}
				v = shared[n]
			case "inlineStr":
				v = c.IS.T
			case "b":
				v = map[string]string{"1": "áno", "0": ""}[c.V]
			}
			for len(row) <= col {
				row = append(row, "")
			}
			row[col] = v
		}
		rows = append(rows, row)
	}
	return trimRows(rows), nil
}

// colIndex prevedie odkaz bunky („C12“) na index stĺpca (0 = A).
func colIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		n = n*26 + int(r-'A'+1)
	}
	return n - 1
}
