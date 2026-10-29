package backup

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// Formát šifrovanej zálohy:
//
//	"FISKUSBK" | verzia (1 B) | soľ (16 B) | nonce (12 B) | AES-256-GCM(dáta)
//
// GCM overuje integritu — poškodená záloha alebo zlé heslo sa odhalí.
const (
	magic      = "FISKUSBK"
	version    = 1
	saltLen    = 16
	nonceLen   = 12
	iterations = 600_000 // PBKDF2-SHA256, odporúčanie OWASP
)

var (
	ErrNotBackup   = errors.New("súbor nie je záloha programu Fiskus")
	ErrBadPassword = errors.New("zlé heslo alebo poškodená záloha")
)

func deriveKey(password string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, iterations, 32)
}

// Encrypt zašifruje dáta heslom.
func Encrypt(w io.Writer, plain []byte, password string) error {
	if password == "" {
		return errors.New("heslo zálohy nesmie byť prázdne")
	}
	salt := make([]byte, saltLen)
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	gcm, err := newGCM(password, salt)
	if err != nil {
		return err
	}
	header := append([]byte(magic), version)
	header = append(header, salt...)
	header = append(header, nonce...)
	// hlavička je súčasťou overenia (additional data)
	sealed := gcm.Seal(nil, nonce, plain, header)
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err = w.Write(sealed)
	return err
}

// Decrypt dešifruje a overí zálohu.
func Decrypt(data []byte, password string) ([]byte, error) {
	hl := len(magic) + 1 + saltLen + nonceLen
	if len(data) < hl || !bytes.Equal(data[:len(magic)], []byte(magic)) {
		return nil, ErrNotBackup
	}
	if data[len(magic)] != version {
		return nil, errors.New("nepodporovaná verzia zálohy")
	}
	header := data[:hl]
	salt := header[len(magic)+1 : len(magic)+1+saltLen]
	nonce := header[len(magic)+1+saltLen:]
	gcm, err := newGCM(password, salt)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, data[hl:], header)
	if err != nil {
		return nil, ErrBadPassword
	}
	return plain, nil
}

func newGCM(password string, salt []byte) (cipher.AEAD, error) {
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
