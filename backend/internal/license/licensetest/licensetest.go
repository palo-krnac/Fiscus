// Package licensetest pripraví licenciu pre testy (vlastný pár kľúčov).
package licensetest

import (
	"crypto/ed25519"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/license"
)

// Unlimited vráti verejný kľúč a licenciu pre neobmedzený počet firiem na rok.
func Unlimited() ([]ed25519.PublicKey, string) {
	pub, priv, err := license.GenerateKey()
	if err != nil {
		panic(err)
	}
	key, err := license.Sign(license.License{ID: "test", Customer: "Testovacia kancelária", Plan: license.PlanMulti,
		Issued: time.Now(), Expires: time.Now().AddDate(1, 0, 0)}, priv)
	if err != nil {
		panic(err)
	}
	return []ed25519.PublicKey{pub}, key
}
