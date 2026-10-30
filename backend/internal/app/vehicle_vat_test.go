package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/logbook"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestVehicleVAT(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // platiteľ DPH, mesačne
	car, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "Octavia", Plate: "BB123AB", Consumption: m.MustParse("5"), Category: "M1"})
	if err != nil {
		t.Fatal(err)
	}
	van, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "Transit", Plate: "ZA123BC", Consumption: m.MustParse("8"), Category: "N1"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-10-01"), Amount: m.MustParse("500"), Purpose: "Vklad"})
	blok := func(vehicle int64) cash.Document {
		d, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.Out, Date: invoice.MustDate("2026-10-02"), Amount: m.MustParse("61.50"), Partner: "Slovnaft",
			Purpose: "Nafta", Tax: &cash.Tax{Rows: []cash.VATRow{{Rate: vat.Rate23, Gross: m.MustParse("61.50")}}, SupplierICDPH: "SK2020372640", Expense: purchase.ExpenseOther, VehicleID: vehicle}})
		if err != nil {
			t.Fatalf("blok: %v", err)
		}
		return d
	}
	// osobné auto: 50 % z 11,50 € = 5,75 €; dodávka N1: plných 11,50 €
	b1, b2 := blok(car.ID), blok(van.ID)
	if b1.Tax.DeductPercent != 50 || b2.Tax.DeductPercent != 0 || !b1.Deductible().Equal(m.MustParse("5.75")) {
		t.Fatalf("odpočet z blokov: %d %d %s", b1.Tax.DeductPercent, b2.Tax.DeductPercent, b1.Deductible())
	}
	p, _, err := a.SavePurchase(ctx, c.ID, purchase.Purchase{SupplierNumber: "S-1", Supplier: invoice.Party{Name: "Autoservis s.r.o.", ICDPH: "SK2020372640", Country: "SK"},
		IssueDate: invoice.MustDate("2026-10-05"), DeliveryDate: invoice.MustDate("2026-10-05"), ReceivedDate: invoice.MustDate("2026-10-06"),
		DueDate: invoice.MustDate("2026-10-20"), Description: "Servis Octavia", VehicleID: car.ID,
		Rates: []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("200")}}})
	if err != nil || p.DeductPercent != 50 || !p.Deductible().Equal(m.MustParse("23")) {
		t.Fatalf("servis: %d %s %v", p.DeductPercent, p.Deductible(), err)
	}
	// klient nevie podstrčiť plný odpočet k osobnému autu
	p.DeductPercent = 0
	if p, _, _ = a.SavePurchase(ctx, c.ID, p); p.DeductPercent != 50 {
		t.Fatalf("odpočet z vozidla: %d", p.DeductPercent)
	}

	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil {
		t.Fatal(err)
	}
	// odpočet: 5,75 + 11,50 (bloky) + 23 (servis) = 40,25 €
	if !ret.Rows["r19"].Equal(m.MustParse("40.25")) {
		t.Fatalf("r. 19: %+v", ret.Rows)
	}
	if len(ret.KV.B2) != 1 || !ret.KV.B2[0].VAT.Equal(m.MustParse("46")) || !ret.KV.B2[0].Deducted.Equal(m.MustParse("23")) {
		t.Fatalf("KV B.2 (D a O): %+v", ret.KV.B2)
	}
	if ret.KV.B31 == nil || !ret.KV.B31.VAT.Equal(m.MustParse("23")) || !ret.KV.B31.Deducted.Equal(m.MustParse("17.25")) {
		t.Fatalf("KV B.3.1: %+v", ret.KV.B31)
	}

	// peňažný denník: neodpočítaná polovica DPH je výdavok
	book, _ := a.Journal(ctx, c.ID, 2026)
	for _, e := range book.Entries {
		if e.Key == "cash:"+itoa(int(b1.ID)) {
			got := map[journal.Column]string{}
			for _, l := range e.Lines {
				got[l.Column] = l.Amount.String()
			}
			if !m.MustParse(got[journal.ExpVAT]).Equal(m.MustParse("5.75")) || !m.MustParse(got[journal.ExpOther]).Equal(m.MustParse("55.75")) {
				t.Fatalf("rozpis bloku: %+v", e.Lines)
			}
		}
	}

	// vyúčtovanie vozidla: DPH z dokladov, nesúlad pri tankovaní z bloku bez vozidla
	plain := blok(0)
	if _, err := a.SaveFuel(ctx, c.ID, logbook.Fuel{VehicleID: car.ID, Date: plain.Date, Quantity: m.MustParse("40"), DocKind: "cash", DocID: plain.ID}); err != nil {
		t.Fatal(err)
	}
	r, err := a.VehicleReport(ctx, c.ID, car.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if r.VAT.Percent != 50 || len(r.VAT.Docs) != 3 || !r.VAT.VAT.Equal(m.MustParse("69")) || !r.VAT.Deducted.Equal(m.MustParse("40.25")) {
		t.Fatalf("DPH vozidla: %+v", r.VAT)
	}
	found := false
	for _, w := range r.Warnings {
		if w == "Doklad "+plain.Number+" má odpočet DPH 100 %, pri tomto vozidle patrí 50 % — vyberte na doklade vozidlo, odpočet sa nastaví sám" {
			found = true
		}
	}
	if !found {
		t.Fatalf("upozornenie na nesúlad: %v", r.Warnings)
	}
}
