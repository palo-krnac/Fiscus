package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/quote"
)

func TestQuoteShare(t *testing.T) {
	a, c, f := mailApp(t) // dnes 29. 10. 2026
	if _, err := a.SetMailSettings(ctx, c.ID, smtpOK, false); err != nil {
		t.Fatal(err)
	}
	dr, _ := a.SaveQuote(ctx, c.ID, validQuote())
	q, err := a.IssueQuote(ctx, c.ID, dr.ID)
	if err != nil || q.Share != nil {
		t.Fatalf("bez verejnej adresy bez odkazu: %+v %v", q.Share, err)
	}
	if _, err := a.ShareQuote(ctx, c.ID, q.ID); !errors.Is(err, ErrNoPublicURL) {
		t.Fatalf("desktop: %v", err)
	}

	a.PublicURL = "https://fiskus.firma.sk/"
	q, err = a.ShareQuote(ctx, c.ID, q.ID)
	if err != nil || q.Share == nil || !strings.HasPrefix(q.Share.URL, "https://fiskus.firma.sk/ponuka/") {
		t.Fatalf("odkaz: %+v %v", q.Share, err)
	}
	token := q.Share.Token
	if again, _ := a.ShareQuote(ctx, c.ID, q.ID); again.Share.Token != token {
		t.Fatal("druhé zdieľanie zmenilo token")
	}
	// e-mail k ponuke obsahuje odkaz
	if d, _ := a.MailDraft(ctx, c.ID, MailQuote, q.ID); !strings.Contains(d.Body, q.Share.URL) {
		t.Fatalf("e-mail bez odkazu:\n%s", d.Body)
	}
	// nová ponuka na serveri dostane odkaz hneď pri vystavení
	dr2, _ := a.SaveQuote(ctx, c.ID, validQuote())
	if q2, _ := a.IssueQuote(ctx, c.ID, dr2.ID); q2.Share == nil || q2.Share.URL == "" {
		t.Fatalf("vystavenie bez odkazu: %+v", q2.Share)
	}

	// zákazník: zobrazenie bez prihlásenia (bez aktéra v kontexte)
	pub := t.Context()
	v, err := a.QuoteForCustomer(pub, token)
	if err != nil || v.Number != q.Number || !v.CanRespond || len(v.Lines) != 1 || v.Lines[0].Amount.String() != "400.00" || v.Supplier.Name != c.Name {
		t.Fatalf("zobrazenie: %+v %v", v, err)
	}
	if got, _ := a.Quote(ctx, c.ID, q.ID); got.Share.Viewed == nil {
		t.Fatal("prvé otvorenie sa nezapísalo")
	}
	if doc, err := a.QuotePDFForCustomer(pub, token); err != nil || len(doc.Data) == 0 {
		t.Fatalf("PDF: %v", err)
	}
	if _, err := a.QuoteForCustomer(pub, "zly-token"); !errors.Is(err, ErrShareNotFound) {
		t.Fatalf("zlý token: %v", err)
	}

	// prijatie s objednávkou → stav, upozornenie firme
	if _, err := a.RespondQuote(pub, token, quote.Response{Accepted: true}); !fieldErr(err, "name") {
		t.Fatalf("bez mena: %v", err)
	}
	v, err = a.RespondQuote(pub, token, quote.Response{Accepted: true, Name: "Ján Novák", Order: "OBJ-2026-15", Note: "Montáž po 10. 11."})
	if err != nil || v.Status != quote.StatusAccepted || v.CanRespond || v.Response.Order != "OBJ-2026-15" {
		t.Fatalf("prijatie: %+v %v", v, err)
	}
	if len(f.sent) != 1 || !strings.Contains(f.last().Subject, "prijal ponuku "+q.Number) || !strings.Contains(f.last().Body, "OBJ-2026-15") {
		t.Fatalf("upozornenie: %d %+v", len(f.sent), f.last())
	}
	if _, err := a.RespondQuote(pub, token, quote.Response{Name: "Ján"}); !errors.Is(err, quote.ErrAnswered) {
		t.Fatalf("druhá odpoveď: %v", err)
	}
	if got, _ := a.Quote(ctx, c.ID, q.ID); got.Status != quote.StatusAccepted || got.Share.Response.Name != "Ján Novák" {
		t.Fatalf("ponuka po prijatí: %+v", got)
	}

	// zrušený odkaz už neplatí
	if _, err := a.UnshareQuote(ctx, c.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.QuoteForCustomer(pub, token); !errors.Is(err, ErrShareNotFound) {
		t.Fatalf("po zrušení: %v", err)
	}
	// kópia ponuky odkaz nepreberie
	if cp, _ := a.CopyQuote(ctx, c.ID, q.ID); cp.Share != nil {
		t.Fatal("kópia s odkazom")
	}
}
