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

// Chain hľadá najprv v RPO, potom v RÚZ. Ak RPO firmu nájde, z RÚZ doplní
// DIČ (bez čakania na výsledok dlhšie ako pár sekúnd).
type Chain struct {
	Primary, Fallback Lookup
}

func (c Chain) ByICO(ctx context.Context, ico string) (Company, error) {
	co, err := c.Primary.ByICO(ctx, ico)
	if c.Fallback == nil || errors.Is(err, ErrInvalidICO) {
		return co, err
	}
	if err == nil {
		if co.DIC == "" {
			sub, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if f, ferr := c.Fallback.ByICO(sub, ico); ferr == nil && f.DIC != "" {
				co.DIC = f.DIC
			}
		}
		return co, nil
	}
	f, ferr := c.Fallback.ByICO(ctx, ico)
	switch {
	case ferr == nil:
		return f, nil
	case errors.Is(err, ErrUnavailable) || errors.Is(ferr, ErrUnavailable):
		// jeden zdroj mlčí — „nenašla sa“ by mohlo byť nepravdivé
		if errors.Is(err, ErrUnavailable) {
			return Company{}, err
		}
		return Company{}, ferr
	}
	return Company{}, ErrNotFound
}
