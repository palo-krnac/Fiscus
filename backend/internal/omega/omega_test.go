package omega

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	raw, err := os.ReadFile("testdata/export.txt")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Sections) != 4 || len(f.Docs("T04")) != 2 || len(f.Docs("T01")) != 4 || len(f.Docs("T00")) != 5 {
		t.Fatalf("časti: %+v", f.Sections)
	}
	p := f.Docs("T04")[0].Head
	if p.Col(PName) != "Kováč a syn s.r.o." || p.Col(PICO) != "36123455" || p.Col(PICDPH) != "2020123457" || p.Col(PVATCode) != "SK" {
		t.Errorf("partner: %q", p)
	}
	fa := f.Docs("T01")[0]
	if fa.Head.Col(FNumber) != "2026001" || len(fa.Items) != 3 || fa.Items[1].Col(FItemCode) != "REL-24" || fa.Head.Col(FTotal) != "173,31" {
		t.Errorf("faktúra: %q %d", fa.Head, len(fa.Items))
	}
	if card := f.Docs("T03")[0].Items[0]; card.Col(SName) != "Relé 24 V" || card.Col(SPrice) != "12,18" {
		t.Errorf("karta: %q", card)
	}
	if of := f.Docs("T00")[0]; len(of.Items) != 2 || of.Items[1].Col(EItemType) != "1" || of.Line != 16 {
		t.Errorf("EUD: %d %d", len(of.Items), of.Line)
	}
	if (Rec{"R01"}).Col(5) != "" {
		t.Error("chýbajúci stĺpec má byť prázdny")
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ name, in, err string }{
		{"prázdny", "", "R00"},
		{"XML", "<?xml version=\"1.0\"?><MoneyData/>", "R00"},
		{"hlavička bez R00", "R01\tx", "R00"},
		{"položka bez hlavičky", "R00\tT01\r\nR02\tx", "bez hlavičky"},
		{"neznáma veta", "R00\tT01\r\nR09\tx", "neznáma veta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("%v", err)
			}
			if tc.err == "R00" && !errors.Is(err, ErrNotOmega) {
				t.Errorf("chcem ErrNotOmega: %v", err)
			}
		})
	}
}
