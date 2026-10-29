package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Zabezpečenie inštalácie:
//   - desktop: voliteľný PIN — rozhranie v prehliadači sa odomkne PIN-om a po
//     PINIdle nečinnosti (alebo po reštarte programu) sa zamkne;
//   - server: správca inštalácie môže vyžadovať 2FA od všetkých používateľov.

const (
	pinSetting        = "desktop.pin"
	requireMFASetting = "security.require_mfa"
	// PINIdle je čas nečinnosti, po ktorom sa desktop zamkne.
	PINIdle = 30 * time.Minute
	// MinPINLen je najkratší PIN.
	MinPINLen = 4
)

var (
	// ErrPINLocked: desktop je zamknutý PIN-om.
	ErrPINLocked = errors.New("Fiškus je zamknutý — zadajte PIN")
	ErrBadPIN    = errors.New("Nesprávny PIN")
	// ErrMFASetup: inštalácia vyžaduje 2FA a používateľ ho ešte nemá.
	ErrMFASetup = errors.New("Správca vyžaduje dvojfázové overenie — najprv si ho zapnite")
	// nastavenie patrí inej verzii (PIN desktopu, povinné 2FA servera)
	ErrPINDesktopOnly = errors.New("PIN je iba v desktopovej verzii; server používa heslá a 2FA")
	ErrMFAServerOnly  = errors.New("Povinné 2FA je iba v serverovej verzii")
)

type pinSessions struct {
	mu   sync.Mutex
	last map[string]time.Time // odtlačok tokenu → posledné použitie
}

// SecurityStatus je stav zabezpečenia pre nastavenia.
type SecurityStatus struct {
	PIN        bool `json:"pin"`        // desktop: PIN je nastavený
	RequireMFA bool `json:"requireMfa"` // server: 2FA je povinné
}

// Security vráti stav zabezpečenia (iba správca inštalácie).
func (a *App) Security(ctx context.Context) (SecurityStatus, error) {
	if !auth.ActorFrom(ctx).SuperAdmin {
		return SecurityStatus{}, ErrForbidden
	}
	return SecurityStatus{PIN: a.PINEnabled(ctx), RequireMFA: a.RequireMFA(ctx)}, nil
}

// PINEnabled vráti true, ak je desktop chránený PIN-om.
func (a *App) PINEnabled(ctx context.Context) bool {
	if a.Caps.Mode != edition.Desktop {
		return false
	}
	h, err := a.Store.GetSetting(ctx, pinSetting)
	return err == nil && h != ""
}

// SetPIN nastaví, zmení alebo (prázdny pin) zruší PIN desktopu. Zmena
// a zrušenie vyžadujú súčasný PIN. Ostatné odomknuté okná sa zamknú.
func (a *App) SetPIN(ctx context.Context, current, pin string) error {
	if a.Caps.Mode != edition.Desktop {
		return ErrPINDesktopOnly
	}
	if !auth.ActorFrom(ctx).SuperAdmin {
		return ErrForbidden
	}
	old, err := a.Store.GetSetting(ctx, pinSetting)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if old != "" && !auth.CheckPassword(old, current) {
		return mfaFieldErr("current", "Súčasný PIN nesedí")
	}
	pin = strings.TrimSpace(pin)
	hash := ""
	if pin != "" {
		if utf8.RuneCountInString(pin) < MinPINLen || utf8.RuneCountInString(pin) > 32 {
			return mfaFieldErr("pin", "PIN musí mať 4 až 32 znakov")
		}
		if hash, err = auth.HashPassword(pin); err != nil {
			return err
		}
	}
	if err := a.Store.SetSetting(ctx, pinSetting, hash); err != nil {
		return err
	}
	a.pins.mu.Lock()
	a.pins.last = nil // všetky odomknuté okná sa zamknú
	a.pins.mu.Unlock()
	what := "Nastavený PIN desktopu"
	switch {
	case pin == "":
		what = "Zrušený PIN desktopu"
	case old != "":
		what = "Zmenený PIN desktopu"
	}
	a.record(ctx, 0, audit.CatAuth, "pin", "settings", 0, what, nil, nil)
	return nil
}

// UnlockPIN overí PIN a vráti token odomknutia (do cookie).
func (a *App) UnlockPIN(ctx context.Context, pin string) (string, error) {
	h, err := a.Store.GetSetting(ctx, pinSetting)
	if err != nil || h == "" {
		return "", errors.New("PIN nie je nastavený")
	}
	if !auth.CheckPassword(h, strings.TrimSpace(pin)) {
		a.record(ctx, 0, audit.CatAuth, "login_failed", "settings", 0, "Nesprávny PIN desktopu", nil, nil)
		return "", ErrBadPIN
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	a.pins.mu.Lock()
	if a.pins.last == nil {
		a.pins.last = map[string]time.Time{}
	}
	a.pins.last[hash] = a.now()
	a.pins.mu.Unlock()
	return token, nil
}

// PINSession overí token odomknutia a predĺži ho (nečinnosť PINIdle zamkne).
func (a *App) PINSession(token string) bool {
	if token == "" {
		return false
	}
	h := auth.TokenHash(token)
	now := a.now()
	a.pins.mu.Lock()
	defer a.pins.mu.Unlock()
	t, ok := a.pins.last[h]
	if !ok || now.Sub(t) > PINIdle {
		delete(a.pins.last, h)
		return false
	}
	a.pins.last[h] = now
	return true
}

// LockPIN zamkne okno (zruší token odomknutia).
func (a *App) LockPIN(token string) {
	a.pins.mu.Lock()
	defer a.pins.mu.Unlock()
	delete(a.pins.last, auth.TokenHash(token))
}

// RequireMFA vráti true, ak inštalácia vyžaduje 2FA od všetkých.
func (a *App) RequireMFA(ctx context.Context) bool {
	if a.Caps.Mode != edition.Server {
		return false
	}
	v, err := a.Store.GetSetting(ctx, requireMFASetting)
	return err == nil && v == "true"
}

// SetRequireMFA zapne alebo vypne povinné 2FA (iba správca inštalácie).
// Správca, ktorý ho zapína, musí mať 2FA sám — inak by sa zamkol.
func (a *App) SetRequireMFA(ctx context.Context, on bool) error {
	if a.Caps.Mode != edition.Server {
		return ErrMFAServerOnly
	}
	act := auth.ActorFrom(ctx)
	if !act.SuperAdmin || act.KeyID != 0 {
		return ErrForbidden
	}
	if on {
		u, err := a.Store.GetUser(ctx, act.UserID)
		if err != nil {
			return err
		}
		if !u.MFA.Enabled {
			return mfaFieldErr("requireMfa", "Najprv si zapnite dvojfázové overenie sami (ponuka pri vašom mene)")
		}
	}
	v := "false"
	if on {
		v = "true"
	}
	if err := a.Store.SetSetting(ctx, requireMFASetting, v); err != nil {
		return err
	}
	what := "Vypnuté povinné dvojfázové overenie"
	if on {
		what = "Zapnuté povinné dvojfázové overenie pre všetkých používateľov"
	}
	a.record(ctx, 0, audit.CatAuth, "require_mfa", "settings", 0, what, nil, nil)
	return nil
}
