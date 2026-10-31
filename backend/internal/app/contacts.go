package app

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
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
	for _, e := range normalizeContactLists(&c) {
		add(e.Field, e.Message)
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
	var before any
	if c.ID != 0 {
		if old, err := a.Store.GetContact(ctx, companyID, c.ID); err == nil {
			before = old
		}
	}
	if err := a.Store.SaveContact(ctx, companyID, &c); err != nil {
		return c, err
	}
	if before == nil {
		a.record(ctx, companyID, audit.CatContacts, "create", "contact", c.ID, "Nový kontakt "+c.Name, nil, c)
	} else {
		a.record(ctx, companyID, audit.CatContacts, "update", "contact", c.ID, "Úprava kontaktu "+c.Name, before, c)
	}
	return c, nil
}

// DeleteContact zmaže kontakt z adresára.
func (a *App) DeleteContact(ctx context.Context, companyID, id int64) error {
	old, err := a.Store.GetContact(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteContact(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatContacts, "delete", "contact", id, "Zmazaný kontakt "+old.Name, old, nil)
	return nil
}

// normalizeContactLists očistí osoby a adresy: prázdne riadky vynechá,
// predvolená môže byť iba jedna osoba a jedna adresa každého druhu.
func normalizeContactLists(c *store.Contact) []invoice.FieldError {
	var fe []invoice.FieldError
	persons := c.Persons[:0:0]
	primary := false
	for _, p := range c.Persons {
		p.Name, p.Role, p.Email, p.Phone, p.Note = strings.TrimSpace(p.Name), strings.TrimSpace(p.Role), strings.TrimSpace(p.Email),
			strings.TrimSpace(p.Phone), strings.TrimSpace(p.Note)
		if p.Name+p.Role+p.Email+p.Phone+p.Note == "" {
			continue
		}
		i := len(persons)
		if p.Name == "" {
			fe = append(fe, invoice.FieldError{Field: fmt.Sprintf("persons[%d].name", i), Message: "Zadajte meno osoby"})
		}
		if p.Email != "" && !validEmail(p.Email) {
			fe = append(fe, invoice.FieldError{Field: fmt.Sprintf("persons[%d].email", i), Message: "Neplatný e-mail"})
		}
		if p.Primary && primary {
			p.Primary = false
		}
		primary = primary || p.Primary
		persons = append(persons, p)
	}
	c.Persons = persons
	addresses := c.Addresses[:0:0]
	defaults := map[string]bool{}
	for _, ad := range c.Addresses {
		ad.Name, ad.Street, ad.City, ad.Zip, ad.Note = strings.TrimSpace(ad.Name), strings.TrimSpace(ad.Street), strings.TrimSpace(ad.City),
			strings.ReplaceAll(strings.TrimSpace(ad.Zip), " ", ""), strings.TrimSpace(ad.Note)
		if ad.Name+ad.Street+ad.City+ad.Zip+ad.Note == "" {
			continue
		}
		i := len(addresses)
		switch ad.Kind {
		case store.AddressDelivery, store.AddressPostal, store.AddressBranch, store.AddressOther:
		case "":
			ad.Kind = store.AddressDelivery
		default:
			fe = append(fe, invoice.FieldError{Field: fmt.Sprintf("addresses[%d].kind", i), Message: "Neznámy druh adresy"})
		}
		if ad.Street == "" || ad.City == "" {
			fe = append(fe, invoice.FieldError{Field: fmt.Sprintf("addresses[%d].street", i), Message: "Zadajte ulicu a obec"})
		}
		ad.Country = strings.ToUpper(strings.TrimSpace(ad.Country))
		if ad.Country == "" {
			ad.Country = "SK"
		}
		if ad.Default && defaults[ad.Kind] {
			ad.Default = false
		}
		defaults[ad.Kind] = defaults[ad.Kind] || ad.Default
		addresses = append(addresses, ad)
	}
	c.Addresses = addresses
	return fe
}

// DocExtras vráti predvolenú dodaciu adresu a kontaktnú osobu kontaktu
// na nový doklad (nil, ak nie sú).
func DocExtras(c store.Contact) (*invoice.Address, *invoice.Person) {
	var d *invoice.Address
	for _, ad := range c.Addresses {
		if ad.Kind == store.AddressDelivery && ad.Default {
			d = &invoice.Address{Name: ad.Name, Street: ad.Street, City: ad.City, Zip: ad.Zip, Country: ad.Country}
			break
		}
	}
	var p *invoice.Person
	for _, x := range c.Persons {
		if x.Primary {
			p = &invoice.Person{Name: x.Name, Email: x.Email, Phone: x.Phone}
			break
		}
	}
	return d, p
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

// withDocExtras doplní na nový doklad z adresára predvolenú dodaciu adresu
// a kontaktnú osobu (pravidelná faktúra, faktúra z ponuky).
func (a *App) withDocExtras(ctx context.Context, companyID int64, inv invoice.Invoice) invoice.Invoice {
	if inv.ContactID == 0 || (inv.Delivery != nil && inv.Attention != nil) {
		return inv
	}
	c, err := a.Store.GetContact(ctx, companyID, inv.ContactID)
	if err != nil {
		return inv
	}
	d, p := DocExtras(c)
	if inv.Delivery == nil {
		inv.Delivery = d
	}
	if inv.Attention == nil {
		inv.Attention = p
	}
	return inv
}
