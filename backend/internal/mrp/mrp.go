// Package mrp číta XML tabuľky z programu MRP-K/S (MRP Slovensko):
// vydané faktúry FAKVY.XML, ich položky FAKVYPOL.XML a adresár
// FV_ADRES.XML — samostatne alebo zabalené v ZIP. Každá tabuľka má
// tvar document/datasets/dataset0/rows/row/fields s poľami ako prvkami
// (kódovanie Windows-1250, dátumy RRRR-MM-DD). Druh tabuľky sa určí
// podľa názvu súboru, inak podľa polí (idfak+cislo, idr+cenamj, idradr).
package mrp

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/xmlenc"
)

// Row sú polia jedného riadku tabuľky (kľúče malými písmenami).
type Row map[string]string

// Get vráti orezanú hodnotu poľa.
func (r Row) Get(k string) string { return strings.TrimSpace(r[k]) }

// Data sú načítané tabuľky.
type Data struct {
	Invoices  []Row // FAKVY
	Items     []Row // FAKVYPOL
	Addresses []Row // FV_ADRES
	Unknown   []string
}

// ErrNotMRP — súbor nie je XML tabuľka z MRP.
var ErrNotMRP = errors.New("súbor nie je XML z MRP-K/S (FAKVY.XML, FAKVYPOL.XML, FV_ADRES.XML alebo ZIP s nimi)")

// ReadTable prečíta jednu XML tabuľku MRP.
func ReadTable(data []byte) ([]Row, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = xmlenc.CharsetReader
	var rows []Row
	var stack []string
	var cur Row
	field := ""
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("chybné XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if len(stack) == 0 && name != "document" {
				return nil, ErrNotMRP
			}
			stack = append(stack, name)
			switch {
			case name == "fields" && len(stack) >= 2 && stack[len(stack)-2] == "row":
				cur = Row{}
			case cur != nil && len(stack) >= 2 && stack[len(stack)-2] == "fields":
				field = name
				text.Reset()
			}
		case xml.CharData:
			if field != "" {
				text.Write(t)
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch {
			case field != "" && name == field:
				cur[field] = text.String()
				field = ""
			case name == "fields" && cur != nil:
				rows = append(rows, cur)
				cur = nil
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if stack == nil {
		return nil, ErrNotMRP
	}
	return rows, nil
}

// kind určí tabuľku podľa názvu súboru, inak podľa polí prvého riadku.
func kind(name string, rows []Row) string {
	n := strings.ToUpper(strings.TrimSuffix(path.Base(name), path.Ext(name)))
	switch {
	case n == "FAKVYPOL":
		return "items"
	case n == "FAKVY":
		return "invoices"
	case strings.Contains(n, "ADRES") && !strings.Contains(n, "BAN"):
		return "addresses"
	case strings.Contains(n, "BAN"):
		return "bank" // bankové účty partnerov — Fiškus ich neprenáša
	}
	if len(rows) == 0 {
		return ""
	}
	r := rows[0]
	_, idfak := r["idfak"]
	_, cislo := r["cislo"]
	_, cena := r["cenamj"]
	_, idradr := r["idradr"]
	switch {
	case idfak && cena:
		return "items"
	case idfak && cislo:
		return "invoices"
	case idradr:
		return "addresses"
	}
	return ""
}

// Parse prečíta jednu tabuľku alebo ZIP s tabuľkami (maxBytes obmedzí
// rozbalený obsah).
func Parse(data []byte, name string, maxBytes int64) (Data, error) {
	var d Data
	add := func(name string, b []byte) error {
		rows, err := ReadTable(b)
		if err != nil {
			return err
		}
		switch kind(name, rows) {
		case "invoices":
			d.Invoices = append(d.Invoices, rows...)
		case "items":
			d.Items = append(d.Items, rows...)
		case "addresses":
			d.Addresses = append(d.Addresses, rows...)
		case "bank":
		default:
			d.Unknown = append(d.Unknown, path.Base(name))
		}
		return nil
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		if err := add(name, data); err != nil {
			return d, err
		}
		return d, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return d, fmt.Errorf("poškodený archív ZIP: %w", err)
	}
	var total int64
	files := 0
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || !strings.EqualFold(path.Ext(zf.Name), ".xml") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return d, fmt.Errorf("%s: %w", zf.Name, err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxBytes-total+1))
		rc.Close()
		if err != nil {
			return d, fmt.Errorf("%s: %w", zf.Name, err)
		}
		if total += int64(len(b)); total > maxBytes {
			return d, fmt.Errorf("rozbalený archív je väčší ako %d MB", maxBytes>>20)
		}
		if err := add(zf.Name, b); err != nil {
			return d, fmt.Errorf("%s: %w", path.Base(zf.Name), err)
		}
		files++
	}
	if files == 0 {
		return d, ErrNotMRP
	}
	return d, nil
}
