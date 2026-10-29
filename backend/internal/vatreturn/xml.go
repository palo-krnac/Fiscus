package vatreturn

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// KVNamespace je menný priestor schémy kontrolného výkazu KVDPHv25.
const KVNamespace = "https://ekr.financnasprava.sk/Formulare/XSD/kv_dph_2025.xsd"

type xw struct{ b bytes.Buffer }

func (w *xw) raw(s string) { w.b.WriteString(s) }

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// el zapíše <name>text</name>.
func (w *xw) el(name, text string) { fmt.Fprintf(&w.b, "<%s>%s</%s>", name, esc(text), name) }

func amount(d money.Dec) string {
	if d.IsZero() {
		return ""
	}
	return d.RoundCents().String()
}

// amountReq je povinná suma (v KV aj nula).
func amountReq(d money.Dec) string { return d.RoundCents().String() }

// splitStreet rozdelí „Námestie SNP 12/A“ na ulicu a číslo.
func splitStreet(s string) (street, number string) {
	s = strings.TrimSpace(s)
	i := strings.LastIndex(s, " ")
	if i > 0 && strings.IndexFunc(s[i+1:], unicode.IsDigit) >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func dpDate(d invoice.Date) string {
	if d.IsZero() {
		return ""
	}
	y, m, day := d.String()[:4], d.String()[5:7], d.String()[8:10]
	return day + "." + m + "." + y
}

// DPHXML vytvorí riadne daňové priznanie k DPH vo formáte eDane (DPHv25).
func DPHXML(r Return, tp Taxpayer, declared invoice.Date) []byte {
	var w xw
	b01 := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	street, number := splitStreet(tp.Street)
	w.raw(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<dokument><hlavicka>")
	w.raw("<identifikacneCislo>")
	w.el("kodStatu", "SK")
	w.el("cislo", digits(tp.ICDPH))
	w.raw("</identifikacneCislo>")
	w.el("dic", digits(tp.DIC))
	w.el("danovyUrad", tp.TaxOffice)
	w.el("nevzniklaPov", "0")
	w.raw("<typDP>")
	w.el("rdp", "1")
	w.el("odp", "0")
	w.el("ddp", "0")
	w.el("datumZisteniaDdp", "")
	w.raw("</typDP><osoba>")
	w.el("platitel", "1")
	w.el("registrovana", "0")
	w.el("inaPovinna", "0")
	w.el("zdanitelna", "0")
	w.el("zastupca", "0")
	w.el("zastupca69aa", "0")
	w.raw("</osoba><zdanObd>")
	month, quarter := "", ""
	if r.Period.Quarter > 0 {
		quarter = strconv.Itoa(r.Period.Quarter)
	} else {
		month = strconv.Itoa(r.Period.Month)
	}
	w.el("mesiac", month)
	w.el("stvrtrok", quarter)
	w.el("rok", strconv.Itoa(r.Period.Year))
	w.raw("</zdanObd><meno>")
	w.el("riadok", tp.Name)
	w.raw("</meno><adresa>")
	w.el("ulica", street)
	w.el("cislo", number)
	w.el("psc", strings.ReplaceAll(tp.Zip, " ", ""))
	w.el("obec", tp.City)
	w.el("telefon", tp.Phone)
	w.el("email", tp.Email)
	w.raw("</adresa><opravnenaOsoba>")
	w.el("menoPriezvisko", "")
	w.el("telefon", "")
	w.el("email", "")
	w.raw("</opravnenaOsoba>")
	w.el("datumVyhlasenia", dpDate(declared))
	w.raw("</hlavicka><telo>")
	for _, row := range Rows {
		w.el(row.Code, amount(r.Rows[row.Code]))
		if row.Code == "r32" {
			w.el("splneniePodmienok", b01(false))
		}
	}
	w.raw("</telo></dokument>\n")
	return w.b.Bytes()
}

// KVXML vytvorí riadny kontrolný výkaz vo formáte eDane (KVDPHv25).
func KVXML(r Return, tp Taxpayer) []byte {
	var w xw
	street, number := splitStreet(tp.Street)
	w.raw(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	w.raw(`<KVDPH_2025 xmlns="` + KVNamespace + `"><Identifikacia>`)
	w.el("IcDphPlatitela", strings.ToUpper(noSpaces(tp.ICDPH)))
	w.el("Druh", "R")
	w.raw("<Obdobie>")
	w.el("Rok", strconv.Itoa(r.Period.Year))
	if r.Period.Quarter > 0 {
		w.el("Stvrtrok", strconv.Itoa(r.Period.Quarter))
	} else {
		w.el("Mesiac", strconv.Itoa(r.Period.Month))
	}
	w.raw("</Obdobie>")
	w.el("Nazov", tp.Name)
	w.el("Stat", "Slovenská republika")
	w.el("Obec", tp.City)
	optional := func(name, v string) {
		if v != "" {
			w.el(name, v)
		}
	}
	optional("PSC", strings.ReplaceAll(tp.Zip, " ", ""))
	optional("Ulica", street)
	optional("Cislo", number)
	optional("Tel", tp.Phone)
	optional("Email", tp.Email)
	w.raw("</Identifikacia><Transakcie>")
	attr := func(name, v string) string { return fmt.Sprintf(` %s="%s"`, name, esc(v)) }
	for _, l := range r.KV.A1 {
		w.raw("<A1" + attr("Odb", l.Partner) + attr("F", l.Number) + attr("Den", l.Date.String()) +
			attr("Z", amountReq(l.Base)) + attr("D", amountReq(l.VAT)) + attr("S", strconv.Itoa(int(l.Rate))) + "/>")
	}
	for _, l := range r.KV.A2 {
		el := "<A2" + attr("Odb", l.Partner) + attr("F", l.Number) + attr("Den", l.Date.String()) + attr("Z", amountReq(l.Base))
		if l.Code != "" {
			el += attr("TK", l.Code)
		}
		if l.Goods != "" {
			el += attr("TD", l.Goods)
		}
		if l.Quantity != nil {
			el += attr("Mn", amountReq(*l.Quantity)) + attr("MJ", l.Unit)
		}
		w.raw(el + "/>")
	}
	for _, l := range r.KV.B1 {
		odb := ""
		if l.Partner != "" {
			odb = attr("Dod", l.Partner)
		}
		w.raw("<B1" + odb + attr("F", l.Number) + attr("Den", l.Date.String()) +
			attr("Z", amountReq(l.Base)) + attr("D", amountReq(l.VAT)) + attr("S", strconv.Itoa(int(l.Rate))) +
			attr("O", amountReq(l.Deducted)) + "/>")
	}
	for _, l := range r.KV.B2 {
		w.raw("<B2" + attr("Dod", l.Partner) + attr("F", l.Number) + attr("Den", l.Date.String()) +
			attr("Z", amountReq(l.Base)) + attr("D", amountReq(l.VAT)) + attr("S", strconv.Itoa(int(l.Rate))) +
			attr("O", amountReq(l.Deducted)) + "/>")
	}
	for _, c := range r.KV.C1 {
		w.raw("<C1" + attr("Odb", c.Partner) + attr("FO", c.Number) + attr("FP", c.Original) +
			attr("ZR", amountReq(c.BaseDiff)) + attr("DR", amountReq(c.VATDiff)) + attr("S", strconv.Itoa(int(c.Rate))) + "/>")
	}
	for _, c := range r.KV.C2 {
		w.raw("<C2" + attr("Dod", c.Partner) + attr("FO", c.Number) + attr("FP", c.Original) +
			attr("ZR", amountReq(c.BaseDiff)) + attr("DR", amountReq(c.VATDiff)) + attr("S", strconv.Itoa(int(c.Rate))) +
			attr("OR", amountReq(c.DeductedDiff)) + "/>")
	}
	if d := r.KV.D1; d.Count > 0 {
		w.raw("<D1" + attr("Z", amountReq(d.Base)) + attr("D", amountReq(d.VAT)) +
			attr("ZZn", amountReq(d.BaseReduced)) + attr("DZn", amountReq(d.VATReduced)) + "/>")
	}
	if d := r.KV.D2; d.Count > 0 {
		w.raw("<D2" + attr("Z", amountReq(d.Base)) + attr("D", amountReq(d.VAT)) +
			attr("ZZn", amountReq(d.BaseReduced)) + attr("DZn", amountReq(d.VATReduced)) + "/>")
	}
	w.raw("</Transakcie></KVDPH_2025>\n")
	return w.b.Bytes()
}
