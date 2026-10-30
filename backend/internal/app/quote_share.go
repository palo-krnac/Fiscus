package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/quote"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Odkaz na ponuku pre zákazníka. Token je tajný (256 bitov) a v nastaveniach
// je index token → firma a ponuka. Odkaz funguje iba na serveri s verejnou
// adresou (-public-url); na desktope by zákazník odkaz neotvoril.

var (
	// ErrNoPublicURL — server nemá verejnú adresu, odkaz by nefungoval.
	ErrNoPublicURL = errors.New("odkaz pre zákazníka funguje iba na serveri dostupnom z internetu — spustite Fiškus s parametrom -public-url")
	// ErrShareNotFound — neplatný alebo zrušený odkaz.
	ErrShareNotFound = errors.New("odkaz na ponuku neplatí — mohol byť zrušený")
)

// customer je aktér pre akcie zákazníka cez odkaz (audit).
var customer = auth.Actor{Name: "Zákazník cez odkaz", Channel: auth.ChannelSystem}

func shareKey(token string) string { return "quote.share." + token }

// QuoteURL vráti odkaz na ponuku pre zákazníka ("" bez odkazu alebo verejnej adresy).
func (a *App) QuoteURL(q quote.Quote) string {
	base := strings.TrimRight(a.PublicURL, "/")
	if q.Share == nil || base == "" {
		return ""
	}
	return base + "/ponuka/" + q.Share.Token
}

// ShareQuote vytvorí odkaz na vystavenú ponuku (ak ešte nie je).
func (a *App) ShareQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	if a.PublicURL == "" {
		return quote.Quote{}, ErrNoPublicURL
	}
	q, err := a.Quote(ctx, companyID, id)
	if err != nil {
		return q, err
	}
	if !q.Shareable() {
		return q, quote.ErrBadTransition
	}
	if q.Share != nil {
		return q, nil
	}
	if err := a.share(ctx, companyID, &q); err != nil {
		return q, err
	}
	a.record(ctx, companyID, audit.CatQuotes, "share", "quote", q.ID, "Odkaz pre zákazníka na ponuku "+q.Number, nil, nil)
	a.linkInvoice(ctx, companyID, &q)
	return q, nil
}

func (a *App) share(ctx context.Context, companyID int64, q *quote.Quote) error {
	token, err := quote.NewToken()
	if err != nil {
		return err
	}
	if err := a.Store.SetSetting(ctx, shareKey(token), fmt.Sprintf("%d:%d", companyID, q.ID)); err != nil {
		return err
	}
	q.Share = &quote.Share{Token: token, Created: a.clock().UTC()}
	return a.Store.SaveQuote(ctx, companyID, q)
}

// UnshareQuote zruší odkaz (zákazník ho už neotvorí).
func (a *App) UnshareQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Quote(ctx, companyID, id)
	if err != nil || q.Share == nil {
		return q, err
	}
	// index sa vyprázdni (úložisko nastavení nemaže) — prázdna hodnota = neplatný odkaz
	if err := a.Store.SetSetting(ctx, shareKey(q.Share.Token), ""); err != nil {
		return q, err
	}
	q.Share = nil
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	a.record(ctx, companyID, audit.CatQuotes, "unshare", "quote", q.ID, "Zrušený odkaz pre zákazníka na ponuku "+q.Number, nil, nil)
	return q, nil
}

// sharedQuote nájde ponuku podľa tokenu.
func (a *App) sharedQuote(ctx context.Context, token string) (int64, quote.Quote, error) {
	if !quote.ValidToken(token) {
		return 0, quote.Quote{}, ErrShareNotFound
	}
	v, err := a.Store.GetSetting(ctx, shareKey(token))
	if err != nil {
		return 0, quote.Quote{}, ErrShareNotFound
	}
	cs, is, ok := strings.Cut(v, ":")
	cid, err1 := strconv.ParseInt(cs, 10, 64)
	id, err2 := strconv.ParseInt(is, 10, 64)
	if !ok || err1 != nil || err2 != nil {
		return 0, quote.Quote{}, ErrShareNotFound
	}
	q, err := a.Store.GetQuote(ctx, cid, id)
	if err != nil || q.Share == nil || q.Share.Token != token {
		return 0, quote.Quote{}, ErrShareNotFound
	}
	return cid, q, nil
}

