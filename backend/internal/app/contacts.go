package app

import (
	"context"
	"net/mail"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// SaveContact overí a uloží kontakt v adresári.
func (a *App) SaveContact(ctx context.Context, companyID int64, c store.Contact) (store.Contact, error) {
	if _, err := a.Store.GetCompany(ctx, companyID); err != nil {
		return c, err
	}
	c.Name = strings.TrimSpace(c.Name)
	c.ICO = party.NormalizeID(c.ICO)
	c.DIC = party.NormalizeID(c.DIC)
	c.ICDPH = party.NormalizeID(c.ICDPH)
	c.IBAN = formatIBAN(c.IBAN)
	c.Email = strings.TrimSpace(c.Email)
	if c.Country == "" {
		c.Country = "SK"
	}
	c.Country = strings.ToUpper(c.Country)

	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if c.Name == "" {
		add("name", "Zadajte názov alebo meno")
	}
	if c.ICO != "" && c.Country == "SK" && !party.ValidICO(c.ICO) {
		add("ico", "Neplatné IČO")
	}
	if c.ICDPH != "" && strings.HasPrefix(c.ICDPH, "SK") && !party.ValidICDPH(c.ICDPH) {
		add("icdph", "Neplatné IČ DPH")
	}
	if c.IBAN != "" && !party.ValidIBAN(c.IBAN) {
		add("iban", "Neplatný IBAN")
	}
	if c.Email != "" && !validEmail(c.Email) {
		add("email", "Neplatný e-mail")
	}
	if c.DueDays < 0 || c.DueDays > 365 {
		add("dueDays", "Splatnosť musí byť 0 až 365 dní")
	}
	// rovnaké IČO dvakrát v adresári je takmer vždy omyl
	if c.ICO != "" {
		list, err := a.Store.ListContacts(ctx, companyID, c.ICO)
		if err != nil {
			return c, err
		}
		for _, o := range list {
			if o.ICO == c.ICO && o.ID != c.ID {
				add("ico", "Firma s týmto IČO už v adresári je: "+o.Name)
				break
			}
		}
	}
	if len(fe) > 0 {
		return c, &invoice.ValidationError{Fields: fe}
	}
	err := a.Store.SaveContact(ctx, companyID, &c)
	return c, err
}

// PartyFromContact prevedie kontakt na odberateľa faktúry (kópia údajov —
// neskoršia zmena adresára nezmení už vystavené faktúry).
func PartyFromContact(c store.Contact) invoice.Party {
	return invoice.Party{
		Name: c.Name, Street: c.Street, City: c.City, Zip: c.Zip, Country: c.Country,
		ICO: c.ICO, DIC: c.DIC, ICDPH: c.ICDPH, Email: c.Email,
	}
}

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s && strings.Contains(s[strings.LastIndex(s, "@"):], ".")
}

// formatIBAN zapíše IBAN po štvoriciach: "SK3112000000198742637541" → "SK31 1200 0000 1987 4263 7541".
func formatIBAN(s string) string {
	n := party.NormalizeID(s)
	var b strings.Builder
	for i, r := range n {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
