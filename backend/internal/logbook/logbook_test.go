package logbook

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func trip(date string, kind TripKind, start, end int64) Trip {
	return Trip{ID: start, Date: invoice.MustDate(date), From: "Banská Bystrica", To: "Zvolen", Purpose: "Montáž", Kind: kind, OdoStart: start, OdoEnd: end, Km: end - start}
}

func fuel(date, qty, amount string) Fuel {
	return Fuel{Date: invoice.MustDate(date), Quantity: m.MustParse(qty), Amount: m.MustParse(amount)}
}

func TestValidate(t *testing.T) {
	v := Vehicle{Name: "Škoda Octavia", Plate: "bb-123ab", Consumption: m.MustParse("5.2")}
	v.Normalize()
	if err := v.Validate(); err != nil || v.Plate != "BB-123AB" || v.Fuel != Diesel || v.Method != MethodLog {
		t.Fatalf("vozidlo: %+v %v", v, err)
	}
	cases := []struct {
		name  string
		edit  func(*Vehicle)
		field string
	}{
		{"bez názvu", func(v *Vehicle) { v.Name = "" }, "name"},
		{"zlé EČV", func(v *Vehicle) { v.Plate = "BB/123" }, "plate"},
		{"kniha bez spotreby", func(v *Vehicle) { v.Consumption = m.Zero }, "consumption"},
		{"zlá pohonná látka", func(v *Vehicle) { v.Fuel = "uhlie" }, "fuel"},
	}
	for _, c := range cases {
		x := v
		c.edit(&x)
		if err := x.Validate(); err == nil || !strings.Contains(err.Error(), c.field) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	flat := v
	flat.Method, flat.Consumption = MethodFlat, m.Zero
	if err := flat.Validate(); err != nil {
		t.Errorf("paušál bez spotreby: %v", err)
	}

	tr := Trip{Date: invoice.MustDate("2026-10-01"), From: "BB", To: "ZV", Purpose: "Servis", OdoStart: 1000, OdoEnd: 1042, Depart: "7:30"}
	tr.Normalize()
	if err := tr.Validate(); err != nil || tr.Km != 42 || tr.Kind != TripBusiness {
		t.Fatalf("jazda: %+v %v", tr, err)
	}
	tripCases := []struct {
		name  string
		edit  func(*Trip)
		field string
	}{
		{"bez účelu", func(t *Trip) { t.Purpose = "" }, "purpose"},
		{"súkromná bez účelu je ok", func(t *Trip) { t.Purpose, t.Kind = "", TripPrivate }, ""},
		{"tachometer dozadu", func(t *Trip) { t.OdoEnd = 900 }, "odoEnd"},
		{"zlý čas", func(t *Trip) { t.Arrive = "25:00" }, "arrive"},
		{"bez km", func(t *Trip) { t.OdoStart, t.OdoEnd, t.Km = 0, 0, 0 }, "km"},
	}
	for _, c := range tripCases {
		x := tr
		c.edit(&x)
		x.Normalize()
		err := x.Validate()
		if (c.field == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.field)) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	f := fuel("2026-10-02", "40", "60.00")
	if err := f.Validate(); err != nil || !f.Price().Equal(m.MustParse("1.5")) {
		t.Fatalf("tankovanie: %v %s", err, f.Price())
	}
	bad := fuel("2026-10-02", "40", "6000")
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "nereálna") {
		t.Fatalf("nereálna cena: %v", err)
	}
}

func TestRateOn(t *testing.T) {
	cases := []struct {
		date string
		moto bool
		want string
	}{
		{"2026-01-01", false, "0.3130"},
		{"2025-12-31", false, "0.2960"},
		{"2025-05-31", false, "0.2810"},
		{"2026-03-01", true, "0.0900"},
	}
	for _, c := range cases {
		got, err := RateOn(DefaultKmRates, invoice.MustDate(c.date), c.moto)
		if err != nil || !got.Equal(m.MustParse(c.want)) {
			t.Errorf("%s: %s %v", c.date, got, err)
		}
	}
	if _, err := RateOn(DefaultKmRates, invoice.MustDate("2020-01-01"), false); err == nil {
		t.Error("sadzba pred prvou")
	}
}

