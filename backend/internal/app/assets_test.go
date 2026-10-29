package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/asset"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestAssetRegister(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	save := func(x asset.Asset) asset.Asset {
		t.Helper()
		out, err := a.SaveAsset(ctx, c.ID, x)
		if err != nil {
			t.Fatalf("uloženie %s: %v", x.Name, err)
		}
		return out
	}
	car := save(asset.Asset{Name: "Dodávka", Group: 1, Acquired: invoice.MustDate("2025-03-15"), Cost: m.MustParse("24000")})
	sw := save(asset.Asset{Name: "Softvér", Kind: asset.Intangible, Years: 3, Acquired: invoice.MustDate("2026-01-10"), Cost: m.MustParse("3000")})
	printer := save(asset.Asset{Name: "Tlačiareň", Group: 1, Acquired: invoice.MustDate("2024-06-01"), Cost: m.MustParse("2000"),
		Disposed: invoice.MustDate("2026-05-01"), DisposalKind: asset.DisposalLiquidation})
	if car.Number != "M001" || sw.Number != "M002" || printer.Number != "M003" || car.Share != 100 || car.Method != asset.Straight {
		t.Fatalf("predvolené hodnoty: %+v %+v", car, sw)
	}

	// chyby: duplicitné číslo, neexistujúca faktúra, lacný majetok, cudzia firma
	cases := []struct {
		name  string
		x     asset.Asset
		field string
	}{
		{"duplicitné číslo", asset.Asset{Number: "m001", Name: "X", Group: 1, Acquired: invoice.MustDate("2026-01-01"), Cost: m.MustParse("5000")}, "number"},
		{"faktúra neexistuje", asset.Asset{Name: "X", Group: 1, Acquired: invoice.MustDate("2026-01-01"), Cost: m.MustParse("5000"), PurchaseID: 999}, "purchaseId"},
		{"pod hranicou", asset.Asset{Name: "X", Group: 1, Acquired: invoice.MustDate("2026-01-01"), Cost: m.MustParse("1700")}, "cost"},
	}
	for _, tc := range cases {
		if _, err := a.SaveAsset(ctx, c.ID, tc.x); !fieldErr(err, tc.field) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	other, _ := a.Store.CreateCompany(ctx, store.Company{Name: "Iná"})
	if err := a.DeleteAsset(ctx, other.ID, car.ID); err == nil {
		t.Fatal("zmazanie cez inú firmu")
	}
	ghost := car
	ghost.ID = 9999
	if _, err := a.SaveAsset(ctx, c.ID, ghost); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("úprava neexistujúceho: %v", err)
	}

	reg, err := a.Assets(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	tt := reg.Totals
	if tt.Claimed.String() != "7000.00" || tt.Disposal.String() != "1208.33" || tt.Expense.String() != "8208.33" ||
		tt.Tangible.String() != "13000.00" || tt.Intangible.String() != "2000.00" || !reg.Posted.IsZero() {
		t.Fatalf("súčty 2026: %+v posted %s", tt, reg.Posted)
	}
	byName := map[string]AssetItem{}
	for _, it := range reg.Items {
		byName[it.Name] = it
	}
	if it := byName["Dodávka"]; it.Opening.String() != "19000.00" || it.Residual.String() != "13000.00" || it.Period != 4 || len(it.Plan) != 5 {
		t.Fatalf("dodávka: %+v", it)
	}
	if it := byName["Softvér"]; it.Opening.String() != "3000.00" || it.Year.Claimed.String() != "1000.00" {
		t.Fatalf("softvér: %+v", it)
	}

	// zaúčtovanie do uzávierky; opakovane prepíše, nie zdvojí
	for i := 0; i < 2; i++ {
		sum, err := a.PostDepreciation(ctx, c.ID, 2026)
		if err != nil || sum.String() != "8208.33" {
			t.Fatalf("zaúčtovanie: %s %v", sum, err)
		}
	}
	book, err := a.Journal(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if got := book.Totals.Columns[journal.ExpOther]; got.String() != "8208.33" {
		t.Fatalf("e6 v denníku: %s", got)
	}
	if reg, _ = a.Assets(ctx, c.ID, 2026); reg.Posted.String() != "8208.33" {
		t.Fatalf("zaúčtované: %s", reg.Posted)
	}

	// výkazy: r02 a tab. 1a z registra + ručne zadaný majetok mimo registra
	if err := a.SetStatementInputs(ctx, c.ID, 2026, journal.AssetInputs{TangibleDM: m.MustParse("500")}); err != nil {
		t.Fatal(err)
	}
	st, err := a.StatementsFor(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]journal.Row{}
	for _, r := range st.Assets {
		rows[r.Code] = r
	}
	if rows["02"].Current.String() != "13500.00" || rows["01"].Current.String() != "2000.00" || rows["02"].Previous.String() != "20208.00" {
		t.Fatalf("výkaz r01/r02: %+v %+v", rows["01"], rows["02"])
	}
	if r := st.TaxB.Table1a[0]; r.Current.String() != "13500.00" || r.Previous.String() != "20208.33" {
		t.Fatalf("tab. 1a r. 1: %+v", r)
	}

	// po zmazaní všetkého sa uzávierková operácia pri ďalšom zaúčtovaní zruší
	for _, id := range []int64{car.ID, sw.ID, printer.ID} {
		if err := a.DeleteAsset(ctx, c.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if sum, err := a.PostDepreciation(ctx, c.ID, 2026); err != nil || !sum.IsZero() {
		t.Fatalf("nulové odpisy: %s %v", sum, err)
	}
	if book, _ = a.Journal(ctx, c.ID, 2026); !book.Totals.Columns[journal.ExpOther].IsZero() {
		t.Fatal("uzávierková operácia ostala")
	}
}
