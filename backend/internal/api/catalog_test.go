package api

import (
	"fmt"
	"os"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestCatalogAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/items", e.cid)
	var it struct {
		ID         int64
		Name       string
		PriceGross string `json:"priceGross"`
	}
	e.do("POST", base, `{"code":"M1","name":"Montáž","unit":"hod","price":"40","vatRate":23}`, 201, &it)
	if it.PriceGross != "49.20" {
		t.Fatalf("cena s DPH: %+v", it)
	}
	e.do("POST", base, `{"code":"m1","name":"Iné","vatRate":23}`, 422, nil)
	e.do("PUT", fmt.Sprintf("%s/%d", base, it.ID), `{"code":"M1","name":"Montáž rozvádzača","unit":"hod","price":"45","priceMode":"gross","vatRate":23}`, 200, &it)
	var list []struct {
		Name     string
		PriceNet string `json:"priceNet"`
	}
	e.do("GET", base, nil, 200, &list)
	if len(list) != 1 || list[0].Name != "Montáž rozvádzača" || list[0].PriceNet != "36.59" {
		t.Fatalf("zoznam: %+v", list)
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, it.ID), nil, 204, nil)
	e.do("DELETE", fmt.Sprintf("%s/%d", base, it.ID), nil, 404, nil)
}

func TestImportMigrateAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/import/", e.cid)
	money, err := os.ReadFile("../moneys3/testdata/export.xml")
	if err != nil {
		t.Fatal(err)
	}
	type count struct{ New, Existing, Skipped int }
	var rep struct {
		Source   string
		Invoices count
		Done     bool
	}
	tests := []struct {
		name, url, body string
		status          int
	}{
		{"neznámy zdroj", "xyz", "<x/>", 404},
		{"tabuľka bez druhu", "table", "Číslo;Odberateľ;Dátum;Spolu\n1;A;2.3.2026;1\n", 422},
		{"tabuľka vydaných", "table?kind=invoices", "Číslo;Odberateľ;Dátum;Spolu\n1;A;2.3.2026;1\n", 200},
		{"chybný dátum", "money?paidUntil=31.3.2026", string(money), 400},
		{"iný formát", "money", `<?xml version="1.0"?><dataPack/>`, 422},
		{"náhľad", "money?paidUntil=2026-03-31", string(money), 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e.do("POST", base+tc.url, tc.body, tc.status, nil)
		})
	}
	e.do("POST", base+"money?apply=1", string(money), 200, &rep)
	if rep.Source != "Money S3" || rep.Invoices.New != 3 || !rep.Done {
		t.Fatalf("prenos: %+v", rep)
	}
	e.do("POST", base+"pohoda", string(money), 422, nil) // súbor Money nie je POHODA
	e.do("POST", base+"omega", string(money), 422, nil)
	omega, _ := os.ReadFile("../omega/testdata/export.txt")
	e.do("POST", base+"omega", string(omega), 200, nil)
	e.do("POST", base+"idoklad", string(money), 422, nil)
	pohoda, _ := os.ReadFile("../pohoda/testdata/export.xml")
	e.do("POST", base+"idoklad", string(pohoda), 200, nil)
	e.do("POST", base+"mrp", string(pohoda), 422, nil)
	mrpZip, _ := os.ReadFile("../mrp/testdata/export.zip")
	e.do("POST", base+"mrp", string(mrpZip), 200, nil)
	e.do("POST", base+"superfaktura", string(pohoda), 200, nil)
	e.do("POST", base+"fakturoid", string(money), 200, nil)
	e.do("POST", base+"fakturoid", "Názov;IČO\n", 422, nil)
}

func TestImportContactsAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	url := fmt.Sprintf("/api/companies/%d/contacts/import", e.cid)
	vcf, _ := os.ReadFile("../vcard/testdata/kontakty.vcf")
	var rep struct{ Contacts struct{ New, Existing int } }
	e.do("POST", url, string(vcf), 200, &rep)
	if rep.Contacts.New != 2 {
		t.Fatalf("náhľad: %+v", rep)
	}
	e.do("POST", url+"?apply=1", "Názov;IČO\nFirma;36123455\n", 200, &rep)
	e.do("POST", url, "\x00\x01", 422, nil)
}

func TestImportOpeningAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	url := fmt.Sprintf("/api/companies/%d/ledger/opening/import", e.cid)
	var res struct {
		Lines    []struct{ Account, Debit, Credit string }
		Balanced bool
	}
	e.do("POST", url, "Účet;MD;D\n211;100;\n411;;100\n", 200, &res)
	if len(res.Lines) != 2 || !res.Balanced || res.Lines[0].Account != "211" {
		t.Fatalf("%+v", res)
	}
	e.do("POST", url, "Názov;MD\nx;1\n", 422, nil)
}
