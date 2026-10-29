package ubl

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// ErrNotUBL — súbor nie je e-faktúra UBL.
var ErrNotUBL = errors.New("súbor nie je e-faktúra UBL (Invoice ani CreditNote)")

type amount struct {
	Value    string `xml:",chardata"`
	Currency string `xml:"currencyID,attr"`
}

type xParty struct {
	Endpoint struct {
		Value  string `xml:",chardata"`
		Scheme string `xml:"schemeID,attr"`
	} `xml:"EndpointID"`
	IDs     []string `xml:"PartyIdentification>ID"`
	Name    string   `xml:"PartyName>Name"`
	Address struct {
		Street  string `xml:"StreetName"`
		City    string `xml:"CityName"`
		Zip     string `xml:"PostalZone"`
		Country string `xml:"Country>IdentificationCode"`
	} `xml:"PostalAddress"`
	TaxIDs []struct {
		CompanyID string `xml:"CompanyID"`
		Scheme    string `xml:"TaxScheme>ID"`
	} `xml:"PartyTaxScheme"`
	Legal struct {
		Name      string `xml:"RegistrationName"`
		CompanyID string `xml:"CompanyID"`
	} `xml:"PartyLegalEntity"`
	Email string `xml:"Contact>ElectronicMail"`
}

type xDoc struct {
	XMLName      xml.Name
	ID           string   `xml:"ID"`
	IssueDate    string   `xml:"IssueDate"`
	DueDate      string   `xml:"DueDate"`
	TaxPointDate string   `xml:"TaxPointDate"`
	Notes        []string `xml:"Note"`
	Currency     string   `xml:"DocumentCurrencyCode"`
	Supplier     xParty   `xml:"AccountingSupplierParty>Party"`
	Delivery     string   `xml:"Delivery>ActualDeliveryDate"`
	Payment      []struct {
		Code    string `xml:"PaymentMeansCode"`
		Due     string `xml:"PaymentDueDate"`
		ID      string `xml:"PaymentID"`
		Account string `xml:"PayeeFinancialAccount>ID"`
	} `xml:"PaymentMeans"`
	TaxTotals []struct {
		Amount    amount `xml:"TaxAmount"`
		Subtotals []struct {
			Base     amount `xml:"TaxableAmount"`
			Tax      amount `xml:"TaxAmount"`
			Category string `xml:"TaxCategory>ID"`
			Percent  string `xml:"TaxCategory>Percent"`
		} `xml:"TaxSubtotal"`
	} `xml:"TaxTotal"`
	Payable string   `xml:"LegalMonetaryTotal>PayableAmount"`
	Items   []string `xml:"InvoiceLine>Item>Name"`
	CItems  []string `xml:"CreditNoteLine>Item>Name"`
}

// Received je prijatá e-faktúra prevedená na prijatú faktúru (koncept na kontrolu).
type Received struct {
	Purchase purchase.Purchase `json:"purchase"`
	Warnings []string          `json:"warnings"`
}

