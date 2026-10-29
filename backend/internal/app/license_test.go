package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/license"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

func TestLicenseUnlocksCompanies(t *testing.T) {
	pub, priv, _ := license.GenerateKey()
	_, foreign, _ := license.GenerateKey()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	key := func(months, companies int) string {
		k, _ := license.Sign(license.License{ID: "txn", Customer: "Kancelária", Plan: license.PlanMulti,
			Companies: companies, Issued: now, Expires: now.AddDate(0, months, 0)}, priv)
		return k
	}
	forged, _ := license.Sign(license.License{Plan: license.PlanMulti, Expires: now.AddDate(1, 0, 0)}, foreign)

	caps, _ := edition.For(edition.Server)
	a := New(memstore.New(), caps)
	a.Now = func() time.Time { return now }
	a.LicenseKeys = []ed25519.PublicKey{pub}
	var saved string
	a.SaveLicense = func(k string) error { saved = k; return nil }

	admin := auth.WithActor(context.Background(), auth.Actor{Name: "Správca", SuperAdmin: true, Channel: auth.ChannelWeb})
	user := auth.WithActor(context.Background(), auth.Actor{UserID: 2, Name: "Jana", Channel: auth.ChannelWeb})

	if _, err := a.CreateCompany(admin, store.Company{Name: "Prvá"}); err != nil {
		t.Fatal(err)
	}
	_, err := a.CreateCompany(admin, store.Company{Name: "Druhá"})
	if !errors.Is(err, ErrCompanyLimit) || !strings.Contains(err.Error(), "99 €") {
		t.Fatalf("bez licencie: %v", err)
	}

	if _, err := a.SetLicense(user, key(12, 0)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bežný používateľ nahral licenciu: %v", err)
	}
	if _, err := a.SetLicense(admin, forged); !errors.Is(err, license.ErrSignature) {
		t.Fatalf("falošná licencia: %v", err)
	}
	if _, err := a.SetLicense(admin, key(-3, 0)); !errors.Is(err, license.ErrExpired) {
		t.Fatalf("vypršaná licencia: %v", err)
	}
	if saved != "" {
		t.Fatal("neplatná licencia sa uložila")
	}
	c, err := a.SetLicense(admin, key(12, 3))
	if err != nil || !c.MultiCompany || saved == "" {
		t.Fatalf("platná licencia: %+v %v", c, err)
	}
	if _, err := a.CreateCompany(admin, store.Company{Name: "Druhá"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCompany(admin, store.Company{Name: "Tretia"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCompany(admin, store.Company{Name: "Štvrtá"}); !errors.Is(err, ErrCompanyLimit) || !strings.Contains(err.Error(), "3 firiem") {
		t.Fatalf("limit licencie: %v", err)
	}

	// po vypršaní a dobe na obnovenie: firmy zostávajú, nové nie
	a.Now = func() time.Time { return now.AddDate(1, 2, 0) }
	if c := a.Capabilities(); c.MultiCompany || c.License == nil || c.License.Valid {
		t.Fatalf("po vypršaní: %+v", c)
	}
	if n, _ := a.Store.CountCompanies(context.Background()); n != 3 {
		t.Fatalf("firiem %d", n)
	}

	// audit
	list, _, _ := a.Store.ListAudit(context.Background(), audit.Filter{Category: audit.CatSettings, Search: "licen"})
	if len(list) != 2 { // neplatný podpis + nahraná
		t.Fatalf("audit licencie: %d", len(list))
	}
}

func TestLicenseDesktop(t *testing.T) {
	caps, _ := edition.For(edition.Desktop)
	a := New(memstore.New(), caps)
	admin := auth.WithActor(context.Background(), auth.Local)
	if _, err := a.SetLicense(admin, "FISKUS1.x.y"); !errors.Is(err, ErrLicenseDesktop) {
		t.Fatal(err)
	}
}
