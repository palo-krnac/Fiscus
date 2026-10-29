package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/postman"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/ubl"
)

func TestEInvoiceExchange(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.DIC, c.Street, c.City, c.Zip, c.IBAN = "2020372640", "Hlavná 1", "Zvolen", "960 01", "SK3112000000198742637541"
	if err := a.Store.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	d := validDraft()
	d.Customer = invoice.Party{Name: "Odberateľ s.r.o.", City: "Bratislava", Country: "SK", DIC: "2120000003"}
	inv, err := a.SaveDraft(ctx, c.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	if inv, err = a.Issue(ctx, c.ID, inv.ID); err != nil {
		t.Fatal(err)
	}

	// bez nastavenia: e-faktúra sa iba stiahne
	if _, err := a.SendEInvoice(ctx, c.ID, inv.ID); !errors.Is(err, postman.ErrOff) {
		t.Fatalf("vypnuté: %v", err)
	}
	dir := t.TempDir()
	set := postman.Settings{Mode: postman.ModeFolder, OutDir: filepath.Join(dir, "von"), InDir: filepath.Join(dir, "dnu"), Provider: "Môj poštár"}
	if _, err := a.SetEInvoiceSettings(ctx, c.ID, postman.Settings{Mode: postman.ModeFolder}); err == nil {
		t.Fatal("neúplné nastavenie")
	}
	if _, err := a.SetEInvoiceSettings(ctx, c.ID, set); err != nil {
		t.Fatal(err)
	}

	// odoslanie
	e, err := a.SendEInvoice(ctx, c.ID, inv.ID)
	if err != nil || e.PeppolID != "0245:2120000003" || e.MessageID == "" || e.Error != "" {
		t.Fatalf("odoslanie: %+v %v", e, err)
	}
	if _, err := os.Stat(filepath.Join(set.OutDir, e.MessageID)); err != nil {
		t.Fatal("súbor v priečinku odoslaných chýba")
	}
	if list, _ := a.EInvoices(ctx, c.ID, store.EInvoiceFilter{Direction: "out", DocID: inv.ID}); len(list) != 1 {
		t.Fatalf("zoznam odoslaných %+v", list)
	}

	// prijatie: e-faktúra od dodávateľa v schránke
	xml, err := ubl.Write(inv, ubl.Seller{Name: "Dodávateľ s.r.o.", Street: "Dlhá 2", City: "Žilina", ICO: "36631124", DIC: "2021774654",
		ICDPH: "SK2021774654", IBAN: "SK3112000000198742637541", VATPayer: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(set.InDir, 0o755)
	_ = os.WriteFile(filepath.Join(set.InDir, "od-dodavatela.xml"), xml, 0o644)
	_ = os.WriteFile(filepath.Join(set.InDir, "pokazena.xml"), []byte("<html/>"), 0o644)
	box, err := a.EInvoiceInbox(ctx, c.ID)
	if err != nil || len(box) != 2 {
		t.Fatalf("schránka %+v %v", box, err)
	}
	var good InboxItem
	for _, it := range box {
		if it.ID == "od-dodavatela.xml" {
			good = it
		} else if it.Error == "" {
			t.Error("nečitateľný súbor bez chyby")
		}
	}
	if good.Supplier != "Dodávateľ s.r.o." || good.Number != inv.Number || good.Total != "492.00" {
		t.Fatalf("prehľad %+v", good)
	}
	draft, err := a.TakeEInvoice(ctx, c.ID, "od-dodavatela.xml")
	if err != nil || draft.Purchase.Supplier.ICO != "36631124" || draft.Purchase.Summary.Total.String() != "492.00" {
		t.Fatalf("prevzatie: %+v %v", draft.Purchase, err)
	}
	if box, _ := a.EInvoiceInbox(ctx, c.ID); len(box) != 1 {
		t.Fatal("prevzatá zostala v schránke")
	}
	in, _ := a.EInvoices(ctx, c.ID, store.EInvoiceFilter{Direction: "in"})
	if len(in) != 1 || in[0].Partner != "Dodávateľ s.r.o." || in[0].PeppolID != "0245:2021774654" {
		t.Fatalf("zoznam prijatých %+v", in)
	}
	again, err := a.ReopenEInvoice(ctx, c.ID, in[0].ID)
	if err != nil || again.Purchase.SupplierNumber != inv.Number {
		t.Fatalf("znova otvoriť: %v", err)
	}
	if _, err := a.TakeEInvoice(ctx, c.ID, "neexistuje.xml"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	// token sa nevracia
	if s, _ := a.EInvoiceSettings(ctx, c.ID); s.Token != "" || s.Provider != "Môj poštár" {
		t.Fatalf("%+v", s)
	}
}

// Na serveri sú priečinky disk servera — nastaví ich iba správca inštalácie.
func TestEInvoiceFolderOnServer(t *testing.T) {
	o := newOffice(t)
	dir := t.TempDir()
	set := postman.Settings{Mode: postman.ModeFolder, OutDir: filepath.Join(dir, "a"), InDir: filepath.Join(dir, "b")}
	ctxAdmin := as(o.admin)
	u, err := o.a.AddMember(ctxAdmin, o.alfa.ID, NewMember{Email: "spravca@alfa.sk", Name: "Správca Alfa", Password: "spravca-heslo-1", Role: auth.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	full, _ := o.a.Store.GetUser(ctx, u.User.ID)
	firmAdmin := actorOf(full, auth.Actor{Channel: auth.ChannelWeb})
	if _, err := o.a.SetEInvoiceSettings(as(firmAdmin), o.alfa.ID, set); !errors.Is(err, ErrForbidden) {
		t.Fatalf("správca firmy nastavil priečinky servera: %v", err)
	}
	api := postman.Settings{Mode: postman.ModeHTTP, URL: "https://api.postar.sk", Token: "t"}
	if _, err := o.a.SetEInvoiceSettings(as(firmAdmin), o.alfa.ID, api); err != nil {
		t.Fatalf("API poštára môže nastaviť správca firmy: %v", err)
	}
	if _, err := o.a.SetEInvoiceSettings(ctxAdmin, o.alfa.ID, set); err != nil {
		t.Fatal(err)
	}
}
