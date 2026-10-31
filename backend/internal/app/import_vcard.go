package app

import (
	"bytes"
	"context"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/tabimport"
	"github.com/palo-krnac/fiscus/backend/internal/table"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
	"github.com/palo-krnac/fiscus/backend/internal/vcard"
)

// ImportVCard prenesie kontakty z vizitiek vCard (.vcf) do adresára
// (apply=false: iba náhľad). Názov je firma, inak meno osoby; osoba pri
// firme ide do poznámky.
func (a *App) ImportVCard(ctx context.Context, companyID int64, data []byte, opt MigrateOptions, apply bool) (ImportReport, error) {
	if err := a.requireMigrate(ctx, companyID); err != nil {
		return ImportReport{}, err
	}
	cards, err := vcard.Parse(data)
	if err != nil {
		return ImportReport{Source: "vCard", Warnings: []string{}, Other: map[string]int{}}, mfaFieldErr("file", err.Error())
	}
	return a.migrate(ctx, companyID, vcardMigration(cards), opt, apply)
}

func vcardMigration(cards []vcard.Card) migration {
	m := migration{Source: "vCard"}
	for _, c := range cards {
		p, warn := tableParty(tabimport.Party{Name: c.Name(), Street: c.Address.Street, City: c.Address.City, Zip: c.Address.Zip,
			Country: c.Address.Country, Email: c.Email})
		if warn != "" {
			m.Warnings = append(m.Warnings, "Vizitka "+warn)
		}
		m.Contacts = append(m.Contacts, migContact{Party: p, Phone: c.Phone, Note: contactNote(p.Name, c.Person(), c.Note)})
	}
	return m
}

// ImportContacts prenesie kontakty do adresára z vizitiek vCard alebo
// z tabuľky (CSV, Excel, export z Outlooku či Google Kontaktov). Stačí
// právo zapisovať adresár.
func (a *App) ImportContacts(ctx context.Context, companyID int64, data []byte, apply bool) (ImportReport, error) {
	if err := a.Require(ctx, companyID, auth.PermContactsWrite); err != nil {
		return ImportReport{}, err
	}
	empty := ImportReport{Warnings: []string{}, Other: map[string]int{}}
	if bytes.Contains(bytes.ToUpper(data[:min(len(data), 4096)]), []byte("BEGIN:VCARD")) {
		cards, err := vcard.Parse(data)
		if err != nil {
			return empty, mfaFieldErr("file", err.Error())
		}
		return a.migrate(ctx, companyID, vcardMigration(cards), MigrateOptions{}, apply)
	}
	rows, err := table.Read(data)
	if err != nil {
		return empty, mfaFieldErr("file", "Súbor nie je vizitka vCard ani tabuľka CSV/Excel")
	}
	sh, err := tabimport.Parse(rows, tabimport.KindContacts, vat.Rate23)
	if err != nil {
		return empty, mfaFieldErr("file", err.Error())
	}
	m := migration{Source: "Excel/CSV"}
	tableMigration(&m, sh)
	return a.migrate(ctx, companyID, m, MigrateOptions{}, apply)
}
