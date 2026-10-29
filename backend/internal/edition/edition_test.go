package edition

import (
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/license"
)

func TestFreeEditions(t *testing.T) {
	for _, m := range []Mode{Desktop, Server} {
		c, err := For(m)
		if err != nil {
			t.Fatal(err)
		}
		if c.Plan != PlanFree || c.MultiCompany || !c.CanAddCompany(0) || c.CanAddCompany(1) {
			t.Fatalf("%s zadarmo: %+v", m, c)
		}
	}
}

func TestLicense(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	lic := func(months, companies int) *license.License {
		return &license.License{Plan: license.PlanMulti, Companies: companies, Expires: now.AddDate(0, months, 0)}
	}
	desktop, _ := For(Desktop)
	server, _ := For(Server)
	tests := []struct {
		name   string
		base   Capabilities
		lic    *license.License
		multi  bool
		canAdd int // koľko firiem ešte dovolí pri 1 existujúcej (−1 = bez obmedzenia)
	}{
		{"server bez licencie", server, nil, false, 0},
		{"server s licenciou", server, lic(12, 0), true, -1},
		{"licencia na 5 firiem", server, lic(12, 5), true, 4},
		{"v dobe na obnovenie", server, lic(-0, 0), true, -1},
		{"vypršaná", server, lic(-3, 0), false, 0},
		{"desktop licenciu ignoruje", desktop, lic(12, 0), false, 0},
	}
	for _, tt := range tests {
		c := tt.base.WithLicense(tt.lic, now)
		if c.MultiCompany != tt.multi {
			t.Errorf("%s: multi %v", tt.name, c.MultiCompany)
		}
		n := 0
		for n < 100 && c.CanAddCompany(1+n) {
			n++
		}
		if tt.canAdd == -1 && n != 100 || tt.canAdd >= 0 && n != tt.canAdd {
			t.Errorf("%s: dovolí ďalších %d firiem", tt.name, n)
		}
		if tt.lic != nil && tt.base.Mode == Server && c.License == nil {
			t.Errorf("%s: chýba stav licencie", tt.name)
		}
	}
}

func TestUnknownMode(t *testing.T) {
	if _, err := For("cloud"); err == nil {
		t.Fatal("neznámy režim mal zlyhať")
	}
}
