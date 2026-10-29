package shop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Paddle API (sandbox: https://sandbox-api.paddle.com).
const (
	LiveAPI    = "https://api.paddle.com"
	SandboxAPI = "https://sandbox-api.paddle.com"
)

// API načíta zákazníka z Paddle Billing API (kľúč s právom customer.read).
type API struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
}

// Customer vráti e-mail a meno zákazníka.
func (a API) Customer(ctx context.Context, id string) (string, string, error) {
	c := a.HTTP
	if c == nil {
		c = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.BaseURL, "/")+"/customers/"+id, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.Key)
	res, err := c.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("Paddle API: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("Paddle API: zákazník %s: stav %d", id, res.StatusCode)
	}
	var out struct {
		Data struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return "", "", fmt.Errorf("Paddle API: %w", err)
	}
	return out.Data.Email, out.Data.Name, nil
}

// Webhook je HTTP koncový bod pre notifikácie Paddle (POST).
type Webhook struct {
	Secret string // tajomstvo cieľa notifikácií (pdl_ntfset_…)
	Issuer *Issuer
	Now    func() time.Time
	Log    *log.Logger
}

func (h *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "iba POST", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "telo", http.StatusBadRequest)
		return
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	if err := Verify(r.Header.Get("Paddle-Signature"), body, h.Secret, now); err != nil {
		h.logf("odmietnutý webhook: %v", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "neplatný JSON", http.StatusBadRequest)
		return
	}
	res, err := h.Issuer.Handle(r.Context(), ev)
	if err != nil {
		// 5xx: Paddle doručenie zopakuje; vystavená licencia sa nezdvojí
		h.logf("udalosť %s (%s): %v", ev.EventID, ev.EventType, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.logf("udalosť %s (%s): %s %s", ev.EventID, ev.EventType, res.Action, res.Detail)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *Webhook) logf(f string, a ...any) {
	if h.Log != nil {
		h.Log.Printf(f, a...)
	} else {
		log.Printf(f, a...)
	}
}
