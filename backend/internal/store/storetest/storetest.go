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
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
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
		c.TaxOffice, c.VATPeriod = "Daňový úrad Banská Bystrica", store.VATPeriodQuarter
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
		inv.SupplyKind = invoice.SupplyServices
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
		if got.ContactID != 42 || !got.NonVATPayer || got.PaymentMethod != invoice.PayCash || got.SupplyKind != invoice.SupplyServices {
			t.Fatalf("načítané: contact=%d nonVat=%v", got.ContactID, got.NonVATPayer)
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

	t.Run("prijaté faktúry", func(t *testing.T) {
		s := newStore(t)
		a, _ := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
		b, _ := s.CreateCompany(ctx, store.Company{Name: "Beta"})
		mk := func(num, supplier, received string) *purchase.Purchase {
			p := &purchase.Purchase{
				Number: num, SupplierNumber: "D-" + num, Supplier: invoice.Party{Name: supplier, ICO: "31322832", ICDPH: "SK2020372640"},
				IssueDate: invoice.MustDate(received), ReceivedDate: invoice.MustDate(received), DueDate: invoice.MustDate(received).AddDays(14),
				SupplierIBAN: "SK31 1200 0000 1987 4263 7541", ContactID: 3, NoDeduction: true, ReverseCharge: true, SupplyKind: invoice.SupplyGoods,
				Description: "Pohonné hmoty", Rates: []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("100")}, {Rate: vat.Rate5, Base: m.MustParse("10"), VAT: m.MustParse("0.50")}},
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
