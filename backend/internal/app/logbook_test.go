package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/asset"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/logbook"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestLogbookBusinessCar(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	car, err := a.SaveAsset(ctx, c.ID, asset.Asset{Name: "Škoda Octavia", Group: 1, Acquired: invoice.MustDate("2026-01-10"), Cost: m.MustParse("24000"), Share: 80})
	if err != nil {
		t.Fatal(err)
	}
	v, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "Škoda Octavia", Plate: "bb-123ab", Consumption: m.MustParse("5"),
		AssetID: car.ID, Driver: "Pavol Krnáč", Odometer: 1000, OdometerDate: invoice.MustDate("2026-01-10")})
	if err != nil || v.Plate != "BB-123AB" || v.Ownership != logbook.Business {
		t.Fatalf("vozidlo: %+v %v", v, err)
	}
	if _, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "Iné", Plate: "BB-123AB", Consumption: m.MustParse("5")}); !fieldErr(err, "plate") {
		t.Fatalf("duplicitné EČV: %v", err)
	}
	if _, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "X", Plate: "BB999XX", Consumption: m.MustParse("5"), AssetID: 999}); !fieldErr(err, "assetId") {
		t.Fatalf("neexistujúci majetok: %v", err)
	}

	trip := func(date string, kind logbook.TripKind, start, end int64) logbook.Trip {
		tr, err := a.SaveTrip(ctx, c.ID, logbook.Trip{VehicleID: v.ID, Date: invoice.MustDate(date), From: "Banská Bystrica", To: "Zvolen",
			Purpose: "Montáž rozvádzača", Kind: kind, OdoStart: start, OdoEnd: end})
		if err != nil {
			t.Fatalf("jazda %s: %v", date, err)
		}
		return tr
	}
	t1 := trip("2026-03-02", logbook.TripBusiness, 1000, 1500)
	trip("2026-03-03", logbook.TripPrivate, 1500, 1700)
	trip("2026-03-05", logbook.TripBusiness, 1700, 2000)
	if t1.Driver != "Pavol Krnáč" || t1.Km != 500 {
		t.Fatalf("predvolený vodič a km: %+v", t1)
	}
	if _, err := a.SaveTrip(ctx, c.ID, logbook.Trip{VehicleID: v.ID, Date: invoice.MustDate("2026-03-06"), From: "A", To: "B", Purpose: "x", OdoStart: 900, OdoEnd: 950}); !fieldErr(err, "odoStart") {
		t.Fatalf("tachometer pod začiatkom evidencie: %v", err)
	}

	// tankovanie z pokladničného bloku: suma sa prevezme, doklad iba raz
	_, _ = a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-03-01"), Amount: m.MustParse("500"), Purpose: "Vklad"})
	blok, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.Out, Date: invoice.MustDate("2026-03-04"), Amount: m.MustParse("90"), Partner: "Slovnaft", Purpose: "Nafta"})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := a.FuelDocs(ctx, c.ID, 2026)
	if err != nil || len(docs) != 1 || docs[0].ID != blok.ID {
		t.Fatalf("doklady na tankovanie: %+v %v", docs, err)
	}
	f, err := a.SaveFuel(ctx, c.ID, logbook.Fuel{VehicleID: v.ID, Date: blok.Date, Quantity: m.MustParse("60"), DocKind: "cash", DocID: blok.ID, Full: true})
	if err != nil || f.DocNumber != blok.Number || !f.Amount.Equal(m.MustParse("90")) {
		t.Fatalf("tankovanie: %+v %v", f, err)
	}
	if _, err := a.SaveFuel(ctx, c.ID, logbook.Fuel{VehicleID: v.ID, Date: blok.Date, Quantity: m.MustParse("10"), DocKind: "cash", DocID: blok.ID}); !fieldErr(err, "docId") {
		t.Fatalf("doklad dvakrát: %v", err)
	}
	if docs, _ = a.FuelDocs(ctx, c.ID, 2026); len(docs) != 0 {
		t.Fatalf("priradený doklad sa ponúka znova: %+v", docs)
	}

	// vyúčtovanie: 80 % služobne, PHM 72 € → limit TP 60 €, vylúčiť 30 €
	r, err := a.VehicleReport(ctx, c.ID, v.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	// odpis 24 000 / 4 × 12/12 (január) × 80 % = 4 800 €
	if !r.FuelExpense.Equal(m.MustParse("60")) || !r.Exclude.Equal(m.MustParse("30")) || !r.Depreciation.Equal(m.MustParse("4800")) ||
		!r.Posting.Equal(m.MustParse("-30")) || !r.Posted.IsZero() {
		t.Fatalf("vyúčtovanie: PHM %s vylúčiť %s odpis %s zaúčtovať %s", r.FuelExpense, r.Exclude, r.Depreciation, r.Posting)
	}
	for i := 0; i < 2; i++ {
		if sum, err := a.PostLogbook(ctx, c.ID, v.ID, 2026); err != nil || !sum.Equal(m.MustParse("-30")) {
			t.Fatalf("zaúčtovanie: %s %v", sum, err)
		}
	}
	book, _ := a.Journal(ctx, c.ID, 2026)
	if got := book.Totals.Columns[journal.ExpOther]; !got.Equal(m.MustParse("-30")) {
		t.Fatalf("e6 v denníku: %s", got)
	}
	if r, _ = a.VehicleReport(ctx, c.ID, v.ID, 2026); !r.Posted.Equal(m.MustParse("-30")) {
		t.Fatalf("zaúčtované: %s", r.Posted)
	}

	// vozidlo so záznamami sa nezmaže, archivované neprijme jazdu
	if err := a.DeleteVehicle(ctx, c.ID, v.ID); !fieldErr(err, "vehicle") {
		t.Fatalf("zmazanie vozidla s jazdami: %v", err)
	}
	v.Archived = true
	if _, err := a.SaveVehicle(ctx, c.ID, v); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveTrip(ctx, c.ID, logbook.Trip{VehicleID: v.ID, Date: invoice.MustDate("2026-04-01"), From: "A", To: "B", Purpose: "x", Km: 5}); !fieldErr(err, "vehicleId") {
		t.Fatalf("jazda archivovaného vozidla: %v", err)
	}
	list, _ := a.Vehicles(ctx, c.ID)
	if len(list) != 1 || list[0].Trips != 3 || list[0].Fuels != 1 || list[0].Odometer != 2000 || list[0].AssetName == "" {
		t.Fatalf("zoznam vozidiel: %+v", list)
	}
}

