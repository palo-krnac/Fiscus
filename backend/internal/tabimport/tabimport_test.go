package tabimport

import (
	"strconv"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestDate(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"2026-03-02", "2026-03-02", true},
		{"2026-03-02T00:00:00", "2026-03-02", true},
		{"2.3.2026", "2026-03-02", true},
		{"02. 03. 2026", "2026-03-02", true},
		{"2/3/2026", "2026-03-02", true},
		{"46083", "2026-03-02", true}, // Excel
		{"", "", true},
		{"zajtra", "", false},
		{"31.2.2026", "", false},
	} {
		d, ok := Date(tc.in)
		got := ""
		if !d.IsZero() {
			got = d.String()
		}
		if ok != tc.ok || got != tc.want {
			t.Errorf("%q: %q %v", tc.in, got, ok)
		}
	}
}

func TestNumber(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"1 234,50", "1234.50"}, {"1.234,50", "1234.50"}, {"1,234.50", "1234.50"}, {"-12,5 €", "-12.50"}, {"173.31", "173.31"},
	} {
		if d, err := Number(tc.in); err != nil || d.RoundCents().String() != tc.want {
			t.Errorf("%q: %s %v", tc.in, d, err)
		}
	}
}

func split(s string) [][]string {
	var out [][]string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		out = append(out, strings.Split(l, ";"))
	}
	return out
}

func TestDetect(t *testing.T) {
	for _, tc := range []struct{ head, want string }{
		{"Kód;Názov;Cena bez DPH;DPH (%)", KindItems},
		{"Názov;IČO;DIČ;Obec", KindContacts},
		{"Číslo;Odberateľ;Vystavená;Spolu", ""},
		{"Číslo;Dodávateľ;Dátum;Základ 23 %;DPH 23 %", ""},
		{"Nič", ""},
	} {
		if got := Detect(split(tc.head)); got != tc.want {
			t.Errorf("%s: %q", tc.head, got)
		}
	}
}