func TestComputeBusinessLog(t *testing.T) {
	// auto v majetku, TP 5 l/100 km; 800 km služobne, 200 km súkromne;
	// natankované 60 l za 90 € (1,50 €/l), skutočná spotreba 6 l/100 km
	v := Vehicle{ID: 1, Name: "Octavia", Plate: "BB123AB", Fuel: Diesel, Consumption: m.MustParse("5"), Ownership: Business, Method: MethodLog, AssetID: 3}
	in := Input{Vehicle: v, Year: 2026, Rates: DefaultKmRates, Depreciation: m.MustParse("2400"), AssetShare: 80,
		Trips: []Trip{trip("2026-03-01", TripBusiness, 1000, 1500), trip("2026-03-02", TripPrivate, 1500, 1700), trip("2026-03-05", TripBusiness, 1700, 2000),
			trip("2025-12-30", TripBusiness, 900, 1000)},
		Fuel: []Fuel{fuel("2026-03-01", "30", "45"), fuel("2026-03-04", "30", "45"), fuel("2025-12-30", "10", "15")}}
	r := Compute(in)
	if r.KmBusiness != 800 || r.KmPrivate != 200 || r.KmTotal != 1000 {
		t.Errorf("km: %d/%d/%d", r.KmBusiness, r.KmPrivate, r.KmTotal)
	}
	// služobne 80 % z 90 € = 72 €, limit TP 800 × 5/100 × 1,5 = 60 € → nadspotreba 12 €
	checks := []struct {
		name string
		got  m.Dec
		want string
	}{
		{"podiel", r.Share, "80"}, {"cena", r.AvgPrice, "1.5"}, {"spotreba", r.Actual, "6"},
		{"PHM", r.FuelExpense, "60"}, {"nadspotreba", r.FuelOveruse, "12"}, {"súkromné", r.FuelPrivate, "18"},
		{"vylúčiť", r.Exclude, "30"}, {"spolu", r.Total, "2460"}, {"na km", r.PerKm, "3.075"},
	}
	for _, c := range checks {
		if !c.got.Equal(m.MustParse(c.want)) {
			t.Errorf("%s = %s, chcem %s", c.name, c.got, c.want)
		}
	}
	if len(r.Trips) != 2 || !r.Trips[0].Fuel.Equal(m.MustParse("37.5")) || !r.Trips[0].Wear.Equal(m.MustParse("1500")) {
		t.Errorf("náklady jázd: %+v", r.Trips)
	}
	if r.Months[2].KmBusiness != 800 || !r.Months[2].FuelCost.Equal(m.MustParse("90")) {
		t.Errorf("marec: %+v", r.Months[2])
	}
	if hasWarning(r, "o viac ako 20 %") {
		t.Errorf("6 l je presne +20 %%, upozornenie nemá byť: %v", r.Warnings)
	}
	in.Vehicle.Consumption = m.MustParse("4.5")
	if r := Compute(in); !hasWarning(r, "o viac ako 20 %") {
		t.Errorf("chýba upozornenie na spotrebu: %v", r.Warnings)
	}
}

func hasWarning(r Report, part string) bool {
	for _, w := range r.Warnings {
		if strings.Contains(w, part) {
			return true
		}
	}
	return false
}

