package app

import (
	"bytes"
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestVATReturn(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.DIC = "2020372640"
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	// vydaná faktúra 400 € + 23 % pre platiteľa
	d := validDraft()
	d.Customer.ICDPH = "SK2020372640"
	d.IssueDate = invoice.MustDate("2026-10-10")
	d.DeliveryDate = invoice.MustDate("2026-10-10")
	draft, err := a.SaveDraft(ctx, c.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Issue(ctx, c.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	// prijatá faktúra 81,30 + 18,70
	if _, _, err := a.SavePurchase(ctx, c.ID, validPurchase()); err != nil {
		t.Fatal(err)
	}

	r, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil {
		t.Fatal(err)
	}
	if r.Rows["r04"].String() != "92.00" || r.Rows["r19"].String() != "18.70" || r.Rows["r32"].String() != "73.30" {
		t.Fatalf("riadky: %v", r.Rows)
	}
	if len(r.KV.A1) != 1 || len(r.KV.B2) != 1 {
		t.Fatalf("KV: %+v", r.KV)
	}

	tests := []struct {
		period string
		want   error
	}{
		{"2025-06", ErrPeriodTooOld},
		{"2026-12", ErrPeriodInFuture},
		{"2026-Q4", nil},
	}
	for _, tt := range tests {
		if _, _, err := a.VATReturn(ctx, c.ID, tt.period, m.Zero); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, chcem %v", tt.period, err, tt.want)
		}
	}
	var ve *invoice.ValidationError
	if _, _, err := a.VATReturn(ctx, c.ID, "zlé", m.Zero); !errors.As(err, &ve) {
		t.Errorf("neplatné obdobie: %v", err)
	}

	// export do XML sa zapíše do auditu
	out, name, err := a.VATExport(ctx, c.ID, "kv", "2026-10", m.Zero)
	if err != nil || name != "kv-dph-2026-10.xml" || !bytes.Contains(out, []byte(`<A1 Odb="SK2020372640"`)) {
		t.Fatalf("export KV: %s %v\n%s", name, err, out)
	}
	if _, name, _ := a.VATExport(ctx, c.ID, "dph", "2026-10", m.Zero); name != "dph-2026-10.xml" {
		t.Fatal(name)
	}
	list, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatTaxes})
	if len(list) != 2 {
		t.Fatalf("audit exportov: %d", len(list))
	}
}

func TestVATReturnRules(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	// chýba DIČ → export nejde, výpočet áno
	if _, _, err := a.VATExport(ctx, c.ID, "dph", "2026-10", m.Zero); !errors.Is(err, ErrMissingVATData) {
		t.Fatalf("bez DIČ: %v", err)
	}
	c.VATPayer = false
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero); !errors.Is(err, ErrNotVATPayer) {
		t.Fatalf("neplatiteľ: %v", err)
	}
}

func TestVATPermissions(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	c := o.alfa
	c.DIC = "2020372640"
	if _, err := o.a.UpdateCompany(actx, c); err != nil {
		t.Fatal(err)
	}
	member := func(email string, role auth.Role) auth.Actor {
		mb, err := o.a.AddMember(actx, c.ID, NewMember{Email: email, Name: email, Password: "heslo-1234567", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
		return actorOf(u, auth.Actor{Channel: auth.ChannelWeb})
	}
	tests := []struct {
		name  string
		actor auth.Actor
		want  error
	}{
		{"správca", o.admin, nil},
		{"účtovníčka", o.jana, nil},
		{"klient kancelárie", member("klient@alfa.sk", auth.RoleClient), nil},
		{"len čítanie", member("audit@alfa.sk", auth.RoleViewer), nil},
		{"fakturant", member("fero@alfa.sk", auth.RoleInvoicer), ErrForbidden},
		{"cudzí klient", o.klient, store.ErrNotFound},
	}
	for _, tt := range tests {
		if _, _, err := o.a.VATReturn(as(tt.actor), c.ID, "2026-10", m.Zero); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, chcem %v", tt.name, err, tt.want)
		}
	}
}
