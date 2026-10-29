package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/party"
)

// VIESBaseURL je REST API Európskej komisie na overenie IČ DPH (VIES).
//
//	GET {base}/ms/{krajina}/vat/{číslo} → {"isValid": …, "name": …, "address": …, "userError": …}
const VIESBaseURL = "https://ec.europa.eu/taxation_customs/vies/rest-api"

// ErrBadVATID — IČ DPH nemá tvar „kód krajiny + číslo“.
var ErrBadVATID = errors.New("Zadajte IČ DPH s kódom krajiny, napr. SK2020372640 alebo CZ12345678")

// VATCheck je výsledok overenia IČ DPH vo VIES.
type VATCheck struct {
	VATID   string `json:"vatId"` // napr. CZ12345678
	Country string `json:"country"`
	Valid   bool   `json:"valid"`
	Name    string `json:"name,omitempty"`
	Street  string `json:"street,omitempty"`
	Zip     string `json:"zip,omitempty"`
	City    string `json:"city,omitempty"`
	Address string `json:"address,omitempty"` // celá adresa, ako ju vráti VIES
	Checked string `json:"checked"`           // dátum overenia
}

// VIES overuje IČ DPH v registri platiteľov DPH členských štátov EÚ.
type VIES struct {
	BaseURL string
	HTTP    *http.Client
}

// NewVIES vytvorí klienta. Prázdne baseURL = VIESBaseURL.
func NewVIES(baseURL string) *VIES {
	if baseURL == "" {
		baseURL = VIESBaseURL
	}
	return &VIES{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 8 * time.Second}}
}

// SplitVATID rozdelí IČ DPH na kód krajiny a číslo („GR“ je vo VIES „EL“).
func SplitVATID(id string) (country, number string, err error) {
	id = strings.ToUpper(party.NormalizeID(id))
	if len(id) < 4 || id[0] < 'A' || id[0] > 'Z' || id[1] < 'A' || id[1] > 'Z' {
		return "", "", ErrBadVATID
	}
	country, number = id[:2], id[2:]
	if country == "GR" {
		country = "EL"
	}
	return country, number, nil
}

// Check overí IČ DPH a vráti názov a adresu, ak ich štát zverejňuje.
func (v *VIES) Check(ctx context.Context, vatID string) (VATCheck, error) {
	country, number, err := SplitVATID(vatID)
	if err != nil {
		return VATCheck{}, err
	}
	u := fmt.Sprintf("%s/ms/%s/vat/%s", v.BaseURL, url.PathEscape(country), url.PathEscape(number))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return VATCheck{}, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := v.HTTP.Do(req)
	if err != nil {
		return VATCheck{}, fmt.Errorf("%w (VIES: %v)", ErrUnavailable, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return VATCheck{}, fmt.Errorf("%w (VIES: HTTP %d)", ErrUnavailable, res.StatusCode)
	}
	var body struct {
		IsValid     bool   `json:"isValid"`
		RequestDate string `json:"requestDate"`
		UserError   string `json:"userError"`
		Name        string `json:"name"`
		Address     string `json:"address"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return VATCheck{}, fmt.Errorf("%w (VIES: neplatná odpoveď)", ErrUnavailable)
	}
	switch body.UserError {
	case "", "VALID", "INVALID":
	default: // MS_UNAVAILABLE, TIMEOUT, MS_MAX_CONCURRENT_REQ…
		return VATCheck{}, fmt.Errorf("%w (VIES: %s)", ErrUnavailable, body.UserError)
	}
	out := VATCheck{VATID: strings.ReplaceAll(country, "EL", "GR") + number, Country: strings.ReplaceAll(country, "EL", "GR"),
		Valid: body.IsValid, Checked: firstDate(body.RequestDate)}
	if body.IsValid {
		out.Name = clean(body.Name)
		out.Address = strings.TrimSpace(body.Address)
		out.Street, out.Zip, out.City = SplitAddress(body.Address)
		if (out.Country == "SK" || out.Country == "CZ") && len(out.Zip) == 5 {
			out.Zip = formatZip(out.Zip) // 82412 → 824 12
		}
	}
	return out, nil
}

func clean(s string) string {
	s = strings.TrimSpace(s)
	if s == "---" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func firstDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// SplitAddress rozdelí adresu z VIES („Ulica 1\n82412 Mesto“ alebo
// „Ulica 1, 602 00 Brno“) na ulicu, PSČ a obec. Neznámy tvar ostane v ulici.
func SplitAddress(a string) (street, zip, city string) {
	a = strings.TrimSpace(strings.ReplaceAll(a, "\r", ""))
	if a == "" || a == "---" {
		return "", "", ""
	}
	parts := strings.Split(a, "\n")
	if len(parts) == 1 {
		parts = strings.Split(a, ",")
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	last := parts[len(parts)-1]
	// PSČ: číslice (aj s medzerou) alebo kód s písmenami a číslicami na začiatku
	fields := strings.Fields(last)
	n := 0
	for n < len(fields) && strings.IndexFunc(fields[n], func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 {
		n++
	}
	if n == 0 || n == len(fields) || len(parts) == 1 {
		return strings.Join(parts, ", "), "", ""
	}
	zip = strings.Join(fields[:n], " ")
	city = strings.Join(fields[n:], " ")
	street = strings.Join(parts[:len(parts)-1], ", ")
	return street, zip, city
}
