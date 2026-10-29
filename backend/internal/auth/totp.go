package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Dvojfázové overenie: jednorazové kódy TOTP (RFC 6238) z aplikácie v mobile
// (Google Authenticator, Microsoft Authenticator, Aegis…) a záložné kódy.
//
// Parametre sú tie, ktoré podporujú všetky aplikácie: SHA-1, 6 číslic, 30 s.

// MFA sú údaje dvojfázového overenia používateľa.
type MFA struct {
	Enabled  bool     `json:"enabled"`
	Secret   string   `json:"secret,omitempty"`   // base32; pri nastavovaní už je, Enabled ešte nie
	LastStep int64    `json:"lastStep,omitempty"` // posledný použitý časový krok — kód sa nedá použiť dvakrát
	Recovery []string `json:"recovery,omitempty"` // odtlačky nepoužitých záložných kódov
}

const (
	totpPeriod = 30
	totpDigits = 6
	// totpSkew: povolený rozdiel hodín telefónu a servera (± jeden krok).
	totpSkew = 1
	// RecoveryCount je počet záložných kódov.
	RecoveryCount = 10
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret vytvorí náhodný kľúč (160 bitov) v base32.
func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

// totpStep je časový krok pre daný čas.
func totpStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// totpAt vypočíta kód pre krok (RFC 4226, dynamické orezanie).
func totpAt(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil || len(key) == 0 {
		return "", fmt.Errorf("neplatný kľúč TOTP")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, v%1_000_000), nil
}

// TOTPCode vráti aktuálny kód (pre testy a ukážky).
func TOTPCode(secret string, t time.Time) (string, error) { return totpAt(secret, totpStep(t)) }

// VerifyTOTP overí kód. Vráti použitý krok; kód z kroku, ktorý už bol
// použitý (lastStep), sa odmietne, aby sa odchytený kód nedal zopakovať.
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	code = normalizeCode(code)
	if len(code) != totpDigits {
		return 0, false
	}
	cur := totpStep(now)
	for d := -totpSkew; d <= totpSkew; d++ {
		step := cur + int64(d)
		if step <= lastStep {
			continue
		}
		want, err := totpAt(secret, step)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// TOTPURI je adresa otpauth:// pre QR kód v aplikácii.
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpPeriod))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Záložné kódy: 10 znakov z abecedy bez zameniteľných znakov (0/O, 1/l/I),
// zobrazené ako „abcde-fghij“ (asi 50 bitov), uložené iba ako odtlačok.
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// NewRecoveryCodes vytvorí záložné kódy a ich odtlačky na uloženie.
func NewRecoveryCodes(n int) (codes, hashes []string, err error) {
	for range n {
		b := make([]byte, 0, 10)
		var buf [16]byte
		for len(b) < 10 {
			if _, err := rand.Read(buf[:]); err != nil {
				return nil, nil, err
			}
			for _, x := range buf {
				// odmietnutie hodnôt nad násobkom dĺžky abecedy: rovnomerné rozdelenie
				if int(x) < 256-256%len(recoveryAlphabet) && len(b) < 10 {
					b = append(b, recoveryAlphabet[int(x)%len(recoveryAlphabet)])
				}
			}
		}
		c := string(b[:5]) + "-" + string(b[5:])
		codes = append(codes, c)
		hashes = append(hashes, recoveryHash(c))
	}
	return codes, hashes, nil
}

func recoveryHash(code string) string { return TokenHash("recovery:" + normalizeCode(code)) }

// UseRecoveryCode nájde záložný kód medzi odtlačkami. Vráti zoznam bez neho
// (každý kód platí iba raz).
func UseRecoveryCode(hashes []string, code string) ([]string, bool) {
	h := recoveryHash(code)
	for i, x := range hashes {
		if subtle.ConstantTimeCompare([]byte(x), []byte(h)) == 1 {
			rest := append([]string{}, hashes[:i]...)
			return append(rest, hashes[i+1:]...), true
		}
	}
	return hashes, false
}

// normalizeCode odstráni medzery a pomlčky a zmení písmená na malé.
func normalizeCode(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	return strings.NewReplacer(" ", "", "-", "").Replace(c)
}

// IsTOTPCode rozlíši kód z aplikácie (6 číslic) od záložného kódu.
func IsTOTPCode(c string) bool {
	c = normalizeCode(c)
	if len(c) != totpDigits {
		return false
	}
	for _, r := range c {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
