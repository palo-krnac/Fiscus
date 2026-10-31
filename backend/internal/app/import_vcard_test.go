package app

import (
	"os"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestImportContacts(t *testing.T) {
	vcf, err := os.ReadFile("../vcard/testdata/kontakty.vcf")
	if err != nil {
		t.Fatal(err)
	}
	outlook := "First Name;Last Name;Company;E-mail Address;Business Phone;Business Street;Business City;Business Postal Code;Business Country/Region;IBAN\n" +
		"Ján;Novák;Novák s.r.o.;jan@novak.sk;+421 900 111 222;Lipová 1;Žilina;010 01;Slovakia;SK31 1200 0000 1987 4263 7541\n"
	a, c := newApp(t, edition.Desktop)
	tests := []struct {
		name string
		run  func() (ImportReport, error)
		want ImportCount
	}{
		{"vCard", func() (ImportReport, error) { return a.ImportVCard(ctx, c.ID, vcf, MigrateOptions{}, true) }, ImportCount{New: 2}},
		{"vCard znova", func() (ImportReport, error) { return a.ImportVCard(ctx, c.ID, vcf, MigrateOptions{}, true) }, ImportCount{Existing: 2}},
		{"Outlook CSV", func() (ImportReport, error) {
			r, err := a.ImportTable(ctx, c.ID, []byte(outlook), MigrateOptions{}, true)
			return r.ImportReport, err
		}, ImportCount{New: 1}},
		{"vCard nie", func() (ImportReport, error) { return a.ImportVCard(ctx, c.ID, []byte(outlook), MigrateOptions{}, false) }, ImportCount{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.run()
			if tc.want == (ImportCount{}) {
				if !fieldErr(err, "file") {
					t.Fatalf("chcem chybu súboru: %v", err)
				}
				return
			}
			if err != nil || res.Contacts != tc.want {
				t.Fatalf("%+v %v", res.Contacts, err)
			}
		})
	}
	list, _ := a.Store.ListContacts(ctx, c.ID, "")
	by := map[string]int{}
	for i, k := range list {
		by[k.Name] = i
	}
	k := list[by["Kováč a syn s.r.o."]]
	if k.Email != "fakturacia@kovac.sk" || k.City != "Nitra" || k.Zip != "94901" || !strings.Contains(k.Note, "Kontaktná osoba: Peter Kováč") {
		t.Errorf("vCard: %+v", k)
	}
	n := list[by["Novák s.r.o."]]
	if n.IBAN != "SK31 1200 0000 1987 4263 7541" || n.Phone != "+421 900 111 222" || !strings.Contains(n.Note, "Ján Novák") || n.Zip != "01001" {
		t.Errorf("Outlook: %+v", n)
	}
}