func TestParseDocs(t *testing.T) {
	tests := []struct {
		name, csv string
		kind      string
		total     string
		amounts   string // "sadzba:základ:daň,…"
		credit    bool
		paid      string // "" | "áno" | dátum
		err       string
	}{
		{"po sadzbách", "Číslo;Odberateľ;Vystavená;Základ 23 %;DPH 23 %;Základ 19 %;DPH 19 %;Spolu\n1;Kováč;2.3.2026;100;23;10;1,90;134,90",
			KindInvoices, "134.90", "19:10.00:1.90,23:100.00:23.00", false, "", ""},
		{"základ bez dane sa dopočíta", "Číslo;Odberateľ;Vystavená;Základ 23 %;Základ 0 %\n1;Kováč;2026-03-02;100;5",
			KindInvoices, "128.00", "0:5.00:0.00,23:100.00:23.00", false, "", ""},
		{"základ a DPH, sadzba z pomeru", "Číslo;Odberateľ;Dátum;Základ;DPH;Spolu\n1;Kováč;2.3.2026;100;19;119",
			KindInvoices, "119.00", "19:100.00:19.00", false, "", ""},
		{"iba suma so sadzbou", "Číslo;Odberateľ;Dátum;Sadzba DPH;Spolu\n1;Kováč;2.3.2026;5;105",
			KindInvoices, "105.00", "5:100.00:5.00", false, "", ""},
		{"iba suma, predvolená sadzba", "Číslo;Odberateľ;Dátum;Spolu\n1;Kováč;2.3.2026;123",
			KindInvoices, "123.00", "23:100.00:23.00", false, "", ""},
		{"dobropis zo zápornej sumy", "Číslo;Odberateľ;Dátum;Spolu;Pôvodná faktúra\n2;Kováč;2.3.2026;-12,30;1",
			KindInvoices, "-12.30", "23:-10.00:-2.30", true, "", ""},
		{"uhradená s dátumom", "Číslo;Odberateľ;Dátum;Spolu;Dátum úhrady\n1;Kováč;2.3.2026;123;10.3.2026",
			KindInvoices, "123.00", "23:100.00:23.00", false, "2026-03-10", ""},
		{"uhradená áno", "Číslo;Dodávateľ;Dátum;Spolu;Uhradená\n1;Kováč;2.3.2026;123;áno",
			KindPurchases, "123.00", "23:100.00:23.00", false, "áno", ""},
		{"chybný dátum", "Číslo;Odberateľ;Dátum;Spolu\n1;Kováč;zajtra;123", KindInvoices, "", "", false, "", "chybný dátum vystavenia"},
		{"bez sumy", "Číslo;Odberateľ;Dátum;Spolu\n1;Kováč;2.3.2026;", KindInvoices, "", "", false, "", "nemá sumu"},
		{"záloha", "Číslo;Odberateľ;Dátum;Spolu;Druh\n1;Kováč;2.3.2026;100;Zálohová faktúra", KindInvoices, "", "", false, "", "zálohová"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sh, err := Parse(split(tc.csv), tc.kind, vat.Rate23)
			if err != nil {
				t.Fatal(err)
			}
			d := sh.Docs[0]
			if tc.err != "" {
				if !strings.Contains(d.Err, tc.err) {
					t.Errorf("chyba %q, chcem %q", d.Err, tc.err)
				}
				return
			}
			if d.Err != "" {
				t.Fatal(d.Err)
			}
			var am []string
			for _, a := range d.Amounts {
				am = append(am, strings.Join([]string{strconv.Itoa(int(a.Rate)), a.Base.RoundCents().String(), a.VAT.RoundCents().String()}, ":"))
			}
			if d.Total.RoundCents().String() != tc.total || strings.Join(am, ",") != tc.amounts || d.Credit != tc.credit {
				t.Errorf("suma %s, sadzby %s, dobropis %v", d.Total, strings.Join(am, ","), d.Credit)
			}
			paid := ""
			if d.Paid {
				paid = "áno"
				if !d.PaidDate.IsZero() {
					paid = d.PaidDate.String()
				}
			}
			if paid != tc.paid {
				t.Errorf("úhrada %q, chcem %q", paid, tc.paid)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ name, csv, kind, err string }{
		{"prázdna", "Číslo;Odberateľ", KindInvoices, "prázdna"},
		{"bez partnera", "Číslo;Dátum;Spolu\n1;2.3.2026;1", KindInvoices, "názvom partnera"},
		{"bez čísla", "Odberateľ;Dátum;Spolu\nA;2.3.2026;1", KindInvoices, "číslom faktúry"},
		{"bez sumy", "Číslo;Odberateľ;Dátum\n1;A;2.3.2026", KindInvoices, "chýba suma"},
		{"bez druhu", "Číslo;Odberateľ;Dátum;Spolu\n1;A;2.3.2026;1", "", "vydané alebo prijaté"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(split(tc.csv), tc.kind, vat.Rate23); err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("chyba %v, chcem %q", err, tc.err)
			}
		})
	}
}

func TestParseContacts(t *testing.T) {
	sh, err := Parse(split("Názov;IČO;DIČ;IČ DPH;Ulica;Obec;PSČ;Štát\nKováč s.r.o.;361234;;SK2020123457;Hlavná 1;Nitra;949 01;Slovensko\n;;;;;;;\nOsoba;;SK2021757441;;;;;"), KindContacts, vat.Rate23)
	if err != nil || len(sh.Contacts) != 2 {
		t.Fatalf("%v %+v", err, sh)
	}
	p := sh.Contacts[0].Party
	if p.ICO != "00361234" || p.DIC != "2020123457" || p.ICDPH != "SK2020123457" || p.Zip != "94901" || sh.Contacts[0].Line != 2 {
		t.Errorf("kontakt: %+v", p)
	}
	if q := sh.Contacts[1].Party; q.ICDPH != "SK2021757441" || q.DIC != "2021757441" || sh.Contacts[1].Line != 4 {
		t.Errorf("DIČ v tvare IČ DPH: %+v", q)
	}
	if strings.Join(sh.Columns, ",") != "Názov,IČO,DIČ,IČ DPH,Ulica,Obec,PSČ,Štát" {
		t.Errorf("stĺpce: %v", sh.Columns)
	}
}
