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
	for _, cu := range customers {
		if !party.ValidICO(cu.ico) {
			t.Errorf("neplatné demo IČO %s", cu.ico)
		}
	}
}
