// Package catalog je cenník: opakované položky faktúr (služby a tovar)
// s jednotkou, cenou a sadzbou DPH.
package catalog

import (
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Item je položka cenníka.
type Item struct {
	ID        int64         `json:"id"`
	CompanyID int64         `json:"companyId"`
	Code      string        `json:"code"` // vlastný kód (nepovinný, v rámci firmy jedinečný)
	Name      string        `json:"name"` // text položky na faktúre
	Unit      string        `json:"unit"`
	Price     money.Dec     `json:"price"`     // jednotková cena
	PriceMode vat.PriceMode `json:"priceMode"` // cena bez DPH alebo s DPH
	Rate      vat.Rate      `json:"vatRate"`
	Note      string        `json:"note"` // interná poznámka (na faktúru sa nedostane)
	Archived  bool          `json:"archived"`

	// Skladová karta produktu: identifikácia, zaradenie, nákup a parametre.
	EAN           string    `json:"ean"`           // čiarový kód (EAN-8/13, GTIN-14)
	Category      string    `json:"category"`      // kategória (napr. „Káble / Silové“)
	Brand         string    `json:"brand"`         // výrobca / značka
	Supplier      string    `json:"supplier"`      // hlavný dodávateľ
	SupplierCode  string    `json:"supplierCode"`  // kód u dodávateľa
	PurchasePrice money.Dec `json:"purchasePrice"` // posledná nákupná cena bez DPH
	Weight        money.Dec `json:"weight"`        // hmotnosť v kg
	Description   string    `json:"description"`   // popis produktu (e-shop, ponuka)
	Params        []Param   `json:"params"`        // technické parametre
}

// Param je parameter produktu (napr. Prierez = 2,5 mm²).
type Param struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

// Limity skladovej karty.
const (
	MaxParams      = 50
	MaxDescription = 5000
)

// ValidEAN overí kontrolnú číslicu kódu EAN-8, UPC (12), EAN-13 alebo GTIN-14.
func ValidEAN(s string) bool {
	switch len(s) {
	case 8, 12, 13, 14:
	default:
		return false
	}
	sum := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i == len(s)-1 {
			break
		}
		// váhy 3 a 1 striedavo sprava (od číslice pred kontrolnou)
		if (len(s)-1-i)%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return (10-sum%10)%10 == int(s[len(s)-1]-'0')
}

// Prices sú jednotkové ceny pre editor faktúry v oboch režimoch.
type Prices struct {
	Net   money.Dec `json:"priceNet"`   // bez DPH
	Gross money.Dec `json:"priceGross"` // s DPH
}

// Normalize očistí texty a doplní predvolené hodnoty.
func (it *Item) Normalize() {
	it.Code = strings.TrimSpace(it.Code)
	it.Name = strings.TrimSpace(it.Name)
	it.Unit = strings.TrimSpace(it.Unit)
	it.Note = strings.TrimSpace(it.Note)
	it.EAN = strings.ReplaceAll(strings.TrimSpace(it.EAN), " ", "")
	it.Category = strings.TrimSpace(it.Category)
	it.Brand = strings.TrimSpace(it.Brand)
	it.Supplier = strings.TrimSpace(it.Supplier)
	it.SupplierCode = strings.TrimSpace(it.SupplierCode)
	it.Description = strings.TrimSpace(it.Description)
	params := []Param{}
	for _, p := range it.Params {
		p.Name, p.Value, p.Unit = strings.TrimSpace(p.Name), strings.TrimSpace(p.Value), strings.TrimSpace(p.Unit)
		if p.Name != "" || p.Value != "" {
			params = append(params, p)
		}
	}
	it.Params = params
	if it.Unit == "" {
		it.Unit = "ks"
	}
	if it.PriceMode == "" {
		it.PriceMode = vat.PriceNet
	}
}

// Validate skontroluje položku.
func (it Item) Validate() error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if it.Name == "" {
		add("name", "Zadajte názov položky")
	} else if len([]rune(it.Name)) > 200 {
		add("name", "Názov môže mať najviac 200 znakov")
	}
	if len([]rune(it.Code)) > 30 {
		add("code", "Kód môže mať najviac 30 znakov")
	}
	if len([]rune(it.Unit)) > 10 {
		add("unit", "Merná jednotka môže mať najviac 10 znakov")
	}
	if it.PriceMode != vat.PriceNet && it.PriceMode != vat.PriceGross {
		add("priceMode", "Cena je bez DPH alebo s DPH")
	}
	if !it.Rate.Known() {
		add("vatRate", "Neznáma sadzba DPH")
	}
	if it.EAN != "" && !ValidEAN(it.EAN) {
		add("ean", "Neplatný čiarový kód EAN (8, 12, 13 alebo 14 číslic s kontrolnou číslicou)")
	}
	for f, v := range map[string]string{"category": it.Category, "brand": it.Brand, "supplier": it.Supplier, "supplierCode": it.SupplierCode} {
		if len([]rune(v)) > 100 {
			add(f, "Najviac 100 znakov")
		}
	}
	if it.PurchasePrice.Sign() < 0 {
		add("purchasePrice", "Nákupná cena nemôže byť záporná")
	}
	if it.Weight.Sign() < 0 {
		add("weight", "Hmotnosť nemôže byť záporná")
	}
	if len([]rune(it.Description)) > MaxDescription {
		add("description", fmt.Sprintf("Popis môže mať najviac %d znakov", MaxDescription))
	}
	if len(it.Params) > MaxParams {
		add("params", fmt.Sprintf("Najviac %d parametrov", MaxParams))
	}
	seen := map[string]bool{}
	for i, p := range it.Params {
		key := strings.ToLower(p.Name)
		switch {
		case p.Name == "":
			add(fmt.Sprintf("params[%d].name", i), "Zadajte názov parametra")
		case seen[key]:
			add(fmt.Sprintf("params[%d].name", i), "Parameter „"+p.Name+"“ je dvakrát")
		case len([]rune(p.Name)) > 60 || len([]rune(p.Value)) > 200 || len([]rune(p.Unit)) > 20:
			add(fmt.Sprintf("params[%d].value", i), "Parameter je príliš dlhý")
		}
		seen[key] = true
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Prices prepočíta jednotkovú cenu na cenu bez DPH a s DPH (zaokrúhlené
// na centy), aby editor faktúry nemusel počítať DPH.
func (it Item) Prices() Prices {
	hundred := money.FromInt(100)
	rate := money.FromInt(int64(it.Rate))
	if it.PriceMode == vat.PriceGross {
		return Prices{Net: it.Price.MulRatio(hundred, hundred.Add(rate)).RoundCents(), Gross: it.Price}
	}
	return Prices{Net: it.Price, Gross: it.Price.MulRatio(hundred.Add(rate), hundred).RoundCents()}
}
