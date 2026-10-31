package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/tabimport"
	"github.com/palo-krnac/fiscus/backend/internal/table"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Prenos z tabuľky Excel/CSV z ľubovoľného programu: adresár, cenník,
// vydané alebo prijaté faktúry (jeden riadok = jedna faktúra, sumy po
// sadzbách DPH). Druh tabuľky sa odhadne z hlavičky, pri faktúrach ho
// treba zvoliť (MigrateOptions.Kind).

// TableReport je náhľad prenosu z tabuľky: navyše druh a rozpoznané stĺpce.
type TableReport struct {
	ImportReport
	Kind    string   `json:"kind"`
	Columns []string `json:"columns"`
}

// ImportTable prenesie dáta z tabuľky (apply=false: iba náhľad).
func (a *App) ImportTable(ctx context.Context, companyID int64, data []byte, opt MigrateOptions, apply bool) (TableReport, error) {
	out := TableReport{ImportReport: ImportReport{Source: "Excel/CSV", Warnings: []string{}, Other: map[string]int{}}}
	if err := a.requireMigrate(ctx, companyID); err != nil {
		return out, err
	}
	rows, err := table.Read(data)
	if err != nil {
		return out, mfaFieldErr("file", err.Error())
	}
	kind := opt.Kind
	if kind == "" {
		kind = tabimport.Detect(rows)
	}
	out.Kind = kind
	m := migration{Source: "Excel/CSV"}
	if kind == tabimport.KindItems {
		parsed, err := catalog.ParseItems(rows, vat.Rate23)
		if err != nil {
			return out, mfaFieldErr("file", err.Error())
		}
		out.Columns = []string{"Kód", "Názov", "MJ", "Cena", "DPH"}
		for _, r := range parsed {
			if len(r.Errors) > 0 {
				m.Skips = append(m.Skips, migSkip{Agenda: agItems, Text: fmt.Sprintf("Cenník, riadok %d: %s", r.Line, r.Errors[0].Message)})
				continue
			}
			m.Items = append(m.Items, r.Item)
		}
	} else {
		c, err := a.Store.GetCompany(ctx, companyID)
		if err != nil {
			return out, err
		}
		def := vat.Rate23
		if !c.VATPayer {
			def = vat.Rate0
		}
		sh, err := tabimport.Parse(rows, kind, def)
		out.Columns = sh.Columns
		if err != nil {
			if errors.Is(err, tabimport.ErrNeedKind) {
				return out, mfaFieldErr("kind", err.Error())
			}
			return out, mfaFieldErr("file", err.Error())
		}
		tableMigration(&m, sh)
	}
	rep, err := a.migrate(ctx, companyID, m, opt, apply)
	out.ImportReport = rep
	return out, err
}

// tableParty prevedie partnera z tabuľky; neznámy štát → upozornenie.
func tableParty(p tabimport.Party) (invoice.Party, string) {
	country := party.CountryCode(p.Country)
	warn := ""
	if country == "" && len(p.ICDPH) > 2 {
		country = p.ICDPH[:2]
	}
	if country == "" {
		if p.Country != "" {
			warn = fmt.Sprintf("%s: neznámy štát %q — doplňte ho v adresári", p.Name, p.Country)
		}
		country = "SK"
	}
	return invoice.Party{Name: p.Name, Street: p.Street, City: p.City, Zip: p.Zip, Country: country, ICO: p.ICO, DIC: p.DIC, ICDPH: p.ICDPH,
		Email: p.Email}, warn
}

func tableMigration(m *migration, sh tabimport.Sheet) {
	for _, c := range sh.Contacts {
		p, warn := tableParty(c.Party)
		if warn != "" {
			m.Warnings = append(m.Warnings, fmt.Sprintf("Adresár, riadok %d: %s", c.Line, warn))
		}
		m.Contacts = append(m.Contacts, migContact{Party: p, Phone: c.Party.Phone, Note: c.Party.Note})
	}
	for _, d := range sh.Docs {
		ref := fmt.Sprintf("riadok %d", d.Line)
		p, warn := tableParty(d.Party)
		if warn != "" {
			m.Warnings = append(m.Warnings, fmt.Sprintf("Riadok %d: %s", d.Line, warn))
		}
		var err error
		if d.Err != "" {
			err = errors.New(d.Err)
		}
		if sh.Kind == tabimport.KindPurchases {
			mp := migPurchase{Ref: firstNonBlank(d.SupplierNumber, d.Number, ref), Paid: d.Paid, PaidDate: d.PaidDate, Err: err}
			if err == nil {
				mp.P = purchase.Purchase{SupplierNumber: firstNonBlank(d.SupplierNumber, d.Number), VariableSymbol: d.VS, Supplier: p,
					IssueDate: d.Issue, DeliveryDate: d.Delivery, ReceivedDate: d.Delivery, DueDate: d.Due,
					Description: firstNonBlank(d.Subject, "Prenos z tabuľky"), Expense: purchase.ExpenseServices}
				for _, am := range d.Amounts {
					mp.P.Rates = append(mp.P.Rates, purchase.RateRow{Rate: am.Rate, Base: am.Base.RoundCents(), VAT: am.VAT.RoundCents()})
				}
			}
			m.Purchases = append(m.Purchases, mp)
			continue
		}
		mi := migInvoice{Number: d.Number, Ref: ref, Credit: d.Credit, Original: d.Original, Reason: d.Subject, Total: d.Total,
			PaidAll: d.Paid, PaidDate: d.PaidDate, Err: err}
		if err == nil {
			mi.Inv = invoice.Invoice{Customer: p, IssueDate: d.Issue, DeliveryDate: d.Delivery, DueDate: d.Due, VariableSymbol: d.VS,
				PriceMode: vat.PriceNet, Subject: migSubject(d.Subject)}
			for _, am := range d.Amounts {
				mi.Summary = append(mi.Summary, invoice.Line{Description: firstNonBlank(d.Subject, "Plnenie podľa faktúry"), Quantity: money.FromInt(1),
					Unit: "ks", UnitPrice: am.Base.RoundCents(), Rate: am.Rate})
			}
		}
		if strings.TrimSpace(mi.Number) == "" && err == nil {
			mi.Err = errors.New("chýba číslo faktúry")
		}
		m.Invoices = append(m.Invoices, mi)
	}
}
