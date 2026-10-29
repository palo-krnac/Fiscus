package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Heslá sa ukladajú ako PBKDF2-SHA256 (600 000 iterácií, odporúčanie OWASP):
//
//	pbkdf2-sha256$600000$<soľ base64>$<hash base64>
//
// HashIterations je počet iterácií PBKDF2 pre nové heslá. Testy ho znižujú,
// aby bežali rýchlo; v programe sa nemení.
var HashIterations = 600_000

const (
	hashSaltLen = 16
	hashKeyLen  = 32
	hashPrefix  = "pbkdf2-sha256"
)

// MinPasswordLen je minimálna dĺžka hesla.
const MinPasswordLen = 10

// ValidatePassword overí silu hesla.
func ValidatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < MinPasswordLen {
		return fmt.Errorf("Heslo musí mať aspoň %d znakov", MinPasswordLen)
	}
	if strings.TrimSpace(pw) == "" {
		return errors.New("Heslo nesmie byť iba z medzier")
	}
	return nil
}

// HashPassword vytvorí odtlačok hesla na uloženie.
func HashPassword(pw string) (string, error) {
	return hashWith(pw, HashIterations)
}

func hashWith(pw string, iter int) (string, error) {
	salt := make([]byte, hashSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, iter, hashKeyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", hashPrefix, iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// CheckPassword porovná heslo s odtlačkom v konštantnom čase.
func CheckPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != hashPrefix {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// NewToken vráti náhodný token (relácia, API kľúč) a jeho SHA-256 odtlačok.
// Do databázy sa ukladá iba odtlačok — únik databázy neprezradí tokeny.
func NewToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, TokenHash(token), nil
}

// TokenHash vráti odtlačok tokenu.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
