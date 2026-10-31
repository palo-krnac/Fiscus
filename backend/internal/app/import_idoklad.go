package app

import (
	"context"
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/pohoda"
)

// Prenos z iDokladu: iDoklad exportuje pre účtovníka balík XML vo formáte
// POHODY (Nastavenia → Aplikácie → Export, „idokladImportPohoda.zip“,
// jeden súbor na druh dokladu a mesiac). Adresár v ňom nie je, preto sa
// kontakty založia z odberateľov a dodávateľov na faktúrach.

// ImportIDoklad prenesie dáta z exportu iDokladu (ZIP alebo jednotlivé XML).
func (a *App) ImportIDoklad(ctx context.Context, companyID int64, data []byte, opt MigrateOptions, apply bool) (ImportReport, error) {
	if err := a.requireMigrate(ctx, companyID); err != nil {
		return ImportReport{}, err
	}
	empty := ImportReport{Source: "iDoklad", Warnings: []string{}, Other: map[string]int{}}
	d, err := pohoda.ParseArchive(data, MaxMigrateBytes)
	if err != nil {
		msg := err.Error()
		if errors.Is(err, pohoda.ErrNotPohoda) {
			msg = "súbor nie je export z iDokladu pre POHODU — v iDoklade zvoľte Nastavenia → Aplikácie → Export, program POHODA"
		}
		return empty, mfaFieldErr("file", msg)
	}
	if len(d.Contacts)+len(d.Stocks)+len(d.Invoices) == 0 {
		empty.Other = d.Other
		return empty, mfaFieldErr("file", "V exporte nie sú faktúry — v iDoklade vyberte vydané a prijaté faktúry a dobropisy")
	}
	m := pohodaMigration(d, a.Today())
	m.Source = "iDoklad"
	m.ContactsFromDocs = true
	return a.migrate(ctx, companyID, m, opt, apply)
}
