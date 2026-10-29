// Package edition určuje, ktoré funkcie sú povolené v danom režime behu.
//
// Rozhoduje backend — frontend sa len pýta /api/capabilities a podľa toho
// zobrazí alebo skryje prvky. Obísť sa to teda nedá úpravou rozhrania.
package edition

import "fmt"

// Mode je režim behu programu.
type Mode string

const (
	Desktop Mode = "desktop" // lokálna inštalácia, jedna firma
	Server  Mode = "server"  // beh na serveri, prístup cez prehliadač
)

// Capabilities sú povolené funkcie.
type Capabilities struct {
	Mode         Mode `json:"mode"`
	MultiCompany bool `json:"multiCompany"`
	MaxCompanies int  `json:"maxCompanies"` // 0 = bez obmedzenia
}

// For vráti schopnosti pre daný režim.
func For(m Mode) (Capabilities, error) {
	switch m {
	case Desktop:
		return Capabilities{Mode: Desktop, MultiCompany: false, MaxCompanies: 1}, nil
	case Server:
		return Capabilities{Mode: Server, MultiCompany: true}, nil
	}
	return Capabilities{}, fmt.Errorf("neznámy režim %q (desktop | server)", m)
}

// CanAddCompany vráti true, ak sa dá založiť ďalšia firma.
func (c Capabilities) CanAddCompany(existing int) bool {
	return c.MaxCompanies == 0 || existing < c.MaxCompanies
}
