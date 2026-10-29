// Package storetest je spoločná sada testov, ktorú musí prejsť každá
// implementácia store.Store (pamäť, SQLite...). Hlavne overuje, že sa
// dáta rôznych firiem nikdy nepremiešajú.
package storetest

import (
	"context"
	"errors"
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
}
