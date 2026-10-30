package chart

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

func field(err error) string {
	var ve *invoice.ValidationError
	if errors.As(err, &ve) {
		return ve.Fields[0].Field
	}
	return ""
}

func TestFramework(t *testing.T) {
	seen := map[string]bool{}
	prev := ""
	for _, a := range framework {
		if len(a.Number) != 3 || a.Number <= prev || seen[a.Number] || !a.Type.Valid() || a.Name == "" || !a.Active {
			t.Errorf("osnova: %+v (predchádzajúci %s)", a, prev)
		}
		seen[a.Number], prev = true, a.Number
		switch a.Class() {
		case 5:
			if a.Type != Expense {
				t.Errorf("%s: trieda 5 je nákladová", a.Number)
			}
		case 6:
			if a.Type != Revenue {
				t.Errorf("%s: trieda 6 je výnosová", a.Number)
			}
		}
	}
	for _, n := range []string{"211", "221", "311", "321", "343", "411", "491", "501", "518", "602", "604", "710"} {
		if !seen[n] {
			t.Errorf("v osnove chýba %s", n)
		}
	}
	if a, _ := Default().Find("513"); !a.NonTax {
		t.Error("reprezentácia je nedaňový náklad")
	}
}

func TestAccounts(t *testing.T) {
	c := Default()
	tests := []struct {
		name  string
		acc   Account
		old   string
		field string
	}{
		{"analytický", Account{Number: "221.001", Name: "Tatra banka"}, "", ""},
		{"analytický s pomlčkou", Account{Number: "311-100", Name: "Odberatelia tuzemsko"}, "", ""},
		{"duplicitný", Account{Number: "221001", Name: "Iná"}, "", "number"},
		{"bez syntetického", Account{Number: "999001", Name: "x"}, "", "number"},
		{"trieda 8", Account{Number: "801", Name: "x", Type: Expense}, "", "number"},
		{"krátke číslo", Account{Number: "22", Name: "x"}, "", "number"},
		{"bez názvu", Account{Number: "221002"}, "", "name"},
		{"vlastný syntetický", Account{Number: "504", Name: "x", Type: Expense}, "", "number"}, // už je
		{"zmena čísla syntetického", Account{Number: "519", Name: "Iné služby", Type: Expense}, "518", "number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Save(tt.acc, tt.old)
			if f := field(err); f != tt.field {
				t.Fatalf("pole %q, chcem %q (%v)", f, tt.field, err)
			}
		})
	}
	a, ok := c.Find("221001")
	if !ok || a.Type != Asset || Display(a.Number) != "221.001" || !a.Analytic() || a.Synthetic() != "221" {
		t.Fatalf("analytický účet: %+v", a)
	}
	// analytický účet k nedaňovému syntetickému je nedaňový
	r, err := c.Save(Account{Number: "513001", Name: "Pohostenie"}, "")
	if err != nil || !r.NonTax || r.Type != Expense {
		t.Fatalf("513.001: %+v %v", r, err)
	}
	// syntetický účet s analytikami sa nezmaže, osnovový sa iba skryje
	if err := c.Delete("221"); !errors.Is(err, ErrInUse) {
		t.Errorf("zmazanie 221: %v", err)
	}
	if err := c.Delete("221001"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("221"); err != nil {
		t.Fatal(err)
	}
	if a, ok := c.Find("221"); !ok || a.Active {
		t.Errorf("osnovový účet sa iba skryje: %+v", a)
	}
	if err := c.Delete("777"); !errors.Is(err, ErrNotFound) {
		t.Errorf("neexistujúci: %v", err)
	}
	// nová verzia osnovy doplní chýbajúce účty
	c.Accounts = c.Accounts[1:]
	if n := c.Seed(); n != 1 || c.Accounts[0].Number != "012" {
		t.Errorf("doplnenie osnovy: %d %s", n, c.Accounts[0].Number)
	}
}

func TestDims(t *testing.T) {
	var d Dims
	if _, err := d.SaveCenter(Center{Code: " ba ", Name: "Bratislava", Active: true}, ""); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		c     Center
		old   string
		field string
	}{
		{Center{Code: "BA", Name: "x"}, "", "code"},
		{Center{Code: "", Name: "x"}, "", "code"},
		{Center{Code: "B B", Name: "x"}, "", "code"},
		{Center{Code: "BB"}, "", "name"},
		{Center{Code: "BA", Name: "Bratislava – centrum"}, "BA", ""},
	}
	for _, tt := range tests {
		if _, err := d.SaveCenter(tt.c, tt.old); field(err) != tt.field {
			t.Errorf("%+v: %v", tt.c, err)
		}
	}
	if len(d.Centers) != 1 || d.Centers[0].Code != "BA" || d.Centers[0].Name != "Bratislava – centrum" {
		t.Fatalf("strediská: %+v", d.Centers)
	}
	if _, err := d.SaveJob(Job{Code: "Z-2026-01", Name: "Rozvádzač", From: invoice.MustDate("2026-10-01"), To: invoice.MustDate("2026-09-01")}, ""); field(err) != "to" {
		t.Errorf("koniec pred začiatkom: %v", err)
	}
	if _, err := d.SaveJob(Job{Code: "z-2026-01", Name: "Rozvádzač", Active: true}, ""); err != nil || d.Jobs[0].Code != "Z-2026-01" {
		t.Fatalf("zákazka: %+v %v", d.Jobs, err)
	}
	if err := d.DeleteJob("Z-2026-01"); err != nil || len(d.Jobs) != 0 {
		t.Fatal(err)
	}
	if err := d.DeleteCenter("XX"); !errors.Is(err, ErrNotFound) {
		t.Error(err)
	}
}
