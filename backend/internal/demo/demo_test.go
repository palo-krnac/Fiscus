package demo

import (
	"context"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

func TestSeed(t *testing.T) {
	caps, _ := edition.For(edition.Desktop)
	a := app.New(memstore.New(), caps)
	c, err := Seed(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := a.Store.ListInvoices(context.Background(), c.ID, store.InvoiceFilter{})
	// + koncept z ponuky, dnešná pravidelná faktúra, 2 zálohové faktúry,
	// doklad k prijatej platbe, koncept vyúčtovania a faktúra s prenesením pri položkách
	rc := 0
	for _, inv := range list {
		if inv.DomesticReverse() && inv.Status == invoice.StatusIssued {
			rc++
		}
	}
	if len(list) != 50 || rc != 1 {
		t.Fatalf("%d faktúr, s prenesením %d", len(list), rc)
	}
	rec, _ := a.Recurring(context.Background(), c.ID)
	if len(rec) != 3 || rec[0].Generated != 1 || rec[0].History[0].Number == "" || rec[2].Active {
		t.Fatalf("pravidelné: %+v", rec)
	}
	if rs, _ := a.Reminders(context.Background(), c.ID, 0); len(rs) != 1 || rs[0].Level != 1 || len(rs[0].Items) == 0 {
		t.Fatalf("upomienky: %+v", rs)
	}
	quotes, _ := a.Quotes(context.Background(), c.ID, 0)
	states := map[string]int{}
	for _, q := range quotes {
		states[string(q.Status)]++
		if q.Expired(a.Today()) {
			states["expired"]++
		}
	}
	if len(quotes) != 6 || states["invoiced"] != 1 || states["rejected"] != 1 || states["draft"] != 1 || states["expired"] != 1 {
		t.Fatalf("ponuky: %d %v", len(quotes), states)
	}
	purs, _ := a.Store.ListPurchases(context.Background(), c.ID, store.PurchaseFilter{})
	usd := 0
	for _, p := range purs {
		if p.Currency == "USD" && p.Local != nil && p.FXRate.Sign() > 0 {
			usd++
		}
	}
	if len(purs) != 7 || usd != 1 {
		t.Fatalf("%d prijatých faktúr, v USD s kurzom %d", len(purs), usd)
	}
	txs, _ := a.Store.ListBankTransactions(context.Background(), c.ID, store.BankFilter{})
	matched := 0
	for _, tx := range txs {
		if tx.Status == "matched" {
			matched++
		}
	}
	if len(txs) != 6 || matched != 3 {
		t.Fatalf("výpis: %d pohybov, %d spárovaných", len(txs), matched)
	}
	for _, cu := range customers {
		if !party.ValidICO(cu.ico) {
			t.Errorf("neplatné demo IČO %s", cu.ico)
		}
	}
}

func TestSeedUsers(t *testing.T) {
	caps, _ := edition.For(edition.Server)
	a := app.New(memstore.New(), caps)
	ctx := context.Background()
	c, err := Seed(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if err := SeedUsers(ctx, a, c.ID); err != nil {
		t.Fatal(err)
	}
	members, err := a.CompanyUsers(ctx, c.ID)
	if err != nil || len(members) != len(demoUsers) {
		t.Fatalf("členovia %d, %v", len(members), err)
	}
	if _, _, err := a.Login(ctx, "fakturant@demo.sk", DemoPassword); err != nil {
		t.Fatal(err)
	}
}
