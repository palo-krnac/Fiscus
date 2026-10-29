package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// ErrBadKey — neplatný, zrušený alebo neznámy API kľúč.
var ErrBadKey = errors.New("neplatný alebo zrušený API kľúč")

// KeyPrefix je začiatok každého kľúča Fiškusa (ľahko sa rozpozná v konfigurácii).
const KeyPrefix = "fk_"

// CreateAPIKey vytvorí kľúč pre prihláseného používateľa. Kľúč sa vráti iba
// raz — uloží sa len jeho odtlačok.
func (a *App) CreateAPIKey(ctx context.Context, name string, readOnly bool) (string, auth.APIKey, error) {
	act := auth.ActorFrom(ctx)
	if act.KeyID != 0 || act.Channel == auth.ChannelSystem {
		return "", auth.APIKey{}, ErrForbidden // kľúč nemôže vytvárať ďalšie kľúče
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return "", auth.APIKey{}, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "name", Message: "Zadajte názov kľúča (napr. Claude, najviac 60 znakov)"}}}
	}
	raw, _, err := auth.NewToken()
	if err != nil {
		return "", auth.APIKey{}, err
	}
	token := KeyPrefix + raw
	k := auth.APIKey{UserID: act.UserID, Name: name, Prefix: token[:10], Hash: auth.TokenHash(token), ReadOnly: readOnly, CreatedAt: time.Now().Unix()}
	if err := a.Store.CreateAPIKey(ctx, &k); err != nil {
		return "", k, err
	}
	mode := "s plnými právami používateľa"
	if readOnly {
		mode = "iba na čítanie"
	}
	a.record(ctx, 0, audit.CatAI, "key_create", "api_key", k.ID, "Vytvorený API kľúč „"+name+"“ ("+k.Prefix+"…) "+mode, nil, k)
	return token, k, nil
}

// ListAPIKeys vráti kľúče prihláseného používateľa.
func (a *App) ListAPIKeys(ctx context.Context) ([]auth.APIKey, error) {
	return a.Store.ListAPIKeys(ctx, auth.ActorFrom(ctx).UserID)
}

// RevokeAPIKey zruší kľúč prihláseného používateľa.
func (a *App) RevokeAPIKey(ctx context.Context, id int64) error {
	act := auth.ActorFrom(ctx)
	if act.KeyID != 0 {
		return ErrForbidden
	}
	if err := a.Store.RevokeAPIKey(ctx, act.UserID, id); err != nil {
		return err
	}
	a.record(ctx, 0, audit.CatAI, "key_revoke", "api_key", id, "Zrušený API kľúč", nil, nil)
	return nil
}

// KeyActor overí API kľúč a vráti actora s právami jeho vlastníka.
func (a *App) KeyActor(ctx context.Context, token string, channel auth.Channel, ip string) (auth.Actor, error) {
	if !strings.HasPrefix(token, KeyPrefix) {
		return auth.Actor{}, ErrBadKey
	}
	k, err := a.Store.GetAPIKeyByHash(ctx, auth.TokenHash(token))
	if errors.Is(err, store.ErrNotFound) || (err == nil && k.Revoked) {
		return auth.Actor{}, ErrBadKey
	}
	if err != nil {
		return auth.Actor{}, err
	}
	var act auth.Actor
	if k.UserID == 0 {
		// kľúč lokálneho používateľa platí iba v desktopovej verzii
		if a.Caps.Mode != edition.Desktop {
			return auth.Actor{}, ErrBadKey
		}
		act = auth.Local
	} else {
		u, err := a.Store.GetUser(ctx, k.UserID)
		if err != nil || !u.Active {
			return auth.Actor{}, ErrBadKey
		}
		act = actorOf(u, auth.Actor{})
	}
	act.Channel, act.IP, act.ReadOnly, act.KeyID = channel, ip, k.ReadOnly, k.ID
	act.Name += " · " + k.Name // v audite je vidieť, ktorý kľúč (AI) akciu urobil
	if now := time.Now().Unix(); now-k.LastUsedAt > 60 {
		_ = a.Store.TouchAPIKey(ctx, k.ID, now)
	}
	return act, nil
}