func TestComputeFlatAndPrivate(t *testing.T) {
	trips := []Trip{trip("2026-02-01", TripBusiness, 1000, 1100), trip("2026-02-03", TripBusiness, 1100, 1250), trip("2026-02-04", TripPrivate, 1250, 1300)}
	fuels := []Fuel{fuel("2026-02-02", "20", "31.80")}
	cases := []struct {
		name      string
		v         Vehicle
		trips     []Trip
		fuelExp   string
		base      string
		allowance string
		exclude   string
	}{
		// paušál 80 % bez jázd
		{"v majetku paušál", Vehicle{Ownership: Business, Method: MethodFlat}, nil, "25.44", "0", "0", "6.36"},
		// paušál, ale kniha preukazuje 83,33 % → stále 80 %
		{"paušál s jazdami", Vehicle{Ownership: Leased, Method: MethodFlat}, trips, "25.44", "0", "0", "6.36"},
		// súkromné vozidlo: 250 km × 0,313 = 78,25 €; PHM 250 × 6/100 × 1,59 = 23,85 €
		{"súkromné podľa knihy", Vehicle{Ownership: Private, Method: MethodLog, Consumption: m.MustParse("6")}, trips, "23.85", "78.25", "102.10", "0"},
		// súkromné paušál 50 %
		{"súkromné paušál", Vehicle{Ownership: Private, Method: MethodFlat}, trips, "15.90", "0", "15.90", "0"},
		// motocykel: 250 × 0,090 = 22,50 €
		{"motocykel", Vehicle{Ownership: Private, Method: MethodLog, Moto: true, Consumption: m.MustParse("4")}, trips, "15.90", "22.50", "38.40", "0"},
	}
	for _, c := range cases {
		c.v.Fuel = Petrol
		r := Compute(Input{Vehicle: c.v, Year: 2026, Trips: c.trips, Fuel: fuels, Rates: DefaultKmRates})
		if !r.FuelExpense.Equal(m.MustParse(c.fuelExp)) || !r.BaseAllowance.Equal(m.MustParse(c.base)) ||
			!r.Allowance.Equal(m.MustParse(c.allowance)) || !r.Exclude.Equal(m.MustParse(c.exclude)) {
			t.Errorf("%s: PHM %s základná %s náhrady %s vylúčiť %s", c.name, r.FuelExpense, r.BaseAllowance, r.Allowance, r.Exclude)
		}
	}
	// súkromné vozidlo bez dokladu o tankovaní
	r := Compute(Input{Vehicle: Vehicle{Ownership: Private, Method: MethodLog, Consumption: m.MustParse("6")}, Year: 2026, Trips: trips, Rates: DefaultKmRates})
	if !r.FuelExpense.IsZero() || !hasWarning(r, "Bez dokladu o tankovaní") {
		t.Errorf("bez tankovania: %s %v", r.FuelExpense, r.Warnings)
	}
}

func TestGapsAndOdometer(t *testing.T) {
	v := Vehicle{Ownership: Business, Method: MethodLog, Consumption: m.MustParse("5"), Odometer: 900}
	trips := []Trip{trip("2026-01-02", TripBusiness, 1000, 1100), trip("2026-01-05", TripBusiness, 1150, 1200)}
	r := Compute(Input{Vehicle: v, Year: 2026, Trips: trips, Rates: DefaultKmRates})
	if !hasWarning(r, "chýba v knihe 50 km") {
		t.Errorf("medzera: %v", r.Warnings)
	}
	if got := NextOdometer(v, trips, invoice.Date{}); got != 1200 {
		t.Errorf("ďalší stav: %d", got)
	}
	if got := NextOdometer(v, nil, invoice.Date{}); got != 900 {
		t.Errorf("počiatočný stav: %d", got)
	}
}

func TestFuelWithoutDocument(t *testing.T) {
	v := Vehicle{Ownership: Business, Method: MethodLog, Consumption: m.MustParse("5"), AssetID: 1}
	in := Input{Vehicle: v, Year: 2026, Trips: []Trip{trip("2026-01-02", TripBusiness, 1000, 1100)}, Fuel: []Fuel{fuel("2026-01-02", "5", "7.50")}, Rates: DefaultKmRates}
	if r := Compute(in); !hasWarning(r, "bez priradeného dokladu (pokladničný blok alebo prijatá faktúra): 1") {
		t.Fatalf("upozornenie: %v", r.Warnings)
	}
	in.Fuel[0].DocKind, in.Fuel[0].DocID = "cash", 3
	if r := Compute(in); hasWarning(r, "bez priradeného dokladu") {
		t.Fatalf("s dokladom: %v", r.Warnings)
	}
	in.Vehicle.Ownership, in.Fuel[0].DocKind = Private, ""
	if r := Compute(in); hasWarning(r, "bez priradeného dokladu") {
		t.Fatalf("súkromné auto doklad nepotrebuje vo Fiškuse: %v", r.Warnings)
	}
}
