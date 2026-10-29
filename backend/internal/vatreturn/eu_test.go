package vatreturn

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func foreignInv(id int64, num, country, icdph string, kind invoice.SupplyKind, base string) invoice.Invoice {
	i := inv(id, num, "2026-10-10", icdph, map[vat.Rate]string{0: base})
	i.Customer.Country = country
	i.ReverseCharge = icdph != ""
	i.SupplyKind = kind
	return i
}

func TestEUAndExport(t *testing.T) {
	p, _ := ParsePeriod("2026-10")
	credit := foreignInv(7, "DB20260002", "DE", "DE123456789", invoice.SupplyGoods, "-100")
	credit.Kind = invoice.KindCreditNote
	in := Input{Period: p,
		Invoices: []invoice.Invoice{
			foreignInv(1, "FA1", "DE", "DE123456789", invoice.SupplyGoods, "1000"),
			foreignInv(2, "FA2", "DE", "de 123456789", invoice.SupplyGoods, "50"), // ten istý odberateľ → jeden riadok SV
			foreignInv(3, "FA3", "AT", "ATU12345678", invoice.SupplyServices, "500"),
			foreignInv(4, "FA4", "CH", "", invoice.SupplyGoods, "300"),              // vývoz
			foreignInv(5, "FA5", "US", "", invoice.SupplyServices, "200"),           // služba mimo EÚ
			foreignInv(6, "FA6", "FR", "", invoice.SupplyGoods, "70"),               // bez IČ DPH → upozornenie
			foreignInv(8, "FA8", "GR", "EL987654321", invoice.SupplyServices, "40"), // Grécko = EL
			credit,
		},
		Purchases: func() []purchase.Purchase {
			goods := pur(1, "PF1", "2026-10-12", "DE111111111", 23, "1000", "230")
			goods.Supplier.Country, goods.ReverseCharge, goods.SupplyKind = "DE", true, invoice.SupplyGoods
			goods.Normalize()
			svc := pur(2, "PF2", "2026-10-13", "CZ12345678", 23, "100", "23")
			svc.Supplier.Country, svc.ReverseCharge, svc.SupplyKind = "CZ", true, invoice.SupplyServices
			svc.Normalize()
			imp := pur(3, "PF3", "2026-10-14", "", 23, "10", "2.30")
			imp.Supplier.Country, imp.ReverseCharge, imp.SupplyKind = "CN", true, invoice.SupplyGoods
			imp.Normalize()
			noKind := pur(4, "PF4", "2026-10-15", "CZ12345678", 23, "10", "2.30")
			noKind.Supplier.Country, noKind.ReverseCharge = "CZ", true
			noKind.Normalize()
			return []purchase.Purchase{goods, svc, imp, noKind}
		}(),
	}
	r := Compute(in)
	want := map[string]string{
		"r13": "1250", "r14": "950", "r15": "300", // 1000 + 50 − 100 do EÚ, 300 vývoz
		"r07": "1000", "r08": "230", // nadobudnutie tovaru z EÚ
		"r09b": "100", "r10b": "23", // služba z EÚ
		"r17": "253", "r19": "253", "r33": "",
	}
	for k, v := range want {
		got := r.Rows[k]
		if v == "" {
			if !got.IsZero() {
				t.Errorf("%s = %s, chcem 0", k, got)
			}
			continue
		}
		if !got.Equal(m(v)) {
			t.Errorf("%s = %s, chcem %s", k, got, v)
		}
	}
	type svKey struct {
		c, id string
		code  int
	}
	sv := map[svKey]string{}
	for _, l := range r.SV {
		sv[svKey{l.Country, l.VATID, l.Code}] = l.Value.String()
	}
	wantSV := map[svKey]string{
		{"DE", "123456789", SVGoods}:    "950.00",
		{"AT", "U12345678", SVServices}: "500.00",
		{"EL", "987654321", SVServices}: "40.00",
	}
	if len(sv) != len(wantSV) {
		t.Fatalf("SV: %+v", r.SV)
	}
	for k, v := range wantSV {
		if sv[k] != v {
			t.Errorf("SV %v = %q, chcem %s", k, sv[k], v)
		}
	}
	if len(r.KV.B1) != 2 || len(r.KV.A1) != 0 {
		t.Errorf("KV: B1 %d, A1 %d", len(r.KV.B1), len(r.KV.A1))
	}
	var docs []string
	for _, w := range r.Warnings {
		docs = append(docs, w.Doc)
	}
	if strings.Join(docs, ",") != "FA6,PF3,PF4" {
		t.Errorf("upozornenia: %v", r.Warnings)
	}
	if !r.SVTotal().Equal(m("1490")) {
		t.Errorf("SV spolu %s", r.SVTotal())
	}
}

func TestSVXML(t *testing.T) {
	p, _ := ParsePeriod("2026-Q4")
	r := Return{Period: p}
	for i := 0; i < 13; i++ { // 13 riadkov → 2 strany
		r.SV = append(r.SV, SVLine{Country: "DE", VATID: "12345678" + string(rune('0'+i%10)), Value: m("10.5"), Code: SVGoods})
	}
	out := SVXML(r, Taxpayer{Name: "Veľmi dlhé obchodné meno spoločnosti s ručením obmedzeným a ešte dlhšie", ICDPH: "SK2120000003", City: "Zvolen"}, d("2026-11-20"))
	var doc struct {
		Dic    string   `xml:"hlavicka>identifikacneCislo>dic"`
		Riadny string   `xml:"hlavicka>druhSV>riadny"`
		Stv    string   `xml:"hlavicka>obdobie>stvrtrok"`
		Meno   []string `xml:"hlavicka>obchodneMeno>riadok"`
		Celkom string   `xml:"hlavicka>celkovaHodnota"`
		Den    string   `xml:"hlavicka>datumVyhlasenia>den"`
		Strany []struct {
			Zaznam []struct {
				Kod string `xml:"kod"`
			} `xml:"zaznam"`
			Cast2 []struct{} `xml:"zaznamCast2"`
		} `xml:"telo>strana"`
	}
	if err := xml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("XML: %v\n%s", err, out)
	}
	if doc.Dic != "2120000003" || doc.Riadny != "1" || doc.Stv != "4" || len(doc.Meno) != 4 || doc.Meno[1] == "" || doc.Celkom != "136.50" || doc.Den != "20" {
		t.Errorf("hlavička: %+v", doc)
	}
	if len(doc.Strany) != 2 || len(doc.Strany[0].Zaznam) != 12 || len(doc.Strany[1].Zaznam) != 12 || len(doc.Strany[1].Cast2) != 12 {
		t.Fatalf("strany: %d", len(doc.Strany))
	}
	if doc.Strany[1].Zaznam[0].Kod != "0" || doc.Strany[1].Zaznam[1].Kod != "" {
		t.Errorf("druhá strana: %+v", doc.Strany[1].Zaznam[:2])
	}
}
