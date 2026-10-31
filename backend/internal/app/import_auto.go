package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"path"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/moneys3"
	"github.com/palo-krnac/fiscus/backend/internal/mrp"
	"github.com/palo-krnac/fiscus/backend/internal/omega"
	"github.com/palo-krnac/fiscus/backend/internal/pohoda"
	"github.com/palo-krnac/fiscus/backend/internal/xmlenc"
)

// Fakturačné programy (SuperFaktúra, Fakturoid…) exportujú pre účtovníka
// do formátov účtovných programov: POHODA XML, Money S3 XML, KROS Omega
// TXT alebo MRP. ImportExport formát rozpozná sám a prenesie ho rovnako
// ako pri danom programe; adresár v týchto exportoch nebýva, preto sa
// kontakty založia z faktúr.

// Formáty exportu.
const (
	fmtPohoda = "pohoda"
	fmtMoney  = "money"
	fmtOmega  = "omega"
	fmtMRP    = "mrp"
)

// xmlRoot vráti názov koreňového prvku XML ("" ak to nie je XML).
func xmlRoot(b []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.CharsetReader = xmlenc.CharsetReader
	for i := 0; i < 50; i++ {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local
		}
	}
	return ""
}

// formatOf rozpozná formát jedného súboru.
func formatOf(b []byte) string {
	if bytes.HasPrefix(bytes.TrimLeft(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), " \r\n"), []byte("R00")) {
		return fmtOmega
	}
	switch strings.ToLower(xmlRoot(b)) {
	case "datapack", "responsepack":
		return fmtPohoda
	case "moneydata":
		return fmtMoney
	case "document":
		return fmtMRP
	}
	return ""
}

// sniffFormat rozpozná formát súboru alebo archívu ZIP (podľa prvého
// rozpoznaného súboru v ňom).
func sniffFormat(data []byte) string {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return formatOf(data)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	for _, zf := range zr.File {
		ext := strings.ToLower(path.Ext(zf.Name))
		if zf.FileInfo().IsDir() || (ext != ".xml" && ext != ".txt") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		head, _ := io.ReadAll(io.LimitReader(rc, 64<<10))
		rc.Close()
		if f := formatOf(head); f != "" {
			return f
		}
	}
	return ""
}

// firstOmega vráti prvý súbor TXT z archívu (Omega má jeden súbor).
func firstOmega(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return data
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return data
	}
	for _, zf := range zr.File {
		if strings.EqualFold(path.Ext(zf.Name), ".txt") {
			rc, err := zf.Open()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(io.LimitReader(rc, MaxMigrateBytes))
			rc.Close()
			return b
		}
	}
	return data
}

// ImportExport prenesie export z fakturačného programu (source je jeho
// názov do prehľadu a auditu; apply=false: iba náhľad).
func (a *App) ImportExport(ctx context.Context, companyID int64, source string, data []byte, opt MigrateOptions, apply bool) (ImportReport, error) {
	if err := a.requireMigrate(ctx, companyID); err != nil {
		return ImportReport{}, err
	}
	empty := ImportReport{Source: source, Warnings: []string{}, Other: map[string]int{}}
	var m migration
	switch sniffFormat(data) {
	case fmtPohoda:
		d, err := pohoda.ParseArchive(data, MaxMigrateBytes)
		if err != nil {
			return empty, mfaFieldErr("file", err.Error())
		}
		m = pohodaMigration(d, a.Today())
	case fmtMoney:
		d, err := moneys3.ParseArchive(data, MaxMigrateBytes)
		if err != nil {
			return empty, mfaFieldErr("file", err.Error())
		}
		m = moneyMigration(d)
	case fmtOmega:
		f, err := omega.Parse(firstOmega(data))
		if err != nil {
			return empty, mfaFieldErr("file", err.Error())
		}
		m = omegaMigration(f)
	case fmtMRP:
		d, err := mrp.Parse(data, "", MaxMigrateBytes)
		if err != nil {
			return empty, mfaFieldErr("file", err.Error())
		}
		m = mrpMigration(d)
	default:
		return empty, mfaFieldErr("file", "Súbor nie je export pre POHODU, Money S3, KROS Omega ani MRP — v "+source+" zvoľte export pre účtovníka do jedného z týchto programov")
	}
	if len(m.Contacts)+len(m.Items)+len(m.Invoices)+len(m.Purchases)+len(m.Skips) == 0 {
		empty.Other = m.Other
		return empty, mfaFieldErr("file", "V exporte nie sú faktúry ani kontakty")
	}
	m.Source = source
	m.ContactsFromDocs = true
	return a.migrate(ctx, companyID, m, opt, apply)
}
