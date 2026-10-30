package quote

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// Odkaz pre zákazníka: vystavenú ponuku si zákazník otvorí bez prihlásenia
// cez tajný odkaz a môže ju prijať (aj s číslom objednávky) alebo odmietnuť.

// Share je zdieľanie ponuky odkazom.
type Share struct {
	Token    string     `json:"token"`
	URL      string     `json:"url,omitempty"` // dopĺňa aplikácia pri čítaní (podľa verejnej adresy)
	Created  time.Time  `json:"created"`
	Viewed   *time.Time `json:"viewed,omitempty"` // prvé otvorenie zákazníkom
	Response *Response  `json:"response,omitempty"`
}

// Response je odpoveď zákazníka cez odkaz.
type Response struct {
	Accepted bool      `json:"accepted"`
	Name     string    `json:"name"`  // kto odpovedal
	Order    string    `json:"order"` // číslo objednávky zákazníka (nepovinné)
	Note     string    `json:"note"`
	At       time.Time `json:"at"`
}

// Limity textov odpovede.
const (
	maxName  = 120
	maxOrder = 60
	maxNote  = 1000
)

var (
	// ErrExpired — platnosť ponuky uplynula, prijať sa už nedá.
	ErrExpired = errors.New("platnosť ponuky uplynula — kontaktujte nás, pripravíme novú")
	// ErrAnswered — zákazník už odpovedal (alebo ponuku spracoval dodávateľ).
	ErrAnswered = errors.New("na ponuku už bola daná odpoveď")
)

// NewToken vráti náhodný token odkazu (256 bitov).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ValidToken overí tvar tokenu (pred hľadaním v úložisku).
func ValidToken(t string) bool {
	if len(t) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(t)
	return err == nil
}

// Shareable: odkaz má zmysel pri vystavenej ponuke (aj prijatej/odmietnutej).
func (q *Quote) Shareable() bool {
	return q.Status == StatusIssued || q.Status == StatusAccepted || q.Status == StatusRejected
}

// Respond zapíše odpoveď zákazníka a zmení stav ponuky. Odpovedať sa dá
// iba raz a iba na vystavenú ponuku v jej platnosti.
func (q *Quote) Respond(r Response, today invoice.Date, now time.Time) error {
	if q.Share == nil {
		return ErrBadTransition
	}
	if q.Status != StatusIssued || q.Share.Response != nil {
		return ErrAnswered
	}
	if r.Accepted && today.After(q.ValidUntil) {
		return ErrExpired
	}
	r.Name, r.Order, r.Note = clean(r.Name), clean(r.Order), strings.TrimSpace(r.Note)
	var fe []invoice.FieldError
	switch {
	case r.Name == "":
		fe = append(fe, invoice.FieldError{Field: "name", Message: "Zadajte svoje meno"})
	case utf8.RuneCountInString(r.Name) > maxName:
		fe = append(fe, invoice.FieldError{Field: "name", Message: "Meno je príliš dlhé"})
	}
	if utf8.RuneCountInString(r.Order) > maxOrder {
		fe = append(fe, invoice.FieldError{Field: "order", Message: "Číslo objednávky je príliš dlhé"})
	}
	if utf8.RuneCountInString(r.Note) > maxNote {
		fe = append(fe, invoice.FieldError{Field: "note", Message: "Poznámka je príliš dlhá (najviac 1000 znakov)"})
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	r.At = now
	q.Share.Response = &r
	if r.Accepted {
		return q.Accept()
	}
	return q.Reject()
}

// clean zjednotí medzery a odstráni konce riadkov (meno, číslo objednávky).
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }
