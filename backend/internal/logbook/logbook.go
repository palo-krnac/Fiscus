// Package logbook je kniha jázd: vozidlá, jazdy a tankovanie a z nich
// daňové výdavky na vozidlo podľa zákona č. 595/2003 Z. z. o dani z príjmov
// a zákona č. 283/2002 Z. z. o cestovných náhradách:
//
//   - vozidlo v obchodnom majetku (aj prenajaté, požičané) — výdavky na PHM
//     podľa kníhy jázd: iba služobné jazdy a najviac podľa spotreby
//     z technického preukazu (nadspotreba nie je daňový výdavok), alebo
//     paušálne 80 % PHM bez knihy jázd (§ 19 ods. 2 písm. l); odpisy auta
//     sú v registri majetku (podiel na podnikaní);
//   - súkromné vozidlo nezahrnuté v obchodnom majetku (§ 19 ods. 2 písm. e)
//     — cestovné náhrady za služobné jazdy: základná náhrada za 1 km
//     (opotrebenie, „amortizácia“) a náhrada za PHM podľa spotreby
//     z technického preukazu a ceny z dokladov o tankovaní, alebo paušálne
//     50 % PHM (vtedy bez základnej náhrady);
//   - cena PHM sa preukazuje dokladmi o nákupe (nie cenou z internetu),
//     paušál vyžaduje stav tachometra na začiatku a konci obdobia.
package logbook

import (
	"errors"
	"regexp"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Ownership je vzťah vozidla k podnikaniu.
type Ownership string

const (
	Business Ownership = "business" // v obchodnom majetku (odpisuje sa)
	Leased   Ownership = "leased"   // prenajaté, na leasing, požičané
	Private  Ownership = "private"  // súkromné, nie v obchodnom majetku
)

// Method je spôsob uplatnenia výdavkov na PHM.
type Method string

const (
	MethodLog  Method = "log"  // podľa knihy jázd
	MethodFlat Method = "flat" // paušálne (80 % / 50 % PHM)
)

// FuelKind je druh pohonnej látky.
type FuelKind string

const (
	Petrol   FuelKind = "petrol"
	Diesel   FuelKind = "diesel"
	LPG      FuelKind = "lpg"
	CNG      FuelKind = "cng"
	Electric FuelKind = "electric"
)

// Unit vráti mernú jednotku pohonnej látky (l, kg, kWh).
func (f FuelKind) Unit() string {
	switch f {
	case CNG:
		return "kg"
	case Electric:
		return "kWh"
	}
	return "l"
}

// Vehicle je vozidlo v knihe jázd.
type Vehicle struct {
	ID        int64    `json:"id"`
	CompanyID int64    `json:"companyId"`
	Name      string   `json:"name"`  // napr. Škoda Octavia
	Plate     string   `json:"plate"` // evidenčné číslo
	Moto      bool     `json:"moto"`  // jednostopové vozidlo (iná sadzba náhrady)
	Fuel      FuelKind `json:"fuel"`
	// Consumption je kombinovaná spotreba z technického preukazu (l, kg
	// alebo kWh na 100 km).
	Consumption money.Dec `json:"consumption"`
	Ownership   Ownership `json:"ownership"`
	Method      Method    `json:"method"`
	AssetID     int64     `json:"assetId,omitempty"` // položka v registri majetku (odpisy)
	Driver      string    `json:"driver"`            // predvolený vodič
	// Odometer je stav tachometra na začiatku evidencie (OdometerDate).
	Odometer     int64        `json:"odometer"`
	OdometerDate invoice.Date `json:"odometerDate"`
	Archived     bool         `json:"archived"`
	Note         string       `json:"note"`
	// Category je kategória vozidla podľa technického preukazu (M1, L1e,
	// L3e, N1 …) — rozhoduje o obmedzení odpočtu DPH (§ 85n zákona o DPH).
	Category string `json:"category"`
	VIN      string `json:"vin"`
	// FullVAT: plný odpočet DPH — vozidlo výlučne na podnikanie s podrobnou
	// elektronickou evidenciou (§ 85n ods. 5, 6 a oznámenie daňovému úradu)
	// alebo výnimka podľa § 85n ods. 4 (VATReason).
	FullVAT   bool         `json:"fullVat"`
	VATReason string       `json:"vatReason,omitempty"`
	Notified  invoice.Date `json:"notified"` // oznámenie daňovému úradu (plný odpočet)
}

// Dôvody plného odpočtu DPH pri osobnom vozidle (§ 85n).
const (
	VATExclusive = "exclusive" // výlučne na podnikanie, podrobná evidencia
	VATRental    = "rental"    // krátkodobý alebo iný nájom (požičovňa)
	VATTransport = "transport" // preprava osôb, taxi
	VATSchool    = "school"    // výcvik v autoškole
	VATDemo      = "demo"      // predvádzacie, skúšobné, náhradné vozidlo
)

// LimitedCategories sú kategórie s obmedzeným odpočtom DPH (§ 85n).
var LimitedCategories = map[string]bool{"M1": true, "L1e": true, "L3e": true}

// LimitFrom a LimitTo je obdobie obmedzenia odpočtu DPH na 50 % (§ 85n).
var (
	LimitFrom = invoice.MustDate("2026-01-01")
	LimitTo   = invoice.MustDate("2028-06-30")
)

// VATPercent vráti, koľko percent DPH z nákupu k vozidlu (PHM, servis,
// pneumatiky, nájom, kúpa) možno odpočítať k dňu d: 50 % pri osobnom
// vozidle (M1, L1e, L3e) používanom aj súkromne od 1. 1. 2026 do
// 30. 6. 2028, inak 100 %.
func (v Vehicle) VATPercent(d invoice.Date) int {
	if !LimitedCategories[v.Category] || v.FullVAT || d.Before(LimitFrom) || d.After(LimitTo) {
		return 100
	}
	return 50
}

// FlatRate vráti percento paušálnych výdavkov na PHM: 80 % pri vozidle
// v obchodnom majetku alebo prenajatom, 50 % pri súkromnom.
func (v Vehicle) FlatRate() money.Dec {
	if v.Ownership == Private {
		return money.FromInt(50)
	}
	return money.FromInt(80)
}

var plateRe = regexp.MustCompile(`^[A-Z0-9 -]{2,12}$`)

// Normalize upraví texty a predvolené hodnoty.
func (v *Vehicle) Normalize() {
	v.Name, v.Driver, v.Note = strings.TrimSpace(v.Name), strings.TrimSpace(v.Driver), strings.TrimSpace(v.Note)
	v.Plate = strings.ToUpper(strings.TrimSpace(v.Plate))
	if v.Fuel == "" {
		v.Fuel = Diesel
	}
	if v.Ownership == "" {
		v.Ownership = Business
	}
	if v.Method == "" {
		v.Method = MethodLog
	}
	if v.Ownership != Business {
		v.AssetID = 0 // odpisuje sa iba vozidlo v obchodnom majetku
	}
	v.VIN = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v.VIN), " ", ""))
	if v.Category == "" {
		v.Category = "M1"
		if v.Moto {
			v.Category = "L3e"
		}
	}
	if !v.FullVAT {
		v.VATReason, v.Notified = "", invoice.Date{}
	} else if v.VATReason == "" {
		v.VATReason = VATExclusive
	}
}

