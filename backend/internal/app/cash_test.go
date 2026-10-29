package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
)

func TestCash(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d := func(kind cash.Kind, date, amount, purpose string) cash.Document {
		return cash.Document{Kind: kind, Date: invoice.MustDate(date), Amount: m.MustParse(amount), Purpose: purpose}
	}
	// výdavok do prázdnej pokladne nejde
	if _, err := a.CreateCash(ctx, c.ID, d(cash.Out, "2026-10-01", "10", "Známky")); !fieldErr(err, "amount") {
		t.Fatalf("záporný zostatok: %v", err)
	}
	vklad, err := a.CreateCash(ctx, c.ID, d(cash.In, "2026-10-01", "500", "Vklad do pokladne"))
	if err != nil || vklad.Number != "PPD20260001" {
		t.Fatalf("vklad: %+v %v", vklad, err)
	}

	// úhrada vydanej faktúry v hotovosti (492,00 €)
	dr, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, _ := a.Issue(ctx, c.ID, dr.ID)
	pay := d(cash.In, "2026-10-02", "600", "Úhrada")
	pay.LinkKind, pay.LinkID = "invoice", inv.ID
	if _, err := a.CreateCash(ctx, c.ID, pay); !fieldErr(err, "linkId") {
		t.Fatalf("suma nad zostatok: %v", err)
	}
	// čiastočná úhrada v hotovosti, potom zvyšok
	pay.Amount = m.MustParse("100")
	part, err := a.CreateCash(ctx, c.ID, pay)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Store.GetInvoice(ctx, c.ID, inv.ID); got.Status != invoice.StatusIssued || got.PaidAmount.String() != "100.00" {
		t.Fatalf("čiastočne: %s %s", got.Status, got.PaidAmount)
	}
	pay.Amount = m.MustParse("392")
	ppd, err := a.CreateCash(ctx, c.ID, pay)
	if err != nil || ppd.Number != "PPD20260003" || ppd.LinkNumber != inv.Number || part.Number != "PPD20260002" {
		t.Fatalf("úhrada faktúry: %+v %v", ppd, err)
	}
	if got, _ := a.Store.GetInvoice(ctx, c.ID, inv.ID); got.Status != invoice.StatusPaid || got.PaidDate.String() != "2026-10-02" {
		t.Fatalf("faktúra: %s %s", got.Status, got.PaidDate)
	}
	// druhá úhrada tej istej faktúry
	if _, err := a.CreateCash(ctx, c.ID, pay); !fieldErr(err, "linkId") {
		t.Fatalf("dvojitá úhrada: %v", err)
	}

	// úhrada prijatej faktúry (100,00 €) výdavkom
	p, _, _ := a.SavePurchase(ctx, c.ID, validPurchase())
	vpd := d(cash.Out, "2026-10-08", "100", "Pohonné hmoty")
	vpd.LinkKind, vpd.LinkID = "purchase", p.ID
	in := vpd
	in.Kind = cash.In
	if _, err := a.CreateCash(ctx, c.ID, in); !fieldErr(err, "linkId") {
		t.Fatalf("príjem na prijatú faktúru: %v", err)
	}
	out, err := a.CreateCash(ctx, c.ID, vpd)
	if err != nil || out.Number != "VPD20260001" {
		t.Fatalf("výdavok: %+v %v", out, err)
	}
	if gp, _ := a.Store.GetPurchase(ctx, c.ID, p.ID); gp.Status != purchase.StatusPaid {
		t.Fatal("prijatá faktúra nie je uhradená")
	}

	book, err := a.CashBook(ctx, c.ID, 2026)
	if err != nil || len(book.Documents) != 4 || !book.Balance.Equal(m.MustParse("892")) || !book.In.Equal(m.MustParse("992")) || !book.Out.Equal(m.MustParse("100")) {
		t.Fatalf("kniha: %+v %v", book, err)
	}
	if b27, _ := a.CashBook(ctx, c.ID, 2027); !b27.Opening.Equal(m.MustParse("892")) || len(b27.Documents) != 0 {
		t.Fatalf("rok 2027: %+v", b27)
	}

	// storno úhrady faktúry vráti faktúru medzi neuhradené
	if _, err := a.CancelCash(ctx, c.ID, ppd.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Store.GetInvoice(ctx, c.ID, inv.ID); got.Status != invoice.StatusIssued || got.PaidAmount.String() != "100.00" {
		t.Fatalf("po storne: %s %s", got.Status, got.PaidAmount)
	}
	if _, err := a.CancelCash(ctx, c.ID, ppd.ID); !errors.Is(err, cash.ErrCancelled) {
		t.Fatalf("dvojité storno: %v", err)
	}
	if _, err := a.CancelCash(ctx, c.ID, part.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Store.GetInvoice(ctx, c.ID, inv.ID); !got.PaidAmount.IsZero() {
		t.Fatalf("po storne oboch: %s", got.PaidAmount)
	}
	// teraz by storno vkladu poslalo pokladňu do mínusu (výdavok 100)
	if _, err := a.CancelCash(ctx, c.ID, vklad.ID); !errors.Is(err, cash.ErrNegative) {
		t.Fatalf("storno vkladu: %v", err)
	}
	// úprava textov
	upd, err := a.UpdateCash(ctx, c.ID, vklad.ID, "Majiteľ", "Vklad hotovosti", "z účtu")
	if err != nil || upd.Partner != "Majiteľ" || !upd.Amount.Equal(m.MustParse("500")) {
		t.Fatalf("úprava: %+v %v", upd, err)
	}
	list, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatCash})
	if len(list) != 7 { // 4 doklady, 2 storná, úprava
		t.Fatalf("audit pokladne: %d", len(list))
	}
}

func TestCashPermissions(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	member := func(email string, role auth.Role) auth.Actor {
		mb, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: email, Name: email, Password: "heslo-1234567", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
		return actorOf(u, auth.Actor{Channel: auth.ChannelWeb})
	}
	doc := cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-10-01"), Amount: m.MustParse("10"), Purpose: "Vklad"}
	tests := []struct {
		name  string
		actor auth.Actor
		write error
	}{
		{"fakturant", member("f@alfa.sk", auth.RoleInvoicer), nil},
		{"len čítanie", member("v@alfa.sk", auth.RoleViewer), ErrForbidden},
	}
	for _, tt := range tests {
		if _, err := o.a.CreateCash(as(tt.actor), o.alfa.ID, doc); !errors.Is(err, tt.write) {
			t.Errorf("%s: %v", tt.name, err)
		}
		if _, err := o.a.CashBook(as(tt.actor), o.alfa.ID, 2026); err != nil {
			t.Errorf("%s čítanie: %v", tt.name, err)
		}
	}
}

func fieldErr(err error, field string) bool {
	var ve *invoice.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return true
		}
	}
	return false
}
