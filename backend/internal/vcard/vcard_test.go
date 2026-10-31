package vcard

import (
	"errors"
	"os"
	"testing"
)

func TestParse(t *testing.T) {
	read := func(name string) []byte {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	tests := []struct {
		name, file string
		index      int
		want       Card
	}{
		{"firma s pracovnou adresou", "kontakty.vcf", 0, Card{FullName: "Peter Kováč", Given: "Peter", Family: "Kováč", Org: "Kováč a syn s.r.o.",
			Address: Address{Street: "Hlavná 12, Budova B", City: "Nitra", Zip: "949 01", Country: "Slovensko"}, Email: "fakturacia@kovac.sk",
			Phone: "+421 37 123 456", Note: "Stály odberateľ, platí včas"}},
		{"osoba vCard 4.0", "kontakty.vcf", 1, Card{FullName: "Eva Malá", Given: "Eva", Family: "Malá", Email: "eva@mala.sk",
			Phone: "+421900111222", Note: "Dlhá poznámka, ktorá je zalomená na dva riadky"}},
		{"vCard 2.1 v cp1250 a QP", "stary.vcf", 0, Card{Given: "Ján", Family: "Šťastný", Phone: "02 123 456",
			Address: Address{Street: "Lipová 3", City: "Žilina", Zip: "01001", Country: "Slovensko"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cards, err := Parse(read(tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if got := cards[tc.index]; got != tc.want {
				t.Errorf("\n%+v\n%+v", got, tc.want)
			}
		})
	}
	cards, _ := Parse(read("kontakty.vcf"))
	if cards[0].Name() != "Kováč a syn s.r.o." || cards[1].Name() != "Eva Malá" {
		t.Errorf("názvy: %q %q", cards[0].Name(), cards[1].Name())
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"iný súbor", "Názov;IČO\n"},
		{"neukončená", "BEGIN:VCARD\nFN:X\n"},
		{"END bez BEGIN", "BEGIN:VCARD\nEND:VCARD\nEND:VCARD\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.in)); err == nil {
				t.Error("chcem chybu")
			}
		})
	}
	if _, err := Parse([]byte("x")); !errors.Is(err, ErrNotVCard) {
		t.Errorf("%v", err)
	}
}
