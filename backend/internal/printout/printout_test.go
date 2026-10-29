package printout

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

var supplier = Supplier{Company: store.Company{
	Name: "Krnáč s.r.o.", Street: "Hlavná 12", Zip: "040 01", City: "Košice", ICO: "12345678", DIC: "2020123456",
	ICDPH: "SK2020123456", IBAN: "SK31 1200 0000 1987 4263 7541", Email: "faktury@krnac.sk", VATPayer: true,
}}

func sample(mod func(*invoice.Invoice)) invoice.Invoice {
	inv := invoice.Invoice{
		Kind: invoice.KindInvoice, Status: invoice.StatusIssued, Number: "2026001", VariableSymbol: "2026001",
		Customer:  invoice.Party{Name: "Žltý dom, a.s.", Street: "Dlhá 5", Zip: "811 01", City: "Bratislava", ICO: "87654321", ICDPH: "SK2021111111"},
		IssueDate: invoice.MustDate("2026-10-01"), DeliveryDate: invoice.MustDate("2026-10-01"), DueDate: invoice.MustDate("2026-10-15"),
		Lines: []invoice.Line{
			{Description: "Vedenie účtovníctva za september", Quantity: money.MustParse("1"), Unit: "mes.", UnitPrice: money.MustParse("1250"), Rate: 23},
			{Description: "Spracovanie miezd", Quantity: money.MustParse("12.5"), Unit: "ks", UnitPrice: money.MustParse("8.4"), Rate: 23},
			{Description: "Kniha o daniach", Quantity: money.MustParse("2"), Unit: "ks", UnitPrice: money.MustParse("19.90"), Rate: 5},
		},
	}
	if mod != nil {
		mod(&inv)
	}
	if err := inv.Recalculate(); err != nil {
		panic(err)
	}
	return inv
}

func text(t *testing.T) func(*sheet, error) (string, int) {
	return func(s *sheet, err error) (string, int) { return check(t, s, err) }
}

func check(t *testing.T, s *sheet, err error) (string, int) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(s.out, []byte("%PDF-")) {
		t.Fatal("výstup nie je PDF")
	}
	if dir := os.Getenv("PRINTOUT_DIR"); dir != "" { // na vizuálnu kontrolu
		name := strings.NewReplacer(" ", "_", "/", "_").Replace(t.Name())
		_ = os.WriteFile(dir+"/"+name+".pdf", s.out, 0o644)
	}
	return s.doc.PlainText(), s.doc.Pages()
}

