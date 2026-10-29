package shop

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/license"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
)

// CustomerLookup zistí e-mail a meno zákazníka v Paddle (ak ich web
// neposlal v custom_data).
type CustomerLookup interface {
	Customer(ctx context.Context, id string) (email, name string, err error)
}

// Issuer vydáva licencie z dokončených transakcií.
type Issuer struct {
	Key     ed25519.PrivateKey
	PriceID string // cena „Server – viac firiem“ v Paddle (pri_…)
	Months  int    // platnosť jedného kusu (predvolene 12)
	Store   Store
	Mail    mail.Sender
	SMTP    mail.Settings
	Lookup  CustomerLookup // nepovinné
	Now     func() time.Time
}

// Result opisuje, čo sa s udalosťou stalo (do denníka a odpovede webhooku).
type Result struct {
	Action string `json:"action"` // issued, duplicate, ignored
	Detail string `json:"detail,omitempty"`
}

// ErrNoCustomer — v transakcii chýba e-mail zákazníka.
var ErrNoCustomer = errors.New("transakcia nemá e-mail zákazníka (custom_data.email) a Paddle API nie je nastavené")

// Handle spracuje udalosť Paddle. Opakované doručenie tej istej transakcie
// licenciu nevystaví znova (Paddle webhook opakuje, kým nedostane 2xx).
func (is *Issuer) Handle(ctx context.Context, ev Event) (Result, error) {
	if ev.EventType != "transaction.completed" {
		return Result{"ignored", "udalosť " + ev.EventType}, nil
	}
	var t Transaction
	if err := json.Unmarshal(ev.Data, &t); err != nil {
		return Result{}, fmt.Errorf("transakcia: %w", err)
	}
	qty := t.Quantity(is.PriceID)
	if qty == 0 {
		return Result{"ignored", "transakcia " + t.ID + " neobsahuje licenciu Fiškus"}, nil
	}
	if r, ok := is.Store.Get(t.ID); ok {
		if !r.Emailed {
			return is.send(ctx, r, "duplicate")
		}
		return Result{"duplicate", t.ID}, nil
	}
	email, name := t.Custom("email"), t.Custom("company")
	if (email == "" || name == "") && is.Lookup != nil && t.CustomerID != "" {
		e, n, err := is.Lookup.Customer(ctx, t.CustomerID)
		if err != nil {
			return Result{}, err
		}
		if email == "" {
			email = e
		}
		if name == "" {
			name = n
		}
	}
	if email == "" {
		return Result{}, ErrNoCustomer
	}
	if name == "" {
		name = email
	}
	now := is.now()
	start := now
	// obnova: nová platnosť nadväzuje na koniec doteraz platnej licencie
	if last, ok := is.Store.LastFor(email); ok && last.Expires.After(now) {
		start = last.Expires
	}
	months := is.Months
	if months <= 0 {
		months = 12
	}
	l := license.License{ID: t.ID, Customer: name, Email: email, Plan: license.PlanMulti,
		Issued: now, Expires: start.AddDate(0, months*qty, 0)}
	key, err := license.Sign(l, is.Key)
	if err != nil {
		return Result{}, err
	}
	r := Record{TransactionID: t.ID, Customer: name, Email: email, Issued: now, Expires: l.Expires, Key: key}
	if err := is.Store.Add(r); err != nil {
		return Result{}, err
	}
	return is.send(ctx, r, "issued")
}

func (is *Issuer) now() time.Time {
	if is.Now != nil {
		return is.Now().UTC().Truncate(time.Second)
	}
	return time.Now().UTC().Truncate(time.Second)
}

// send pošle kľúč zákazníkovi; pri chybe vráti chybu (Paddle webhook zopakuje
// a licencia sa už nevystaví znova, iba sa pošle).
func (is *Issuer) send(ctx context.Context, r Record, action string) (Result, error) {
	if is.Mail == nil || !is.SMTP.Configured() {
		return Result{action, "e-mail nie je nastavený — kľúč pošlite ručne"}, nil
	}
	err := is.Mail.Send(ctx, is.SMTP, LicenseMail(is.SMTP, r))
	if err != nil {
		return Result{}, fmt.Errorf("e-mail s licenciou pre %s: %w", r.Email, err)
	}
	r.Emailed = true
	if err := is.Store.Add(r); err != nil {
		return Result{}, err
	}
	return Result{action, "licencia odoslaná na " + r.Email}, nil
}

// LicenseMail je správa s licenčným kľúčom a návodom.
func LicenseMail(s mail.Settings, r Record) mail.Message {
	body := strings.Join([]string{
		"Dobrý deň,",
		"",
		"ďakujeme za nákup licencie Fiškus – Server pre viac firiem.",
		"",
		"Licenčný kľúč:",
		"",
		r.Key,
		"",
		"Platnosť do: " + r.Expires.Format("2. 1. 2006"),
		"Zákazník: " + r.Customer,
		"Číslo platby: " + r.TransactionID,
		"",
		"Ako licenciu nahrať: prihláste sa ako správca inštalácie, otvorte",
		"Firmy a licencia a vložte kľúč. Prípadne ho uložte do súboru",
		"fiskus.license vedľa databázy alebo do premennej FISKUS_LICENSE.",
		"",
		"Faktúru za nákup vystavila spoločnosť Paddle, ktorá je predajcom",
		"(Merchant of Record) a poslala ju na váš e-mail samostatne.",
		"",
		"S pozdravom",
		"Fiškus – fiskus.sk",
	}, "\n")
	return mail.Message{FromName: s.FromName, From: s.FromEmail, ReplyTo: s.ReplyTo, To: []string{r.Email},
		Subject: "Licencia Fiškus – Server pre viac firiem", Body: body}
}
