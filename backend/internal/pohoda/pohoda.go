// Package pohoda číta XML export z programu POHODA (STORMWARE): adresár,
// zásoby (cenník), vydané a prijaté faktúry. Číta export dátovej
// komunikácie (dataPack) aj odpoveď na export (responsePack, listInvoice…);
// prvky sa hľadajú podľa lokálneho mena bez ohľadu na menný priestor.
// Formát: https://www.stormware.sk/xml/ (schémy version_2).
package pohoda

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
	dec.CharsetReader = xmlenc.CharsetReader
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

// ErrNoXML — v archíve ZIP nie je žiadny súbor XML.
var ErrNoXML = errors.New("v archíve ZIP nie je žiadny súbor XML")

// ParseArchive prečíta jeden XML export alebo archív ZIP s viacerými
// (napr. export z iDokladu „idokladImportPohoda.zip“ — jeden súbor na
// druh dokladu) a záznamy spojí. maxBytes obmedzí rozbalený obsah.
func ParseArchive(data []byte, maxBytes int64) (Data, error) {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return Parse(data)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Data{}, fmt.Errorf("poškodený archív ZIP: %w", err)
	}
	out := Data{Other: map[string]int{}}
	var total int64
	files := 0
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || !strings.EqualFold(path.Ext(zf.Name), ".xml") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return out, fmt.Errorf("%s: %w", zf.Name, err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxBytes-total+1))
		rc.Close()
		if err != nil {
			return out, fmt.Errorf("%s: %w", zf.Name, err)
		}
		if total += int64(len(b)); total > maxBytes {
			return out, fmt.Errorf("rozbalený archív je väčší ako %d MB", maxBytes>>20)
		}
		d, err := Parse(b)
		if err != nil {
			return out, fmt.Errorf("%s: %w", path.Base(zf.Name), err)
		}
		files++
		out.Contacts = append(out.Contacts, d.Contacts...)
		out.Stocks = append(out.Stocks, d.Stocks...)
		out.Invoices = append(out.Invoices, d.Invoices...)
		for k, v := range d.Other {
			out.Other[k] += v
		}
	}
	if files == 0 {
		return out, ErrNoXML
	}
	return out, nil
}
