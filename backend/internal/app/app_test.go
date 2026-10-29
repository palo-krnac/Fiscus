package app

import (
	"context"
	"errors"
	"github.com/palo-krnac/fiscus/backend/internal/license/licensetest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

var ctx = context.Background()

func newApp(t *testing.T, mode edition.Mode) (*App, store.Company) {
	t.Helper()
	caps, _ := edition.For(mode)
	a := New(memstore.New(), caps)
	a.Today = func() invoice.Date { return invoice.MustDate("2026-10-29") }
	unlock(t, a)
	c, err := a.CreateCompany(ctx, store.Company{Name: "Moja firma", ICO: "31322832", VATPayer: true, ICDPH: "SK2020372640"})
	if err != nil {
		t.Fatal(err)
	}
	return a, c
}

// unlock nahrá testovaciu licenciu pre viac firiem (iba serverová verzia ju uplatní).
func unlock(t *testing.T, a *App) {
	t.Helper()
	keys, key := licensetest.Unlimited()
	a.LicenseKeys = keys
	if err := a.LoadLicense(key); err != nil {
		t.Fatal(err)
	}
}

func validDraft() invoice.Invoice {
	return invoice.Invoice{
		Customer: invoice.Party{Name: "Odberateľ s.r.o."},
		Lines: []invoice.Line{{Description: "Práca", Quantity: m.MustParse("10"),
			Unit: "hod", UnitPrice: m.MustParse("40"), Rate: vat.Rate23}},
	}
}

func TestDesktopAllowsOnlyOneCompany(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	if _, err := a.CreateCompany(ctx, store.Company{Name: "Druhá"}); !errors.Is(err, ErrCompanyLimit) {
		t.Fatalf("druhá firma v desktope: %v", err)
	}
}

func TestServerAllowsManyCompanies(t *testing.T) {
	a, _ := newApp(t, edition.Server)
	for i := 0; i < 3; i++ {
		if _, err := a.CreateCompany(ctx, store.Company{Name: "Ďalšia"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompanyValidation(t *testing.T) {
	a, _ := newApp(t, edition.Server)
	_, err := a.CreateCompany(ctx, store.Company{Name: "X", ICO: "11111111"})
	var ve *invoice.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("zlé IČO malo zlyhať: %v", err)
	}
}

func TestDraftDefaults(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	inv, err := a.SaveDraft(ctx, c.ID, validDraft())
	if err != nil {
		t.Fatal(err)
	}
	if inv.IssueDate.String() != "2026-10-29" || inv.DeliveryDate.String() != "2026-10-29" ||
		inv.DueDate.String() != "2026-11-12" || inv.Status != invoice.StatusDraft {
		t.Fatalf("predvolené hodnoty: %+v", inv)
	}
	if inv.Summary.Total.String() != "492.00" {
		t.Fatalf("spolu %s", inv.Summary.Total)
	}
}

func TestIncompleteDraftCanBeSavedButNotIssued(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	inv, err := a.SaveDraft(ctx, c.ID, invoice.Invoice{})
	if err != nil {
		t.Fatalf("neúplný koncept sa má dať uložiť: %v", err)
	}
	if _, err := a.Issue(ctx, c.ID, inv.ID); err == nil {
		t.Fatal("neúplný koncept sa nesmie vystaviť")
	}
	// nepodarené vystavenie nesmie spotrebovať číslo z radu
	good, _ := a.SaveDraft(ctx, c.ID, validDraft())
	issued, err := a.Issue(ctx, c.ID, good.ID)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Number != "FA20260001" {
		t.Fatalf("v číselnom rade vznikla diera: %s", issued.Number)
	}
}

func TestIssueNumbersSequentially(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	var nums []string
	for i := 0; i < 3; i++ {
		d, _ := a.SaveDraft(ctx, c.ID, validDraft())
		inv, err := a.Issue(ctx, c.ID, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		nums = append(nums, inv.Number)
	}
	want := []string{"FA20260001", "FA20260002", "FA20260003"}
	for i := range want {
		if nums[i] != want[i] {
			t.Fatalf("čísla %v, chcem %v", nums, want)
		}
	}
}

func TestIssuedInvoiceIsLocked(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, _ := a.Issue(ctx, c.ID, d.ID)
	inv.Customer.Name = "Zmenené"
	if _, err := a.SaveDraft(ctx, c.ID, inv); !errors.Is(err, invoice.ErrLocked) {
		t.Fatalf("vystavená faktúra sa nesmie meniť: %v", err)
	}
	stored, _ := a.Store.GetInvoice(ctx, c.ID, inv.ID)
	if stored.Customer.Name != "Odberateľ s.r.o." {
		t.Fatal("vystavená faktúra bola prepísaná")
	}
}

func TestPayAndCancel(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, _ := a.Issue(ctx, c.ID, d.ID)
	paid, err := a.MarkPaid(ctx, c.ID, inv.ID, invoice.Date{})
	if err != nil || paid.Status != invoice.StatusPaid || paid.PaidDate.String() != "2026-10-29" {
		t.Fatalf("úhrada: %v %+v", err, paid)
	}
	if _, err := a.Cancel(ctx, c.ID, inv.ID); !errors.Is(err, invoice.ErrBadTransition) {
		t.Fatal("uhradenú nemožno stornovať")
	}
}

func TestOtherCompanyCannotIssue(t *testing.T) {
	a, c := newApp(t, edition.Server)
	other, _ := a.CreateCompany(ctx, store.Company{Name: "Cudzia"})
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	if _, err := a.Issue(ctx, other.ID, d.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cudzia firma vystavila faktúru: %v", err)
	}
}

func TestNonVATPayerCompany(t *testing.T) {
	caps, _ := edition.For(edition.Desktop)
	a := New(memstore.New(), caps)
	a.Today = func() invoice.Date { return invoice.MustDate("2026-10-29") }
	c, err := a.CreateCompany(ctx, store.Company{Name: "Živnostník", VATPayer: false})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	if !d.NonVATPayer || !d.Summary.VAT.IsZero() || d.Summary.Total.String() != "400.00" {
		t.Fatalf("neplatiteľ: %+v", d.Summary)
	}
	// po registrácii za platiteľa sa nové faktúry počítajú s DPH
	c.VATPayer, c.ICDPH = true, "SK2020372640"
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	p, _ := a.Preview(ctx, c.ID, validDraft())
	if p.NonVATPayer || p.Summary.VAT.String() != "92.00" {
		t.Fatalf("platiteľ: %+v", p.Summary)
	}
}

func TestUpdateCompanyValidation(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	cases := []struct {
		modify func(*store.Company)
		field  string
	}{
		{func(c *store.Company) { c.Name = "" }, "name"},
		{func(c *store.Company) { c.ICDPH = "" }, "icdph"}, // platiteľ bez IČ DPH
		{func(c *store.Company) { c.IBAN = "SK00 0000" }, "iban"},
		{func(c *store.Company) { c.Email = "nie-je-email" }, "email"},
		{func(c *store.Company) { c.DIC = "123" }, "dic"},
		{func(c *store.Company) { c.DefaultDueDays = 400 }, "defaultDueDays"},
	}
	for _, cs := range cases {
		x := c
		cs.modify(&x)
		_, err := a.UpdateCompany(ctx, x)
		var ve *invoice.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != cs.field {
			t.Errorf("očakávaná chyba %s, dostal som %v", cs.field, err)
		}
	}
	c.IBAN = "sk3112000000198742637541"
	got, err := a.UpdateCompany(ctx, c)
	if err != nil || got.IBAN != "SK31 1200 0000 1987 4263 7541" {
		t.Fatalf("formát IBAN: %q %v", got.IBAN, err)
	}
}

func TestContacts(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	ct, err := a.SaveContact(ctx, c.ID, store.Contact{Name: " Slovnaft ", ICO: "31 322 832", DueDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if ct.Name != "Slovnaft" || ct.ICO != "31322832" || ct.Country != "SK" {
		t.Fatalf("normalizácia: %+v", ct)
	}
	// duplicitné IČO
	_, err = a.SaveContact(ctx, c.ID, store.Contact{Name: "Iný", ICO: "31322832"})
	var ve *invoice.ValidationError
	if !errors.As(err, &ve) || ve.Fields[0].Field != "ico" {
		t.Fatalf("duplicitné IČO: %v", err)
	}
	// úprava toho istého kontaktu s rovnakým IČO je v poriadku
	ct.Phone = "+421 900 000 000"
	if _, err := a.SaveContact(ctx, c.ID, ct); err != nil {
		t.Fatal(err)
	}
	// zahraničná firma: IČO sa nekontroluje slovenským algoritmom
	if _, err := a.SaveContact(ctx, c.ID, store.Contact{Name: "Brno s.r.o.", ICO: "12345678", Country: "CZ"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []store.Contact{{Name: ""}, {Name: "x", Email: "a@b"}, {Name: "x", ICO: "11111111"}, {Name: "x", DueDays: -1}} {
		if _, err := a.SaveContact(ctx, c.ID, bad); err == nil {
			t.Errorf("mal zlyhať: %+v", bad)
		}
	}

	// splatnosť z kontaktu má prednosť pred firmou
	d := validDraft()
	d.ContactID = ct.ID
	inv, _ := a.SaveDraft(ctx, c.ID, d)
	if inv.DueDate.String() != "2026-11-28" {
		t.Fatalf("splatnosť z kontaktu: %s", inv.DueDate)
	}
}

func TestPartyFromContact(t *testing.T) {
	p := PartyFromContact(store.Contact{Name: "A", ICO: "1", ICDPH: "SK2", City: "BB", Email: "e@x.sk"})
	if p.Name != "A" || p.ICO != "1" || p.ICDPH != "SK2" || p.City != "BB" || p.Email != "e@x.sk" {
		t.Fatalf("%+v", p)
	}
}

func TestCustomNumberFormat(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.NumberFormats = map[string]string{"invoice": "{RRRR}/{NNN}"}
	c, err := a.UpdateCompany(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if c.NumberFormats["credit_note"] != "DB{RRRR}{NNNN}" {
		t.Fatalf("chýbajúci formát sa má doplniť predvoleným: %v", c.NumberFormats)
	}
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, err := a.Issue(ctx, c.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Number != "2026/001" || inv.VariableSymbol != "2026001" {
		t.Fatalf("číslo %s, VS %s", inv.Number, inv.VariableSymbol)
	}
	// zmena formátu počas roka pokračuje v rade, nezačína od 1
	c.NumberFormats["invoice"] = "FA{RR}{NNNN}"
	_, _ = a.UpdateCompany(ctx, c)
	d2, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv2, _ := a.Issue(ctx, c.ID, d2.ID)
	if inv2.Number != "FA260002" {
		t.Fatalf("po zmene formátu: %s", inv2.Number)
	}
}

func TestCompanyCodeLists(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if len(c.Units) == 0 || c.Units[0] != "ks" {
		t.Fatalf("predvolené jednotky: %v", c.Units)
	}
	c.Units = []string{" ks ", "hod", "KS", "", "m²"}
	c, _ = a.UpdateCompany(ctx, c)
	if strings.Join(c.Units, ",") != "ks,hod,m²" {
		t.Fatalf("jednotky bez duplicít a prázdnych: %v", c.Units)
	}
	for f, field := range map[string]string{
		"FA{NNNN}":           "numberFormats.invoice", // bez roka
		"FA{RRRR}":           "numberFormats.invoice", // bez poradia
		"FA č. {RRRR}{NNNN}": "numberFormats.invoice", // diakritika a medzera
	} {
		x := c
		x.NumberFormats = map[string]string{"invoice": f}
		_, err := a.UpdateCompany(ctx, x)
		var ve *invoice.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != field {
			t.Errorf("%q: %v", f, err)
		}
	}
	x := c
	x.Units = []string{"veľmi dlhá jednotka"}
	if _, err := a.UpdateCompany(ctx, x); err == nil {
		t.Error("príliš dlhá jednotka mala byť odmietnutá")
	}
}

func TestCreditNoteFlow(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d, _ := a.SaveDraft(ctx, c.ID, validDraft()) // 10 × 40 € + 23 % = 492 €
	orig, _ := a.Issue(ctx, c.ID, d.ID)

	// ku konceptu sa dobropis nedá
	d2, _ := a.SaveDraft(ctx, c.ID, validDraft())
	if _, err := a.CreateCreditNote(ctx, c.ID, d2.ID); err == nil {
		t.Fatal("dobropis ku konceptu")
	}
	// dobropis sa nedá založiť cez bežné uloženie
	fake := validDraft()
	fake.Kind = invoice.KindCreditNote
	if _, err := a.SaveDraft(ctx, c.ID, fake); err == nil {
		t.Fatal("dobropis bez pôvodnej faktúry")
	}

	cn, err := a.CreateCreditNote(ctx, c.ID, orig.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cn.Kind != invoice.KindCreditNote || cn.Summary.Total.String() != "-492.00" {
		t.Fatalf("%+v", cn.Summary)
	}
	// čiastočný dobropis: 4 hodiny, dôvod; klient nemôže zmeniť pôvodnú faktúru
	cn.Lines[0].Quantity = m.MustParse("-4")
	cn.Correction.Reason = "Zľava za oneskorenie"
	cn.Correction.OriginalID = 999
	cn.Kind = invoice.KindInvoice
	cn, err = a.SaveDraft(ctx, c.ID, cn)
	if err != nil {
		t.Fatal(err)
	}
	if cn.Kind != invoice.KindCreditNote || cn.Correction.OriginalID != orig.ID || cn.Correction.Reason != "Zľava za oneskorenie" {
		t.Fatalf("druh alebo odkaz sa zmenil: %s %+v", cn.Kind, cn.Correction)
	}
	issued, err := a.Issue(ctx, c.ID, cn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Number != "DB20260001" || issued.Summary.Total.String() != "-196.80" {
		t.Fatalf("%s %s", issued.Number, issued.Summary.Total)
	}

	// druhý dobropis nad zostatok neprejde
	cn2, _ := a.CreateCreditNote(ctx, c.ID, orig.ID) // celých -492, zostáva 295.20
	cn2.Correction.Reason = "Vrátenie"
	cn2, _ = a.SaveDraft(ctx, c.ID, cn2)
	if _, err := a.Issue(ctx, c.ID, cn2.ID); err == nil {
		t.Fatal("dobropisy presiahli sumu faktúry")
	}
	cn2.Lines[0].Quantity = m.MustParse("-6") // 6 × 40 × 1.23 = 295.20 presne zostatok
	cn2, _ = a.SaveDraft(ctx, c.ID, cn2)
	if _, err := a.Issue(ctx, c.ID, cn2.ID); err != nil {
		t.Fatalf("dobropis na zostatok: %v", err)
	}
}
