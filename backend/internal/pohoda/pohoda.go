// Package pohoda číta XML export z programu POHODA (STORMWARE): adresár,
// zásoby (cenník), vydané a prijaté faktúry. Číta export dátovej
// komunikácie (dataPack) aj odpoveď na export (responsePack, listInvoice…);
// prvky sa hľadajú podľa lokálneho mena bez ohľadu na menný priestor.
// Formát: https://www.stormware.sk/xml/ (schémy version_2).
package pohoda

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Address je adresa z POHODY (typ:addressType).
type Address struct {
	Company  string `xml:"company"`
	Division string `xml:"division"`
	Name     string `xml:"name"`
	City     string `xml:"city"`
	Street   string `xml:"street"`
	Zip      string `xml:"zip"`
	ICO      string `xml:"ico"`
	DIC      string `xml:"dic"`
	ICDPH    string `xml:"icDph"`
	Email    string `xml:"email"`
	Phone    string `xml:"phone"`
	Mobil    string `xml:"mobilPhone"`
	Country  ref    `xml:"country"`
}

// DisplayName vráti názov firmy, inak meno osoby.
func (a Address) DisplayName() string {
	if s := strings.TrimSpace(a.Company); s != "" {
		return s
	}
	return strings.TrimSpace(a.Name)
}

type ref struct {
	IDs string `xml:"ids"`
}

type identity struct {
	Address Address `xml:"address"`
}

// Contact je záznam adresára (adb:addressbook).
type Contact struct {
	Header struct {
		Identity identity `xml:"identity"`
		Email    string   `xml:"email"`
		Phone    string   `xml:"phone"`
		Mobil    string   `xml:"mobil"`
		Web      string   `xml:"web"`
		Note     string   `xml:"note"`
	} `xml:"addressbookHeader"`
}

// Address vráti adresu s doplneným e-mailom a telefónom z hlavičky.
func (c Contact) Address() Address {
	a := c.Header.Identity.Address
	if a.Email == "" {
		a.Email = c.Header.Email
	}
	if a.Phone == "" {
		a.Phone = c.Header.Phone
	}
	if a.Phone == "" {
		a.Phone = firstNonEmpty(a.Mobil, c.Header.Mobil)
	}
	return a
}

// price je cena s príznakom „vrátane DPH“.
type price struct {
	Value  string `xml:",chardata"`
	PayVAT string `xml:"payVAT,attr"`
}

// Stock je zásoba (stk:stock) — položka cenníka.
type Stock struct {
	Header struct {
		StockType     string `xml:"stockType"`
		Code          string `xml:"code"`
		EAN           string `xml:"EAN"`
		Name          string `xml:"name"`
		Unit          string `xml:"unit"`
		SellingRate   string `xml:"sellingRateVAT"`
		PurchaseRate  string `xml:"purchasingRateVAT"`
		SellingPrice  price  `xml:"sellingPrice"`
		PurchasePrice price  `xml:"purchasingPrice"`
		Count         string `xml:"count"`
		Note          string `xml:"note"`
	} `xml:"stockHeader"`
}

// Item je položka faktúry (inv:invoiceItem).
type Item struct {
	Text       string `xml:"text"`
	Quantity   string `xml:"quantity"`
	Unit       string `xml:"unit"`
	PayVAT     string `xml:"payVAT"`
	RateVAT    string `xml:"rateVAT"`
	PercentVAT string `xml:"percentVAT"`
	Code       string `xml:"code"`
	Home       struct {
		UnitPrice string `xml:"unitPrice"`
		Price     string `xml:"price"`
		PriceVAT  string `xml:"priceVAT"`
		PriceSum  string `xml:"priceSum"`
	} `xml:"homeCurrency"`
	Foreign struct {
		UnitPrice string `xml:"unitPrice"`
	} `xml:"foreignCurrency"`
	Note string `xml:"note"`
}

// Invoice je faktúra (inv:invoice).
type Invoice struct {
	Header struct {
		Type   string `xml:"invoiceType"`
		Number struct {
			Requested string `xml:"numberRequested"`
		} `xml:"number"`
		SymVar           string   `xml:"symVar"`
		OriginalDocument string   `xml:"originalDocument"`
		Date             string   `xml:"date"`
		DateTax          string   `xml:"dateTax"`
		DateDue          string   `xml:"dateDue"`
		DateDelivery     string   `xml:"dateDelivery"`
		DateAccounting   string   `xml:"dateAccounting"`
		Text             string   `xml:"text"`
		Partner          identity `xml:"partnerIdentity"`
		PaymentType      struct {
			PaymentType string `xml:"paymentType"`
		} `xml:"paymentType"`
		Liquidation struct {
			AmountHome string `xml:"amountHome"`
		} `xml:"liquidation"`
		Note    string `xml:"note"`
		IntNote string `xml:"intNote"`
	} `xml:"invoiceHeader"`
	Items   []Item `xml:"invoiceDetail>invoiceItem"`
	Summary struct {
		Home struct {
			PriceNone    string `xml:"priceNone"`
			PriceLow     string `xml:"priceLow"`
			PriceLowVAT  string `xml:"priceLowVAT"`
			PriceHigh    string `xml:"priceHigh"`
			PriceHighVAT string `xml:"priceHighVAT"`
			Price3       string `xml:"price3"`
			Price3VAT    string `xml:"price3VAT"`
		} `xml:"homeCurrency"`
		Foreign struct {
			Currency ref    `xml:"currency"`
			Rate     string `xml:"rate"`
			Amount   string `xml:"amount"`
			PriceSum string `xml:"priceSum"`
		} `xml:"foreignCurrency"`
	} `xml:"invoiceSummary"`
}

