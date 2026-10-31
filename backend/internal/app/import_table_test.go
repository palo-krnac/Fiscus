package app

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestImportTable(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	invoices := "Číslo faktúry;Odberateľ;IČO;IČ DPH;Obec;Štát;Dátum vystavenia;Splatnosť;Predmet;Základ 23 %;DPH 23 %;Základ 0 %;Spolu;Uhradená;Pôvodná faktúra\n" +
		"FV1;Kováč a syn s.r.o.;36123455;SK2020123457;Nitra;Slovensko;2.3.2026;16.3.2026;Montáž;100;23;;123;10.3.2026;\n" +
		"FV2;Kováč a syn s.r.o.;36123455;SK2020123457;Nitra;Slovensko;20.3.2026;3.4.2026;Vrátenie;-10;-2,30;;-12,30;;FV1\n" +
		"FV3;Ján Šťastný;;;Žilina;Thajsko;1.4.2026;15.4.2026;Servis;50;11,50;5;66,50;nie;\n" +
		";Bez čísla;;;;;1.4.2026;;;10;2,30;;12,30;;\n" +
		"FV4;Chyba;;;;;zajtra;;;10;2,30;;12,30;;\n"
	tests := []struct {
		name, kind, data string
		want             ImportReport
		warn             string
		err              string
	}{
		{"vydané", "invoices", invoices, ImportReport{Invoices: ImportCount{New: 3, Skipped: 2}}, "neznámy štát", ""},
		{"prijaté", "purchases", "Dodávateľ;IČO;IČ DPH;Číslo dodávateľa;Dátum;Základ;DPH;Spolu;Zaplatené\nElektro a.s.;35757442;SK2021757441;2026001234;5.3.2026;100;23;123;áno\n",
			ImportReport{Purchases: ImportCount{New: 1}}, "", ""},
		{"adresár", "", "Názov;IČO;Obec\nKováč a syn s.r.o.;36123455;Nitra\nNový;;Zvolen\n", ImportReport{Contacts: ImportCount{New: 2}}, "", ""},
		{"cenník", "", "Kód;Názov;Cena bez DPH;DPH (%)\nM1;Montáž;40;23\nM2;;5;23\n", ImportReport{Items: ImportCount{New: 1, Skipped: 1}}, "riadok 3", ""},
		{"faktúry bez druhu", "", "Číslo;Odberateľ;Dátum;Spolu\n1;A;2.3.2026;1\n", ImportReport{}, "", "kind"},
		{"nie tabuľka", "", "\x00\x01", ImportReport{}, "", "file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := a.ImportTable(ctx, c.ID, []byte(tc.data), MigrateOptions{Kind: tc.kind}, true)
			if tc.err != "" {
				if !fieldErr(err, tc.err) {
					t.Fatalf("chcem chybu poľa %s, mám %v", tc.err, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			g := res.ImportReport
			if g.Contacts != tc.want.Contacts || g.Items != tc.want.Items || g.Invoices != tc.want.Invoices || g.Purchases != tc.want.Purchases {
				t.Errorf("počty: %+v\n%s", g, strings.Join(g.Warnings, "\n"))
			}
			if tc.warn != "" && !strings.Contains(strings.Join(g.Warnings, "\n"), tc.warn) {
				t.Errorf("chýba upozornenie %q: %v", tc.warn, g.Warnings)
			}
		})
	}
	invs, _ := a.Store.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
	by := map[string]invoice.Invoice{}
	for _, inv := range invs {
		by[inv.Number] = inv
	}
	if fa := by["FV1"]; fa.Status != invoice.StatusPaid || fa.PaidDate.String() != "2026-03-10" || fa.Summary.Total.String() != "123.00" || fa.Subject != "Montáž" {
		t.Errorf("FV1: %s %s %s %q", fa.Status, fa.PaidDate, fa.Summary.Total, fa.Subject)
	}
	if db := by["FV2"]; db.Kind != invoice.KindCreditNote || db.Correction == nil || db.Correction.OriginalNumber != "FV1" || db.Summary.Total.String() != "-12.30" {
		t.Errorf("FV2: %s %+v %s", db.Kind, db.Correction, db.Summary.Total)
	}
	if f3 := by["FV3"]; f3.Status != invoice.StatusIssued || len(f3.Lines) != 2 || f3.Summary.Total.String() != "66.50" {
		t.Errorf("FV3: %s %d %s", f3.Status, len(f3.Lines), f3.Summary.Total)
	}
	purs, _ := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	if len(purs) != 1 || purs[0].Status != purchase.StatusPaid || purs[0].SupplierNumber != "2026001234" || purs[0].Summary.Total.String() != "123.00" {
		t.Errorf("prijatá: %+v", purs)
	}
	// náhľad bez druhu povie, čo rozpoznal
	prev, err := a.ImportTable(ctx, c.ID, []byte("Názov;IČO\nX;\n"), MigrateOptions{}, false)
	if err != nil || prev.Kind != "contacts" || strings.Join(prev.Columns, ",") != "Názov,IČO" {
		t.Errorf("náhľad: %+v %v", prev, err)
	}
}
