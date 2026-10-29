package auth

import "context"

// Channel je kanál, cez ktorý prišla akcia (pre audit).
type Channel string

const (
	ChannelWeb    Channel = "web"    // rozhranie v prehliadači / desktop
	ChannelAPI    Channel = "api"    // API kľúč
	ChannelMCP    Channel = "mcp"    // AI asistent cez MCP
	ChannelSystem Channel = "system" // program sám (zálohy, demo, migrácie)
)

// Actor je ten, kto vykonáva akciu.
type Actor struct {
	UserID     int64   `json:"userId"`
	Name       string  `json:"name"`
	Channel    Channel `json:"channel"`
	IP         string  `json:"ip,omitempty"`
	SuperAdmin bool    `json:"superAdmin"`
	// ReadOnly obmedzí actora na čítanie (API kľúč „len na čítanie“).
	ReadOnly bool `json:"readOnly,omitempty"`
	// KeyID je API kľúč, ktorým sa actor prihlásil (0 = nie kľúčom).
	KeyID int64 `json:"keyId,omitempty"`
	// MFASetup: inštalácia vyžaduje 2FA a používateľ ho ešte nemá — smie
	// iba nastaviť dvojfázové overenie alebo sa odhlásiť.
	MFASetup bool `json:"-"`
}

// System je actor pre akcie programu.
var System = Actor{Name: "Systém", Channel: ChannelSystem}

// Local je jediný používateľ desktopovej verzie (bez prihlasovania).
var Local = Actor{Name: "Lokálny používateľ", Channel: ChannelWeb, SuperAdmin: true}

type ctxKey struct{}

// WithActor uloží actora do kontextu.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// ActorFrom vráti actora z kontextu (System, ak chýba).
func ActorFrom(ctx context.Context) Actor {
	if a, ok := ctx.Value(ctxKey{}).(Actor); ok {
		return a
	}
	return System
}
