// Package registry vyhľadáva firmy v Registri právnických osôb (RPO),
// ktorý prevádzkuje Štatistický úrad SR. Verejné API, bez kľúča,
// dáta pod licenciou CC BY 4.0.
//
//	GET {base}/search?identifier=31322832 → {"results": [ ... ]}
//
// RPO neobsahuje DIČ ani IČ DPH — tie sú v registroch finančnej správy.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/party"
)

// DefaultBaseURL je adresa verejného RPO API.
const DefaultBaseURL = "https://api.statistics.sk/rpo/v1"

var (
	ErrInvalidICO  = errors.New("Neplatné IČO")
	ErrNotFound    = errors.New("Firma s týmto IČO sa v registri nenašla")
	ErrUnavailable = errors.New("Register nie je dostupný, skontrolujte pripojenie na internet")
)

// Company sú údaje firmy z registra.
type Company struct {
	ICO         string `json:"ico"`
	Name        string `json:"name"`
	Street      string `json:"street"`
	Zip         string `json:"zip"`
	City        string `json:"city"`
	Country     string `json:"country"` // ISO kód
	Established string `json:"established,omitempty"`
	Terminated  string `json:"terminated,omitempty"` // dátum zániku, ak firma zanikla
	DIC         string `json:"dic,omitempty"`        // z registra účtovných závierok
	ICDPH       string `json:"icdph,omitempty"`      // ak VIES potvrdí SK+DIČ (platiteľ DPH)
	Source      string `json:"source"`               // napr. „Obchodný register“
}

// Lookup je zdroj údajov o firmách (v testoch sa dá nahradiť).
type Lookup interface {
	ByICO(ctx context.Context, ico string) (Company, error)
}

// RPO je klient registra s jednoduchou vyrovnávacou pamäťou.
type RPO struct {
	BaseURL string
	HTTP    *http.Client
	TTL     time.Duration // ako dlho platí výsledok v pamäti
	// NotFoundTTL: „nenašlo sa“ platí kratšie (RPO občas vráti prázdny
	// výsledok aj pre existujúcu firmu)
	NotFoundTTL time.Duration

	mu    sync.Mutex
	cache map[string]cached
	now   func() time.Time
}

type cached struct {
	c   Company
	at  time.Time
	err error
}

// NewRPO vytvorí klienta. Prázdne baseURL = DefaultBaseURL.
func NewRPO(baseURL string) *RPO {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &RPO{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		HTTP:        &http.Client{Timeout: 10 * time.Second},
		TTL:         12 * time.Hour,
		NotFoundTTL: 15 * time.Minute,
		cache:       map[string]cached{},
		now:         time.Now,
	}
}

// ByICO nájde firmu podľa IČO.
func (r *RPO) ByICO(ctx context.Context, ico string) (Company, error) {
	ico = party.NormalizeID(ico)
	if len(ico) == 6 {
		ico = "00" + ico
	}
	if !party.ValidICO(ico) {
		return Company{}, ErrInvalidICO
	}

	r.mu.Lock()
	if c, ok := r.cache[ico]; ok && r.now().Sub(c.at) < r.ttl(c.err) {
		r.mu.Unlock()
		return c.c, c.err
	}
	r.mu.Unlock()

	c, err := r.fetch(ctx, ico)
	if errors.Is(err, ErrUnavailable) && ctx.Err() == nil {
		c, err = r.fetch(ctx, ico) // RPO občas zlyhá — jeden pokus navyše
	}
	if err == nil || errors.Is(err, ErrNotFound) { // výpadky sa necachujú
		r.mu.Lock()
		r.cache[ico] = cached{c, r.now(), err}
		r.mu.Unlock()
	}
	return c, err
}

func (r *RPO) ttl(err error) time.Duration {
	if err != nil && r.NotFoundTTL > 0 {
		return r.NotFoundTTL
	}
	return r.TTL
}

func (r *RPO) fetch(ctx context.Context, ico string) (Company, error) {
	u := r.BaseURL + "/search?identifier=" + url.QueryEscape(ico)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Company{}, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := r.HTTP.Do(req)
	if err != nil {
		return Company{}, fmt.Errorf("%w (%v)", ErrUnavailable, err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return Company{}, ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		return Company{}, fmt.Errorf("%w (HTTP %d)", ErrUnavailable, res.StatusCode)
	}
	var body searchResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return Company{}, fmt.Errorf("%w (neplatná odpoveď: %v)", ErrUnavailable, err)
	}
	return pick(body.Results, ico)
}

