package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strconv"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/license"
)

// ErrLicenseDesktop — desktopová verzia licenciu nepotrebuje.
var ErrLicenseDesktop = errors.New("desktopová verzia je zadarmo a licenciu nepotrebuje")

// Capabilities vráti aktuálne schopnosti vrátane licencie.
func (a *App) Capabilities() edition.Capabilities {
	return a.Caps.WithLicense(a.lic.Load(), a.now())
}

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *App) licenseKeys() []ed25519.PublicKey {
	if a.LicenseKeys != nil {
		return a.LicenseKeys
	}
	return license.TrustedKeys()
}

// LoadLicense načíta uložený kľúč pri štarte (bez auditu).
func (a *App) LoadLicense(key string) error {
	l, err := license.Parse(key, a.licenseKeys())
	if err != nil {
		return err
	}
	a.lic.Store(&l)
	return nil
}

// SetLicense overí a uloží nový licenčný kľúč (iba správca inštalácie).
func (a *App) SetLicense(ctx context.Context, key string) (edition.Capabilities, error) {
	act := auth.ActorFrom(ctx)
	if !act.SuperAdmin {
		return edition.Capabilities{}, ErrForbidden
	}
	if a.Caps.Mode != edition.Server {
		return edition.Capabilities{}, ErrLicenseDesktop
	}
	l, err := license.Parse(key, a.licenseKeys())
	if err != nil {
		a.record(ctx, 0, audit.CatSettings, "license_failed", "license", 0, "Neplatný licenčný kľúč: "+err.Error(), nil, nil)
		return edition.Capabilities{}, err
	}
	if !l.Valid(a.now()) {
		return edition.Capabilities{}, license.ErrExpired
	}
	if a.SaveLicense != nil {
		if err := a.SaveLicense(key); err != nil {
			return edition.Capabilities{}, err
		}
	}
	a.lic.Store(&l)
	a.record(ctx, 0, audit.CatSettings, "license", "license", 0,
		"Nahraná licencia pre "+l.Customer+", platná do "+l.Expires.Format("2. 1. 2006"), nil,
		map[string]any{"id": l.ID, "customer": l.Customer, "companies": l.Companies, "expires": l.Expires})
	return a.Capabilities(), nil
}

// limitError je ErrCompanyLimit s vysvetlením podľa edície.
type limitError struct{ msg string }

func (e limitError) Error() string        { return e.msg }
func (e limitError) Is(target error) bool { return target == ErrCompanyLimit }

func (a *App) companyLimitError() error {
	c := a.Capabilities()
	switch {
	case c.Mode != edition.Server:
		return ErrCompanyLimit
	case c.License != nil && !c.License.Valid:
		return limitError{"Licencia vypršala — nové firmy nemožno zakladať, kým ju neobnovíte. Existujúce firmy fungujú ďalej."}
	case c.Plan == edition.PlanMulti:
		return limitError{"Licencia je na " + itoa(c.MaxCompanies) + " firiem a všetky sú využité."}
	}
	return limitError{"Bezplatná serverová verzia je pre jednu firmu. Pre viac firiem nahrajte licenciu (" + edition.PriceMulti + ")."}
}

func itoa(n int) string { return strconv.Itoa(n) }
