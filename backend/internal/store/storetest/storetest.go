// Package storetest je spoločná sada testov, ktorú musí prejsť každá
// implementácia store.Store (pamäť, SQLite...). Hlavne overuje, že sa
// dáta rôznych firiem nikdy nepremiešajú.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/asset"
	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/logbook"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/recurring"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/stock"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Factory vytvorí nové prázdne úložisko pre jeden test.
type Factory func(t *testing.T) store.Store

func draft(customer string, issue string) *invoice.Invoice {
	inv := &invoice.Invoice{
		Kind:         invoice.KindInvoice,
		Status:       invoice.StatusDraft,
		Customer:     invoice.Party{Name: customer, ICO: "31322832"},
		IssueDate:    invoice.MustDate(issue),
		DeliveryDate: invoice.MustDate(issue),
		DueDate:      invoice.MustDate(issue).AddDays(14),
		Lines: []invoice.Line{{Description: "Služba", Quantity: m.MustParse("1"),
			Unit: "ks", UnitPrice: m.MustParse("99.90"), Rate: vat.Rate23}},
	}
	_ = inv.Recalculate()
	return inv
}

// Run spustí celú sadu.
func Run(t *testing.T, newStore Factory) {
	ctx := context.Background()

	t.Run("firmy", func(t *testing.T) {
		s := newStore(t)
		c, err := s.CreateCompany(ctx, store.Company{Name: "Alfa", ICO: "31322832", VATPayer: true})
		if err != nil || c.ID == 0 {
			t.Fatalf("CreateCompany: %v %+v", err, c)
		}
		got, err := s.GetCompany(ctx, c.ID)
		if err != nil || got.Name != "Alfa" || !got.VATPayer {
			t.Fatalf("GetCompany: %v %+v", err, got)
		}
		if n, _ := s.CountCompanies(ctx); n != 1 {
			t.Fatalf("CountCompanies = %d", n)
		}
		if _, err := s.GetCompany(ctx, 9999); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("neexistujúca firma: %v", err)
		}
	})

	t.Run("faktúra uloženie a načítanie", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		inv := draft("Odberateľ", "2026-10-01")
		if err := s.SaveInvoice(ctx, c.ID, inv); err != nil || inv.ID == 0 {
			t.Fatalf("SaveInvoice: %v", err)
		}
		got, err := s.GetInvoice(ctx, c.ID, inv.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Customer.Name != "Odberateľ" || got.Summary.Total.String() != "122.88" ||
			got.Lines[0].UnitPrice.String() != "99.90" || got.DueDate.String() != "2026-10-15" {
			t.Fatalf("načítaná faktúra sa líši: %+v", got)
		}
		// položka s tuzemským prenesením a kódom colného sadzobníka
		got.Lines = append(got.Lines, invoice.Line{Description: "Oceľ", Quantity: m.MustParse("2"), Unit: "t", UnitPrice: m.MustParse("2600"),
			Rate: vat.Rate23, Reverse: invoice.RCMetals, Commodity: "7308", A2: &invoice.A2Qty{Quantity: m.MustParse("12.5"), Unit: "t"}, ItemID: 42})
		if err := got.Recalculate(); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveInvoice(ctx, c.ID, &got); err != nil {
			t.Fatal(err)
		}
		rc, _ := s.GetInvoice(ctx, c.ID, inv.ID)
		if l := rc.Lines[len(rc.Lines)-1]; l.Reverse != invoice.RCMetals || l.Commodity != "7308" || !rc.Summary.Rates[len(rc.Summary.Rates)-1].Reverse ||
			l.A2 == nil || l.A2.Quantity.String() != "12.50" || l.A2.Unit != "t" || rc.Lines[0].A2 != nil || l.ItemID != 42 || rc.Lines[0].ItemID != 0 {
			t.Fatalf("prenesenie pri položke sa neuložilo: %+v %+v", l, rc.Summary.Rates)
		}
		got = rc
		// zmena v načítanej kópii nesmie zmeniť uložené dáta
		got.Customer.Name = "Iný"
		again, _ := s.GetInvoice(ctx, c.ID, inv.ID)
		if again.Customer.Name != "Odberateľ" {
			t.Fatal("úložisko vracia zdieľanú referenciu")
		}
	})

	t.Run("oddelenie firiem", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		invA := draft("Zákazník Alfy", "2026-10-01")
		_ = s.SaveInvoice(ctx, a.ID, invA)

		if _, err := s.GetInvoice(ctx, b.ID, invA.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B vidí faktúru firmy A")
		}
		list, _ := s.ListInvoices(ctx, b.ID, store.InvoiceFilter{})
		if len(list) != 0 {
			t.Fatal("zoznam firmy B obsahuje cudzie faktúry")
		}
		hijack := *invA
		hijack.Customer.Name = "Prepísané"
		if err := s.SaveInvoice(ctx, b.ID, &hijack); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B prepísala faktúru firmy A")
		}
		orig, _ := s.GetInvoice(ctx, a.ID, invA.ID)
		if orig.Customer.Name != "Zákazník Alfy" {
			t.Fatal("faktúra firmy A bola zmenená")
		}
	})

	t.Run("zmazanie faktúry", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		inv := draft("Odberateľ", "2026-10-01")
		keep := draft("Iný", "2026-10-02")
		_ = s.SaveInvoice(ctx, a.ID, inv)
		_ = s.SaveInvoice(ctx, a.ID, keep)
		if err := s.DeleteInvoice(ctx, b.ID, inv.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("firma B zmazala faktúru firmy A: %v", err)
		}
		if err := s.DeleteInvoice(ctx, a.ID, inv.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetInvoice(ctx, a.ID, inv.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("faktúra ostala: %v", err)
		}
		if err := s.DeleteInvoice(ctx, a.ID, inv.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("druhé zmazanie: %v", err)
		}
		if k, err := s.GetInvoice(ctx, a.ID, keep.ID); err != nil || len(k.Lines) != 1 {
			t.Fatalf("iná faktúra sa poškodila: %+v %v", k, err)
		}
	})

	t.Run("filter", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		_ = s.SaveInvoice(ctx, c.ID, draft("Novák s.r.o.", "2025-12-30"))
		_ = s.SaveInvoice(ctx, c.ID, draft("Kováč", "2026-01-05"))
		paid := draft("Horváth", "2026-02-10")
		paid.Status = invoice.StatusPaid
		_ = s.SaveInvoice(ctx, c.ID, paid)

		cases := []struct {
			f    store.InvoiceFilter
			want int
		}{
			{store.InvoiceFilter{}, 3},
			{store.InvoiceFilter{Year: 2026}, 2},
			{store.InvoiceFilter{Status: invoice.StatusPaid}, 1},
			{store.InvoiceFilter{Search: "novák"}, 1},
			{store.InvoiceFilter{Search: "31322832"}, 3},
			{store.InvoiceFilter{Year: 2024}, 0},
		}
		for _, cs := range cases {
			list, err := s.ListInvoices(ctx, c.ID, cs.f)
			if err != nil || len(list) != cs.want {
				t.Errorf("filter %+v: %d, chcem %d (%v)", cs.f, len(list), cs.want, err)
			}
		}
		list, _ := s.ListInvoices(ctx, c.ID, store.InvoiceFilter{})
		if list[0].Customer.Name != "Horváth" {
			t.Errorf("najnovšia faktúra má byť prvá, je %s", list[0].Customer.Name)
		}
	})

	t.Run("číselný rad bez duplicít pri súbežnosti", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		const n = 50
		var wg sync.WaitGroup
		seen := make(chan int, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, err := s.NextSequence(ctx, c.ID, "FA", 2026)
				if err != nil {
					t.Error(err)
				}
				seen <- v
			}()
		}
		wg.Wait()
		close(seen)
		got := map[int]bool{}
		for v := range seen {
			if got[v] {
				t.Fatalf("číslo %d pridelené dvakrát", v)
			}
			got[v] = true
		}
		for i := 1; i <= n; i++ {
			if !got[i] {
				t.Fatalf("v rade chýba číslo %d", i)
			}
		}
		// nový rok aj iný rad začínajú od 1
		if v, _ := s.NextSequence(ctx, c.ID, "FA", 2027); v != 1 {
			t.Fatalf("nový rok: %d", v)
		}
		if v, _ := s.NextSequence(ctx, c.ID, "DB", 2026); v != 1 {
			t.Fatalf("iný rad: %d", v)
		}
	})
	t.Run("zmazanie firmy", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		for _, c := range []store.Company{a, b} {
			inv := draft("Odberateľ", "2026-10-01")
			if err := s.SaveInvoice(ctx, c.ID, inv); err != nil {
				t.Fatal(err)
			}
			if err := s.SaveContact(ctx, c.ID, &store.Contact{Name: "Kontakt " + c.Name}); err != nil {
				t.Fatal(err)
			}
			if err := s.SetSetting(ctx, fmt.Sprintf("stock.%d", c.ID), "{}"); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.SetSetting(ctx, "legal.vat", "x"); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteCompany(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetCompany(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("firma ostala: %v", err)
		}
		if l, _ := s.ListContacts(ctx, a.ID, ""); len(l) != 0 {
			t.Errorf("adresár ostal: %+v", l)
		}
		if l, _ := s.ListInvoices(ctx, a.ID, store.InvoiceFilter{}); len(l) != 0 {
			t.Errorf("faktúry ostali: %d", len(l))
		}
		if _, err := s.GetSetting(ctx, fmt.Sprintf("stock.%d", a.ID)); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("nastavenie ostalo: %v", err)
		}
		// druhá firma a spoločné nastavenia ostanú
		if l, _ := s.ListContacts(ctx, b.ID, ""); len(l) != 1 {
			t.Errorf("adresár inej firmy: %+v", l)
		}
		if l, _ := s.ListInvoices(ctx, b.ID, store.InvoiceFilter{}); len(l) != 1 {
			t.Errorf("faktúry inej firmy: %d", len(l))
		}
		for _, k := range []string{fmt.Sprintf("stock.%d", b.ID), "legal.vat"} {
			if _, err := s.GetSetting(ctx, k); err != nil {
				t.Errorf("%s: %v", k, err)
			}
		}
		if err := s.DeleteCompany(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("druhé zmazanie: %v", err)
		}
	})
	t.Run("úprava firmy", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa", DefaultDueDays: 14})
		c.Name, c.IBAN, c.VATPayer, c.DefaultDueDays, c.Email = "Alfa s.r.o.", "SK31 1200", true, 30, "a@b.sk"
		c.Units = []string{"ks", "hod", "m²"}
		c.NumberFormats = map[string]string{"invoice": "{RRRR}/{NNN}", "credit_note": "DB{RRRR}{NNNN}"}
		c.TaxOffice, c.VATPeriod = "Daňový úrad Banská Bystrica", store.VATPeriodQuarter
		c.Bookkeeping, c.SetupDone, c.MigratedFrom, c.MigratedAt = store.BookkeepingDouble, true, "POHODA", "2026-10-31T12:00:00Z"
		if err := s.UpdateCompany(ctx, c); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetCompany(ctx, c.ID)
		if !reflect.DeepEqual(got, c) {
			t.Fatalf("po úprave:\n got %+v\nwant %+v", got, c)
		}
		// zmena načítanej kópie nesmie zmeniť uložené dáta
		got.Units[0] = "zmenené"
		got.NumberFormats["invoice"] = "zmenené"
		again, _ := s.GetCompany(ctx, c.ID)
		if again.Units[0] != "ks" || again.NumberFormats["invoice"] != "{RRRR}/{NNN}" {
			t.Fatal("úložisko vracia zdieľané číselníky firmy")
		}
		if err := s.UpdateCompany(ctx, store.Company{ID: 9999, Name: "x"}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("neexistujúca firma: %v", err)
		}
	})

	t.Run("adresár", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		for _, n := range []struct{ name, city, ico string }{
			{"Žilinská strojáreň", "Žilina", "31322832"},
			{"Agro Liptov", "Liptovský Mikuláš", ""},
			{"Kováč Peter", "Banská Bystrica", ""},
		} {
			c := &store.Contact{Name: n.name, City: n.city, ICO: n.ico, Country: "SK", DueDays: 30}
			if err := s.SaveContact(ctx, a.ID, c); err != nil || c.ID == 0 {
				t.Fatalf("SaveContact: %v", err)
			}
		}
		list, _ := s.ListContacts(ctx, a.ID, "")
		if len(list) != 3 || list[0].Name != "Agro Liptov" || list[2].Name != "Žilinská strojáreň" {
			t.Fatalf("zoradenie podľa názvu (Ž za K): %v", names(list))
		}
		for q, want := range map[string]int{"kovac": 1, "ZILINA": 1, "3132": 1, "bystrica": 1, "nič": 0} {
			if got, _ := s.ListContacts(ctx, a.ID, q); len(got) != want {
				t.Errorf("hľadanie %q: %d, chcem %d", q, len(got), want)
			}
		}

		// úprava
		c := list[1]
		c.Email = "agro@liptov.sk"
		if err := s.SaveContact(ctx, a.ID, &c); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetContact(ctx, a.ID, c.ID)
		if got.Email != "agro@liptov.sk" || got.DueDays != 30 || got.CompanyID != a.ID {
			t.Fatalf("po úprave: %+v", got)
		}

		// oddelenie firiem
		if list, _ := s.ListContacts(ctx, b.ID, ""); len(list) != 0 {
			t.Fatal("firma B vidí adresár firmy A")
		}
		if _, err := s.GetContact(ctx, b.ID, c.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B načítala kontakt firmy A")
		}
		stolen := c
		if err := s.SaveContact(ctx, b.ID, &stolen); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B prepísala kontakt firmy A")
		}
		if err := s.DeleteContact(ctx, b.ID, c.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B zmazala kontakt firmy A")
		}

		// zmazanie
		if err := s.DeleteContact(ctx, a.ID, c.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetContact(ctx, a.ID, c.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("kontakt nebol zmazaný")
		}
	})

	t.Run("kontaktné osoby a adresy", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		c := &store.Contact{Name: "Kováč a syn s.r.o.", Country: "SK",
			Persons:   []store.ContactPerson{{Name: "Peter Kováč", Role: "konateľ", Email: "peter@kovac.sk", Primary: true}, {Name: "Eva Malá", Role: "účtovníčka"}},
			Addresses: []store.ContactAddress{{Kind: store.AddressDelivery, Name: "Sklad", Street: "Priemyselná 1", City: "Nitra", Zip: "94901", Country: "SK", Default: true}}}
		if err := s.SaveContact(ctx, a.ID, c); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetContact(ctx, a.ID, c.ID)
		if len(got.Persons) != 2 || got.Persons[0].Role != "konateľ" || !got.Persons[0].Primary || len(got.Addresses) != 1 ||
			got.Addresses[0].Name != "Sklad" || !got.Addresses[0].Default {
			t.Fatalf("osoby a adresy: %+v", got)
		}
		// zmena kópie nemení uložené dáta
		got.Persons[0].Name = "Iný"
		if again, _ := s.GetContact(ctx, a.ID, c.ID); again.Persons[0].Name != "Peter Kováč" {
			t.Fatal("úložisko vracia zdieľaný zoznam osôb")
		}
		// odstránenie osôb a adries
		got.Persons, got.Addresses = nil, nil
		if err := s.SaveContact(ctx, a.ID, &got); err != nil {
			t.Fatal(err)
		}
		if again, _ := s.GetContact(ctx, a.ID, c.ID); len(again.Persons) != 0 || len(again.Addresses) != 0 {
			t.Fatalf("po odstránení: %+v", again)
		}
		if list, _ := s.ListContacts(ctx, a.ID, ""); len(list) != 1 || len(list[0].Persons) != 0 {
			t.Fatalf("zoznam: %+v", list)
		}
		// dodacia adresa a kontaktná osoba na faktúre
		inv := draft("Kováč a syn s.r.o.", "2026-10-01")
		inv.Delivery = &invoice.Address{Name: "Sklad", Street: "Priemyselná 1", City: "Nitra", Zip: "94901", Country: "SK"}
		inv.Attention = &invoice.Person{Name: "Peter Kováč", Phone: "+421 905 123 456"}
		if err := s.SaveInvoice(ctx, a.ID, inv); err != nil {
			t.Fatal(err)
		}
		fi, _ := s.GetInvoice(ctx, a.ID, inv.ID)
		if fi.Delivery == nil || fi.Delivery.Street != "Priemyselná 1" || fi.Attention == nil || fi.Attention.Phone != "+421 905 123 456" {
			t.Fatalf("faktúra: %+v %+v", fi.Delivery, fi.Attention)
		}
		fi.Delivery, fi.Attention = nil, nil
		_ = s.SaveInvoice(ctx, a.ID, &fi)
		if fi2, _ := s.GetInvoice(ctx, a.ID, inv.ID); fi2.Delivery != nil || fi2.Attention != nil {
			t.Fatal("dodacia adresa ostala")
		}
	})

	t.Run("faktúra si pamätá kontakt a režim neplatiteľa", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		inv := draft("Odberateľ", "2026-10-01")
		inv.ContactID = 42
		inv.NonVATPayer = true
		inv.PaymentMethod = invoice.PayCash
		inv.SupplyKind = invoice.SupplyServices
		inv.Subject = "Oprava servera"
		inv.Intro = "Fakturujeme vám opravu."
		inv.Kind = invoice.KindCreditNote
		inv.Correction = &invoice.Correction{OriginalID: 5, OriginalNumber: "FA20260005",
			OriginalIssueDate: invoice.MustDate("2026-09-01"), OriginalDeliveryDate: invoice.MustDate("2026-08-31"), Reason: "Vrátenie"}
		_ = s.SaveInvoice(ctx, c.ID, inv)
		got, _ := s.GetInvoice(ctx, c.ID, inv.ID)
		if got.Correction == nil || *got.Correction != *inv.Correction || got.Kind != invoice.KindCreditNote {
			t.Fatalf("údaje dobropisu: %+v", got.Correction)
		}
		plain := draft("Bez opravy", "2026-10-01")
		_ = s.SaveInvoice(ctx, c.ID, plain)
		if p, _ := s.GetInvoice(ctx, c.ID, plain.ID); p.Correction != nil {
			t.Fatal("bežná faktúra nemá mať údaje dobropisu")
		}
		if got.Advance != nil {
			t.Fatal("dobropis nemá zálohu")
		}
		if got.Subject != "Oprava servera" || got.Intro != "Fakturujeme vám opravu." {
			t.Fatalf("predmet a úvodný text: %q %q", got.Subject, got.Intro)
		}
		if l, _ := s.ListInvoices(ctx, c.ID, store.InvoiceFilter{Search: "oprava serv"}); len(l) != 1 || l[0].ID != inv.ID {
			t.Fatalf("hľadanie podľa predmetu: %d", len(l))
		}
		if got.ContactID != 42 || !got.NonVATPayer || got.PaymentMethod != invoice.PayCash || got.SupplyKind != invoice.SupplyServices {
			t.Fatalf("načítané: contact=%d nonVat=%v", got.ContactID, got.NonVATPayer)
		}
	})

	t.Run("pokladňa", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		d1 := cash.Document{Number: "PPD20260001", Kind: cash.In, Date: invoice.MustDate("2026-10-01"), Amount: m.MustParse("500"),
			Partner: "Majiteľ", Purpose: "Vklad do pokladne", Note: "poznámka"}
		d2 := cash.Document{Number: "VPD20260001", Kind: cash.Out, Date: invoice.MustDate("2026-10-05"), Amount: m.MustParse("12.30"),
			Purpose: "Úhrada PF20260001", LinkKind: "purchase", LinkID: 9, LinkNumber: "PF20260001"}
		d3 := cash.Document{Number: "PPD20250001", Kind: cash.In, Date: invoice.MustDate("2025-12-31"), Amount: m.MustParse("1"), Purpose: "x"}
		d4 := cash.Document{Number: "VPD20260002", Kind: cash.Out, Date: invoice.MustDate("2026-10-06"), Amount: m.MustParse("24.60"),
			Purpose: "Pohonné hmoty", Tax: &cash.Tax{Rows: []cash.VATRow{{Rate: 23, Gross: m.MustParse("24.60"), Base: m.MustParse("20.00"), VAT: m.MustParse("4.60")}},
				SupplierICDPH: "SK2020372640", Expense: "other"}}
		for _, d := range []*cash.Document{&d1, &d2, &d3, &d4} {
			if err := s.SaveCashDocument(ctx, a.ID, d); err != nil || d.ID == 0 || d.CompanyID != a.ID {
				t.Fatalf("uloženie: %v", err)
			}
		}
		got, err := s.GetCashDocument(ctx, a.ID, d2.ID)
		if err != nil || !reflect.DeepEqual(got, d2) {
			t.Fatalf("načítanie:\n got %+v\nwant %+v", got, d2)
		}
		if got, err := s.GetCashDocument(ctx, a.ID, d4.ID); err != nil || !reflect.DeepEqual(got, d4) {
			t.Fatalf("doklad s DPH:\n got %+v\nwant %+v", got, d4)
		}
		if _, err := s.GetCashDocument(ctx, b.ID, d2.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("doklad inej firmy")
		}
		list, _ := s.ListCashDocuments(ctx, a.ID, 2026)
		if len(list) != 3 || list[1].ID != d2.ID {
			t.Fatalf("zoznam: %+v", list)
		}
		if all, _ := s.ListCashDocuments(ctx, a.ID, 0); len(all) != 4 {
			t.Fatal("všetky roky")
		}
		d1.Cancelled = true
		if err := s.SaveCashDocument(ctx, a.ID, &d1); err != nil {
			t.Fatal(err)
		}
		if g, _ := s.GetCashDocument(ctx, a.ID, d1.ID); !g.Cancelled {
			t.Fatal("storno sa neuložilo")
		}
		if err := s.SaveCashDocument(ctx, b.ID, &d1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("úprava dokladu inej firmy")
		}
		// druhá pokladňa v cudzej mene
		d5 := cash.Document{Number: "CZPPD20260001", Kind: cash.In, Date: invoice.MustDate("2026-10-07"), Amount: m.MustParse("2500"),
			Purpose: "Tržba", RegisterID: 3, Currency: "CZK", FXRate: m.MustParse("25.125"), FXDate: invoice.MustDate("2026-10-06"),
			AmountEUR: m.MustParse("99.50")}
		if err := s.SaveCashDocument(ctx, a.ID, &d5); err != nil {
			t.Fatal(err)
		}
		if got, err := s.GetCashDocument(ctx, a.ID, d5.ID); err != nil || !reflect.DeepEqual(got, d5) {
			t.Fatalf("doklad v cudzej mene:\n got %+v\nwant %+v", got, d5)
		}
	})

	t.Run("cenové ponuky", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		q1 := quote.Quote{Status: quote.StatusDraft, Customer: invoice.Party{Name: "Odberateľ", Country: "SK", ICO: "35757442"},
			IssueDate: invoice.MustDate("2026-10-01"), ValidUntil: invoice.MustDate("2026-10-31"), ContactID: 4,
			Lines: []invoice.Line{{Description: "Montáž", Quantity: m.MustParse("2"), Unit: "hod", UnitPrice: m.MustParse("40"), Rate: 23}},
			Note:  "poznámka", SupplyKind: invoice.SupplyServices}
		if err := q1.Recalculate(); err != nil {
			t.Fatal(err)
		}
		q2 := q1.Copy(invoice.MustDate("2026-10-20"), 30)
		q3 := q1.Copy(invoice.MustDate("2025-12-01"), 30)
		for _, q := range []*quote.Quote{&q1, &q2, &q3} {
			if err := s.SaveQuote(ctx, a.ID, q); err != nil || q.ID == 0 || q.CompanyID != a.ID {
				t.Fatalf("uloženie: %v", err)
			}
		}
		got, err := s.GetQuote(ctx, a.ID, q1.ID)
		if err != nil || !reflect.DeepEqual(got, q1) {
			t.Fatalf("načítanie:\n got %+v\nwant %+v", got, q1)
		}
		if _, err := s.GetQuote(ctx, b.ID, q1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("ponuka inej firmy")
		}
		list, _ := s.ListQuotes(ctx, a.ID, 2026)
		if len(list) != 2 || list[0].ID != q2.ID {
			t.Fatalf("zoznam: %+v", list)
		}
		if all, _ := s.ListQuotes(ctx, a.ID, 0); len(all) != 3 {
			t.Fatal("všetky roky")
		}
		if other, _ := s.ListQuotes(ctx, b.ID, 0); len(other) != 0 {
			t.Fatal("zoznam inej firmy")
		}
		if err := q1.Issue("CP20260001"); err != nil {
			t.Fatal(err)
		}
		_ = q1.MarkInvoiced(12, "FA20260012")
		if err := s.SaveQuote(ctx, a.ID, &q1); err != nil {
			t.Fatal(err)
		}
		if g, _ := s.GetQuote(ctx, a.ID, q1.ID); g.Status != quote.StatusInvoiced || g.Number != "CP20260001" || g.InvoiceID != 12 {
			t.Fatalf("zmena stavu: %+v", g)
		}
		got.Lines[0].Description = "zmenené mimo úložiska"
		if g, _ := s.GetQuote(ctx, a.ID, q1.ID); g.Lines[0].Description != "Montáž" {
			t.Fatal("úložisko zdieľa položky s volajúcim")
		}
		if err := s.SaveQuote(ctx, b.ID, &q1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("úprava ponuky inej firmy")
		}
	})

	t.Run("dlhodobý majetok", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		car := asset.Asset{Number: "M001", Name: "Osobné auto", Kind: asset.Tangible, Group: 1, Method: asset.Straight,
			Acquired: invoice.MustDate("2025-03-15"), Cost: m.MustParse("24000"), Share: 80,
			PurchaseID: 7, PurchaseNo: "PF2025007", Note: "služobné"}
		pc := asset.Asset{Number: "M002", Name: "Server", Kind: asset.Tangible, Group: 1, Method: asset.Accelerated,
			Acquired: invoice.MustDate("2024-11-02"), Cost: m.MustParse("3200")}
		for _, x := range []*asset.Asset{&car, &pc} {
			if err := s.SaveAsset(ctx, a.ID, x); err != nil || x.ID == 0 || x.CompanyID != a.ID {
				t.Fatalf("uloženie: %v", err)
			}
		}
		got, err := s.GetAsset(ctx, a.ID, car.ID)
		if err != nil || !reflect.DeepEqual(got, car) {
			t.Fatalf("načítanie:\n got %+v\nwant %+v", got, car)
		}
		if _, err := s.GetAsset(ctx, b.ID, car.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("majetok inej firmy")
		}
		list, _ := s.ListAssets(ctx, a.ID)
		if len(list) != 2 || list[0].ID != pc.ID {
			t.Fatalf("zoznam podľa zaradenia: %+v", list)
		}
		if other, _ := s.ListAssets(ctx, b.ID); len(other) != 0 {
			t.Fatal("zoznam inej firmy")
		}
		car.Disposed, car.DisposalKind = invoice.MustDate("2026-06-30"), asset.DisposalSale
		if err := s.SaveAsset(ctx, a.ID, &car); err != nil {
			t.Fatal(err)
		}
		if g, _ := s.GetAsset(ctx, a.ID, car.ID); g.DisposalKind != asset.DisposalSale || g.Disposed != car.Disposed {
			t.Fatalf("vyradenie: %+v", g)
		}
		if err := s.SaveAsset(ctx, b.ID, &car); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("úprava majetku inej firmy")
		}
		if err := s.DeleteAsset(ctx, b.ID, car.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zmazanie majetku inej firmy")
		}
		if err := s.DeleteAsset(ctx, a.ID, car.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetAsset(ctx, a.ID, car.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zmazaný majetok")
		}
	})

	t.Run("kniha jázd", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		car := logbook.Vehicle{Name: "Octavia", Plate: "BB123AB", Fuel: logbook.Diesel, Consumption: m.MustParse("5.2"),
			Ownership: logbook.Business, Method: logbook.MethodLog, AssetID: 4, Odometer: 12000, OdometerDate: invoice.MustDate("2026-01-01")}
		van := logbook.Vehicle{Name: "Transit", Plate: "BB456CD", Fuel: logbook.Diesel, Ownership: logbook.Private, Method: logbook.MethodFlat}
		for _, v := range []*logbook.Vehicle{&car, &van} {
			if err := s.SaveVehicle(ctx, a.ID, v); err != nil || v.ID == 0 {
				t.Fatalf("vozidlo: %v", err)
			}
		}
		if got, err := s.GetVehicle(ctx, a.ID, car.ID); err != nil || !reflect.DeepEqual(got, car) {
			t.Fatalf("načítanie vozidla:\n got %+v\nwant %+v", got, car)
		}
		if _, err := s.GetVehicle(ctx, b.ID, car.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("vozidlo inej firmy")
		}
		if list, _ := s.ListVehicles(ctx, a.ID); len(list) != 2 {
			t.Fatalf("vozidlá: %+v", list)
		}
		trips := []logbook.Trip{
			{VehicleID: car.ID, Date: invoice.MustDate("2026-10-02"), Depart: "07:30", Arrive: "08:10", From: "Banská Bystrica", To: "Zvolen",
				Purpose: "Montáž", Kind: logbook.TripBusiness, Driver: "Pavol", OdoStart: 12000, OdoEnd: 12025, Km: 25},
			{VehicleID: car.ID, Date: invoice.MustDate("2026-10-01"), From: "BB", To: "BB", Kind: logbook.TripPrivate, Km: 10},
			{VehicleID: van.ID, Date: invoice.MustDate("2025-12-30"), From: "BB", To: "Brno", Purpose: "Nákup", Kind: logbook.TripBusiness, Km: 300},
		}
		for i := range trips {
			if err := s.SaveTrip(ctx, a.ID, &trips[i]); err != nil || trips[i].ID == 0 {
				t.Fatalf("jazda: %v", err)
			}
		}
		if got, err := s.GetTrip(ctx, a.ID, trips[0].ID); err != nil || !reflect.DeepEqual(got, trips[0]) {
			t.Fatalf("načítanie jazdy:\n got %+v\nwant %+v", got, trips[0])
		}
		if list, _ := s.ListTrips(ctx, a.ID, car.ID, 2026); len(list) != 2 || list[0].ID != trips[1].ID {
			t.Fatalf("jazdy auta podľa dátumu: %+v", list)
		}
		if list, _ := s.ListTrips(ctx, a.ID, 0, 0); len(list) != 3 {
			t.Fatalf("všetky jazdy: %+v", list)
		}
		if list, _ := s.ListTrips(ctx, b.ID, 0, 0); len(list) != 0 {
			t.Fatal("jazdy inej firmy")
		}
		f := logbook.Fuel{VehicleID: car.ID, Date: invoice.MustDate("2026-10-03"), Quantity: m.MustParse("41.5"), Amount: m.MustParse("63.90"),
			Odometer: 12030, Full: true, Station: "Slovnaft", DocKind: "cash", DocID: 7, DocNumber: "VPD20260007"}
		if err := s.SaveFuel(ctx, a.ID, &f); err != nil || f.ID == 0 {
			t.Fatalf("tankovanie: %v", err)
		}
		if got, err := s.GetFuel(ctx, a.ID, f.ID); err != nil || !reflect.DeepEqual(got, f) {
			t.Fatalf("načítanie tankovania:\n got %+v\nwant %+v", got, f)
		}
		if _, err := s.GetTrip(ctx, a.ID, f.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("tankovanie načítané ako jazda")
		}
		if list, _ := s.ListFuel(ctx, a.ID, car.ID, 2026); len(list) != 1 {
			t.Fatalf("tankovania: %+v", list)
		}
		trips[0].Km, trips[0].OdoEnd = 30, 12030
		if err := s.SaveTrip(ctx, a.ID, &trips[0]); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveTrip(ctx, b.ID, &trips[0]); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("úprava jazdy inej firmy")
		}
		for _, del := range []func() error{
			func() error { return s.DeleteTrip(ctx, b.ID, trips[0].ID) },
			func() error { return s.DeleteFuel(ctx, b.ID, f.ID) },
			func() error { return s.DeleteVehicle(ctx, b.ID, car.ID) },
		} {
			if err := del(); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("zmazanie v inej firme")
			}
		}
		if s.DeleteTrip(ctx, a.ID, trips[0].ID) != nil || s.DeleteFuel(ctx, a.ID, f.ID) != nil || s.DeleteVehicle(ctx, a.ID, van.ID) != nil {
			t.Fatal("zmazanie")
		}
		if list, _ := s.ListTrips(ctx, a.ID, car.ID, 0); len(list) != 1 {
			t.Fatalf("po zmazaní: %+v", list)
		}
	})

	t.Run("pravidelná fakturácia", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		t1 := recurring.Template{Name: "Paušál", Active: true, Interval: recurring.Monthly, StartDate: invoice.MustDate("2026-01-15"),
			Customer: invoice.Party{Name: "Odberateľ", Country: "SK"}, DueDays: 10, AutoIssue: true,
			Lines: []invoice.Line{{Description: "Paušál {mesiac}", Quantity: m.MustParse("1"), Unit: "mes", UnitPrice: m.MustParse("100"), Rate: 23}}}
		t1.Record(recurring.Run{Period: invoice.MustDate("2026-01-15"), InvoiceID: 3, Issued: true})
		t2 := recurring.Template{Name: "Nájom", Interval: recurring.Quarterly, StartDate: invoice.MustDate("2026-04-01")}
		for _, x := range []*recurring.Template{&t1, &t2} {
			if err := s.SaveRecurring(ctx, a.ID, x); err != nil || x.ID == 0 || x.CompanyID != a.ID {
				t.Fatalf("uloženie: %v", err)
			}
		}
		got, err := s.GetRecurring(ctx, a.ID, t1.ID)
		if err != nil || !reflect.DeepEqual(got, t1) {
			t.Fatalf("načítanie:\n got %+v\nwant %+v", got, t1)
		}
		if _, err := s.GetRecurring(ctx, b.ID, t1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("šablóna inej firmy")
		}
		if list, _ := s.ListRecurring(ctx, a.ID); len(list) != 2 || list[0].ID != t1.ID {
			t.Fatalf("zoznam: %+v", list)
		}
		if list, _ := s.ListRecurring(ctx, b.ID); len(list) != 0 {
			t.Fatal("zoznam inej firmy")
		}
		got.History[0].InvoiceID = 99
		if g, _ := s.GetRecurring(ctx, a.ID, t1.ID); g.History[0].InvoiceID != 3 {
			t.Fatal("úložisko zdieľa históriu s volajúcim")
		}
		t1.Active = false
		if err := s.SaveRecurring(ctx, a.ID, &t1); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveRecurring(ctx, b.ID, &t1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("úprava šablóny inej firmy")
		}
		if err := s.DeleteRecurring(ctx, b.ID, t1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zmazanie šablóny inej firmy")
		}
		if err := s.DeleteRecurring(ctx, a.ID, t1.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetRecurring(ctx, a.ID, t1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("po zmazaní")
		}
	})

	t.Run("upomienky", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		inv := invoice.Invoice{ID: 5, Number: "FA20260005", Kind: invoice.KindInvoice, Status: invoice.StatusIssued,
			Customer: invoice.Party{Name: "Dlžník", ICO: "31322832"}, IssueDate: invoice.MustDate("2026-09-01"),
			DueDate: invoice.MustDate("2026-09-15"), Summary: vat.Summary{Total: m.MustParse("100")}}
		r1, err := reminder.Build([]invoice.Invoice{inv}, 1, reminder.Options{Date: invoice.MustDate("2026-10-01"),
			PayBy: invoice.MustDate("2026-10-08"), Interest: reminder.InterestStatutory, ChargeFee: true, Note: "pozn."})
		if err != nil {
			t.Fatal(err)
		}
		r1.Number = "UP20260001"
		r2 := r1
		r2.Number, r2.Date, r2.Level = "UP20260002", invoice.MustDate("2026-10-20"), 2
		r3 := r1
		r3.Number, r3.Date = "UP20250001", invoice.MustDate("2025-12-01")
		for _, x := range []*reminder.Reminder{&r1, &r2, &r3} {
			if err := s.SaveReminder(ctx, a.ID, x); err != nil || x.ID == 0 || x.CompanyID != a.ID {
				t.Fatalf("uloženie: %v", err)
			}
		}
		got, err := s.GetReminder(ctx, a.ID, r1.ID)
		if err != nil || !reflect.DeepEqual(got, r1) {
			t.Fatalf("načítanie:\n got %+v\nwant %+v", got, r1)
		}
		if _, err := s.GetReminder(ctx, b.ID, r1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("upomienka inej firmy")
		}
		if list, _ := s.ListReminders(ctx, a.ID, 2026); len(list) != 2 || list[0].ID != r2.ID {
			t.Fatalf("zoznam: %+v", list)
		}
		if all, _ := s.ListReminders(ctx, a.ID, 0); len(all) != 3 {
			t.Fatal("všetky roky")
		}
		if other, _ := s.ListReminders(ctx, b.ID, 0); len(other) != 0 {
			t.Fatal("zoznam inej firmy")
		}
		got.Items[0].Amount = m.MustParse("1")
		if g, _ := s.GetReminder(ctx, a.ID, r1.ID); !g.Items[0].Amount.Equal(m.MustParse("100")) {
			t.Fatal("úložisko zdieľa položky s volajúcim")
		}
		if err := s.SaveReminder(ctx, b.ID, &r1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("úprava upomienky inej firmy")
		}
		if err := s.DeleteReminder(ctx, b.ID, r1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zmazanie upomienky inej firmy")
		}
		if err := s.DeleteReminder(ctx, a.ID, r1.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetReminder(ctx, a.ID, r1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("po zmazaní")
		}
	})

	t.Run("nastavenia programu", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.GetSetting(ctx, "legal"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("chýbajúce nastavenie: %v", err)
		}
		for _, v := range []string{`{"a":1}`, `{"a":2,"text":"č"}`} {
			if err := s.SetSetting(ctx, "legal", v); err != nil {
				t.Fatal(err)
			}
			if got, err := s.GetSetting(ctx, "legal"); err != nil || got != v {
				t.Fatalf("načítanie: %q %v", got, err)
			}
		}
		if _, err := s.GetSetting(ctx, "iny"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("iný kľúč")
		}
	})

	t.Run("skladová karta a obrázky", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		it := &catalog.Item{Code: "CYKY", Name: "Kábel CYKY 3x2,5", Unit: "m", Price: m.MustParse("1.20"), PriceMode: vat.PriceNet, Rate: 23,
			EAN: "5901234123457", Category: "Káble / Silové", Brand: "NKT", Supplier: "Elektro VO", SupplierCode: "NKT-3x25",
			PurchasePrice: m.MustParse("0.8123"), Weight: m.MustParse("0.125"), Description: "Silový kábel\ndo zeme",
			Params: []catalog.Param{{Name: "Prierez", Value: "2,5", Unit: "mm²"}, {Name: "Počet žíl", Value: "3"}}}
		if err := s.SaveItem(ctx, a.ID, it); err != nil {
			t.Fatal(err)
		}
		it.Params[0].Value = "zmenené" // úložisko má vlastnú kópiu
		got, err := s.GetItem(ctx, a.ID, it.ID)
		it.Params[0].Value = "2,5"
		if err != nil || !reflect.DeepEqual(got, *it) {
			t.Fatalf("karta:\n got %+v\nwant %+v", got, *it)
		}
		// obrázky: poradie, dáta, cudzia firma, zmazanie s položkou
		var ids []int64
		for i, n := range []string{"a.jpg", "b.png", "c.webp"} {
			im := &store.ItemImage{CompanyID: a.ID, ItemID: it.ID, Name: n, ContentType: "image/jpeg", Data: []byte{byte(i), 1, 2}, Created: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)}
			if err := s.AddItemImage(ctx, im); err != nil || im.ID == 0 || im.Position != i || im.Size != 3 {
				t.Fatalf("pridanie %+v %v", im, err)
			}
			ids = append(ids, im.ID)
		}
		if err := s.AddItemImage(ctx, &store.ItemImage{CompanyID: b.ID, ItemID: it.ID, Data: []byte{1}}); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cudzia firma pridala: %v", err)
		}
		if err := s.SetItemImageOrder(ctx, a.ID, it.ID, []int64{ids[2], ids[0], ids[1]}); err != nil {
			t.Fatal(err)
		}
		list, _ := s.ListItemImages(ctx, a.ID, it.ID)
		if len(list) != 3 || list[0].Name != "c.webp" || list[0].Data != nil || list[1].Name != "a.jpg" {
			t.Fatalf("poradie %+v", list)
		}
		one, err := s.GetItemImage(ctx, a.ID, ids[1])
		if err != nil || one.Name != "b.png" || len(one.Data) != 3 || one.Data[0] != 1 || !one.Created.Equal(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("obrázok %+v %v", one, err)
		}
		if _, err := s.GetItemImage(ctx, b.ID, ids[1]); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cudzia firma číta: %v", err)
		}
		if err := s.SetItemImageOrder(ctx, b.ID, it.ID, ids); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cudzia firma triedi: %v", err)
		}
		if err := s.DeleteItemImage(ctx, a.ID, ids[0]); err != nil {
			t.Fatal(err)
		}
		if l, _ := s.ListItemImages(ctx, a.ID, it.ID); len(l) != 2 {
			t.Errorf("po zmazaní %d", len(l))
		}
		if err := s.DeleteItem(ctx, a.ID, it.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetItemImage(ctx, a.ID, ids[1]); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("obrázok po zmazaní položky: %v", err)
		}
	})
	t.Run("cenník", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		items := []*catalog.Item{
			{Code: "SV1", Name: "Servisný výjazd", Unit: "km", Price: m.MustParse("0.45"), PriceMode: vat.PriceNet, Rate: 23, Note: "pozn."},
			{Name: "Úprava HMI", Unit: "hod", Price: m.MustParse("51.66"), PriceMode: vat.PriceGross, Rate: 23, Archived: true},
			{Code: "M1", Name: "montáž", Unit: "hod", Price: m.MustParse("40"), PriceMode: vat.PriceNet, Rate: 23},
		}
		for _, it := range items {
			if err := s.SaveItem(ctx, a.ID, it); err != nil || it.ID == 0 || it.CompanyID != a.ID {
				t.Fatalf("uloženie: %v", err)
			}
		}
		got, err := s.GetItem(ctx, a.ID, items[1].ID)
		if err != nil || !reflect.DeepEqual(got, *items[1]) {
			t.Fatalf("načítanie:\n got %+v\nwant %+v", got, *items[1])
		}
		list, _ := s.ListItems(ctx, a.ID)
		if len(list) != 3 || list[0].Name != "montáž" || list[1].Name != "Servisný výjazd" || list[2].Name != "Úprava HMI" {
			t.Fatalf("poradie podľa názvu bez diakritiky: %+v", list)
		}
		if _, err := s.GetItem(ctx, b.ID, items[0].ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("položka inej firmy")
		}
		if other, _ := s.ListItems(ctx, b.ID); len(other) != 0 {
			t.Fatal("cenník inej firmy")
		}
		dup := catalog.Item{Code: "sv1", Name: "Iný", Unit: "ks", PriceMode: vat.PriceNet, Rate: 23}
		if err := s.SaveItem(ctx, a.ID, &dup); !errors.Is(err, store.ErrDuplicate) {
			t.Fatalf("rovnaký kód: %v", err)
		}
		if err := s.SaveItem(ctx, b.ID, &dup); err != nil {
			t.Fatalf("rovnaký kód v inej firme: %v", err)
		}
		items[2].Code = "SV1"
		if err := s.SaveItem(ctx, a.ID, items[2]); !errors.Is(err, store.ErrDuplicate) {
			t.Fatalf("zmena na existujúci kód: %v", err)
		}
		items[2].Code, items[2].Price = "M1", m.MustParse("42")
		if err := s.SaveItem(ctx, a.ID, items[2]); err != nil {
			t.Fatal(err)
		}
		if g, _ := s.GetItem(ctx, a.ID, items[2].ID); !g.Price.Equal(m.MustParse("42")) {
			t.Fatal("úprava ceny")
		}
		if err := s.SaveItem(ctx, b.ID, items[0]); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("úprava položky inej firmy")
		}
		if err := s.DeleteItem(ctx, b.ID, items[0].ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zmazanie položky inej firmy")
		}
		if err := s.DeleteItem(ctx, a.ID, items[0].ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetItem(ctx, a.ID, items[0].ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("po zmazaní")
		}
	})

	t.Run("kurzy mien", func(t *testing.T) {
		s := newStore(t)
		r := func(cur, date, rate, src string) fx.Rate {
			return fx.Rate{Currency: cur, Date: date, Rate: m.MustParse(rate), Source: src}
		}
		if _, err := s.RateBefore(ctx, "CZK", "2026-10-24"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("bez kurzov: %v", err)
		}
		err := s.SaveRates(ctx, []fx.Rate{
			r("CZK", "2026-10-23", "25.123", fx.SourceECB), r("CZK", "2026-10-22", "25.2", fx.SourceECB),
			r("USD", "2026-10-23", "1.0823", fx.SourceECB), r("CZK", "2025-12-31", "24.9", fx.SourceECB),
		})
		if err != nil {
			t.Fatal(err)
		}
		// ručný kurz prepíše ECB, ECB neprepíše ručný
		if err := s.SaveRates(ctx, []fx.Rate{r("CZK", "2026-10-22", "25.3", fx.SourceManual)}); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveRates(ctx, []fx.Rate{r("CZK", "2026-10-22", "25.2", fx.SourceECB), r("CZK", "2026-10-23", "25.124", fx.SourceECB)}); err != nil {
			t.Fatal(err)
		}
		tests := []struct{ cur, point, date, rate string }{
			{"CZK", "2026-10-24", "2026-10-23", "25.124"},
			{"CZK", "2026-10-23", "2026-10-22", "25.30"},
			{"CZK", "2026-01-05", "2025-12-31", "24.90"},
			{"USD", "2026-10-26", "2026-10-23", "1.0823"},
		}
		for _, tt := range tests {
			got, err := s.RateBefore(ctx, tt.cur, tt.point)
			if err != nil || got.Date != tt.date || got.Rate.String() != m.MustParse(tt.rate).String() {
				t.Errorf("RateBefore(%s, %s) = %+v %v", tt.cur, tt.point, got, err)
			}
		}
		if got, _ := s.RateBefore(ctx, "CZK", "2026-10-23"); got.Source != fx.SourceManual {
			t.Errorf("zdroj: %+v", got)
		}
		list, _ := s.ListRates(ctx, fx.Filter{})
		if len(list) != 4 || list[0].Currency != "CZK" || list[0].Date != "2026-10-23" || list[3].Date != "2025-12-31" {
			t.Fatalf("zoznam: %+v", list)
		}
		if list, _ := s.ListRates(ctx, fx.Filter{Currency: "CZK", Year: 2026}); len(list) != 2 {
			t.Fatalf("filter: %+v", list)
		}
		if err := s.DeleteRate(ctx, "CZK", "2026-10-22"); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteRate(ctx, "CZK", "2026-10-22"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("druhé zmazanie: %v", err)
		}
	})

	t.Run("úhrady faktúry", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		inv := draft("Odberateľ", "2026-10-01")
		if err := inv.Issue("FA1"); err != nil {
			t.Fatal(err)
		}
		if err := inv.AddPayment(invoice.Payment{Date: invoice.MustDate("2026-10-05"), Amount: m.MustParse("10"), Source: invoice.PaymentBank, Ref: 3}); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveInvoice(ctx, c.ID, inv); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetInvoice(ctx, c.ID, inv.ID)
		if err != nil || !reflect.DeepEqual(got.Payments, inv.Payments) || got.PaidAmount.String() != "10.00" || got.Status != invoice.StatusIssued {
			t.Fatalf("načítanie: %+v %s %v", got.Payments, got.PaidAmount, err)
		}
	})

	t.Run("faktúra v cudzej mene", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		inv := draft("Odberateľ", "2026-10-01")
		inv.Currency, inv.FXRate, inv.FXDate, inv.FXManual = "CZK", m.MustParse("25.123"), invoice.MustDate("2026-09-30"), true
		if err := inv.Recalculate(); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveInvoice(ctx, c.ID, inv); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetInvoice(ctx, c.ID, inv.ID)
		if err != nil || got.FXRate != inv.FXRate || got.FXDate != inv.FXDate || !got.FXManual || got.Local == nil ||
			!reflect.DeepEqual(*got.Local, *inv.Local) {
			t.Fatalf("načítanie: %+v %v", got, err)
		}
	})

	t.Run("e-faktúry", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		at := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
		out := store.EInvoice{Direction: "out", DocID: 7, DocNumber: "FA1", Partner: "Odberateľ", PeppolID: "0245:2020372640",
			File: "Faktura-FA1.xml", MessageID: "Faktura-FA1.xml", Status: "odoslané", Created: at, User: "Palo"}
		in := store.EInvoice{Direction: "in", DocNumber: "D-9", Partner: "Dodávateľ", MessageID: "d9.xml", Created: at.Add(time.Minute), Data: []byte("<Invoice/>")}
		for _, e := range []*store.EInvoice{&out, &in} {
			if err := s.AddEInvoice(ctx, a.ID, e); err != nil || e.ID == 0 || e.CompanyID != a.ID {
				t.Fatalf("AddEInvoice: %v %+v", err, e)
			}
		}
		list, _ := s.ListEInvoices(ctx, a.ID, store.EInvoiceFilter{})
		if len(list) != 2 || list[0].ID != in.ID || list[0].Data != nil || !list[1].Created.Equal(at) || list[1].PeppolID != "0245:2020372640" {
			t.Fatalf("zoznam: %+v", list)
		}
		for f, want := range map[store.EInvoiceFilter]int{{Direction: "out"}: 1, {DocID: 7}: 1, {MessageID: "d9.xml"}: 1, {Direction: "in", DocID: 7}: 0} {
			if l, _ := s.ListEInvoices(ctx, a.ID, f); len(l) != want {
				t.Errorf("filter %+v: %d", f, len(l))
			}
		}
		got, err := s.GetEInvoice(ctx, a.ID, in.ID)
		if err != nil || string(got.Data) != "<Invoice/>" || got.Partner != "Dodávateľ" {
			t.Fatalf("GetEInvoice: %+v %v", got, err)
		}
		if l, _ := s.ListEInvoices(ctx, b.ID, store.EInvoiceFilter{}); len(l) != 0 {
			t.Fatal("firma B vidí e-faktúry firmy A")
		}
		if _, err := s.GetEInvoice(ctx, b.ID, in.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B načítala e-faktúru firmy A")
		}
	})

	t.Run("odoslané e-maily", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		at := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
		add := func(cid int64, e store.Email) store.Email {
			t.Helper()
			if err := s.AddEmail(ctx, cid, &e); err != nil || e.ID == 0 || e.CompanyID != cid {
				t.Fatalf("AddEmail: %v %+v", err, e)
			}
			return e
		}
		first := add(a.ID, store.Email{DocKind: "invoice", DocID: 7, DocNumber: "FA20260007", To: "x@firma.sk, y@firma.sk", Cc: "z@firma.sk",
			Subject: "Faktúra FA20260007", File: "Faktura-FA20260007.pdf", Sent: at, User: "Palo"})
		add(a.ID, store.Email{DocKind: "invoice", DocID: 7, To: "x@firma.sk", Subject: "znova", Sent: at.Add(time.Hour), Error: "535 zlé heslo"})
		add(a.ID, store.Email{DocKind: "quote", DocID: 7, To: "x@firma.sk", Subject: "ponuka", Sent: at.Add(-time.Hour)})
		add(a.ID, store.Email{DocKind: "invoice", DocID: 1, To: "x@firma.sk", Subject: "minulý rok", Sent: at.AddDate(-1, 0, 0)})
		add(b.ID, store.Email{DocKind: "invoice", DocID: 7, To: "cudzi@firma.sk", Subject: "iná firma", Sent: at})

		tests := []struct {
			name string
			f    store.EmailFilter
			want []string
		}{
			{"všetky, najnovšie prvé", store.EmailFilter{}, []string{"znova", "Faktúra FA20260007", "ponuka", "minulý rok"}},
			{"rok", store.EmailFilter{Year: 2026}, []string{"znova", "Faktúra FA20260007", "ponuka"}},
			{"doklad", store.EmailFilter{DocKind: "invoice", DocID: 7}, []string{"znova", "Faktúra FA20260007"}},
			{"druh", store.EmailFilter{DocKind: "quote"}, []string{"ponuka"}},
		}
		for _, tt := range tests {
			got, err := s.ListEmails(ctx, a.ID, tt.f)
			if err != nil {
				t.Fatal(err)
			}
			subj := make([]string, len(got))
			for i, e := range got {
				subj[i] = e.Subject
			}
			if !reflect.DeepEqual(subj, tt.want) {
				t.Errorf("%s: %v", tt.name, subj)
			}
		}
		got, _ := s.ListEmails(ctx, a.ID, store.EmailFilter{DocKind: "invoice", DocID: 7})
		if g := got[1]; !g.Sent.Equal(first.Sent) || g.To != first.To || g.Cc != first.Cc || g.File != first.File || g.User != "Palo" || g.DocNumber != "FA20260007" {
			t.Errorf("záznam: %+v", g)
		}
		if got[0].Error != "535 zlé heslo" {
			t.Errorf("chyba: %+v", got[0])
		}
	})

	t.Run("logo firmy", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		if _, err := s.GetLogo(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("bez loga: %v", err)
		}
		at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
		l := store.Logo{ContentType: "image/png", Data: []byte{0x89, 'P', 'N', 'G', 0, 1, 2}, Updated: at}
		if err := s.SaveLogo(ctx, a.ID, l); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetLogo(ctx, a.ID)
		if err != nil || !reflect.DeepEqual(got, l) {
			t.Fatalf("načítanie: %+v %v", got, err)
		}
		if _, err := s.GetLogo(ctx, b.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("logo inej firmy")
		}
		l2 := store.Logo{ContentType: "image/jpeg", Data: []byte{0xff, 0xd8}, Updated: at.Add(time.Hour)}
		if err := s.SaveLogo(ctx, a.ID, l2); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.GetLogo(ctx, a.ID); got.ContentType != "image/jpeg" || len(got.Data) != 2 {
			t.Fatalf("výmena loga: %+v", got)
		}
		if err := s.SaveLogo(ctx, 9999, l); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("logo neexistujúcej firmy: %v", err)
		}
		if err := s.DeleteLogo(ctx, b.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zmazanie neexistujúceho loga")
		}
		if err := s.DeleteLogo(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetLogo(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("po zmazaní")
		}
	})

	t.Run("väzby záloh", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		inv := draft("Odberateľ", "2026-10-20")
		full := vat.Summary{Rates: []vat.RateSummary{{Rate: 23, Base: m.MustParse("100"), VAT: m.MustParse("23"), Total: m.MustParse("123")}},
			Base: m.MustParse("100"), VAT: m.MustParse("23"), Total: m.MustParse("123")}
		inv.Advance = &invoice.Advance{ProformaID: 3, ProformaNumber: "ZF20260001", Supply: &full,
			Deductions: []invoice.Deduction{{ReceiptID: 4, Number: "ZD20260001", Date: invoice.MustDate("2026-10-10"),
				Rates: full.Rates, Total: full.Total}}}
		if err := s.SaveInvoice(ctx, c.ID, inv); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetInvoice(ctx, c.ID, inv.ID)
		if err != nil || !reflect.DeepEqual(got.Advance, inv.Advance) {
			t.Fatalf("záloha:\n got %+v\nwant %+v", got.Advance, inv.Advance)
		}
		got.Advance = &invoice.Advance{FinalID: 9, FinalNumber: "FA20260009"}
		if err := s.SaveInvoice(ctx, c.ID, &got); err != nil {
			t.Fatal(err)
		}
		if g, _ := s.GetInvoice(ctx, c.ID, inv.ID); g.Advance == nil || g.Advance.FinalID != 9 || g.Advance.Supply != nil {
			t.Fatalf("zmena väzby: %+v", g.Advance)
		}
	})

	t.Run("peňažný denník – ručné rozpisy", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		o := store.JournalOverride{Key: "bank:7", Note: "poistné", Lines: []journal.Line{{Column: journal.ExpInsur, Amount: m.MustParse("216.12")}}}
		if err := s.SaveJournalOverride(ctx, a.ID, o); err != nil {
			t.Fatal(err)
		}
		o.Lines = append(o.Lines, journal.Line{Column: journal.ExpOther, Amount: m.MustParse("1")})
		o.Note = "zmena"
		_ = s.SaveJournalOverride(ctx, a.ID, o) // prepíše
		_ = s.SaveJournalOverride(ctx, a.ID, store.JournalOverride{Key: "cash:3", Lines: []journal.Line{}})
		got, err := s.ListJournalOverrides(ctx, a.ID)
		if err != nil || len(got) != 2 || got["bank:7"].Note != "zmena" || len(got["bank:7"].Lines) != 2 ||
			got["bank:7"].Lines[0].Column != journal.ExpInsur || !got["bank:7"].Lines[0].Amount.Equal(m.MustParse("216.12")) {
			t.Fatalf("%+v %v", got, err)
		}
		if other, _ := s.ListJournalOverrides(ctx, b.ID); len(other) != 0 {
			t.Fatal("iná firma")
		}
		if err := s.DeleteJournalOverride(ctx, b.ID, "bank:7"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zmazanie v inej firme")
		}
		if err := s.DeleteJournalOverride(ctx, a.ID, "bank:7"); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.ListJournalOverrides(ctx, a.ID); len(got) != 1 {
			t.Fatal("po zmazaní")
		}
	})

	t.Run("API kľúče", func(t *testing.T) {
		s := newStore(t)
		k1 := auth.APIKey{UserID: 7, Name: "Claude", Prefix: "fk_ab12", Hash: "h1", ReadOnly: true, CreatedAt: 1000}
		k2 := auth.APIKey{UserID: 7, Name: "Skript", Prefix: "fk_cd34", Hash: "h2", CreatedAt: 2000}
		k3 := auth.APIKey{UserID: 8, Name: "Iný", Prefix: "fk_ef56", Hash: "h3", CreatedAt: 3000}
		for _, k := range []*auth.APIKey{&k1, &k2, &k3} {
			if err := s.CreateAPIKey(ctx, k); err != nil || k.ID == 0 {
				t.Fatalf("vytvorenie: %v", err)
			}
		}
		dup := auth.APIKey{UserID: 7, Name: "x", Hash: "h1"}
		if err := s.CreateAPIKey(ctx, &dup); !errors.Is(err, store.ErrDuplicate) {
			t.Fatalf("rovnaký hash: %v", err)
		}
		list, _ := s.ListAPIKeys(ctx, 7)
		if len(list) != 2 || list[0].ID != k2.ID {
			t.Fatalf("zoznam: %+v", list)
		}
		got, err := s.GetAPIKeyByHash(ctx, "h1")
		if err != nil || !reflect.DeepEqual(got, k1) {
			t.Fatalf("podľa hashu: %+v %v", got, err)
		}
		if _, err := s.GetAPIKeyByHash(ctx, "nie"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("neznámy hash")
		}
		if g, err := s.GetAPIKey(ctx, k2.ID); err != nil || g.Hash != "h2" {
			t.Fatalf("podľa ID: %+v %v", g, err)
		}
		if _, err := s.GetAPIKey(ctx, 999); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("neznáme ID")
		}
		// výmena odtlačku iba zo správneho starého (obnovovací token OAuth)
		if err := s.RotateAPIKey(ctx, k2.ID, "zly", "h2b"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("výmena so zlým starým odtlačkom")
		}
		if err := s.RotateAPIKey(ctx, k2.ID, "h2", "h2b"); err != nil {
			t.Fatal(err)
		}
		if g, _ := s.GetAPIKeyByHash(ctx, "h2b"); g.ID != k2.ID {
			t.Fatal("po výmene")
		}
		if err := s.RotateAPIKey(ctx, k2.ID, "h2", "h2c"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("starý odtlačok druhýkrát")
		}
		if err := s.TouchAPIKey(ctx, k1.ID, 5000); err != nil {
			t.Fatal(err)
		}
		if err := s.RevokeAPIKey(ctx, 8, k1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("cudzí používateľ zrušil kľúč")
		}
		if err := s.RevokeAPIKey(ctx, 7, k1.ID); err != nil {
			t.Fatal(err)
		}
		got, _ = s.GetAPIKeyByHash(ctx, "h1")
		if !got.Revoked || got.LastUsedAt != 5000 {
			t.Fatalf("po zrušení: %+v", got)
		}
		if err := s.RotateAPIKey(ctx, k1.ID, "h1", "h1b"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zrušený kľúč sa nevymieňa")
		}
	})

	t.Run("bankové pohyby", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		tx := func(fp, date, amount string, dir bank.Direction) bank.Transaction {
			return bank.Transaction{Account: "SK3112000000198742637541", Date: invoice.MustDate(date), Amount: m.MustParse(amount),
				Currency: "EUR", Direction: dir, VS: "2026001", SS: "1", KS: "308", CounterName: "Odberateľ",
				CounterIBAN: "SK8975000000000012345671", Message: "Úhrada", BankRef: "R-" + fp, Fingerprint: fp}
		}
		batch := []bank.Transaction{tx("a", "2026-10-01", "100.50", bank.In), tx("b", "2026-10-02", "20", bank.Out), tx("c", "2025-12-31", "5", bank.Out)}
		saved, dup, err := s.SaveBankTransactions(ctx, a.ID, batch)
		if err != nil || len(saved) != 3 || dup != 0 || saved[0].ID == 0 || saved[0].Status != bank.StatusOpen || saved[0].CompanyID != a.ID {
			t.Fatalf("uloženie: %+v %d %v", saved, dup, err)
		}
		// opakovaný import: nič nové
		again, dup, err := s.SaveBankTransactions(ctx, a.ID, append(batch, tx("d", "2026-10-03", "7", bank.In)))
		if err != nil || len(again) != 1 || dup != 3 || again[0].Fingerprint != "d" {
			t.Fatalf("opakovaný import: %+v %d %v", again, dup, err)
		}
		// rovnaký odtlačok v inej firme je iný pohyb
		if other, _, _ := s.SaveBankTransactions(ctx, b.ID, batch[:1]); len(other) != 1 {
			t.Fatal("iná firma")
		}
		list, _ := s.ListBankTransactions(ctx, a.ID, store.BankFilter{})
		if len(list) != 4 || list[0].Fingerprint != "d" || list[3].Fingerprint != "c" {
			t.Fatalf("zoznam: %+v", list)
		}
		if y, _ := s.ListBankTransactions(ctx, a.ID, store.BankFilter{Year: 2025}); len(y) != 1 {
			t.Fatalf("rok: %d", len(y))
		}
		got, err := s.GetBankTransaction(ctx, a.ID, saved[0].ID)
		if err != nil || !reflect.DeepEqual(got, saved[0]) {
			t.Fatalf("načítanie:\n got %+v\nwant %+v", got, saved[0])
		}
		if _, err := s.GetBankTransaction(ctx, b.ID, saved[0].ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("pohyb inej firmy")
		}
		got.Status, got.Note = bank.StatusMatched, "spárované"
		got.Match = &bank.Match{Kind: "invoice", ID: 9, Number: "FA20260009", Auto: true, At: time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)}
		got.Amount = m.MustParse("999") // suma sa úpravou nemení
		if err := s.UpdateBankTransaction(ctx, a.ID, got); err != nil {
			t.Fatal(err)
		}
		upd, _ := s.GetBankTransaction(ctx, a.ID, got.ID)
		if upd.Status != bank.StatusMatched || upd.Match == nil || *upd.Match != *got.Match || !upd.Amount.Equal(m.MustParse("100.50")) || upd.Note != "spárované" {
			t.Fatalf("po úprave: %+v", upd)
		}
		if st, _ := s.ListBankTransactions(ctx, a.ID, store.BankFilter{Status: bank.StatusOpen}); len(st) != 3 {
			t.Fatalf("filter stavu: %d", len(st))
		}
		if err := s.UpdateBankTransaction(ctx, b.ID, got); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("úprava pohybu inej firmy")
		}
	})

	t.Run("sklad", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		mk := func(num string, k stock.Kind, date string) *stock.Doc {
			return &stock.Doc{Number: num, Kind: k, Date: invoice.MustDate(date), Warehouse: "HL", Partner: "Dodávateľ", LinkKind: "purchase", LinkID: 7,
				Lines: []stock.Line{{ItemID: 3, Name: "Kábel CYKY 3x1,5", Unit: "m", Quantity: m.MustParse("100"), UnitCost: m.MustParse("0.85"), Total: m.MustParse("85")}}}
		}
		d2 := mk("VY2026001", stock.Issue, "2026-03-02")
		d1 := mk("PR2026001", stock.Receipt, "2026-03-01")
		for _, d := range []*stock.Doc{d2, d1} {
			if err := s.SaveStockDoc(ctx, a.ID, d); err != nil || d.ID == 0 || d.CompanyID != a.ID {
				t.Fatalf("uloženie: %v %+v", err, d)
			}
		}
		got, err := s.GetStockDoc(ctx, a.ID, d1.ID)
		if err != nil || !reflect.DeepEqual(got, *d1) {
			t.Fatalf("načítanie:\n got %+v\nwant %+v", got, *d1)
		}
		list, _ := s.ListStockDocs(ctx, a.ID)
		if len(list) != 2 || list[0].Number != "PR2026001" {
			t.Fatalf("poradie podľa dátumu: %+v", list)
		}
		got.Lines[0].Quantity = m.MustParse("1")
		if again, _ := s.GetStockDoc(ctx, a.ID, d1.ID); !again.Lines[0].Quantity.Equal(m.MustParse("100")) {
			t.Fatal("úložisko vracia zdieľanú referenciu")
		}
		d1.Lines[0].Quantity, d1.Note = m.MustParse("120"), "doplnené"
		if err := s.SaveStockDoc(ctx, a.ID, d1); err != nil {
			t.Fatal(err)
		}
		if again, _ := s.GetStockDoc(ctx, a.ID, d1.ID); again.Note != "doplnené" || again.Lines[0].Quantity.String() != "120.00" {
			t.Fatalf("úprava: %+v", again)
		}
		if _, err := s.GetStockDoc(ctx, b.ID, d1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("doklad inej firmy")
		}
		if err := s.SaveStockDoc(ctx, b.ID, d1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("prepis dokladu inej firmy")
		}
		if other, _ := s.ListStockDocs(ctx, b.ID); len(other) != 0 {
			t.Fatal("doklady inej firmy")
		}
		if err := s.DeleteStockDoc(ctx, b.ID, d2.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("zmazanie dokladu inej firmy")
		}
		if err := s.DeleteStockDoc(ctx, a.ID, d2.ID); err != nil {
			t.Fatal(err)
		}
		if list, _ = s.ListStockDocs(ctx, a.ID); len(list) != 1 {
			t.Fatalf("po zmazaní: %+v", list)
		}
	})

	t.Run("súbor prijatého dokladu", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		p := &purchase.Purchase{Number: "PF1", Supplier: invoice.Party{Name: "Obchod"}, IssueDate: invoice.MustDate("2026-10-31"),
			ReceivedDate: invoice.MustDate("2026-10-31"), DueDate: invoice.MustDate("2026-10-31"), Rates: []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("10"), VAT: m.MustParse("2.30")}}}
		p.Normalize()
		if err := s.SavePurchase(ctx, a.ID, p); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetPurchaseFile(ctx, a.ID, p.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("bez súboru: %v", err)
		}
		f := store.PurchaseFile{CompanyID: a.ID, PurchaseID: p.ID, Name: "blocek.jpg", ContentType: "image/jpeg", Data: []byte{0xff, 0xd8, 1, 2}, Created: time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)}
		if err := s.SavePurchaseFile(ctx, f); err != nil {
			t.Fatal(err)
		}
		f.Data[2] = 9 // úložisko má vlastnú kópiu
		got, err := s.GetPurchaseFile(ctx, a.ID, p.ID)
		if err != nil || got.Name != "blocek.jpg" || got.ContentType != "image/jpeg" || len(got.Data) != 4 || got.Data[2] != 1 || !got.Created.Equal(f.Created) {
			t.Fatalf("súbor %+v %v", got, err)
		}
		f.Name, f.Data = "nova.png", []byte{1}
		if err := s.SavePurchaseFile(ctx, f); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.GetPurchaseFile(ctx, a.ID, p.ID); got.Name != "nova.png" || len(got.Data) != 1 {
			t.Errorf("prepísanie %+v", got)
		}
		// cudzia firma ho nevidí ani nezapíše
		if _, err := s.GetPurchaseFile(ctx, b.ID, p.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cudzia firma číta: %v", err)
		}
		if err := s.SavePurchaseFile(ctx, store.PurchaseFile{CompanyID: b.ID, PurchaseID: p.ID, Data: []byte{1}}); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cudzia firma zapisuje: %v", err)
		}
		if err := s.DeletePurchase(ctx, a.ID, p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetPurchaseFile(ctx, a.ID, p.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("súbor po zmazaní dokladu: %v", err)
		}
	})
	t.Run("prijaté faktúry", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		mk := func(num, supplier, received string) *purchase.Purchase {
			p := &purchase.Purchase{
				Number: num, SupplierNumber: "D-" + num, Supplier: invoice.Party{Name: supplier, ICO: "31322832", ICDPH: "SK2020372640"},
				IssueDate: invoice.MustDate(received), ReceivedDate: invoice.MustDate(received), DueDate: invoice.MustDate(received).AddDays(14),
				SupplierIBAN: "SK31 1200 0000 1987 4263 7541", ContactID: 3, NoDeduction: true, ReverseCharge: true, SupplyKind: invoice.SupplyGoods,
				Description: "Pohonné hmoty", Expense: purchase.ExpenseStock, Rates: []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("100")}, {Rate: vat.Rate5, Base: m.MustParse("10"), VAT: m.MustParse("0.50")}},
			}
			p.Normalize()
			return p
		}
		p1 := mk("PF20260001", "Slovnaft, a.s.", "2026-10-07")
		if err := s.SavePurchase(ctx, a.ID, p1); err != nil || p1.ID == 0 {
			t.Fatalf("SavePurchase: %v", err)
		}
		_ = s.SavePurchase(ctx, a.ID, mk("PF20250099", "Orange Slovensko", "2025-12-30"))
		got, err := s.GetPurchase(ctx, a.ID, p1.ID)
		if err != nil || !reflect.DeepEqual(got, *p1) {
			t.Fatalf("načítaná sa líši:\n got %+v\nwant %+v", got, *p1)
		}
		list, _ := s.ListPurchases(ctx, a.ID, store.PurchaseFilter{})
		if len(list) != 2 || list[0].Number != "PF20260001" {
			t.Fatalf("zoznam: %d", len(list))
		}
		for f, want := range map[store.PurchaseFilter]int{{Year: 2026}: 1, {Search: "orange"}: 1, {Search: "D-PF2026"}: 1, {Year: 2024}: 0} {
			if l, _ := s.ListPurchases(ctx, a.ID, f); len(l) != want {
				t.Errorf("filter %+v: %d, chcem %d", f, len(l), want)
			}
		}
		// cudzia mena: mena, kurz a sumy v eurách sa uložia
		czk := mk("PF20260002", "Česká firma s.r.o.", "2026-10-08")
		czk.Currency, czk.FXRate, czk.FXDate, czk.FXManual = "CZK", m.MustParse("24.5"), invoice.MustDate("2026-10-07"), true
		czk.Normalize()
		if err := s.SavePurchase(ctx, a.ID, czk); err != nil {
			t.Fatal(err)
		}
		if g, err := s.GetPurchase(ctx, a.ID, czk.ID); err != nil || !reflect.DeepEqual(g, *czk) || g.Local == nil {
			t.Fatalf("cudzia mena sa líši:\n got %+v\nwant %+v", g, *czk)
		}
		_ = s.DeletePurchase(ctx, a.ID, czk.ID)
		// čiastočný odpočet (vozidlo), pomerné odpočítanie § 50, číslo opravovanej faktúry
		car := mk("PF20260003", "Autoservis s.r.o.", "2026-10-09")
		car.NoDeduction, car.ReverseCharge, car.SupplyKind = false, false, ""
		car.DeductPercent, car.VehicleID, car.Mixed, car.Coef, car.CorrectsNumber = 50, 4, true, 80, "FV-7"
		car.Normalize()
		if err := s.SavePurchase(ctx, a.ID, car); err != nil {
			t.Fatal(err)
		}
		if g, err := s.GetPurchase(ctx, a.ID, car.ID); err != nil || !reflect.DeepEqual(g, *car) || g.Deducted.IsZero() {
			t.Fatalf("údaje odpočtu sa líšia:\n got %+v\nwant %+v", g, *car)
		}
		_ = s.DeletePurchase(ctx, a.ID, car.ID)
		// oddelenie firiem
		if l, _ := s.ListPurchases(ctx, b.ID, store.PurchaseFilter{}); len(l) != 0 {
			t.Fatal("firma B vidí nákup firmy A")
		}
		if _, err := s.GetPurchase(ctx, b.ID, p1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B načítala nákup firmy A")
		}
		stolen := *p1
		if err := s.SavePurchase(ctx, b.ID, &stolen); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B prepísala nákup firmy A")
		}
		if err := s.DeletePurchase(ctx, b.ID, p1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("firma B zmazala nákup firmy A")
		}
		// úprava a zmazanie
		p1.Status, p1.PaidDate = purchase.StatusPaid, invoice.MustDate("2026-10-10")
		_ = s.SavePurchase(ctx, a.ID, p1)
		if g, _ := s.GetPurchase(ctx, a.ID, p1.ID); g.Status != purchase.StatusPaid || g.PaidDate.String() != "2026-10-10" {
			t.Fatal("úhrada sa neuložila")
		}
		if err := s.DeletePurchase(ctx, a.ID, p1.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetPurchase(ctx, a.ID, p1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("nezmazaná")
		}
	})

	t.Run("používatelia a roly", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		jana, err := s.CreateUser(ctx, auth.User{Email: "Jana@Kancelaria.sk", Name: "Jana", PasswordHash: "h", Active: true})
		if err != nil || jana.ID == 0 {
			t.Fatal(err)
		}
		if _, err := s.CreateUser(ctx, auth.User{Email: "jana@kancelaria.sk"}); !errors.Is(err, store.ErrDuplicate) {
			t.Fatalf("duplicitný e-mail (bez ohľadu na veľkosť písmen): %v", err)
		}
		got, err := s.GetUserByEmail(ctx, " JANA@kancelaria.SK ")
		if err != nil || got.ID != jana.ID || got.PasswordHash != "h" || !got.Active {
			t.Fatalf("podľa e-mailu: %+v %v", got, err)
		}
		peter, _ := s.CreateUser(ctx, auth.User{Email: "peter@x.sk", Name: "Peter", Active: true})
		peter.Email = "JANA@kancelaria.sk"
		if err := s.UpdateUser(ctx, peter); !errors.Is(err, store.ErrDuplicate) {
			t.Fatal("zmena na obsadený e-mail")
		}
		peter.Email, peter.Active, peter.SuperAdmin = "peter@x.sk", false, true
		if err := s.UpdateUser(ctx, peter); err != nil {
			t.Fatal(err)
		}
		if g, _ := s.GetUser(ctx, peter.ID); g.Active || !g.SuperAdmin {
			t.Fatal("úprava používateľa")
		}
		// dvojfázové overenie sa uloží celé a dá sa aj vypnúť
		peter.MFA = auth.MFA{Enabled: true, Secret: "JBSWY3DPEHPK3PXP", LastStep: 42, Recovery: []string{"a", "b"}}
		_ = s.UpdateUser(ctx, peter)
		if g, _ := s.GetUserByEmail(ctx, "peter@x.sk"); !g.MFA.Enabled || g.MFA.Secret != "JBSWY3DPEHPK3PXP" || g.MFA.LastStep != 42 || len(g.MFA.Recovery) != 2 {
			t.Fatalf("2FA: %+v", g.MFA)
		}
		peter.MFA = auth.MFA{}
		_ = s.UpdateUser(ctx, peter)
		if g, _ := s.GetUser(ctx, peter.ID); g.MFA.Enabled || g.MFA.Secret != "" || len(g.MFA.Recovery) != 0 {
			t.Fatalf("vypnuté 2FA: %+v", g.MFA)
		}
		if l, _ := s.ListUsers(ctx); len(l) != 2 {
			t.Fatal("zoznam používateľov")
		}

		// roly po firmách
		_ = s.SetMembership(ctx, auth.Membership{UserID: jana.ID, CompanyID: a.ID, Role: auth.RoleAccountant})
		_ = s.SetMembership(ctx, auth.Membership{UserID: jana.ID, CompanyID: b.ID, Role: auth.RoleClient})
		_ = s.SetMembership(ctx, auth.Membership{UserID: jana.ID, CompanyID: a.ID, Role: auth.RoleAdmin}) // zmena roly
		_ = s.SetMembership(ctx, auth.Membership{UserID: peter.ID, CompanyID: a.ID, Role: auth.RoleViewer})
		ms, _ := s.UserMemberships(ctx, jana.ID)
		if len(ms) != 2 || ms[0].Role != auth.RoleAdmin || ms[1].Role != auth.RoleClient {
			t.Fatalf("roly Jany: %+v", ms)
		}
		if m, _ := s.CompanyMembers(ctx, a.ID); len(m) != 2 {
			t.Fatalf("členovia firmy: %+v", m)
		}
		if err := s.SetMembership(ctx, auth.Membership{UserID: 9999, CompanyID: a.ID, Role: auth.RoleAdmin}); err == nil {
			t.Fatal("rola neexistujúcemu používateľovi")
		}
		if err := s.RemoveMembership(ctx, jana.ID, b.ID); err != nil {
			t.Fatal(err)
		}
		if m, _ := s.UserMemberships(ctx, jana.ID); len(m) != 1 {
			t.Fatal("odobratie z firmy")
		}

		// relácie
		_ = s.CreateSession(ctx, auth.Session{TokenHash: "t1", UserID: jana.ID, ExpiresAt: 100})
		_ = s.CreateSession(ctx, auth.Session{TokenHash: "t2", UserID: jana.ID, ExpiresAt: 200})
		_ = s.CreateSession(ctx, auth.Session{TokenHash: "t3", UserID: peter.ID, ExpiresAt: 300})
		if ss, err := s.GetSession(ctx, "t2"); err != nil || ss.UserID != jana.ID || ss.ExpiresAt != 200 {
			t.Fatal("relácia")
		}
		_ = s.DeleteSession(ctx, "t2")
		if _, err := s.GetSession(ctx, "t2"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("odhlásenie")
		}
		_ = s.DeleteUserSessions(ctx, jana.ID)
		if _, err := s.GetSession(ctx, "t1"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("odhlásenie zo všetkých zariadení")
		}
		if _, err := s.GetSession(ctx, "t3"); err != nil {
			t.Fatal("cudzia relácia nesmie zmiznúť")
		}
	})

	t.Run("audit", func(t *testing.T) {
		s := newStore(t)
		actor := auth.Actor{UserID: 5, Name: "Jana Účtovníčka", Channel: auth.ChannelWeb, IP: "10.0.0.1"}
		mcp := auth.Actor{UserID: 6, Name: "Asistent", Channel: auth.ChannelMCP}
		add := func(a auth.Actor, cid int64, cat audit.Category, summary string) audit.Entry {
			e := audit.New(a, cid, cat, "update", "invoice", 7, summary, map[string]string{"x": "pred"}, map[string]string{"x": "po"})
			if err := s.AppendAudit(ctx, &e); err != nil || e.ID == 0 || e.Hash == "" {
				t.Fatalf("AppendAudit: %v", err)
			}
			return e
		}
		add(actor, 1, audit.CatInvoices, "Vystavená faktúra FA20260001")
		add(mcp, 1, audit.CatPurchases, "Zaevidovaná prijatá faktúra PF20260001")
		add(actor, 2, audit.CatInvoices, "Vystavená faktúra FA20260001 (Beta)")
		last := add(actor, 0, audit.CatAuth, "Prihlásenie")

		all, total, _ := s.ListAudit(ctx, audit.Filter{})
		if total != 4 || all[0].ID != last.ID {
			t.Fatalf("najnovšie prvé, spolu %d", total)
		}
		if string(all[0].After) != `{"x":"po"}` || all[0].IP != "10.0.0.1" || all[0].Channel != auth.ChannelWeb {
			t.Fatalf("obsah záznamu: %+v", all[0])
		}
		cases := []struct {
			f    audit.Filter
			want int
		}{
			{audit.Filter{CompanyIDs: []int64{1}}, 2},
			{audit.Filter{CompanyIDs: []int64{1, 2}}, 3},
			{audit.Filter{CompanyIDs: []int64{}}, 0},
			{audit.Filter{Category: audit.CatInvoices}, 2},
			{audit.Filter{UserID: 6}, 1},
			{audit.Filter{Search: "prijata"}, 1},
			{audit.Filter{Search: "účtovníčka"}, 3},
		}
		for _, c := range cases {
			if _, n, _ := s.ListAudit(ctx, c.f); n != c.want {
				t.Errorf("filter %+v: %d, chcem %d", c.f, n, c.want)
			}
		}
		page, n, _ := s.ListAudit(ctx, audit.Filter{Limit: 2, Offset: 2})
		if n != 4 || len(page) != 2 || page[0].ID != all[2].ID {
			t.Fatal("stránkovanie")
		}
		chain, _ := s.AuditChain(ctx)
		if len(chain) != 4 || chain[0].PrevHash != "" {
			t.Fatal("reťazec")
		}
		if err := audit.Verify(chain); err != nil {
			t.Fatalf("reťazec po načítaní z úložiska: %v", err)
		}
	})
}

func names(list []store.Contact) []string {
	out := make([]string, len(list))
	for i, c := range list {
		out[i] = c.Name
	}
	return out
}
