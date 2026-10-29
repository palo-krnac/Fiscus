package auth

import (
	"context"
	"strings"
	"testing"
)

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role Role
		can  []Perm
		not  []Perm
	}{
		{RoleAdmin, []Perm{PermUsersManage, PermSettingsWrite, PermInvoicesWrite, PermAuditRead}, nil},
		{RoleAccountant, []Perm{PermInvoicesWrite, PermPurchasesWrite, PermAuditRead}, []Perm{PermUsersManage, PermSettingsWrite}},
		{RoleInvoicer, []Perm{PermInvoicesWrite, PermContactsWrite}, []Perm{PermPurchasesRead, PermAuditRead, PermUsersManage}},
		{RoleClient, []Perm{PermInvoicesRead, PermPurchasesRead}, []Perm{PermInvoicesWrite, PermPurchasesWrite, PermContactsWrite, PermAuditRead}},
		{RoleViewer, []Perm{PermInvoicesRead}, []Perm{PermInvoicesWrite, PermUsersManage}},
		{Role("hacker"), nil, []Perm{PermInvoicesRead}},
	}
	for _, c := range cases {
		for _, p := range c.can {
			if !c.role.Can(p) {
				t.Errorf("%s má mať %s", c.role, p)
			}
		}
		for _, p := range c.not {
			if c.role.Can(p) {
				t.Errorf("%s nesmie mať %s", c.role, p)
			}
		}
	}
	for _, r := range Roles {
		if !r.Code.Valid() || len(r.Code.Perms()) == 0 {
			t.Errorf("rola %s", r.Code)
		}
	}
}

func TestPassword(t *testing.T) {
	h, err := hashWith("správne heslo 123", 1000) // menej iterácií = rýchly test
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$1000$") || strings.Contains(h, "správne") {
		t.Fatal(h)
	}
	if !CheckPassword(h, "správne heslo 123") || CheckPassword(h, "zlé heslo") || CheckPassword("nezmysel", "x") {
		t.Fatal("overenie hesla")
	}
	h2, _ := hashWith("správne heslo 123", 1000)
	if h == h2 {
		t.Fatal("soľ musí byť náhodná")
	}
	if ValidatePassword("kratke") == nil || ValidatePassword("          ") == nil || ValidatePassword("dostatočne dlhé") != nil {
		t.Fatal("sila hesla")
	}
}

func TestToken(t *testing.T) {
	tok, hash, err := NewToken()
	if err != nil || len(tok) < 40 || TokenHash(tok) != hash || hash == tok {
		t.Fatal("token")
	}
	t2, _, _ := NewToken()
	if tok == t2 {
		t.Fatal("tokeny sa nesmú opakovať")
	}
}

func TestActorContext(t *testing.T) {
	if ActorFrom(context.Background()).Channel != ChannelSystem {
		t.Fatal("predvolene systém")
	}
	ctx := WithActor(context.Background(), Actor{UserID: 7, Name: "Jana", Channel: ChannelMCP})
	if a := ActorFrom(ctx); a.UserID != 7 || a.Channel != ChannelMCP {
		t.Fatal(a)
	}
}