func TestInvoice(t *testing.T) {
	tests := []struct {
		name string
		mod  func(*invoice.Invoice)
		want []string
		not  []string
	}{
		{"faktúra", nil,
			[]string{"Faktúra – daňový doklad", "2026001", "Dodávateľ", "Krnáč s.r.o.", "Odberateľ", "Žltý dom, a.s.", "IČ DPH", "SK2020123456",
				"Prevodom na účet", "SK31 1200 0000 1987 4263 7541", "Cena za MJ bez DPH", "12,5", "1\u00a0250,00", "23 %", "5 %", "K úhrade", "1\u00a0708,44 €"},
			[]string{"Koncept", "Dodávateľ nie je platiteľ DPH"}},
		{"koncept", func(i *invoice.Invoice) { i.Status = invoice.StatusDraft; i.Number = "" }, []string{"Koncept"}, nil},
		{"stornovaná", func(i *invoice.Invoice) { i.Status = invoice.StatusCancelled }, []string{"Stornovaná"}, nil},
		{"neplatiteľ DPH", func(i *invoice.Invoice) { i.NonVATPayer = true },
			[]string{"Faktúra\n", "Dodávateľ nie je platiteľ DPH.", "Cena za MJ\n"}, []string{"Sadzba DPH", "23 %"}},
		{"ceny s DPH", func(i *invoice.Invoice) { i.PriceMode = vat.PriceGross }, []string{"Cena za MJ s DPH", "Spolu s DPH"}, nil},
		{"tovar do EÚ", func(i *invoice.Invoice) {
			i.ReverseCharge, i.SupplyKind = true, invoice.SupplyGoods
			i.Customer.Country, i.Customer.ICDPH = "DE", "DE123456789"
		}, []string{"Nemecko", "§ 43 zákona o DPH", "—"}, nil},
		{"hotovosť", func(i *invoice.Invoice) { i.PaymentMethod = invoice.PayCash }, []string{"V hotovosti"}, []string{"IBAN"}},
		{"dobropis", func(i *invoice.Invoice) {
			i.Kind = invoice.KindCreditNote
			i.Correction = &invoice.Correction{OriginalNumber: "2026000", OriginalIssueDate: invoice.MustDate("2026-09-01"),
				OriginalDeliveryDate: invoice.MustDate("2026-09-01"), Reason: "Vrátenie tovaru"}
		}, []string{"Dobropis – opravný daňový doklad", "Opravuje faktúru", "2026000", "Vrátenie tovaru", "Suma dobropisu"}, nil},
		{"zálohová faktúra", func(i *invoice.Invoice) { i.Kind = invoice.KindProforma }, []string{"Zálohová faktúra – nie je daňový doklad"}, nil},
		{"doklad k platbe", func(i *invoice.Invoice) {
			i.Kind = invoice.KindAdvanceReceipt
			i.Advance = &invoice.Advance{ProformaNumber: "ZF2026001"}
		}, []string{"Daňový doklad k prijatej platbe", "K zálohovej faktúre", "ZF2026001", "Dátum prijatia platby", "Prijatá platba"},
			[]string{"Variabilný symbol", "Dátum splatnosti"}},
		{"vyúčtovanie zálohy", func(i *invoice.Invoice) {
			i.Advance = &invoice.Advance{ProformaNumber: "ZF2026001", Deductions: []invoice.Deduction{{
				ReceiptID: 9, Number: "ZD2026001", Date: invoice.MustDate("2026-09-20"),
				Rates: []vat.RateSummary{{Rate: 23, Base: money.MustParse("1000"), VAT: money.MustParse("230"), Total: money.MustParse("1230")}},
				Total: money.MustParse("1230"),
			}}}
		}, []string{"Vyúčtovanie zálohovej faktúry", "Daňový doklad k prijatej platbe ZD2026001", "−1\u00a0230,00", "Po odpočte zálohy", "Dodávka spolu", "478,44 €"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, pages := text(t)(invoiceSheet(sample(tt.mod), supplier))
			if pages != 1 {
				t.Errorf("strán: %d", pages)
			}
			for _, w := range tt.want {
				if !strings.Contains(got+"\n", w) {
					t.Errorf("chýba %q", w)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(got+"\n", n) {
					t.Errorf("nemá byť %q", n)
				}
			}
		})
	}
}

func TestInvoiceManyLines(t *testing.T) {
	inv := sample(func(i *invoice.Invoice) {
		for n := 1; n <= 70; n++ {
			i.Lines = append(i.Lines, invoice.Line{Description: fmt.Sprintf("Položka %d s dlhším popisom, ktorý sa môže zalomiť do viacerých riadkov podľa šírky stĺpca", n),
				Quantity: money.MustParse("1"), Unit: "ks", UnitPrice: money.MustParse("10"), Rate: 23})
		}
	})
	got, pages := text(t)(invoiceSheet(inv, supplier))
	if pages < 3 {
		t.Fatalf("strán: %d", pages)
	}
	ps := strings.Split(got, "\f")
	for i, p := range ps {
		if !strings.Contains(p, fmt.Sprintf("strana %d/%d", i+1, pages)) {
			t.Errorf("strana %d nemá číslo", i+1)
		}
		if i > 0 && !strings.Contains(p, "Popis\nMnožstvo") {
			t.Errorf("strana %d nemá hlavičku tabuľky", i+1)
		}
	}
	if !strings.Contains(ps[len(ps)-1], "K úhrade") {
		t.Error("súčet nie je na poslednej strane")
	}
}

func TestQuote(t *testing.T) {
	inv := sample(nil)
	q := quote.Quote{Status: quote.StatusIssued, Number: "CP2026001", Customer: inv.Customer, IssueDate: inv.IssueDate,
		ValidUntil: invoice.MustDate("2026-10-31"), Lines: inv.Lines, Summary: inv.Summary}
	got, _ := text(t)(quoteSheet(q, supplier))
	for _, w := range []string{"Cenová ponuka", "CP2026001", "Pre", "Platná do", "31. 10. 2026", "Cena spolu", "Cenová ponuka nie je daňový doklad."} {
		if !strings.Contains(got, w) {
			t.Errorf("chýba %q", w)
		}
	}
	for _, n := range []string{"Odberateľ", "IBAN", "K úhrade"} {
		if strings.Contains(got, n) {
			t.Errorf("nemá byť %q", n)
		}
	}
}

func TestReminder(t *testing.T) {
	r := reminder.Reminder{Number: "UP2026001", Date: invoice.MustDate("2026-10-29"), PayBy: invoice.MustDate("2026-11-08"), Level: 3,
		Customer: invoice.Party{Name: "Žltý dom, a.s.", Street: "Dlhá 5", Zip: "811 01", City: "Bratislava", ICO: "87654321"},
		Business: true, Interest: reminder.InterestStatutory, ChargeFee: true, Margin: money.MustParse("9"), FeeEach: money.MustParse("40"),
		Items: []reminder.Item{{Number: "2026001", VS: "2026001", IssueDate: invoice.MustDate("2026-09-01"), DueDate: invoice.MustDate("2026-09-15"),
			Amount: money.MustParse("1703.47"), Days: 44, Rate: money.MustParse("11.15"), Interest: money.MustParse("22.90"), Fee: money.MustParse("40")}},
		Principal: money.MustParse("1703.47"), InterestTotal: money.MustParse("22.90"), Fees: money.MustParse("40"), Total: money.MustParse("1766.37"),
	}
	got, _ := text(t)(reminderSheet(r, supplier))
	for _, w := range []string{"Posledná upomienka pred vymáhaním", "UP2026001", "Žltý dom, a.s.", "súdnou cestou", "8. 11. 2026",
		"22,90 (11,15 %)", "Paušálna náhrada nákladov", "1\u00a0766,37 €", "§ 369 Obchodného zákonníka", "S pozdravom", "Košice"} {
		if !strings.Contains(got, w) {
			t.Errorf("chýba %q", w)
		}
	}
}

func TestLogoAndFileName(t *testing.T) {
	tests := []struct {
		kind, number, want string
	}{
		{"Faktura", "2026001", "Faktura-2026001.pdf"},
		{"Faktura", "", "Faktura-koncept.pdf"},
		{"Ponuka", "CP/2026 01", "Ponuka-CP-2026-01.pdf"},
	}
	for _, tt := range tests {
		if got := FileName(tt.kind, tt.number); got != tt.want {
			t.Errorf("FileName(%q) = %q", tt.number, got)
		}
	}
	// WebP ani poškodené logo doklad nezhodí
	sup := supplier
	sup.Logo = []byte("RIFF....WEBP")
	if _, err := Invoice(sample(nil), sup); err != nil {
		t.Fatal(err)
	}
}
