// Package ubl vytvára a číta e-faktúry vo formáte UBL 2.1 podľa EN 16931
// a Peppol BIS Billing 3.0 — formát povinnej e-fakturácie na Slovensku od
// 1. 1. 2027. Účastník siete Peppol sa určuje schémou 0245 a DIČ.
package ubl

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Identifikátory špecifikácie Peppol BIS Billing 3.0.
const (
	CustomizationID = "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0"
	ProfileID       = "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0"
	// SchemeSK je schéma identifikátora účastníka Peppol pre slovenské DIČ.
	SchemeSK = "0245"

	nsInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	nsCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	nsCAC        = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	nsCBC        = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
)

// Seller sú údaje dodávateľa (firma vo Fiškuse).
type Seller struct {
	Name, Street, City, Zip string
	ICO, DIC, ICDPH         string
	IBAN, Email, Phone      string
	VATPayer                bool
}

// Problem je chýbajúci alebo nepodporovaný údaj — e-faktúra by neprešla
// kontrolou poštára.
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// IncompleteError zoskupuje problémy e-faktúry.
type IncompleteError struct{ Problems []Problem }

func (e *IncompleteError) Error() string {
	msgs := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		msgs[i] = p.Message
	}
	return "e-faktúru nemožno vytvoriť: " + strings.Join(msgs, "; ")
}

// ErrKind — druh dokladu sa ako e-faktúra neposiela.
var ErrKind = errors.New("e-faktúra sa vytvára iba z faktúry alebo dobropisu")