// ---- štruktúra odpovede RPO ----

type searchResponse struct {
	Results []entity `json:"results"`
}

type timed struct {
	Value     string `json:"value"`
	ValidFrom string `json:"validFrom"`
	ValidTo   string `json:"validTo"`
}

type codeValue struct {
	Value string `json:"value"`
	Code  string `json:"code"`
}

type address struct {
	ValidFrom      string    `json:"validFrom"`
	ValidTo        string    `json:"validTo"`
	Street         string    `json:"street"`
	RegNumber      int       `json:"regNumber"`
	BuildingNumber string    `json:"buildingNumber"`
	PostalCodes    []string  `json:"postalCodes"`
	Municipality   codeValue `json:"municipality"`
	Country        codeValue `json:"country"`
}

type entity struct {
	ID             int64     `json:"id"`
	Identifiers    []timed   `json:"identifiers"`
	FullNames      []timed   `json:"fullNames"`
	Addresses      []address `json:"addresses"`
	Establishment  string    `json:"establishment"`
	Termination    string    `json:"termination"`
	SourceRegister struct {
		Value codeValue `json:"value"`
	} `json:"sourceRegister"`
}

// pick vyberie zo zoznamu výsledkov správny subjekt: s presne zhodným IČO,
// prednostne existujúci (bez dátumu zániku) a najnovšie založený.
func pick(results []entity, ico string) (Company, error) {
	var matches []entity
	for _, e := range results {
		for _, id := range e.Identifiers {
			if id.Value == ico {
				matches = append(matches, e)
				break
			}
		}
	}
	if len(matches) == 0 {
		return Company{}, ErrNotFound
	}
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if (a.Termination == "") != (b.Termination == "") {
			return a.Termination == ""
		}
		return a.Establishment > b.Establishment
	})
	e := matches[0]
	c := Company{
		ICO:         ico,
		Name:        current(e.FullNames),
		Established: e.Establishment,
		Terminated:  e.Termination,
		Source:      e.SourceRegister.Value.Value,
		Country:     "SK",
	}
	if a, ok := currentAddress(e.Addresses); ok {
		c.Street = streetLine(a)
		if len(a.PostalCodes) > 0 {
			c.Zip = formatZip(a.PostalCodes[0])
		}
		c.City = a.Municipality.Value
		if a.Country.Code != "" && a.Country.Code != "703" {
			c.Country = countryISO[a.Country.Code]
		}
	}
	return c, nil
}

// current vráti platnú hodnotu (bez validTo), inak tú s najneskorším validFrom.
func current(values []timed) string {
	best := timed{}
	for _, v := range values {
		if v.ValidTo == "" && (best.ValidTo != "" || v.ValidFrom >= best.ValidFrom) {
			best = v
		} else if best.Value == "" || (best.ValidTo != "" && v.ValidFrom > best.ValidFrom) {
			best = v
		}
	}
	return strings.TrimSpace(best.Value)
}

func currentAddress(list []address) (address, bool) {
	var best address
	found := false
	for _, a := range list {
		switch {
		case !found:
			best, found = a, true
		case a.ValidTo == "" && best.ValidTo != "":
			best = a
		case (a.ValidTo == "") == (best.ValidTo == "") && a.ValidFrom > best.ValidFrom:
			best = a
		}
	}
	return best, found
}

// streetLine zloží „Vlčie hrdlo 1“, „Hlavná 1234/5“ alebo len súpisné číslo pre obce bez ulíc.
func streetLine(a address) string {
	num := strings.TrimSpace(a.BuildingNumber)
	if a.RegNumber > 0 {
		if num != "" {
			num = fmt.Sprintf("%d/%s", a.RegNumber, num)
		} else {
			num = fmt.Sprint(a.RegNumber)
		}
	}
	street := strings.TrimSpace(a.Street)
	if street == "" {
		street = a.Municipality.Value // obec bez ulíc: „Hrochoť 123“
	}
	return strings.TrimSpace(street + " " + num)
}

// formatZip: "82412" → "824 12".
func formatZip(z string) string {
	z = strings.ReplaceAll(z, " ", "")
	if len(z) == 5 {
		return z[:3] + " " + z[3:]
	}
	return z
}

// countryISO prevádza číselník krajín RPO (ISO 3166 numerický) na alfa-2.
var countryISO = map[string]string{
	"703": "SK", "203": "CZ", "040": "AT", "276": "DE", "348": "HU", "616": "PL",
	"804": "UA", "380": "IT", "250": "FR", "528": "NL", "826": "GB", "840": "US",
}
