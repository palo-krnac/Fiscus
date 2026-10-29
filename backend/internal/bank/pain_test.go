package bank

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestEndToEndID(t *testing.T) {
	tests := []struct{ vs, ss, ks, want string }{
		{"", "", "", "NOTPROVIDED"},
		{"2026001", "", "", "/VS2026001/SS/KS"},
		{"FV 2026/001", "12", "0308", "/VS2026001/SS12/KS0308"},
		{"123456789012", "", "123456", "/VS1234567890/SS/KS1234"},
	}
	for _, tt := range tests {
		if got := EndToEndID(tt.vs, tt.ss, tt.ks); got != tt.want {
			t.Errorf("EndToEndID(%q,%q,%q) = %q, chcem %q", tt.vs, tt.ss, tt.ks, got, tt.want)
		}
	}
}

func TestSepaFold(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"Ľubomír Šťastný", 70, "Lubomir Stastny"},
		{"Žltá & ružová, s.r.o.", 70, "Zlta + ruzova, s.r.o."},
		{"„Úsmev“  <a> ölo", 70, "'Usmev' a olo"},
		{"Dlhý názov firmy", 8, "Dlhy naz"},
		{"  ", 10, ""},
	}
	for _, tt := range tests {
		if got := sepaFold(tt.in, tt.max); got != tt.want {
			t.Errorf("sepaFold(%q) = %q, chcem %q", tt.in, got, tt.want)
		}
	}
}

func validOrder() PaymentOrder {
	return PaymentOrder{
		MsgID: "FISKUS-1-20261029", Created: time.Date(2026, 10, 29, 9, 30, 0, 0, time.UTC),
		Debtor: "Ukážka s.r.o.", DebtorIBAN: "SK31 1200 0000 1987 4263 7541", Date: invoice.MustDate("2026-10-30"),
		Transfers: []Transfer{
			{Name: "Dodávateľ a.s.", IBAN: "SK31 1200 0000 1987 4263 7541", Amount: money.MustParse("123.45"), VS: "2026001", Info: "Faktúra 2026001"},
			{Name: "Iný, s.r.o.", IBAN: "SK3112000000198742637541", Amount: money.MustParse("10.005")},
		},
	}
}

func TestPaymentOrderCheck(t *testing.T) {
	tests := []struct {
		name string
		mod  func(*PaymentOrder)
		want string
	}{
		{"platný", func(*PaymentOrder) {}, ""},
		{"bez úhrad", func(o *PaymentOrder) { o.Transfers = nil }, "žiadnu úhradu"},
		{"zlý IBAN platiteľa", func(o *PaymentOrder) { o.DebtorIBAN = "SK00 1234" }, "platiteľa"},
		{"bez dátumu", func(o *PaymentOrder) { o.Date = invoice.Date{} }, "dátum"},
		{"zlý IBAN príjemcu", func(o *PaymentOrder) { o.Transfers[1].IBAN = "SK99" }, "úhrada 2"},
		{"nulová suma", func(o *PaymentOrder) { o.Transfers[0].Amount = money.Zero }, "kladná"},
		{"bez príjemcu", func(o *PaymentOrder) { o.Transfers[0].Name = " " }, "príjemca"},
	}
	for _, tt := range tests {
		o := validOrder()
		tt.mod(&o)
		err := o.Check()
		if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

func TestPaymentOrderXML(t *testing.T) {
	data, err := validOrder().XML()
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		XMLName xml.Name
		Hdr     struct {
			MsgID   string `xml:"MsgId"`
			NbOfTxs int
			CtrlSum string
		} `xml:"CstmrCdtTrfInitn>GrpHdr"`
		Pmt struct {
			Date    string `xml:"ReqdExctnDt"`
			Dbtr    string `xml:"Dbtr>Nm"`
			IBAN    string `xml:"DbtrAcct>Id>IBAN"`
			CtrlSum string
			Txs     []struct {
				E2E string `xml:"PmtId>EndToEndId"`
				Amt struct {
					Ccy string `xml:"Ccy,attr"`
					V   string `xml:",chardata"`
				} `xml:"Amt>InstdAmt"`
				Name  string `xml:"Cdtr>Nm"`
				IBAN  string `xml:"CdtrAcct>Id>IBAN"`
				Ustrd string `xml:"RmtInf>Ustrd"`
			} `xml:"CdtTrfTxInf"`
		} `xml:"CstmrCdtTrfInitn>PmtInf"`
	}
	if err := xml.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	if d.XMLName.Space != PainNamespace || d.Hdr.NbOfTxs != 2 || d.Hdr.CtrlSum != "133.46" || d.Pmt.CtrlSum != "133.46" {
		t.Errorf("hlavička: %+v %+v\n%s", d.XMLName, d.Hdr, data)
	}
	if d.Pmt.Date != "2026-10-30" || d.Pmt.Dbtr != "Ukazka s.r.o." || d.Pmt.IBAN != "SK3112000000198742637541" {
		t.Errorf("platiteľ: %+v", d.Pmt)
	}
	t0, t1 := d.Pmt.Txs[0], d.Pmt.Txs[1]
	if t0.E2E != "/VS2026001/SS/KS" || t0.Amt.V != "123.45" || t0.Amt.Ccy != "EUR" || t0.Name != "Dodavatel a.s." || t0.Ustrd != "Faktura 2026001" {
		t.Errorf("úhrada 1: %+v", t0)
	}
	if t1.E2E != "NOTPROVIDED" || t1.Amt.V != "10.01" || strings.Contains(string(data), "<RmtInf></RmtInf>") {
		t.Errorf("úhrada 2: %+v\n%s", t1, data)
	}
	if _, err := (PaymentOrder{}).XML(); err == nil {
		t.Error("prázdny príkaz prešiel")
	}
}
