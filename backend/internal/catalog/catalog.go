// Package catalog je cenník: opakované položky faktúr (služby a tovar)
// s jednotkou, cenou a sadzbou DPH.
package catalog

import (
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
