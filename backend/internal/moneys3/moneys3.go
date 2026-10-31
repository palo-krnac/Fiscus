// Package moneys3 číta XML export z programu Money S3 (Seyfor, slovenská
// aj česká verzia): adresár firiem, zásoby (cenník), vydané a prijaté
// faktúry. Koreň MoneyData, zoznamy Seznam… s jednotlivými záznamami
// (SeznamFirem/Firma, SeznamZasoba/Zasoba, SeznamFaktVyd/FaktVyd,
// SeznamFaktPrij/FaktPrij). Ostatné zoznamy sa iba spočítajú.
// Formát: XSD v priečinku programu Data/XMLDE/Schemas, https://money.cz/navod/s3xmlde/
package moneys3

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/xmlenc"
)

// Address je adresa (Ulice, Misto, PSC, Stat).
type Address struct {
	Street   string `xml:"Ulice"`
	City     string `xml:"Misto"`
	Zip      string `xml:"PSC"`
	Country  string `xml:"Stat"`
	CountryC string `xml:"KodStatu"`
}

// IsZero — adresa bez údajov.
func (a Address) IsZero() bool {
	return strings.TrimSpace(a.Street+a.City+a.Zip) == ""
}

type phone struct {
	Pred  string `xml:"Pred"`
	Cislo string `xml:"Cislo"`
}

func (p phone) String() string {
	return strings.TrimSpace(strings.TrimSpace(p.Pred) + " " + strings.TrimSpace(p.Cislo))
}

// Partner je firma z adresára aj odberateľ či dodávateľ na faktúre (DodOdb).
type Partner struct {
	Name         string  `xml:"Nazev"`
	Address      Address `xml:"Adresa"`
	TradeName    string  `xml:"ObchNazev"`
	TradeAddress Address `xml:"ObchAdresa"`
	BillName     string  `xml:"FaktNazev"`
	BillAddress  Address `xml:"FaktAdresa"`
	ICO          string  `xml:"ICO"`
	DIC          string  `xml:"DIC"`
	DICSK        string  `xml:"DICSK"`
	ICDPH        string  `xml:"ICDPH"`
	Email        string  `xml:"EMail"`
	Phone        phone   `xml:"Tel"`
	Mobile       phone   `xml:"Mobil"`
	Note         string  `xml:"Poznamka"`
}

// Display vráti fakturačný názov a adresu (inak obchodné, inak hlavné).
func (p Partner) Display() (string, Address) {
	name := firstNonEmpty(p.BillName, p.TradeName, p.Name)
	addr := p.BillAddress
	if addr.IsZero() {
		addr = p.TradeAddress
	}
	if addr.IsZero() {
		addr = p.Address
	}
	return name, addr
}

// TaxIDs rozdelí DIČ a IČ DPH: Money ich podľa verzie ukladá do DIC, DICSK
// alebo ICDPH; IČ DPH začína kódom krajiny, DIČ sú iba číslice.
func (p Partner) TaxIDs() (dic, icdph string) {
	for _, v := range []string{p.ICDPH, p.DIC, p.DICSK} {
		v = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v), " ", ""))
		switch {
		case v == "":
		case len(v) > 2 && v[0] >= 'A' && v[0] <= 'Z' && v[1] >= 'A' && v[1] <= 'Z':
			if icdph == "" {
				icdph = v
			}
		default:
			if dic == "" {
				dic = v
			}
		}
	}
	if dic == "" && strings.HasPrefix(icdph, "SK") {
		dic = icdph[2:] // slovenské IČ DPH = SK + DIČ
	}
	return dic, icdph
}

// PhoneNumber vráti telefón, inak mobil.
func (p Partner) PhoneNumber() string {
	return firstNonEmpty(p.Phone.String(), p.Mobile.String())
}

// Card je kmeňová karta zásoby (KmKarta).
type Card struct {
	Name    string `xml:"Popis"`
	Short   string `xml:"Zkrat"`
	Catalog string `xml:"Katalog"`
	Unit    string `xml:"MJ"`
	Barcode string `xml:"BarCode"`
	Note    string `xml:"Poznamka"`
	RateOut string `xml:"SazbaDPHVyd"`
}

// Code je katalógové číslo, inak skratka.
func (c Card) Code() string {
	return firstNonEmpty(c.Catalog, c.Short)
}

// Stock je zásoba (Zasoba) alebo samostatná kmeňová karta.
type Stock struct {
	Card  Card `xml:"KmKarta"`
	Price struct {
		Price1 struct {
			Price string `xml:"Cena"`
			Gross string `xml:"SDPH"` // 1 = cena s DPH
		} `xml:"Cena1"`
	} `xml:"PC"`
}

// VATSummary je rekapitulácia DPH (SouhrnDPH): Zaklad0, znížená sadzba
// (Zaklad5/DPH5, percento SazbaDPH1), základná (Zaklad22/DPH22, SazbaDPH2)
// a ďalšie sadzby.
type VATSummary struct {
	Base0    string `xml:"Zaklad0"`
	BaseLow  string `xml:"Zaklad5"`
	BaseHigh string `xml:"Zaklad22"`
	VATLow   string `xml:"DPH5"`
	VATHigh  string `xml:"DPH22"`
	Others   []struct {
		Rate string `xml:"Sazba"`
		Base string `xml:"Zaklad"`
		VAT  string `xml:"DPH"`
	} `xml:"SeznamDalsiSazby>DalsiSazba"`
}

