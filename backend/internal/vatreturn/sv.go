package vatreturn

import (
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// svPerPage je počet riadkov na jednej strane súhrnného výkazu.
const svPerPage = 12

// SVTotal je celková hodnota plnení v súhrnnom výkaze.
func (r Return) SVTotal() money.Dec {
	var t money.Dec
	for _, l := range r.SV {
		t = t.Add(l.Value)
	}
	return t.RoundCents()
}

// SVXML vytvorí riadny súhrnný výkaz vo formáte eDane (SVDPHv20).
func SVXML(r Return, tp Taxpayer, declared invoice.Date) []byte {
	var w xw
	street, number := splitStreet(tp.Street)
	w.raw(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<dokument><hlavicka><identifikacneCislo>")
	w.el("kodStatu", "SK")
	w.el("dic", digits(tp.ICDPH))
	w.raw("</identifikacneCislo>")
	w.el("danovyUrad", tp.TaxOffice)
	w.raw("<druhSV>")
	w.el("riadny", "1")
	w.el("opravny", "0")
	w.el("dodatocny", "0")
	w.raw("</druhSV><obdobie>")
	month, quarter := "", ""
	if r.Period.Quarter > 0 {
		quarter = strconv.Itoa(r.Period.Quarter)
	} else {
		month = strconv.Itoa(r.Period.Month)
	}
	w.el("mesiac", month)
	w.el("stvrtrok", quarter)
	w.el("rok", strconv.Itoa(r.Period.Year))
	w.raw("</obdobie><obchodneMeno>")
	for _, line := range nameLines(tp.Name, 4) {
		w.el("riadok", line)
	}
	w.raw("</obchodneMeno><adresa>")
	w.el("ulica", street)
	w.el("cislo", number)
	w.el("psc", strings.ReplaceAll(tp.Zip, " ", ""))
	w.el("obec", tp.City)
	w.el("tel", tp.Phone)
	w.el("email", tp.Email)
	w.raw("</adresa>")
	w.el("celkovaHodnota", amount(r.SVTotal()))
	w.el("konatel", "")
	ds := ""
	if !declared.IsZero() {
		ds = declared.String()
	}
	w.raw("<datumVyhlasenia>")
	if ds != "" {
		w.el("den", ds[8:10])
		w.el("mesiac", ds[5:7])
		w.el("rok", ds[:4])
	} else {
		w.el("den", "")
		w.el("mesiac", "")
		w.el("rok", "")
	}
	w.raw("</datumVyhlasenia></hlavicka><telo>")
	for start := 0; start < len(r.SV); start += svPerPage {
		w.raw("<strana>")
		for i := start; i < start+svPerPage; i++ {
			w.raw("<zaznam>")
			if i < len(r.SV) {
				l := r.SV[i]
				w.el("kodStatu", l.Country)
				w.el("idCislo", l.VATID)
				w.el("hodnota", amountReq(l.Value))
				w.el("kod", strconv.Itoa(l.Code))
			} else {
				w.el("kodStatu", "")
				w.el("idCislo", "")
				w.el("hodnota", "")
				w.el("kod", "")
			}
			w.raw("</zaznam>")
		}
		for i := 0; i < svPerPage; i++ {
			w.raw("<zaznamCast2>")
			w.el("kodStatu", "")
			w.el("idCislo", "")
			w.el("idCislo2", "")
			w.el("vratenieTovaru", "")
			w.el("oprava", "")
			w.raw("</zaznamCast2>")
		}
		w.raw("</strana>")
	}
	w.raw("</telo></dokument>\n")
	return w.b.Bytes()
}

// nameLines rozdelí obchodné meno na n riadkov (prázdne doplní).
func nameLines(name string, n int) []string {
	const width = 36
	out := make([]string, 0, n)
	line := ""
	for _, word := range strings.Fields(name) {
		if line != "" && len([]rune(line))+1+len([]rune(word)) > width && len(out) < n-1 {
			out = append(out, line)
			line = word
			continue
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	out = append(out, line)
	for len(out) < n {
		out = append(out, "")
	}
	return out
}