// PublicLine je položka ponuky pre zákazníka.
type PublicLine struct {
	Description string    `json:"description"`
	Quantity    money.Dec `json:"quantity"`
	Unit        string    `json:"unit"`
	UnitPrice   money.Dec `json:"unitPrice"`
	Rate        vat.Rate  `json:"vatRate"`
	Amount      money.Dec `json:"amount"`
}

// PublicSupplier sú údaje dodávateľa na stránke ponuky.
type PublicSupplier struct {
	Name, Street, City, Zip, ICO, DIC, ICDPH, Email, Phone string
	VATPayer                                               bool
}

// PublicQuote je ponuka, ako ju vidí zákazník cez odkaz (bez interných údajov).
type PublicQuote struct {
	Supplier    PublicSupplier  `json:"supplier"`
	Number      string          `json:"number"`
	Status      quote.Status    `json:"status"`
	Customer    invoice.Party   `json:"customer"`
	IssueDate   invoice.Date    `json:"issueDate"`
	ValidUntil  invoice.Date    `json:"validUntil"`
	Expired     bool            `json:"expired"`
	Currency    string          `json:"currency"`
	PriceMode   vat.PriceMode   `json:"priceMode"`
	NonVATPayer bool            `json:"nonVatPayer"`
	Subject     string          `json:"subject"`
	Intro       string          `json:"intro"`
	Note        string          `json:"note"`
	Lines       []PublicLine    `json:"lines"`
	Summary     vat.Summary     `json:"summary"`
	Response    *quote.Response `json:"response,omitempty"`
	CanRespond  bool            `json:"canRespond"`
}

func (a *App) publicView(ctx context.Context, cid int64, q quote.Quote) (PublicQuote, error) {
	co, err := a.Store.GetCompany(ctx, cid)
	if err != nil {
		return PublicQuote{}, err
	}
	today := a.Today()
	v := PublicQuote{
		Supplier: PublicSupplier{Name: co.Name, Street: co.Street, City: co.City, Zip: co.Zip, ICO: co.ICO, DIC: co.DIC, ICDPH: co.ICDPH,
			Email: co.Email, Phone: co.Phone, VATPayer: co.VATPayer},
		Number: q.Number, Status: q.Status, Customer: q.Customer, IssueDate: q.IssueDate, ValidUntil: q.ValidUntil,
		Expired: today.After(q.ValidUntil), Currency: q.Currency, PriceMode: q.PriceMode, NonVATPayer: q.NonVATPayer,
		Subject: q.Subject, Intro: q.Intro, Note: q.Note, Summary: q.Summary, Lines: []PublicLine{},
	}
	if q.Share != nil {
		v.Response = q.Share.Response
	}
	v.CanRespond = q.Status == quote.StatusIssued && v.Response == nil
	for _, l := range q.Lines {
		v.Lines = append(v.Lines, PublicLine{Description: l.Description, Quantity: l.Quantity, Unit: l.Unit, UnitPrice: l.UnitPrice,
			Rate: l.Rate, Amount: vat.LineAmount(vat.Line{Quantity: l.Quantity, UnitPrice: l.UnitPrice})})
	}
	return v, nil
}

// QuoteForCustomer vráti ponuku podľa tokenu odkazu a zapíše prvé otvorenie.
func (a *App) QuoteForCustomer(ctx context.Context, token string) (PublicQuote, error) {
	cid, q, err := a.sharedQuote(ctx, token)
	if err != nil {
		return PublicQuote{}, err
	}
	if q.Share.Viewed == nil {
		now := a.clock().UTC()
		q.Share.Viewed = &now
		if err := a.Store.SaveQuote(ctx, cid, &q); err == nil {
			a.record(auth.WithActor(ctx, customer), cid, audit.CatQuotes, "view", "quote", q.ID,
				"Zákazník otvoril ponuku "+q.Number+" cez odkaz", nil, nil)
		}
	}
	return a.publicView(ctx, cid, q)
}