// Check zistí, čo e-faktúre chýba.
func Check(inv invoice.Invoice, s Seller) []Problem {
	var out []Problem
	add := func(f, m string) { out = append(out, Problem{f, m}) }
	if inv.Status != invoice.StatusIssued && inv.Status != invoice.StatusPaid {
		add("status", "Faktúra ešte nie je vystavená")
	}
	if inv.Kind != invoice.KindInvoice && inv.Kind != invoice.KindCreditNote {
		add("kind", ErrKind.Error())
	}
	if inv.Advance != nil && len(inv.Advance.Deductions) > 0 {
		add("advance", "Vyúčtovanie zálohy ako e-faktúra zatiaľ nie je podporované")
	}
	if digits(s.DIC) == "" {
		add("seller.dic", "Doplňte DIČ firmy v nastaveniach — je to adresa v sieti Peppol (0245:DIČ)")
	}
	if strings.TrimSpace(s.Street) == "" || strings.TrimSpace(s.City) == "" {
		add("seller.address", "Doplňte adresu firmy v nastaveniach")
	}
	if s.VATPayer && s.ICDPH == "" {
		add("seller.icdph", "Doplňte IČ DPH firmy v nastaveniach")
	}
	c := inv.Customer
	if !party.Foreign(c.Country) && digits(c.DIC) == "" {
		add("customer.dic", "Doplňte DIČ odberateľa — je to jeho adresa v sieti Peppol")
	}
	if party.Foreign(c.Country) && (noSpaces(c.ICDPH) == "" || easVAT[strings.ToUpper(c.Country)] == "") {
		add("customer.icdph", "Zahraničný odberateľ potrebuje IČ DPH z EÚ — e-faktúra cez Peppol je povinná iba v tuzemsku")
	}
	if strings.TrimSpace(c.City) == "" {
		add("customer.city", "Doplňte obec odberateľa")
	}
	if inv.PaymentMethod == invoice.PayTransfer && s.IBAN == "" {
		add("seller.iban", "Doplňte IBAN firmy v nastaveniach")
	}
	return out
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// category je daňová kategória EN 16931 (UNCL5305) pre riadok rekapitulácie.
type category struct {
	ID, Reason, ReasonCode string
	Percent                money.Dec
}

func taxCategory(inv invoice.Invoice, s Seller, rate vat.Rate, reverse bool) category {
	pct := money.FromInt(int64(rate))
	c := inv.Customer
	switch {
	case !s.VATPayer || inv.NonVATPayer:
		return category{ID: "O", Reason: "Dodávateľ nie je platiteľ DPH", ReasonCode: "VATEX-EU-O"}
	case reverse:
		return category{ID: "AE", Reason: "Prenesenie daňovej povinnosti (§ 69 ods. 12 zákona o DPH)", ReasonCode: "VATEX-EU-AE"}
	case inv.ReverseCharge && party.Foreign(c.Country):
		eu := party.IsEU(c.Country)
		switch {
		case eu && inv.SupplyKind == invoice.SupplyTriangular:
			return category{ID: "AE", Reason: "Trojstranný obchod (§ 45) – daň platí odberateľ", ReasonCode: "VATEX-EU-AE"}
		case eu && inv.SupplyKind == invoice.SupplyGoods:
			return category{ID: "K", Reason: "Dodanie tovaru do iného členského štátu oslobodené od dane (§ 43)", ReasonCode: "VATEX-EU-IC"}
		case eu:
			return category{ID: "AE", Reason: "Prenesenie daňovej povinnosti – daň platí príjemca služby", ReasonCode: "VATEX-EU-AE"}
		case inv.SupplyKind.Goods():
			return category{ID: "G", Reason: "Vývoz tovaru oslobodený od dane (§ 47)", ReasonCode: "VATEX-EU-G"}
		default:
			return category{ID: "O", Reason: "Miesto dodania služby je mimo územia SR", ReasonCode: "VATEX-EU-O"}
		}
	case rate == 0:
		return category{ID: "E", Reason: "Oslobodené od dane"}
	}
	return category{ID: "S", Percent: pct}
}

// UNECE Rec. 20 — kódy merných jednotiek.
var unitCodes = map[string]string{
	"ks": "H87", "kus": "H87", "hod": "HUR", "h": "HUR", "min": "MIN", "deň": "DAY", "den": "DAY",
	"týž": "WEE", "mes": "MON", "mesiac": "MON", "rok": "ANN", "m": "MTR", "bm": "MTR", "km": "KMT",
	"m2": "MTK", "m²": "MTK", "m3": "MTQ", "m³": "MTQ", "kg": "KGM", "g": "GRM", "t": "TNE",
	"l": "LTR", "kwh": "KWH", "bal": "XPK", "sada": "SET", "pár": "PR",
}

// UnitCode prevedie mernú jednotku na kód UN/ECE (neznáma = C62, „kus“ bez jednotky).
func UnitCode(u string) string {
	if c, ok := unitCodes[strings.ToLower(strings.TrimSuffix(strings.TrimSpace(u), "."))]; ok {
		return c
	}
	return "C62"
}

// Write vytvorí e-faktúru (UBL Invoice, pri dobropise CreditNote).
func Write(inv invoice.Invoice, s Seller) ([]byte, error) {
	if ps := Check(inv, s); len(ps) > 0 {
		return nil, &IncompleteError{ps}
	}
	credit := inv.Kind == invoice.KindCreditNote
	sign := money.FromInt(1)
	if credit {
		sign = money.FromInt(-1) // dobropis v UBL má kladné sumy
	}
	cur := inv.Currency
	if cur == "" {
		cur = "EUR"
	}
	amt := func(d money.Dec) string { return d.Mul(sign).RoundCents().String() }

	var w xw
	root, ns := "Invoice", nsInvoice
	if credit {
		root, ns = "CreditNote", nsCreditNote
	}
	w.raw(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	w.raw(fmt.Sprintf(`<%s xmlns="%s" xmlns:cac="%s" xmlns:cbc="%s">`, root, ns, nsCAC, nsCBC))
	w.el("cbc:CustomizationID", CustomizationID)
	w.el("cbc:ProfileID", ProfileID)
	w.el("cbc:ID", inv.Number)
	w.el("cbc:IssueDate", inv.IssueDate.String())
	if !credit && !inv.DueDate.IsZero() {
		w.el("cbc:DueDate", inv.DueDate.String())
	}
	if credit {
		w.el("cbc:CreditNoteTypeCode", "381")
	} else {
		w.el("cbc:InvoiceTypeCode", "380")
	}
	// Peppol pripúšťa iba jednu poznámku dokladu (PEPPOL-EN16931-R002).
	if ns := notes(inv); len(ns) > 0 {
		w.el("cbc:Note", strings.Join(ns, "\n"))
	}
	w.el("cbc:DocumentCurrencyCode", cur)
	if cur != "EUR" {
		w.el("cbc:TaxCurrencyCode", "EUR")
	}
	// BT-10: Peppol vyžaduje referenciu kupujúceho alebo objednávku — variabilný symbol
	w.el("cbc:BuyerReference", firstNonEmpty(inv.VariableSymbol, inv.Number))
	if credit && inv.Correction != nil && inv.Correction.OriginalNumber != "" {
		w.raw("<cac:BillingReference><cac:InvoiceDocumentReference>")
		w.el("cbc:ID", inv.Correction.OriginalNumber)
		w.raw("</cac:InvoiceDocumentReference></cac:BillingReference>")
	}
	w.party("cac:AccountingSupplierParty", partyData{
		Name: s.Name, Street: s.Street, City: s.City, Zip: s.Zip, Country: "SK",
		ICO: s.ICO, DIC: s.DIC, ICDPH: vatID(s.VATPayer, s.ICDPH), Email: s.Email, Phone: s.Phone,
	})
	c := inv.Customer
	w.party("cac:AccountingCustomerParty", partyData{
		Name: c.Name, Street: c.Street, City: c.City, Zip: c.Zip, Country: firstNonEmpty(c.Country, "SK"),
		ICO: c.ICO, DIC: c.DIC, ICDPH: c.ICDPH, Email: c.Email,
	})
	w.raw("<cac:Delivery>")
	w.el("cbc:ActualDeliveryDate", firstNonEmpty(inv.DeliveryDate.String(), inv.IssueDate.String()))
	w.raw("</cac:Delivery>")
	w.paymentMeans(inv, s, credit)

	// rekapitulácia DPH
	w.raw("<cac:TaxTotal>")
	w.raw(fmt.Sprintf(`<cbc:TaxAmount currencyID="%s">%s</cbc:TaxAmount>`, cur, amt(inv.Summary.VAT)))
	for _, r := range inv.Summary.Rates {
		cat := taxCategory(inv, s, r.Rate, r.Reverse)
		w.raw("<cac:TaxSubtotal>")
		w.raw(fmt.Sprintf(`<cbc:TaxableAmount currencyID="%s">%s</cbc:TaxableAmount>`, cur, amt(r.Base)))
		w.raw(fmt.Sprintf(`<cbc:TaxAmount currencyID="%s">%s</cbc:TaxAmount>`, cur, amt(r.VAT)))
		w.taxCategory("cac:TaxCategory", cat, true)
		w.raw("</cac:TaxSubtotal>")
	}
	w.raw("</cac:TaxTotal>")
	if cur != "EUR" && inv.Local != nil {
		// BR-53: DPH v mene účtovania (euro)
		w.raw(fmt.Sprintf(`<cac:TaxTotal><cbc:TaxAmount currencyID="EUR">%s</cbc:TaxAmount></cac:TaxTotal>`, amt(inv.Local.VAT)))
	}

	w.raw("<cac:LegalMonetaryTotal>")
	for _, m := range []struct {
		el string
		v  money.Dec
	}{
		{"LineExtensionAmount", inv.Summary.Base}, {"TaxExclusiveAmount", inv.Summary.Base},
		{"TaxInclusiveAmount", inv.Summary.Total}, {"PayableAmount", inv.Summary.Total},
	} {
		w.raw(fmt.Sprintf(`<cbc:%s currencyID="%s">%s</cbc:%s>`, m.el, cur, amt(m.v), m.el))
	}
	w.raw("</cac:LegalMonetaryTotal>")

	for i, l := range netLines(inv) {
		lineEl, qtyEl := "cac:InvoiceLine", "cbc:InvoicedQuantity"
		if credit {
			lineEl, qtyEl = "cac:CreditNoteLine", "cbc:CreditedQuantity"
		}
		w.raw("<" + lineEl + ">")
		w.el("cbc:ID", fmt.Sprint(i+1))
		w.raw(fmt.Sprintf(`<%s unitCode="%s">%s</%s>`, qtyEl, UnitCode(l.Unit), l.Quantity.Mul(sign).String(), qtyEl))
		w.raw(fmt.Sprintf(`<cbc:LineExtensionAmount currencyID="%s">%s</cbc:LineExtensionAmount>`, cur, amt(l.net)))
		w.raw("<cac:Item>")
		w.el("cbc:Name", firstNonEmpty(strings.TrimSpace(l.Description), "Položka"))
		if l.Commodity != "" {
			// kód colného sadzobníka (KN/HS) pri tuzemskom prenesení
			w.raw(`<cac:CommodityClassification><cbc:ItemClassificationCode listID="HS">` + esc(invoice.CommodityCode(l.Commodity)) + `</cbc:ItemClassificationCode></cac:CommodityClassification>`)
		}
		w.taxCategory("cac:ClassifiedTaxCategory", taxCategory(inv, s, l.Rate, l.Reverse != ""), false)
		w.raw("</cac:Item>")
		w.raw("<cac:Price>")
		if l.base != nil {
			// ceny s DPH: cena bez DPH za celé množstvo (súčet riadkov sedí na základ)
			w.raw(fmt.Sprintf(`<cbc:PriceAmount currencyID="%s">%s</cbc:PriceAmount>`, cur, l.net.RoundCents().Mul(sign).Mul(signOf(l.Quantity)).String()))
			w.raw(fmt.Sprintf(`<cbc:BaseQuantity unitCode="%s">%s</cbc:BaseQuantity>`, UnitCode(l.Unit), l.Quantity.Mul(sign).String()))
		} else {
			w.raw(fmt.Sprintf(`<cbc:PriceAmount currencyID="%s">%s</cbc:PriceAmount>`, cur, l.UnitPrice.String()))
		}
		w.raw("</cac:Price>")
		w.raw("</" + lineEl + ">")
	}
	w.raw("</" + root + ">\n")
	return w.b.Bytes(), nil
}

func signOf(d money.Dec) money.Dec {
	if d.Sign() < 0 {
		return money.FromInt(-1)
	}
	return money.FromInt(1)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func vatID(payer bool, id string) string {
	if !payer {
		return ""
	}
	return id
}

// notes sú predmet, text nad položkami, povinné texty (oslobodenie,
// prenesenie) a text pod položkami.
func notes(inv invoice.Invoice) []string {
	var out []string
	for _, t := range []string{inv.Subject, inv.Intro} {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	if t := invoice.ExemptionNote(inv.ReverseCharge, inv.Customer, inv.SupplyKind); t != "" && !inv.NonVATPayer {
		out = append(out, t)
	}
	if t := invoice.DomesticReverseNote(inv.Lines); t != "" && !inv.NonVATPayer {
		out = append(out, t)
	}
	if inv.NonVATPayer {
		out = append(out, "Dodávateľ nie je platiteľ DPH.")
	}
	if n := strings.TrimSpace(inv.Note); n != "" {
		out = append(out, n)
	}
	return out
}

type netLine struct {
	invoice.Line
	net  money.Dec
	base *money.Dec // pri cenách s DPH: cena bez DPH za celé množstvo
}

// netLines vráti sumy riadkov bez DPH. Pri cenách s DPH sa základ sadzby
// rozdelí na riadky a rozdiel zaokrúhlenia dostane posledný riadok sadzby,
// aby súčet riadkov presne sedel na základ v rekapitulácii (BR-CO-10, BR-S-08).
func netLines(inv invoice.Invoice) []netLine {
	out := make([]netLine, len(inv.Lines))
	gross := inv.PriceMode == vat.PriceGross && !inv.ReverseCharge && !inv.NonVATPayer
	for i, l := range inv.Lines {
		out[i] = netLine{Line: l, net: l.Amount}
	}
	if !gross {
		return out
	}
	for _, r := range inv.Summary.Rates {
		last, sum := -1, money.Zero
		for i, l := range inv.Lines {
			if l.Rate != r.Rate || (l.Reverse != "") != r.Reverse {
				continue
			}
			net := l.Amount
			if !r.Reverse {
				net = l.Amount.MulRatio(money.FromInt(100), money.FromInt(100+int64(l.Rate))).RoundCents()
			}
			out[i].net = net
			b := net
			out[i].base = &b
			sum = sum.Add(net)
			last = i
		}
		if last >= 0 {
			out[last].net = out[last].net.Add(r.Base.Sub(sum))
			b := out[last].net
			out[last].base = &b
		}
	}
	return out
}

// ---- zápis XML ----

type xw struct{ b bytes.Buffer }

func (w *xw) raw(s string)      { w.b.WriteString(s) }
func (w *xw) el(name, v string) { w.raw("<" + name + ">" + esc(v) + "</" + name + ">") }
func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

type partyData struct {
	Name, Street, City, Zip, Country string
	ICO, DIC, ICDPH, Email, Phone    string
}

func (w *xw) party(el string, p partyData) {
	w.raw("<" + el + "><cac:Party>")
	if d := digits(p.DIC); d != "" && strings.EqualFold(p.Country, "SK") {
		w.raw(`<cbc:EndpointID schemeID="` + SchemeSK + `">` + d + `</cbc:EndpointID>`)
	} else if id, scheme := noSpaces(p.ICDPH), easVAT[strings.ToUpper(p.Country)]; id != "" && scheme != "" {
		w.raw(`<cbc:EndpointID schemeID="` + scheme + `">` + esc(strings.ToUpper(id)) + `</cbc:EndpointID>`)
	}
	if ico := party.NormalizeID(p.ICO); ico != "" {
		w.raw("<cac:PartyIdentification>")
		w.el("cbc:ID", ico)
		w.raw("</cac:PartyIdentification>")
	}
	w.raw("<cac:PartyName>")
	w.el("cbc:Name", p.Name)
	w.raw("</cac:PartyName><cac:PostalAddress>")
	if p.Street != "" {
		w.el("cbc:StreetName", p.Street)
	}
	w.el("cbc:CityName", p.City)
	if p.Zip != "" {
		w.el("cbc:PostalZone", p.Zip)
	}
	w.raw("<cac:Country>")
	w.el("cbc:IdentificationCode", strings.ToUpper(p.Country))
	w.raw("</cac:Country></cac:PostalAddress>")
	if id := noSpaces(p.ICDPH); id != "" {
		w.raw("<cac:PartyTaxScheme>")
		w.el("cbc:CompanyID", strings.ToUpper(id))
		w.raw("<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>")
	}
	w.raw("<cac:PartyLegalEntity>")
	w.el("cbc:RegistrationName", p.Name)
	if ico := party.NormalizeID(p.ICO); ico != "" {
		w.el("cbc:CompanyID", ico)
	}
	w.raw("</cac:PartyLegalEntity>")
	if p.Email != "" || p.Phone != "" {
		w.raw("<cac:Contact>")
		if p.Phone != "" {
			w.el("cbc:Telephone", p.Phone)
		}
		if p.Email != "" {
			w.el("cbc:ElectronicMail", p.Email)
		}
		w.raw("</cac:Contact>")
	}
	w.raw("</cac:Party></" + el + ">")
}

// easVAT sú schémy Peppol (EAS) pre IČ DPH partnerov z EÚ.
var easVAT = map[string]string{
	"AT": "9914", "BE": "9925", "BG": "9926", "CY": "9928", "CZ": "9929", "DE": "9930", "EE": "9931",
	"GR": "9933", "ES": "9920", "FR": "9957", "HR": "9934", "HU": "9910", "IE": "9935", "IT": "9906",
	"LT": "9937", "LU": "9938", "LV": "9939", "MT": "9943", "NL": "9944", "PL": "9945", "PT": "9946",
	"RO": "9947", "SI": "9949", "SK": "9950", "SE": "9955", "GB": "9932",
}

func noSpaces(s string) string { return strings.Join(strings.Fields(s), "") }

func (w *xw) taxCategory(el string, c category, withReason bool) {
	w.raw("<" + el + ">")
	w.el("cbc:ID", c.ID)
	if c.ID != "O" {
		w.el("cbc:Percent", c.Percent.RoundCents().String())
	}
	if withReason && c.ID != "S" {
		if c.ReasonCode != "" {
			w.el("cbc:TaxExemptionReasonCode", c.ReasonCode)
		}
		w.el("cbc:TaxExemptionReason", c.Reason)
	}
	w.raw("<cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></" + el + ">")
}

func (w *xw) paymentMeans(inv invoice.Invoice, s Seller, credit bool) {
	code := map[invoice.PaymentMethod]string{
		invoice.PayTransfer: "58", invoice.PayCash: "10", invoice.PayCard: "48", invoice.PayOffset: "97",
	}[inv.PaymentMethod]
	if code == "" {
		code = "1"
	}
	w.raw("<cac:PaymentMeans>")
	w.el("cbc:PaymentMeansCode", code)
	if credit && !inv.DueDate.IsZero() {
		w.el("cbc:PaymentDueDate", inv.DueDate.String())
	}
	if inv.VariableSymbol != "" {
		w.el("cbc:PaymentID", inv.VariableSymbol)
	}
	if inv.PaymentMethod == invoice.PayTransfer && s.IBAN != "" {
		w.raw("<cac:PayeeFinancialAccount>")
		w.el("cbc:ID", noSpaces(strings.ToUpper(s.IBAN)))
		w.el("cbc:Name", s.Name)
		w.raw("</cac:PayeeFinancialAccount>")
	}
	w.raw("</cac:PaymentMeans>")
}