// Read načíta e-faktúru UBL (Invoice alebo CreditNote) ako prijatú faktúru.
func Read(data []byte, today invoice.Date) (Received, error) {
	var d xDoc
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(cs string, r io.Reader) (io.Reader, error) {
		if strings.EqualFold(cs, "utf-8") {
			return r, nil
		}
		return nil, fmt.Errorf("kódovanie %s nie je podporované", cs)
	}
	if err := dec.Decode(&d); err != nil {
		return Received{}, fmt.Errorf("%w: %v", ErrNotUBL, err)
	}
	credit := d.XMLName.Local == "CreditNote"
	if d.XMLName.Local != "Invoice" && !credit {
		return Received{}, ErrNotUBL
	}
	var warn []string
	date := func(s, what string) invoice.Date {
		if s == "" {
			return invoice.Date{}
		}
		dt, err := invoice.ParseDate(strings.TrimSpace(s))
		if err != nil {
			warn = append(warn, "Nečitateľný dátum "+what+": "+s)
		}
		return dt
	}
	sp := d.Supplier
	sup := invoice.Party{
		Name:    firstNonEmpty(sp.Legal.Name, sp.Name),
		Street:  sp.Address.Street,
		City:    sp.Address.City,
		Zip:     sp.Address.Zip,
		Country: strings.ToUpper(firstNonEmpty(sp.Address.Country, "SK")),
		Email:   sp.Email,
	}
	if sp.Endpoint.Scheme == SchemeSK {
		sup.DIC = digits(sp.Endpoint.Value)
	}
	for _, t := range sp.TaxIDs {
		if strings.EqualFold(t.Scheme, "VAT") {
			sup.ICDPH = strings.ToUpper(noSpaces(t.CompanyID))
		}
	}
	// IČO: právnická osoba alebo identifikácia strany (8 číslic)
	for _, id := range append([]string{sp.Legal.CompanyID}, sp.IDs...) {
		if n := party.NormalizeID(id); len(n) == 8 && party.ValidICO(n) {
			sup.ICO = n
			break
		}
	}
	if sup.DIC == "" && strings.HasPrefix(sup.ICDPH, "SK") {
		sup.DIC = strings.TrimPrefix(sup.ICDPH, "SK")
	}

	p := purchase.Purchase{
		Supplier:       sup,
		SupplierNumber: strings.TrimSpace(d.ID),
		IssueDate:      date(d.IssueDate, "vyhotovenia"),
		DeliveryDate:   date(firstNonEmpty(d.Delivery, d.TaxPointDate), "dodania"),
		DueDate:        date(d.DueDate, "splatnosti"),
		ReceivedDate:   today,
		PaymentMethod:  invoice.PayTransfer,
		Note:           strings.Join(d.Notes, "\n"),
	}
	if cur := strings.ToUpper(strings.TrimSpace(d.Currency)); cur != "" && cur != "EUR" {
		p.Currency = cur
	}
	for _, pm := range d.Payment {
		if p.DueDate.IsZero() && pm.Due != "" {
			p.DueDate = date(pm.Due, "splatnosti")
		}
		if pm.Account != "" && p.SupplierIBAN == "" && party.ValidIBAN(pm.Account) {
			p.SupplierIBAN = noSpaces(pm.Account)
		}
		if vs := digits(pm.ID); vs != "" && len(vs) <= 10 && p.VariableSymbol == "" {
			p.VariableSymbol = vs
		}
		switch pm.Code {
		case "10":
			p.PaymentMethod = invoice.PayCash
		case "48", "54", "55":
			p.PaymentMethod = invoice.PayCard
		}
	}
	items := d.Items
	if credit {
		items = d.CItems
	}
	if len(items) > 0 {
		p.Description = items[0]
		if len(items) > 1 {
			p.Description += fmt.Sprintf(" a ďalšie (%d položky)", len(items))
		}
	}

	// rekapitulácia DPH v mene dokladu (prvý TaxTotal so sadzbami)
	sign := money.FromInt(1)
	if credit {
		sign = money.FromInt(-1)
	}
	byRate := map[vat.Rate]*purchase.RateRow{}
	reverse := map[vat.Rate]*purchase.RateRow{} // kategória AE (prenesenie)
	aeNoRate := false
	for _, tt := range d.TaxTotals {
		for _, st := range tt.Subtotals {
			base, err1 := money.Parse(strings.TrimSpace(st.Base.Value))
			tax, err2 := money.Parse(strings.TrimSpace(st.Tax.Value))
			if err1 != nil || err2 != nil {
				warn = append(warn, "Nečitateľná suma v rekapitulácii DPH")
				continue
			}
			var rate vat.Rate
			switch st.Category {
			case "S":
				pct, err := money.Parse(firstNonEmpty(st.Percent, "0"))
				if err != nil || !pct.Equal(pct.RoundTo(0)) {
					warn = append(warn, "Neznáma sadzba DPH "+st.Percent+" %")
					continue
				}
				rate = vat.Rate(pct.Raw() / money.FromInt(1).Raw())
			case "AE":
				// prenesenie: dodávateľ fakturuje bez DPH, daň si vypočítam sám
				pct, _ := money.Parse(firstNonEmpty(st.Percent, "0"))
				rate = vat.Rate(pct.Raw() / money.FromInt(1).Raw())
				if rate == 0 {
					// EN 16931 uvádza pri prenesení sadzbu 0 — daň si určí odberateľ
					rate, aeNoRate = vat.Rate23, true
				}
				tax = money.Zero // vypočíta sa zo základu
			case "K", "G", "E", "Z", "O":
				rate = 0
			default:
				warn = append(warn, "Neznáma daňová kategória "+st.Category)
				continue
			}
			target := byRate
			if st.Category == "AE" {
				target = reverse
			}
			row := target[rate]
			if row == nil {
				row = &purchase.RateRow{Rate: rate}
				target[rate] = row
			}
			row.Base = row.Base.Add(base.Mul(sign))
			row.VAT = row.VAT.Add(tax.Mul(sign))
		}
		if len(byRate)+len(reverse) > 0 {
			break // ďalší TaxTotal je DPH v eurách (BR-53)
		}
	}
	switch {
	case len(reverse) > 0 && len(byRate) == 0:
		p.ReverseCharge, byRate = true, reverse
		if aeNoRate {
			warn = append(warn, "Prenesenie daňovej povinnosti: sadzba nastavená na 23 % — skontrolujte, či platí pre toto plnenie")
		}
	case len(reverse) > 0:
		// prijatá faktúra má prenesenie pre celý doklad — časť s prenesením zvlášť
		base := money.Zero
		for _, r := range reverse {
			base = base.Add(r.Base)
		}
		warn = append(warn, fmt.Sprintf("E-faktúra obsahuje aj položky s prenesením daňovej povinnosti (základ %s) — zaevidujte ich ako samostatnú prijatú faktúru s prenesením", base.Display()))
	}
	rates := make([]vat.Rate, 0, len(byRate))
	for r := range byRate {
		rates = append(rates, r)
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i] > rates[j] })
	for _, r := range rates {
		p.Rates = append(p.Rates, *byRate[r])
	}
	if len(p.Rates) == 0 {
		warn = append(warn, "E-faktúra nemá rekapituláciu DPH — doplňte základ a DPH")
	}
	if party.Foreign(sup.Country) {
		warn = append(warn, "Zahraničný dodávateľ — vyberte, či ide o tovar alebo službu")
	}
	if credit {
		warn = append(warn, "Dobropis od dodávateľa (sumy so znamienkom mínus) — skontrolujte")
	}
	return Received{Purchase: p, Warnings: warn}, nil
}
