package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/party"
)

// RUZBaseURL je verejné API Registra účtovných závierok (bez kľúča).
//
//	GET {base}/uctovne-jednotky?zmenene-od=2000-01-01&ico=… → {"id":[…]}
//	GET {base}/uctovna-jednotka?id=…                        → názov, adresa, DIČ
const RUZBaseURL = "https://www.registeruz.sk/cruz-public/api"

// RUZ je záložný zdroj: firmy a živnostníci, ktorí podávajú účtovné
// závierky. Na rozdiel od RPO obsahuje aj DIČ.
type RUZ struct {
	BaseURL string
	HTTP    *http.Client
}

// NewRUZ vytvorí klienta. Prázdne baseURL = RUZBaseURL.
func NewRUZ(baseURL string) *RUZ {
	if baseURL == "" {
		baseURL = RUZBaseURL
	}
	return &RUZ{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 10 * time.Second}}
}

type ruzUnit struct {
	ID          int64  `json:"id"`
	Name        string `json:"nazovUJ"`
	ICO         string `json:"ico"`
	DIC         string `json:"dic"`
	Street      string `json:"ulica"`
	City        string `json:"mesto"`
	Zip         string `json:"psc"`
	Established string `json:"datumZalozenia"`
	Terminated  string `json:"datumZrusenia"`
	State       string `json:"stav"` // „ZMAZANÉ“ = neplatný záznam
	Updated     string `json:"datumPoslednejUpravy"`
}

// ByICO nájde účtovnú jednotku podľa IČO.
func (r *RUZ) ByICO(ctx context.Context, ico string) (Company, error) {
	ico = party.NormalizeID(ico)
	if len(ico) == 6 {
		ico = "00" + ico
	}
	if !party.ValidICO(ico) {
		return Company{}, ErrInvalidICO
	}
	var ids struct {
		ID []int64 `json:"id"`
	}
	if err := r.get(ctx, "/uctovne-jednotky?zmenene-od=2000-01-01&ico="+url.QueryEscape(ico), &ids); err != nil {
		return Company{}, err
	}
	var units []ruzUnit
	for i, id := range ids.ID {
		if i == 5 {
			break // jedna firma má najviac pár záznamov
		}
		var u ruzUnit
		if err := r.get(ctx, "/uctovna-jednotka?id="+strconv.FormatInt(id, 10), &u); err != nil {
			return Company{}, err
		}
		if u.State != "ZMAZANÉ" && u.Name != "" && party.NormalizeID(u.ICO) == ico {
			units = append(units, u)
		}
	}
	if len(units) == 0 {
		return Company{}, ErrNotFound
	}
	// existujúca pred zrušenou, potom najnovšie upravená
	sort.SliceStable(units, func(i, j int) bool {
		a, b := units[i], units[j]
		if (a.Terminated == "") != (b.Terminated == "") {
			return a.Terminated == ""
		}
		return a.Updated > b.Updated
	})
	u := units[0]
	return Company{ICO: ico, Name: strings.TrimSpace(u.Name), Street: strings.TrimSpace(u.Street), Zip: formatZip(u.Zip),
		City: strings.TrimSpace(u.City), Country: "SK", Established: u.Established, Terminated: u.Terminated,
		DIC: strings.TrimSpace(u.DIC), Source: "Register účtovných závierok"}, nil
}

func (r *RUZ) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := r.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w (RÚZ: %v)", ErrUnavailable, err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w (RÚZ: HTTP %d)", ErrUnavailable, res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("%w (RÚZ: neplatná odpoveď: %v)", ErrUnavailable, err)
	}
	return nil
}

// VATChecker overí IČ DPH (VIES).
type VATChecker interface {
	Check(ctx context.Context, vatID string) (VATCheck, error)
}

// Chain hľadá v RPO a súčasne v RÚZ (záloha a DIČ); z DIČ overí vo VIES,
// či je firma platiteľ DPH, a doplní IČ DPH. Doplnky čakajú najviac
// EnrichWait po odpovedi RPO, aby hľadanie nebolo pomalé.
type Chain struct {
	Primary, Fallback Lookup
	VAT               VATChecker
	EnrichWait        time.Duration // predvolene 2 s
}

type lookupResult struct {
	c   Company
	err error
}

func (c Chain) ByICO(ctx context.Context, ico string) (Company, error) {
	if c.Fallback == nil {
		co, err := c.Primary.ByICO(ctx, ico)
		if err == nil {
			co = c.withVAT(ctx, co)
		}
		return co, err
	}
	sub, cancel := context.WithCancel(ctx)
	defer cancel()
	second := make(chan lookupResult, 1)
	go func() {
		f, err := c.Fallback.ByICO(sub, ico)
		if err == nil {
			f = c.withVAT(sub, f)
		}
		second <- lookupResult{f, err}
	}()

	co, err := c.Primary.ByICO(ctx, ico)
	if errors.Is(err, ErrInvalidICO) {
		return co, err
	}
	if err == nil {
		wait := c.EnrichWait
		if wait <= 0 {
			wait = 2 * time.Second
		}
		select {
		case f := <-second:
			if f.err == nil {
				co.DIC, co.ICDPH = firstNonEmpty(co.DIC, f.c.DIC), firstNonEmpty(co.ICDPH, f.c.ICDPH)
			}
		case <-time.After(wait):
		case <-ctx.Done():
		}
		return co, nil
	}
	var f lookupResult
	select {
	case f = <-second:
	case <-ctx.Done():
		return Company{}, ctx.Err()
	}
	switch {
	case f.err == nil:
		return f.c, nil
	case errors.Is(err, ErrUnavailable):
		return Company{}, err // jeden zdroj mlčí — „nenašla sa“ by mohlo byť nepravdivé
	case errors.Is(f.err, ErrUnavailable):
		return Company{}, f.err
	}
	return Company{}, ErrNotFound
}

// withVAT doplní IČ DPH, ak VIES potvrdí, že SK+DIČ je platné.
func (c Chain) withVAT(ctx context.Context, co Company) Company {
	if c.VAT == nil || co.DIC == "" || co.ICDPH != "" {
		return co
	}
	sub, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if r, err := c.VAT.Check(sub, "SK"+co.DIC); err == nil && r.Valid {
		co.ICDPH = "SK" + co.DIC
	}
	return co
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