// Foreign vráti true pre faktúru v cudzej mene.
func (inv Invoice) Foreign() bool {
	c := strings.ToUpper(strings.TrimSpace(inv.Summary.Foreign.Currency.IDs))
	return c != "" && c != "EUR"
}

// Data sú načítané záznamy.
type Data struct {
	Contacts []Contact
	Stocks   []Stock
	Invoices []Invoice
	// Other sú počty ďalších agend, ktoré Fiškus z POHODY nenačítava.
	Other map[string]int
}

// ErrNotPohoda — súbor nie je XML export z POHODY.
var ErrNotPohoda = errors.New("súbor nie je XML export z programu POHODA (dataPack alebo responsePack)")

// Parse prečíta XML export.
func Parse(data []byte) (Data, error) {
	out := Data{Other: map[string]int{}}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	root := ""
	known := map[string]bool{"addressbook": true, "stock": true, "invoice": true}
	containers := map[string]bool{"dataPack": true, "dataPackItem": true, "responsePack": true, "responsePackItem": true,
		"listAddressBook": true, "listStock": true, "listInvoice": true}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, fmt.Errorf("chybné XML: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		name := se.Name.Local
		if root == "" {
			root = name
			if name != "dataPack" && name != "responsePack" {
				return out, ErrNotPohoda
			}
			continue
		}
		switch {
		case name == "addressbook":
			var c Contact
			if err := dec.DecodeElement(&c, &se); err != nil {
				return out, fmt.Errorf("adresár: %w", err)
			}
			out.Contacts = append(out.Contacts, c)
		case name == "stock":
			var s Stock
			if err := dec.DecodeElement(&s, &se); err != nil {
				return out, fmt.Errorf("zásoby: %w", err)
			}
			out.Stocks = append(out.Stocks, s)
		case name == "invoice":
			var inv Invoice
			if err := dec.DecodeElement(&inv, &se); err != nil {
				return out, fmt.Errorf("faktúry: %w", err)
			}
			out.Invoices = append(out.Invoices, inv)
		case containers[name] || known[name]:
		default:
			// iná agenda (objednávky, banka, pokladňa…): spočítať a preskočiť
			if strings.HasPrefix(name, "list") || name == "responsePackItem" {
				continue
			}
			out.Other[name]++
			if err := dec.Skip(); err != nil {
				return out, fmt.Errorf("chybné XML: %w", err)
			}
		}
	}
	if root == "" {
		return out, ErrNotPohoda
	}
	return out, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// charsetReader prevedie Windows-1250 (predvolené kódovanie POHODY) do UTF-8.
func charsetReader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "utf-8", "utf8":
		return in, nil
	case "windows-1250", "cp1250", "x-cp1250":
		b, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		var sb strings.Builder
		sb.Grow(len(b) + len(b)/4)
		for _, c := range b {
			if c < 0x80 {
				sb.WriteByte(c)
				continue
			}
			r := cp1250[c-0x80]
			if r == 0 {
				r = utf8.RuneError
			}
			sb.WriteRune(r)
		}
		return strings.NewReader(sb.String()), nil
	}
	return nil, fmt.Errorf("nepodporované kódovanie %s", label)
}

// cp1250 sú znaky 0x80–0xFF kódovania Windows-1250.
var cp1250 = [128]rune{
	0x20AC, 0, 0x201A, 0, 0x201E, 0x2026, 0x2020, 0x2021, 0, 0x2030, 0x0160, 0x2039, 0x015A, 0x0164, 0x017D, 0x0179,
	0, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0, 0x2122, 0x0161, 0x203A, 0x015B, 0x0165, 0x017E, 0x017A,
	0x00A0, 0x02C7, 0x02D8, 0x0141, 0x00A4, 0x0104, 0x00A6, 0x00A7, 0x00A8, 0x00A9, 0x015E, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x017B,
	0x00B0, 0x00B1, 0x02DB, 0x0142, 0x00B4, 0x00B5, 0x00B6, 0x00B7, 0x00B8, 0x0105, 0x015F, 0x00BB, 0x013D, 0x02DD, 0x013E, 0x017C,
	0x0154, 0x00C1, 0x00C2, 0x0102, 0x00C4, 0x0139, 0x0106, 0x00C7, 0x010C, 0x00C9, 0x0118, 0x00CB, 0x011A, 0x00CD, 0x00CE, 0x010E,
	0x0110, 0x0143, 0x0147, 0x00D3, 0x00D4, 0x0150, 0x00D6, 0x00D7, 0x0158, 0x016E, 0x00DA, 0x0170, 0x00DC, 0x00DD, 0x0162, 0x00DF,
	0x0155, 0x00E1, 0x00E2, 0x0103, 0x00E4, 0x013A, 0x0107, 0x00E7, 0x010D, 0x00E9, 0x0119, 0x00EB, 0x011B, 0x00ED, 0x00EE, 0x010F,
	0x0111, 0x0144, 0x0148, 0x00F3, 0x00F4, 0x0151, 0x00F6, 0x00F7, 0x0159, 0x016F, 0x00FA, 0x0171, 0x00FC, 0x00FD, 0x0163, 0x02D9,
}