// Item je položka faktúry (Polozka).
type Item struct {
	Text      string `xml:"Popis"`
	Quantity  string `xml:"PocetMJ"`
	Price     string `xml:"Cena"`
	PriceType string `xml:"CenaTyp"` // 0 bez DPH, 1 s DPH, 2 iba DPH, 3 iba základ
	Rate      string `xml:"SazbaDPH"`
	Discount  string `xml:"Sleva"` // %
	Unit      string `xml:"MJ"`
	Note      string `xml:"Poznamka"`
	Stock     struct {
		Card Card `xml:"KmKarta"`
	} `xml:"SklPolozka"`
}

// UnitName vráti mernú jednotku položky alebo jej skladovej karty.
func (it Item) UnitName() string {
	return firstNonEmpty(it.Unit, it.Stock.Card.Unit)
}

// Invoice je vydaná (FaktVyd) alebo prijatá (FaktPrij) faktúra.
type Invoice struct {
	Received       bool       `xml:"-"`
	Number         string     `xml:"Doklad"`
	SupplierNumber string     `xml:"PrijatDokl"`
	Text           string     `xml:"Popis"`
	Issued         string     `xml:"Vystaveno"`
	Accounting     string     `xml:"DatUcPr"`
	TaxDate        string     `xml:"PlnenoDPH"`
	Due            string     `xml:"Splatno"`
	VarSymbol      string     `xml:"VarSymbol"`
	Original       string     `xml:"PuvDoklad"`
	CreditNote     string     `xml:"Dobropis"`
	Kind           string     `xml:"Druh"` // N bežná, L zálohová, D daňový doklad k platbe…
	RateLow        string     `xml:"SazbaDPH1"`
	RateHigh       string     `xml:"SazbaDPH2"`
	Summary        VATSummary `xml:"SouhrnDPH"`
	Total          string     `xml:"Celkem"`
	Discount       string     `xml:"Sleva"`
	Foreign        struct {
		Currency string `xml:"Mena>Kod"`
		Total    string `xml:"Celkem"`
	} `xml:"Valuty"`
	Partner Partner `xml:"DodOdb"`
	Items   []Item  `xml:"SeznamPolozek>Polozka"`
	Company struct {
		Currency string `xml:"MenaKod"`
	} `xml:"MojeFirma"`
	Note string `xml:"Poznamka"`
}

// Credit — dobropis.
func (inv Invoice) Credit() bool {
	return strings.TrimSpace(inv.CreditNote) == "1"
}

// Currency vráti cudziu menu faktúry, "" pri domácej (euro).
func (inv Invoice) Currency() string {
	c := strings.ToUpper(strings.TrimSpace(inv.Foreign.Currency))
	if c == "" || c == inv.HomeCurrency() {
		return ""
	}
	return c
}

// HomeCurrency je mena účtovníctva firmy (slovenská verzia EUR, česká CZK).
func (inv Invoice) HomeCurrency() string {
	if c := strings.ToUpper(strings.TrimSpace(inv.Company.Currency)); c != "" {
		return c
	}
	return "EUR"
}

// Data sú načítané záznamy.
type Data struct {
	Contacts  []Partner
	Stocks    []Stock
	Invoices  []Invoice // vydané aj prijaté (Received)
	Other     map[string]int
	Agenda    string // ExpZkratka, popis exportu
	YearStart string
}

// ErrNotMoney — súbor nie je XML export z Money S3.
var ErrNotMoney = errors.New("súbor nie je XML export z programu Money S3 (MoneyData)")

// Parse prečíta XML export.
func Parse(data []byte) (Data, error) {
	out := Data{Other: map[string]int{}}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = xmlenc.CharsetReader
	depth := 0
	list := ""
	root := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, fmt.Errorf("chybné XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			depth--
			if depth == 1 {
				list = ""
			}
		case xml.StartElement:
			depth++
			name := t.Name.Local
			if depth == 1 {
				if name != "MoneyData" {
					return out, ErrNotMoney
				}
				root = true
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "ExpZkratka":
						out.Agenda = a.Value
					case "HospRokOd":
						out.YearStart = a.Value
					}
				}
				continue
			}
			if depth == 2 {
				list = name
				continue
			}
			// záznam v zozname (depth 3)
			var err error
			switch {
			case list == "SeznamFirem" && name == "Firma":
				var p Partner
				err = dec.DecodeElement(&p, &t)
				out.Contacts = append(out.Contacts, p)
			case list == "SeznamZasoba" && name == "Zasoba":
				var s Stock
				err = dec.DecodeElement(&s, &t)
				out.Stocks = append(out.Stocks, s)
			case list == "SeznamKmKarta" && name == "KmKarta":
				var s Stock
				err = dec.DecodeElement(&s.Card, &t)
				out.Stocks = append(out.Stocks, s)
			case list == "SeznamFaktVyd" && name == "FaktVyd", list == "SeznamFaktPrij" && name == "FaktPrij":
				var inv Invoice
				err = dec.DecodeElement(&inv, &t)
				inv.Received = name == "FaktPrij"
				out.Invoices = append(out.Invoices, inv)
			default:
				// iná agenda (objednávky, pokladňa, banka, daňové doklady k platbám…)
				key := strings.TrimPrefix(list, "Seznam")
				if key == "" {
					key = name
				}
				out.Other[key]++
				err = dec.Skip()
			}
			depth-- // DecodeElement aj Skip prečítajú aj koncový prvok
			if err != nil {
				return out, fmt.Errorf("%s: %w", strings.TrimPrefix(list, "Seznam"), err)
			}
		}
	}
	if !root {
		return out, ErrNotMoney
	}
	return out, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}
