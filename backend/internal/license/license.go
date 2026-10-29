// Package license overuje licenčné kľúče Fiškusa bez pripojenia na internet.
//
// Desktopová verzia a serverová verzia pre jednu firmu sú zadarmo. Licencia
// (39 € ročne cez Paddle) odomkne serverovú verziu pre viac firiem.
//
// Kľúč má tvar  FISKUS1.<údaje base64url>.<podpis Ed25519 base64url>.
// Podpisuje ho licenčná služba súkromným kľúčom; program obsahuje iba
// verejný kľúč, takže kľúč sa nedá sfalšovať ani upraviť (napr. predĺžiť).
package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const prefix = "FISKUS1"

// Plan je druh licencie.
type Plan string

const (
	PlanMulti Plan = "server-multi" // serverová verzia pre viac firiem
)

// Grace je doba po vypršaní, počas ktorej licencia ešte platí (na obnovenie platby).
const Grace = 30 * 24 * time.Hour

// License sú údaje v kľúči.
type License struct {
	ID        string    `json:"id"`        // napr. číslo transakcie v Paddle
	Customer  string    `json:"customer"`  // obchodné meno zákazníka
	Email     string    `json:"email"`     // e-mail zákazníka
	Plan      Plan      `json:"plan"`      // druh licencie
	Companies int       `json:"companies"` // max. počet firiem, 0 = bez obmedzenia
	Issued    time.Time `json:"issued"`
	Expires   time.Time `json:"expires"`
}

// Chyby overenia.
var (
	ErrFormat    = errors.New("neplatný tvar licenčného kľúča")
	ErrSignature = errors.New("licenčný kľúč nie je pravý (nesedí podpis)")
	ErrExpired   = errors.New("licencia vypršala")
)

// trusted sú verejné kľúče, ktorým program verí (hex). Nový kľúč sa pridá
// sem; starý sa nechá, kým platia licencie ním podpísané.
var trusted = []string{
	// vývojový kľúč — pred prvým predajom nahradiť: fiskus-license keygen
	"dc9c3c95f5a4e20a3e129d250f093632e68667d2c9a175be98efa3ebf7813d41",
}

// TrustedKeys vráti verejné kľúče, ktorým program verí.
func TrustedKeys() []ed25519.PublicKey {
	var out []ed25519.PublicKey
	for _, h := range trusted {
		if b, err := hex.DecodeString(h); err == nil && len(b) == ed25519.PublicKeySize {
			out = append(out, ed25519.PublicKey(b))
		}
	}
	return out
}

// GenerateKey vytvorí nový pár kľúčov (pre licenčnú službu).
func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

var b64 = base64.RawURLEncoding

// Sign vytvorí licenčný kľúč.
func Sign(l License, priv ed25519.PrivateKey) (string, error) {
	data, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	payload := prefix + "." + b64.EncodeToString(data)
	sig := ed25519.Sign(priv, []byte(payload))
	return payload + "." + b64.EncodeToString(sig), nil
}

// Parse overí podpis a prečíta údaje. Platnosť v čase neoveruje (pozri Valid).
func Parse(key string, keys []ed25519.PublicKey) (License, error) {
	key = strings.Join(strings.Fields(key), "") // kľúč skopírovaný z e-mailu môže byť zalomený
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != prefix {
		return License{}, ErrFormat
	}
	data, err1 := b64.DecodeString(parts[1])
	sig, err2 := b64.DecodeString(parts[2])
	if err1 != nil || err2 != nil {
		return License{}, ErrFormat
	}
	payload := []byte(parts[0] + "." + parts[1])
	ok := false
	for _, k := range keys {
		if ed25519.Verify(k, payload, sig) {
			ok = true
			break
		}
	}
	if !ok {
		return License{}, ErrSignature
	}
	var l License
	if err := json.Unmarshal(data, &l); err != nil {
		return License{}, ErrFormat
	}
	if l.Plan != PlanMulti || l.Expires.IsZero() {
		return License{}, ErrFormat
	}
	return l, nil
}

// Valid vráti true, ak licencia k danému času platí (vrátane doby na obnovenie).
func (l License) Valid(now time.Time) bool {
	return now.Before(l.Expires.Add(Grace))
}

// InGrace vráti true, ak licencia vypršala, ale ešte beží doba na obnovenie.
func (l License) InGrace(now time.Time) bool {
	return !now.Before(l.Expires) && l.Valid(now)
}

// Status je stav licencie na zobrazenie.
type Status struct {
	Customer  string    `json:"customer"`
	Email     string    `json:"email"`
	Companies int       `json:"companies"`
	Expires   time.Time `json:"expires"`
	Valid     bool      `json:"valid"`
	Warning   string    `json:"warning,omitempty"`
}

// StatusAt vráti stav licencie k času.
func (l License) StatusAt(now time.Time) Status {
	s := Status{Customer: l.Customer, Email: l.Email, Companies: l.Companies, Expires: l.Expires, Valid: l.Valid(now)}
	switch {
	case !s.Valid:
		s.Warning = "Licencia vypršala " + l.Expires.Format("2. 1. 2006") + ". Nové firmy nemožno zakladať, kým ju neobnovíte."
	case l.InGrace(now):
		end := l.Expires.Add(Grace)
		s.Warning = fmt.Sprintf("Licencia vypršala %s. Obnovte ju do %s.", l.Expires.Format("2. 1. 2006"), end.Format("2. 1. 2006"))
	case l.Expires.Sub(now) < 30*24*time.Hour:
		s.Warning = "Licencia vyprší " + l.Expires.Format("2. 1. 2006") + "."
	}
	return s
}
