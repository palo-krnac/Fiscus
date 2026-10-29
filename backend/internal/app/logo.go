package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // dekodér pre kontrolu rozmerov
	_ "image/png"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Limity loga: na faktúre má pár centimetrov, väčší súbor iba spomaľuje tlač.
const (
	MaxLogoBytes = 512 << 10
	maxLogoSide  = 4000
)

// ErrBadLogo: súbor nie je podporovaný obrázok.
var ErrBadLogo = errors.New("logo musí byť obrázok PNG, JPEG alebo WebP")

// logoTypes sú povolené formáty. SVG nie — môže obsahovať skript.
var logoTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

// Logo vráti logo firmy (čítať ho môže každý člen firmy, je na dokladoch).
func (a *App) Logo(ctx context.Context, companyID int64) (store.Logo, error) {
	if err := a.Require(ctx, companyID, ""); err != nil {
		return store.Logo{}, err
	}
	return a.Store.GetLogo(ctx, companyID)
}

// SetLogo overí a uloží logo firmy.
func (a *App) SetLogo(ctx context.Context, companyID int64, data []byte) (store.Logo, error) {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return store.Logo{}, err
	}
	if len(data) == 0 {
		return store.Logo{}, ErrBadLogo
	}
	if len(data) > MaxLogoBytes {
		return store.Logo{}, fmt.Errorf("%w (najviac %d kB, súbor má %d kB)", ErrBadLogo, MaxLogoBytes>>10, (len(data)+1023)>>10)
	}
	ct := http.DetectContentType(data)
	if !logoTypes[ct] {
		return store.Logo{}, ErrBadLogo
	}
	if ct != "image/webp" { // WebP štandardná knižnica nedekóduje
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return store.Logo{}, fmt.Errorf("%w — súbor je poškodený", ErrBadLogo)
		}
		if cfg.Width > maxLogoSide || cfg.Height > maxLogoSide {
			return store.Logo{}, fmt.Errorf("%w — najviac %d × %d bodov", ErrBadLogo, maxLogoSide, maxLogoSide)
		}
	}
	l := store.Logo{ContentType: ct, Data: data, Updated: a.now().UTC()}
	if err := a.Store.SaveLogo(ctx, companyID, l); err != nil {
		return l, err
	}
	type meta struct {
		ContentType string `json:"contentType"`
		Size        int    `json:"size"`
	}
	a.record(ctx, companyID, audit.CatSettings, "update", "logo", companyID, "Nahrané logo firmy", nil, meta{ct, len(data)})
	return l, nil
}

// DeleteLogo odstráni logo firmy.
func (a *App) DeleteLogo(ctx context.Context, companyID int64) error {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return err
	}
	if err := a.Store.DeleteLogo(ctx, companyID); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatSettings, "delete", "logo", companyID, "Odstránené logo firmy", nil, nil)
	return nil
}