func TestLogbookPrivateCar(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	v, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "Ford Focus", Plate: "BB456CD", Fuel: logbook.Petrol, Consumption: m.MustParse("6"), Ownership: logbook.Private})
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range []logbook.Trip{
		{VehicleID: v.ID, Date: invoice.MustDate("2026-02-01"), From: "BB", To: "Žilina", Purpose: "Obhliadka", Km: 100},
		{VehicleID: v.ID, Date: invoice.MustDate("2026-02-03"), From: "BB", To: "Nitra", Purpose: "Revízia", Km: 150},
		{VehicleID: v.ID, Date: invoice.MustDate("2026-02-04"), From: "BB", To: "Tesco", Kind: logbook.TripPrivate, Km: 50},
	} {
		if _, err := a.SaveTrip(ctx, c.ID, tr); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.SaveFuel(ctx, c.ID, logbook.Fuel{VehicleID: v.ID, Date: invoice.MustDate("2026-02-02"), Quantity: m.MustParse("20"), Amount: m.MustParse("31.80")}); err != nil {
		t.Fatal(err)
	}
	r, err := a.VehicleReport(ctx, c.ID, v.ID, 2026)
	// 250 km × 0,313 = 78,25 €; PHM 250 × 6/100 × 1,59 = 23,85 €
	if err != nil || !r.BaseAllowance.Equal(m.MustParse("78.25")) || !r.FuelExpense.Equal(m.MustParse("23.85")) || !r.Posting.Equal(m.MustParse("102.10")) {
		t.Fatalf("náhrady: %+v %v", r.Report, err)
	}
	if sum, err := a.PostLogbook(ctx, c.ID, v.ID, 2026); err != nil || !sum.Equal(m.MustParse("102.10")) {
		t.Fatalf("zaúčtovanie: %s %v", sum, err)
	}
	book, _ := a.Journal(ctx, c.ID, 2026)
	if got := book.Totals.Columns[journal.ExpOther]; !got.Equal(m.MustParse("102.10")) {
		t.Fatalf("e6: %s", got)
	}
}
