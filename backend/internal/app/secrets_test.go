package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/eshop"
	"github.com/palo-krnac/fiscus/backend/internal/postman"
)

// Uložené heslo, token či kľúč ide iba na adresu, pre ktorú bol zadaný. Kto
// zmení adresu, musí tajný údaj zadať znova — inak by ho používateľ, ktorý ho
// nevidí, poslal na vlastný server.
func TestSecretsStayWithAddress(t *testing.T) {
	t.Run("SMTP", func(t *testing.T) {
		a, c, _ := mailApp(t)
		saved, err := a.SetMailSettings(ctx, c.ID, smtpOK, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, tt := range []struct {
			host string
			keep bool
		}{{saved.Host, true}, {" " + saved.Host + " ", true}, {"utocnik.example", false}} {
			in := saved
			in.Host = tt.host
			if _, err := a.SetMailSettings(ctx, c.ID, in, false); err != nil {
				t.Fatal(err)
			}
			if raw, _ := a.loadMail(ctx, c.ID); (raw.Password == "tajne-heslo") != tt.keep {
				t.Errorf("server %q: heslo ponechané=%v", tt.host, raw.Password != "")
			}
		}
	})
	t.Run("poštár", func(t *testing.T) {
		a, c := newApp(t, edition.Desktop)
		set := postman.Settings{Mode: postman.ModeHTTP, URL: "https://postar.example/api", Token: "tajny-token"}
		if _, err := a.SetEInvoiceSettings(ctx, c.ID, set); err != nil {
			t.Fatal(err)
		}
		set.Token = ""
		if _, err := a.SetEInvoiceSettings(ctx, c.ID, set); err != nil {
			t.Fatal(err)
		}
		if raw, _ := a.loadEInvoice(ctx, c.ID); raw.Token != "tajny-token" {
			t.Fatal("token sa pri rovnakej adrese stratil")
		}
		set.URL = "https://utocnik.example/api"
		_, _ = a.SetEInvoiceSettings(ctx, c.ID, set)
		if raw, _ := a.loadEInvoice(ctx, c.ID); raw.Token == "tajny-token" && raw.URL == set.URL {
			t.Fatal("token ide na novú adresu")
		}
	})
	t.Run("e-shop", func(t *testing.T) {
		a, c := newApp(t, edition.Desktop)
		set, err := a.SaveEshops(ctx, c.ID, []EshopConn{{Platform: eshop.WooCommerce, URL: "https://obchod.example", Key: "ck", Secret: "cs"}})
		if err != nil {
			t.Fatal(err)
		}
		conn := set.Conns[0]
		if _, err := a.SaveEshops(ctx, c.ID, []EshopConn{conn}); err != nil {
			t.Fatalf("rovnaká adresa: %v", err)
		}
		conn.URL = "https://utocnik.example"
		if _, err := a.SaveEshops(ctx, c.ID, []EshopConn{conn}); !fieldErr(err, "conns[0].secret") {
			t.Fatalf("nová adresa bez kľúčov: %v", err)
		}
	})
}
