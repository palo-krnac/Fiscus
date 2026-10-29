package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestCatalog(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	it, err := a.SaveItem(ctx, c.ID, catalog.Item{Code: " PLC ", Name: " Programovanie PLC ", Unit: "hod", Price: m.MustParse("48"), Rate: vat.Rate23})
	if err != nil || it.Code != "PLC" || it.Name != "Programovanie PLC" || it.PriceMode != vat.PriceNet || it.Gross.String() != "59.04" {
		t.Fatalf("uloženie: %+v %v", it, err)
	}
	if _, err := a.SaveItem(ctx, c.ID, catalog.Item{Name: "", Rate: 23}); !fieldErr(err, "name") {
		t.Fatalf("bez názvu: %v", err)
	}
	if _, err := a.SaveItem(ctx, c.ID, catalog.Item{Code: "plc", Name: "Iné", Rate: 23}); !fieldErr(err, "code") {
		t.Fatalf("rovnaký kód: %v", err)
	}
	it.Price = m.MustParse("50")
	if it, err = a.SaveItem(ctx, c.ID, it.Item); err != nil || it.Net.String() != "50.00" {
		t.Fatalf("úprava: %v", err)
	}
	list, err := a.Items(ctx, c.ID)
	if err != nil || len(list) != 1 || list[0].Gross.String() != "61.50" {
		t.Fatalf("zoznam: %+v %v", list, err)
	}
	if err := a.DeleteItem(ctx, c.ID, it.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteItem(ctx, c.ID, it.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("druhé zmazanie: %v", err)
	}
	entries, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatCatalog})
	if len(entries) != 3 {
		t.Fatalf("audit cenníka: %d", len(entries))
	}
}

func TestCatalogPermissions(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	mb, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: "v@alfa.sk", Name: "V", Password: "heslo-1234567", Role: auth.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
	viewer := as(actorOf(u, auth.Actor{Channel: auth.ChannelWeb}))
	if _, err := o.a.SaveItem(viewer, o.alfa.ID, catalog.Item{Name: "X", Rate: 23}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("zápis: %v", err)
	}
	if _, err := o.a.Items(viewer, o.alfa.ID); err != nil {
		t.Fatalf("čítanie: %v", err)
	}
	if _, err := o.a.Items(viewer, o.beta.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cudzia firma: %v", err)
	}
}
