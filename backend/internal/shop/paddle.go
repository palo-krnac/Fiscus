// Package shop vydáva licencie po platbe cez Paddle (Merchant of Record):
// overí podpis webhooku, z dokončenej transakcie vystaví podpísaný
// licenčný kľúč a pošle ho zákazníkovi e-mailom.
package shop

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Tolerance je najväčší rozdiel času podpisu a servera (ochrana pred opakovaním).
const Tolerance = 5 * time.Minute

// Chyby overenia webhooku.
var (
	ErrSignature = errors.New("neplatný podpis webhooku Paddle")
	ErrStale     = errors.New("podpis webhooku Paddle je starý")
)

// Sign vytvorí hlavičku Paddle-Signature (pre testy a ručné skúšky).
func Sign(secret string, body []byte, ts time.Time) string {
	t := strconv.FormatInt(ts.Unix(), 10)
	return "ts=" + t + ";h1=" + mac(secret, t, body)
}

func mac(secret, ts string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(ts + ":"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// Verify overí hlavičku Paddle-Signature („ts=…;h1=…“, pri zmene tajomstva
// aj viac h1): HMAC-SHA256 z „ts:telo“ kľúčom cieľa notifikácií.
func Verify(header string, body []byte, secret string, now time.Time) error {
	if secret == "" {
		return ErrSignature
	}
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "ts":
			ts = v
		case "h1":
			sigs = append(sigs, v)
		}
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return ErrSignature
	}
	want := mac(secret, ts, body)
	ok := false
	for _, s := range sigs {
		if hmac.Equal([]byte(s), []byte(want)) {
			ok = true
		}
	}
	if !ok {
		return ErrSignature
	}
	if d := now.Sub(time.Unix(sec, 0)); d > Tolerance || d < -Tolerance {
		return ErrStale
	}
	return nil
}

// Event je notifikácia Paddle Billing.
type Event struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
}

// Transaction sú potrebné údaje transakcie (transaction.completed).
type Transaction struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"`
	CustomerID string         `json:"customer_id"`
	Origin     string         `json:"origin"` // web, subscription_recurring…
	CustomData map[string]any `json:"custom_data"`
	Items      []struct {
		Quantity int `json:"quantity"`
		Price    struct {
			ID string `json:"id"`
		} `json:"price"`
	} `json:"items"`
}

// Quantity vráti počet kusov ceny v transakcii (0 = cena v nej nie je).
func (t Transaction) Quantity(priceID string) int {
	n := 0
	for _, it := range t.Items {
		if it.Price.ID == priceID {
			q := it.Quantity
			if q < 1 {
				q = 1
			}
			n += q
		}
	}
	return n
}

// Custom vráti textovú hodnotu z custom_data (vyplní web pri pokladni).
func (t Transaction) Custom(key string) string {
	if v, ok := t.CustomData[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
