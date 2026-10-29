package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/legal"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// LoadLegal načíta zákonné parametre z databázy a nastaví ich do programu.
// Nová inštalácia sa predvyplní hodnotami z programu; po aktualizácii sa
// doplnia nové predvolené riadky. Volá sa pri štarte.
func (a *App) LoadLegal(ctx context.Context) error {
	var p legal.Params
	raw, err := a.Store.GetSetting(ctx, legal.SettingKey)
	switch {
	case errors.Is(err, store.ErrNotFound):
		p = legal.Defaults()
		err = a.saveLegal(ctx, p)
		if err == nil {
			a.record(auth.WithActor(ctx, auth.System), 0, audit.CatSettings, "create", "legal", 0,
				"Legislatíva predvyplnená hodnotami programu", nil, p)
		}
	case err != nil:
		return err
	default:
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return err
		}
		before := p
		before.ECB, before.VATPeriods = slices.Clone(p.ECB), slices.Clone(p.VATPeriods)
		if p.Seed() {
			if err = a.saveLegal(ctx, p); err == nil {
				a.record(auth.WithActor(ctx, auth.System), 0, audit.CatSettings, "update", "legal", 0,
					"Legislatíva doplnená o nové hodnoty z aktualizácie programu", before, p)
			}
		}
	}
	if err != nil {
		return err
	}
	a.applyLegal(p)
	return nil
}

func (a *App) saveLegal(ctx context.Context, p legal.Params) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, legal.SettingKey, string(b))
}

func (a *App) applyLegal(p legal.Params) {
	vat.SetPeriods(p.VATPeriods)
	a.legal.Store(&p)
}

// legalParams vráti platné parametre (bez načítania z databázy predvolené).
func (a *App) legalParams() legal.Params {
	if p := a.legal.Load(); p != nil {
		return *p
	}
	return legal.Defaults()
}

// Law vráti parametre upomienok (sadzby ECB, prirážky, paušálna náhrada).
func (a *App) Law() reminder.Law { return a.legalParams().Law() }

// Legal vráti zákonné parametre (čítať ich môže každý prihlásený).
func (a *App) Legal(ctx context.Context) legal.Params { return a.legalParams() }

// SetLegal uloží zákonné parametre. Platia pre všetky firmy, preto ich mení
// iba správca programu; každá zmena je v audite.
func (a *App) SetLegal(ctx context.Context, p legal.Params) (legal.Params, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return p, err
	}
	before := a.legalParams()
	p.Seeded = before.Seeded // spravuje program
	p.Normalize()
	if err := p.Validate(); err != nil {
		return p, err
	}
	if err := a.saveLegal(ctx, p); err != nil {
		return p, err
	}
	a.applyLegal(p)
	a.record(ctx, 0, audit.CatSettings, "update", "legal", 0, "Úprava legislatívy (sadzby DPH, ECB, úrok z omeškania)", before, p)
	return p, nil
}

// ResetLegal vráti zákonné parametre na hodnoty z programu.
func (a *App) ResetLegal(ctx context.Context) (legal.Params, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return legal.Params{}, err
	}
	before := a.legalParams()
	p := legal.Defaults()
	if err := a.saveLegal(ctx, p); err != nil {
		return p, err
	}
	a.applyLegal(p)
	a.record(ctx, 0, audit.CatSettings, "reset", "legal", 0, "Legislatíva obnovená na hodnoty programu", before, p)
	return p, nil
}
