// Package storetest je spoločná sada testov, ktorú musí prejsť každá
// implementácia store.Store (pamäť, SQLite...). Hlavne overuje, že sa
// dáta rôznych firiem nikdy nepremiešajú.
package storetest

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
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
	t.Run("úprava firmy", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa", DefaultDueDays: 14})
		c.Name, c.IBAN, c.VATPayer, c.DefaultDueDays, c.Email = "Alfa s.r.o.", "SK31 1200", true, 30, "a@b.sk"
		c.Units = []string{"ks", "hod", "m²"}
		c.NumberFormats = map[string]string{"invoice": "{RRRR}/{NNN}", "credit_note": "DB{RRRR}{NNNN}"}
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

	t.Run("faktúra si pamätá kontakt a režim neplatiteľa", func(t *testing.T) {
		s := newStore(t)
		c, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		inv := draft("Odberateľ", "2026-10-01")
		inv.ContactID = 42
		inv.NonVATPayer = true
		inv.PaymentMethod = invoice.PayCash
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
		if got.ContactID != 42 || !got.NonVATPayer || got.PaymentMethod != invoice.PayCash {
			t.Fatalf("načítané: contact=%d nonVat=%v", got.ContactID, got.NonVATPayer)
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
