// Package demo naplní úložisko ukážkovými dátami (vymyslené firmy).
package demo

import (
	"context"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// ico doplní k 7 číslicam platnú kontrolnú číslicu.
func ico(seven string) string {
	sum := 0
	for i := 0; i < 7; i++ {
		sum += int(seven[i]-'0') * (8 - i)
	}
	return fmt.Sprintf("%s%d", seven, (11-sum%11)%10)
}

type customer struct{ name, city, ico string }

var customers = []customer{
	{"Elektro Hron s.r.o.", "Banská Bystrica", ico("4612345")},
	{"Stavmont Zvolen, a.s.", "Zvolen", ico("3687412")},
	{"Agro Liptov družstvo", "Liptovský Mikuláš", ico("0061234")},
	{"TechnoPack Slovakia s.r.o.", "Žilina", ico("5098765")},
	{"Mlyn Brezno s.r.o.", "Brezno", ico("4477120")},
	{"Ing. Peter Kováč – PK Automation", "Banská Bystrica", ico("5233341")},
	{"Automotive Parts Nitra s.r.o.", "Nitra", ico("4790011")},
}

type item struct {
	desc, unit, qty, price string
	rate                   vat.Rate
}

var items = []item{
	{"Programovanie PLC Siemens S7-1500", "hod", "16", "48", vat.Rate23},
	{"Servisný výjazd", "km", "124", "0.45", vat.Rate23},
	{"Úprava vizualizácie HMI", "hod", "6", "42", vat.Rate23},
	{"Rozvádzač RM1 – montáž a zapojenie", "ks", "1", "1850", vat.Rate23},
	{"Kábel CYKY-J 5x2,5", "m", "85", "2.19", vat.Rate23},
	{"Odborná literatúra – normy STN", "ks", "2", "34.90", vat.Rate5},
	{"Revízia elektrickej inštalácie", "ks", "1", "290", vat.Rate23},
	{"Frekvenčný menič 4 kW", "ks", "2", "412.50", vat.Rate23},
}

// Seed vytvorí firmu a 40 faktúr v rôznych stavoch.
func Seed(ctx context.Context, a *app.App) (store.Company, error) {
	c, err := a.CreateCompany(ctx, store.Company{
		Name: "Demo elektroservis", Street: "Námestie SNP 1", City: "Banská Bystrica", Zip: "974 01",
		ICO: ico("5512345"), DIC: "2120000003", ICDPH: "SK2120000003", IBAN: "SK31 1200 0000 1987 4263 7541", VATPayer: true,
	})
	if err != nil {
		return c, err
	}
	// adresár
	contactIDs := make([]int64, len(customers))
	for i, cu := range customers {
		due := 0
		if i%3 == 0 {
			due = 30
		}
		ct, err := a.SaveContact(ctx, c.ID, store.Contact{Name: cu.name, City: cu.city, ICO: cu.ico, Country: "SK", DueDays: due})
		if err != nil {
			return c, err
		}
		contactIDs[i] = ct.ID
	}

	today := a.Today()
	for i := 39; i >= 0; i-- { // od najstaršej, aby čísla rástli s dátumom
		ci := (i * 5) % len(customers)
		cu := customers[ci]
		issue := today.AddDays(-i*4 - 1)
		inv := invoice.Invoice{
			Customer:     invoice.Party{Name: cu.name, City: cu.city, Country: "SK", ICO: cu.ico},
			ContactID:    contactIDs[ci],
			IssueDate:    issue,
			DeliveryDate: issue,
			DueDate:      issue.AddDays(14),
		}
		for j := 0; j <= i%3; j++ {
			it := items[(i+j*3)%len(items)]
			inv.Lines = append(inv.Lines, invoice.Line{Description: it.desc, Unit: it.unit,
				Quantity: m.MustParse(it.qty), UnitPrice: m.MustParse(it.price), Rate: it.rate})
		}
		d, err := a.SaveDraft(ctx, c.ID, inv)
		if err != nil {
			return c, err
		}
		if i < 2 {
			continue // posledné dve ostanú ako koncepty
		}
		if _, err := a.Issue(ctx, c.ID, d.ID); err != nil {
			return c, err
		}
		switch {
		case i == 9:
			_, err = a.Cancel(ctx, c.ID, d.ID)
		case i%4 != 3 && i > 5: // niektoré staršie ostanú neuhradené → po splatnosti
			_, err = a.MarkPaid(ctx, c.ID, d.ID, issue.AddDays(7+i%6))
		}
		if err != nil {
			return c, err
		}
	}
	return c, nil
}