// QuotePDFForCustomer vráti PDF ponuky podľa tokenu odkazu.
func (a *App) QuotePDFForCustomer(ctx context.Context, token string) (Document, error) {
	cid, q, err := a.sharedQuote(ctx, token)
	if err != nil {
		return Document{}, err
	}
	return a.QuotePDF(auth.WithActor(ctx, customer), cid, q.ID)
}

// RespondQuote zapíše odpoveď zákazníka (prijatie s objednávkou alebo
// odmietnutie) a pošle firme upozornenie e-mailom, ak má nastavenú poštu.
func (a *App) RespondQuote(ctx context.Context, token string, r quote.Response) (PublicQuote, error) {
	a.quoteMu.Lock()
	defer a.quoteMu.Unlock()
	cid, q, err := a.sharedQuote(ctx, token)
	if err != nil {
		return PublicQuote{}, err
	}
	if err := q.Respond(r, a.Today(), a.clock().UTC()); err != nil {
		return PublicQuote{}, err
	}
	if err := a.Store.SaveQuote(ctx, cid, &q); err != nil {
		return PublicQuote{}, err
	}
	resp := q.Share.Response
	what := "odmietol"
	if resp.Accepted {
		what = "prijal"
	}
	summary := fmt.Sprintf("Zákazník %s ponuku %s cez odkaz (%s)", what, q.Number, resp.Name)
	if resp.Order != "" {
		summary += ", objednávka " + resp.Order
	}
	a.record(auth.WithActor(ctx, customer), cid, audit.CatQuotes, map[bool]string{true: "accept", false: "reject"}[resp.Accepted], "quote", q.ID, summary, nil, resp)
	a.notifyQuoteResponse(ctx, cid, q)
	return a.publicView(ctx, cid, q)
}

// notifyQuoteResponse pošle firme správu o odpovedi zákazníka (na adresu
// pre odpovede, inak odosielateľa). Chyba sa iba zapíše do logu.
func (a *App) notifyQuoteResponse(ctx context.Context, companyID int64, q quote.Quote) {
	s, err := a.loadMail(ctx, companyID)
	if err != nil || !s.Configured() {
		return
	}
	to := s.ReplyTo
	if to == "" {
		to = s.FromEmail
	}
	r := q.Share.Response
	verb := "odmietol"
	if r.Accepted {
		verb = "prijal"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Dobrý deň,\n\nzákazník %s %s cenovú ponuku %s na sumu %s.\n\n", q.Customer.Name, verb, q.Number, money2(q.Summary.Total, q.Currency))
	fmt.Fprintf(&b, "Odpovedal: %s\n", r.Name)
	if r.Order != "" {
		fmt.Fprintf(&b, "Číslo objednávky: %s\n", r.Order)
	}
	if r.Note != "" {
		fmt.Fprintf(&b, "Poznámka: %s\n", r.Note)
	}
	b.WriteString("\nPonuku nájdete v programe Fiškus v časti Cenové ponuky")
	if base := strings.TrimRight(a.PublicURL, "/"); base != "" {
		fmt.Fprintf(&b, ": %s/ponuky/%d", base, q.ID)
	}
	b.WriteString(".\n")
	s.BccSelf = false
	m := mail.Message{To: []string{to}, Subject: fmt.Sprintf("Fiškus: zákazník %s ponuku %s", verb, q.Number), Body: b.String()}
	if _, err := a.deliver(auth.WithActor(ctx, customer), companyID, s, m, store.Email{DocKind: MailQuote, DocID: q.ID, DocNumber: q.Number}); err != nil {
		log.Printf("upozornenie na odpoveď k ponuke %s: %v", q.Number, err)
	}
}