var vinRe = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)

// Validate skontroluje vozidlo.
func (v *Vehicle) Validate() error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if v.Name == "" {
		add("name", "Zadajte názov vozidla (napr. Škoda Octavia)")
	}
	if v.Plate == "" {
		add("plate", "Zadajte evidenčné číslo vozidla")
	} else if !plateRe.MatchString(v.Plate) {
		add("plate", "Evidenčné číslo môže mať písmená, číslice, medzeru a pomlčku")
	}
	switch v.Fuel {
	case Petrol, Diesel, LPG, CNG, Electric:
	default:
		add("fuel", "Vyberte pohonnú látku")
	}
	switch v.Ownership {
	case Business, Leased, Private:
	default:
		add("ownership", "Vyberte, či je vozidlo v obchodnom majetku, prenajaté alebo súkromné")
	}
	if v.Method != MethodLog && v.Method != MethodFlat {
		add("method", "Vyberte spôsob uplatnenia výdavkov na PHM")
	}
	if v.Consumption.Sign() < 0 || v.Consumption.Cmp(money.FromInt(100)) > 0 {
		add("consumption", "Spotreba z technického preukazu musí byť 0 až 100 na 100 km")
	} else if v.Consumption.IsZero() && (v.Method == MethodLog) {
		add("consumption", "Zadajte spotrebu z technického preukazu (kombinovaná, na 100 km) — výdavok na PHM sa podľa nej obmedzuje")
	}
	if v.Odometer < 0 {
		add("odometer", "Stav tachometra nemôže byť záporný")
	}
	switch v.Category {
	case "M1", "L1e", "L3e", "N1", "N2", "other":
	default:
		add("category", "Vyberte kategóriu vozidla z technického preukazu")
	}
	if v.VIN != "" && !vinRe.MatchString(v.VIN) {
		add("vin", "VIN má 17 znakov (písmená bez I, O, Q a číslice)")
	}
	if v.FullVAT {
		switch v.VATReason {
		case VATExclusive:
			if v.VIN == "" {
				add("vin", "Pri plnom odpočte DPH (výlučne na podnikanie) zadajte VIN — je súčasťou evidencie podľa § 85n")
			}
			if v.Method != MethodLog {
				add("method", "Plný odpočet DPH vyžaduje podrobnú evidenciu každej jazdy — vyberte knihu jázd, nie paušál")
			}
		case VATRental, VATTransport, VATSchool, VATDemo:
		default:
			add("vatReason", "Vyberte dôvod plného odpočtu DPH")
		}
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// TripKind je druh jazdy.
type TripKind string

const (
	TripBusiness TripKind = "business" // služobná (súvisí s podnikaním)
	TripPrivate  TripKind = "private"  // súkromná
)

// Trip je jedna jazda.
type Trip struct {
	ID        int64        `json:"id"`
	CompanyID int64        `json:"companyId"`
	VehicleID int64        `json:"vehicleId"`
	Date      invoice.Date `json:"date"`
	Depart    string       `json:"depart"` // čas odchodu „07:30“
	Arrive    string       `json:"arrive"` // čas príchodu
	From      string       `json:"from"`
	To        string       `json:"to"`
	Purpose   string       `json:"purpose"` // účel jazdy
	Kind      TripKind     `json:"kind"`
	Driver    string       `json:"driver"`
	// OdoStart a OdoEnd sú stavy tachometra; Km je počet kilometrov.
	OdoStart int64  `json:"odoStart"`
	OdoEnd   int64  `json:"odoEnd"`
	Km       int64  `json:"km"`
	Note     string `json:"note"`
}

var timeRe = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)

