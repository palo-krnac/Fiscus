// Package payqr skladá QR platbu PAY by square (štandard Slovenskej bankovej
// asociácie, verzia 1.2.0) a QR kód na vloženie do PDF dokladu. Obsah je
// rovnaký ako pri knižnici bysquare vo frontende (lib/payqr.js).
package payqr

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"strings"
)

// Payment je jeden platobný príkaz.
type Payment struct {
	Amount         string // suma s bodkou, napr. "269.12"
	Currency       string // ISO 4217, napr. "EUR"
	DueDate        string // YYYYMMDD
	VariableSymbol string
	Note           string
	IBAN           string
	Name           string // príjemca (povinný od verzie 1.2.0)
	Street         string
	City           string
}

// version120 je verzia formátu v hlavičke.
const version120 = 0x02

// ErrInvalid — platba nemá povinné údaje.
var ErrInvalid = errors.New("payqr: chýba IBAN, suma alebo príjemca")

// clip skráti text na n znakov.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func sanitize(s string) string { return strings.ReplaceAll(s, "\t", " ") }

// serialize zapíše platbu ako polia oddelené tabulátorom (tabuľka 15 špecifikácie).
func serialize(invoiceID string, p Payment) string {
	f := []string{
		sanitize(invoiceID), "1",
		"1", // platobný príkaz
		p.Amount, sanitize(p.Currency), sanitize(p.DueDate), sanitize(p.VariableSymbol),
		"", "", "", // konštantný a špecifický symbol, referencia
		sanitize(p.Note),
		"1", sanitize(p.IBAN), "", // jeden účet bez BIC
		"0", "0", // bez trvalého príkazu a inkasa
		sanitize(p.Name), sanitize(p.Street), sanitize(p.City),
	}
	return strings.Join(f, "\t")
}

// base32hex zakóduje bajty bez zarovnania „=“.
func base32hex(in []byte) string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUV"
	var b strings.Builder
	buf, bits := 0, 0
	for _, c := range in {
		buf = buf<<8 | int(c)
		bits += 8
		for bits >= 5 {
			bits -= 5
			b.WriteByte(chars[(buf>>bits)&31])
		}
		buf &= 1<<bits - 1
	}
	if bits > 0 {
		b.WriteByte(chars[(buf<<(5-bits))&31])
	}
	return b.String()
}

// Encode vráti reťazec PAY by square pre jednu platbu. Diakritika sa odstráni.
func Encode(invoiceID string, p Payment) (string, error) {
	p.IBAN = strings.ToUpper(strings.ReplaceAll(p.IBAN, " ", ""))
	if p.IBAN == "" || p.Amount == "" || strings.TrimSpace(p.Name) == "" {
		return "", ErrInvalid
	}
	p.Note, p.Name, p.Street, p.City = Deburr(p.Note), Deburr(p.Name), Deburr(p.Street), Deburr(p.City)
	payload := []byte(serialize(invoiceID, p))
	checked := make([]byte, 4, 4+len(payload))
	binary.LittleEndian.PutUint32(checked, crc32.ChecksumIEEE(payload))
	checked = append(checked, payload...)
	if len(checked) >= 1<<17 {
		return "", errors.New("payqr: platba je príliš dlhá")
	}
	out := []byte{0x00<<4 | version120, 0x00}
	out = binary.LittleEndian.AppendUint16(out, uint16(len(checked)))
	out = append(out, compressLZMA(checked)...)
	return base32hex(out), nil
}
