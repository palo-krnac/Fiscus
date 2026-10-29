package edition

import "testing"

func TestDesktopIsSingleCompany(t *testing.T) {
	c, err := For(Desktop)
	if err != nil {
		t.Fatal(err)
	}
	if c.MultiCompany || !c.CanAddCompany(0) || c.CanAddCompany(1) {
		t.Fatalf("desktop: %+v", c)
	}
}

func TestServerIsMultiCompany(t *testing.T) {
	c, _ := For(Server)
	if !c.MultiCompany || !c.CanAddCompany(500) {
		t.Fatalf("server: %+v", c)
	}
}

func TestUnknownMode(t *testing.T) {
	if _, err := For("cloud"); err == nil {
		t.Fatal("neznámy režim mal zlyhať")
	}
}