// Normalize upraví texty a dopočíta km zo stavov tachometra.
func (t *Trip) Normalize() {
	t.From, t.To, t.Purpose = strings.TrimSpace(t.From), strings.TrimSpace(t.To), strings.TrimSpace(t.Purpose)
	t.Driver, t.Note = strings.TrimSpace(t.Driver), strings.TrimSpace(t.Note)
	t.Depart, t.Arrive = strings.TrimSpace(t.Depart), strings.TrimSpace(t.Arrive)
	if t.Kind == "" {
		t.Kind = TripBusiness
	}
	if t.OdoStart > 0 && t.OdoEnd > 0 {
		t.Km = t.OdoEnd - t.OdoStart
	} else if t.OdoStart > 0 && t.Km > 0 {
		t.OdoEnd = t.OdoStart + t.Km
	}
}

// Validate skontroluje jazdu.
func (t *Trip) Validate() error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if t.Date.IsZero() {
		add("date", "Zadajte dátum jazdy")
	}
	if t.From == "" {
		add("from", "Zadajte, odkiaľ jazda začala")
	}
	if t.To == "" {
		add("to", "Zadajte cieľ jazdy")
	}
	if t.Kind != TripBusiness && t.Kind != TripPrivate {
		add("kind", "Vyberte služobnú alebo súkromnú jazdu")
	}
	if t.Kind == TripBusiness && t.Purpose == "" {
		add("purpose", "Zadajte účel služobnej jazdy (napr. montáž u zákazníka, nákup materiálu)")
	}
	switch {
	case t.OdoStart > 0 && t.OdoEnd > 0 && t.OdoEnd <= t.OdoStart:
		add("odoEnd", "Stav tachometra na konci musí byť vyšší ako na začiatku")
	case t.Km <= 0:
		add("km", "Zadajte počet km alebo stavy tachometra")
	case t.Km > 5000:
		add("km", "Jedna jazda môže mať najviac 5 000 km")
	}
	for f, v := range map[string]string{"depart": t.Depart, "arrive": t.Arrive} {
		if v != "" && !timeRe.MatchString(v) {
			add(f, "Čas zadajte ako hh:mm")
		}
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Fuel je tankovanie (nákup pohonnej látky alebo nabíjanie).
type Fuel struct {
	ID        int64        `json:"id"`
	CompanyID int64        `json:"companyId"`
	VehicleID int64        `json:"vehicleId"`
	Date      invoice.Date `json:"date"`
	Quantity  money.Dec    `json:"quantity"` // litre, kg alebo kWh
	Amount    money.Dec    `json:"amount"`   // cena s DPH v eurách
	Odometer  int64        `json:"odometer"` // stav tachometra pri tankovaní
	Full      bool         `json:"full"`     // plná nádrž
	Station   string       `json:"station"`
	// Doklad: pokladničný blok alebo prijatá faktúra (už sú vo výdavkoch).
	DocKind   string `json:"docKind,omitempty"` // "cash" | "purchase" | ""
	DocID     int64  `json:"docId,omitempty"`
	DocNumber string `json:"docNumber,omitempty"`
	Note      string `json:"note"`
}

// Price vráti jednotkovú cenu (€ za liter), na 3 desatinné miesta.
func (f Fuel) Price() money.Dec {
	if f.Quantity.Sign() <= 0 {
		return money.Zero
	}
	return f.Amount.MulRatio(money.FromInt(1), f.Quantity).RoundTo(3)
}

// Validate skontroluje tankovanie.
func (f *Fuel) Validate() error {
	var fe []invoice.FieldError
	add := func(field, m string) { fe = append(fe, invoice.FieldError{Field: field, Message: m}) }
	f.Station, f.Note = strings.TrimSpace(f.Station), strings.TrimSpace(f.Note)
	if f.Date.IsZero() {
		add("date", "Zadajte dátum tankovania")
	}
	if f.Quantity.Sign() <= 0 || f.Quantity.Cmp(money.FromInt(2000)) > 0 {
		add("quantity", "Zadajte natankované množstvo")
	}
	if f.Amount.Sign() <= 0 {
		add("amount", "Zadajte cenu s DPH z dokladu")
	} else if !f.Amount.Equal(f.Amount.RoundCents()) {
		add("amount", "Cena môže mať najviac dve desatinné miesta")
	}
	if p := f.Price(); f.Quantity.Sign() > 0 && f.Amount.Sign() > 0 && (p.Cmp(money.MustParse("0.05")) < 0 || p.Cmp(money.FromInt(10)) > 0) {
		add("amount", "Cena za jednotku "+p.Display()+" € je nereálna — skontrolujte množstvo a sumu")
	}
	if f.Odometer < 0 {
		add("odometer", "Stav tachometra nemôže byť záporný")
	}
	switch f.DocKind {
	case "", "cash", "purchase":
	default:
		add("docKind", "Neznámy druh dokladu")
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// KmRate je sadzba základnej náhrady za 1 km od dňa (opatrenie MPSVR SR).
type KmRate struct {
	From invoice.Date `json:"from"`
	Car  money.Dec    `json:"car"`  // osobné cestné motorové vozidlo
	Moto money.Dec    `json:"moto"` // jednostopové vozidlo, trojkolka
}

// DefaultKmRates sú sadzby základnej náhrady (od 1. 3. 2025: 0,281 €;
// od 1. 6. 2025: 0,296 €; od 1. 1. 2026: 0,313 € — opatrenie č. 340/2025 Z. z.).
var DefaultKmRates = []KmRate{
	{invoice.MustDate("2024-05-01"), money.MustParse("0.265"), money.MustParse("0.075")},
	{invoice.MustDate("2025-03-01"), money.MustParse("0.281"), money.MustParse("0.080")},
	{invoice.MustDate("2025-06-01"), money.MustParse("0.296"), money.MustParse("0.085")},
	{invoice.MustDate("2026-01-01"), money.MustParse("0.313"), money.MustParse("0.090")},
}

// ErrNoKmRate — pre deň jazdy nie je zadaná sadzba základnej náhrady.
var ErrNoKmRate = errors.New("chýba sadzba základnej náhrady za 1 km")

// RateOn vráti sadzbu platnú v deň d.
func RateOn(rates []KmRate, d invoice.Date, moto bool) (money.Dec, error) {
	var best *KmRate
	for i := range rates {
		r := &rates[i]
		if !r.From.After(d) && (best == nil || r.From.After(best.From)) {
			best = r
		}
	}
	if best == nil {
		return money.Zero, ErrNoKmRate
	}
	if moto {
		return best.Moto, nil
	}
	return best.Car, nil
}
