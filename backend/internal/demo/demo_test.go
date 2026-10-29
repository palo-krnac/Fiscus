package demo

import (
	"context"
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
	if len(list) != 40 {
		t.Fatalf("%d faktúr", len(list))
	}
	if p, _ := a.Store.ListPurchases(context.Background(), c.ID, store.PurchaseFilter{}); len(p) != 6 {
		t.Fatalf("%d prijatých faktúr", len(p))
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
