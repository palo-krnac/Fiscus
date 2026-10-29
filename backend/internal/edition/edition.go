// Package edition určuje, ktoré funkcie sú povolené v danom režime behu.
//
// Rozhoduje backend — frontend sa len pýta /api/capabilities a podľa toho
// zobrazí alebo skryje prvky. Obísť sa to teda nedá úpravou rozhrania.
//
// Ceny Fiškusa:
//   - desktopová verzia: zadarmo (jedna firma)
//   - serverová verzia pre jednu firmu: zadarmo (s používateľmi a rolami)
//   - serverová verzia pre viac firiem: licencia 99 € ročne (Paddle)
package edition

import (
	"fmt"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/license"
)

// Mode je režim behu programu.
type Mode string

const (
	Desktop Mode = "desktop" // lokálna inštalácia, jedna firma
	Server  Mode = "server"  // beh na serveri, prístup cez prehliadač
)

// Plan je cenový plán.
type Plan string

const (
	PlanFree  Plan = "free"  // zadarmo: jedna firma
	PlanMulti Plan = "multi" // licencia: viac firiem
)

// PriceMulti je cena licencie pre viac firiem (na zobrazenie).
const PriceMulti = "99 € ročne"

// Capabilities sú povolené funkcie.
type Capabilities struct {
	Mode         Mode            `json:"mode"`
	Plan         Plan            `json:"plan"`
	MultiCompany bool            `json:"multiCompany"` // dajú sa zakladať ďalšie firmy
	MaxCompanies int             `json:"maxCompanies"` // 0 = bez obmedzenia
	License      *license.Status `json:"license,omitempty"`
	Price        string          `json:"price,omitempty"` // cena licencie (iba server)
}

// For vráti schopnosti pre daný režim bez licencie.
func For(m Mode) (Capabilities, error) {
	switch m {
	case Desktop:
		return Capabilities{Mode: Desktop, Plan: PlanFree, MaxCompanies: 1}, nil
	case Server:
		return Capabilities{Mode: Server, Plan: PlanFree, MaxCompanies: 1, Price: PriceMulti}, nil
	}
	return Capabilities{}, fmt.Errorf("neznámy režim %q (desktop | server)", m)
}

// WithLicense uplatní licenciu k času now. Licencia platí iba pre serverovú
// verziu. Po vypršaní (a dobe na obnovenie) sa nové firmy zakladať nedajú,
// existujúce firmy a ich dáta zostávajú prístupné.
func (c Capabilities) WithLicense(l *license.License, now time.Time) Capabilities {
	if l == nil || c.Mode != Server {
		return c
	}
	st := l.StatusAt(now)
	c.License = &st
	if st.Valid {
		c.Plan = PlanMulti
		c.MultiCompany = true
		c.MaxCompanies = l.Companies
	}
	return c
}

// CanAddCompany vráti true, ak sa dá založiť ďalšia firma.
func (c Capabilities) CanAddCompany(existing int) bool {
	return c.MaxCompanies == 0 || existing < c.MaxCompanies
}
