// Package extract číta údaje z PDF alebo fotky prijatej faktúry pomocou
// Claude API (Anthropic). Model vráti štruktúrované údaje cez nástroj
// (tool use), program ich prevedie na koncept prijatej faktúry — používateľ
// ich vždy skontroluje pred uložením.
package extract

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Predvolené nastavenie.
const (
	DefaultModel   = "claude-sonnet-5-5"
	DefaultBaseURL = "https://api.anthropic.com"
	// MaxBytes je najväčší súbor (limit API pre PDF je 32 MB, faktúra má zvyčajne do 1 MB).
	MaxBytes = 10 << 20
)

var (
	// ErrOff — čítanie dokladov nie je nastavené (chýba kľúč API).
	ErrOff = errors.New("čítanie dokladov cez AI nie je nastavené — správca ho zapne v časti Prístup pre AI (kľúč Claude API)")
	// ErrMedia — nepodporovaný typ súboru.
	ErrMedia = errors.New("doklad musí byť PDF alebo obrázok (PNG, JPEG, WebP)")
)

// Settings je nastavenie Claude API (spoločné pre program).
type Settings struct {
	APIKey string `json:"apiKey,omitempty"`
	KeySet bool   `json:"keySet"`
	Model  string `json:"model"`
}

// Rate je riadok rekapitulácie DPH z dokladu.
type Rate struct {
	Rate float64 `json:"vatRate"`
	Base string  `json:"base"`
	VAT  string  `json:"vat"`
}

// Result sú údaje prečítané z dokladu (texty tak, ako ich vrátil model).
type Result struct {
	SupplierName   string `json:"supplierName"`
	Street         string `json:"street"`
	City           string `json:"city"`
	Zip            string `json:"zip"`
	Country        string `json:"country"`
	ICO            string `json:"ico"`
	DIC            string `json:"dic"`
	ICDPH          string `json:"icdph"`
	IBAN           string `json:"iban"`
	InvoiceNumber  string `json:"invoiceNumber"`
	VariableSymbol string `json:"variableSymbol"`
	IssueDate      string `json:"issueDate"`
	DeliveryDate   string `json:"deliveryDate"`
	DueDate        string `json:"dueDate"`
	Currency       string `json:"currency"`
	Description    string `json:"description"`
	ReverseCharge  bool   `json:"reverseCharge"`
	Rates          []Rate `json:"rates"`
	Total          string `json:"total"`
}

// schema je vstup nástroja, ktorým model vráti údaje.
var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"supplierName":   map[string]any{"type": "string", "description": "obchodné meno dodávateľa (vystaviteľa faktúry), nie odberateľa"},
		"street":         map[string]any{"type": "string"},
		"city":           map[string]any{"type": "string"},
		"zip":            map[string]any{"type": "string"},
		"country":        map[string]any{"type": "string", "description": "ISO kód krajiny dodávateľa, napr. SK"},
		"ico":            map[string]any{"type": "string", "description": "IČO dodávateľa"},
		"dic":            map[string]any{"type": "string", "description": "DIČ dodávateľa"},
		"icdph":          map[string]any{"type": "string", "description": "IČ DPH dodávateľa, napr. SK2020123456"},
		"iban":           map[string]any{"type": "string", "description": "IBAN dodávateľa na úhradu"},
		"invoiceNumber":  map[string]any{"type": "string", "description": "číslo faktúry dodávateľa"},
		"variableSymbol": map[string]any{"type": "string"},
		"issueDate":      map[string]any{"type": "string", "description": "dátum vyhotovenia YYYY-MM-DD"},
		"deliveryDate":   map[string]any{"type": "string", "description": "dátum dodania (zdaniteľného plnenia) YYYY-MM-DD"},
		"dueDate":        map[string]any{"type": "string", "description": "dátum splatnosti YYYY-MM-DD"},
		"currency":       map[string]any{"type": "string", "description": "mena dokladu, napr. EUR"},
		"description":    map[string]any{"type": "string", "description": "stručný predmet plnenia (do 80 znakov)"},
		"reverseCharge":  map[string]any{"type": "boolean", "description": "prenesenie daňovej povinnosti (daň platí odberateľ)"},
		"rates": map[string]any{
			"type":        "array",
			"description": "rekapitulácia DPH podľa sadzieb z dokladu; sumy ako desatinné čísla s bodkou",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"vatRate": map[string]any{"type": "number", "description": "sadzba v percentách, napr. 23"},
					"base":    map[string]any{"type": "string", "description": "základ dane"},
					"vat":     map[string]any{"type": "string", "description": "DPH"},
				},
				"required": []string{"vatRate", "base", "vat"},
			},
		},
		"total": map[string]any{"type": "string", "description": "celková suma na úhradu"},
	},
	"required": []string{"supplierName", "rates", "total"},
}

const prompt = `Toto je prijatá faktúra (doklad od dodávateľa) pre slovenské účtovníctvo. ` +
	`Prečítaj z nej údaje a zapíš ich nástrojom zapis_fakturu. Dodávateľ je ten, kto faktúru vystavil. ` +
	`Dátumy píš ako YYYY-MM-DD, sumy ako čísla s desatinnou bodkou bez medzier a meny. ` +
	`Rekapituláciu DPH opíš presne z dokladu (základ a DPH pre každú sadzbu); ak dodávateľ nie je platiteľ DPH, ` +
	`uveď jeden riadok so sadzbou 0 a základom rovným celkovej sume. Čo na doklade nie je, nechaj prázdne.`

// Client volá Claude API.
type Client struct {
	Settings Settings
	BaseURL  string       // "" = DefaultBaseURL (v testoch falošný server)
	HTTP     *http.Client // nil = s časovým limitom 90 s
}

// MediaType rozpozná typ súboru podľa obsahu.
func MediaType(data []byte) (string, error) {
	switch ct := http.DetectContentType(data); {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return "application/pdf", nil
	case ct == "image/png", ct == "image/jpeg", ct == "image/webp":
		return ct, nil
	}
	return "", ErrMedia
}

// Extract pošle doklad modelu a vráti prečítané údaje.
func (c Client) Extract(ctx context.Context, data []byte) (Result, error) {
	if strings.TrimSpace(c.Settings.APIKey) == "" {
		return Result{}, ErrOff
	}
	if len(data) > MaxBytes {
		return Result{}, fmt.Errorf("súbor je príliš veľký (najviac %d MB)", MaxBytes>>20)
	}
	mt, err := MediaType(data)
	if err != nil {
		return Result{}, err
	}
	block := map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": mt, "data": base64.StdEncoding.EncodeToString(data)}}
	if mt == "application/pdf" {
		block["type"] = "document"
	}
	model := c.Settings.Model
	if model == "" {
		model = DefaultModel
	}
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 2048,
		"tools": []map[string]any{{
			"name": "zapis_fakturu", "description": "Zapíše údaje prijatej faktúry do účtovníctva.", "input_schema": schema,
		}},
		"tool_choice": map[string]any{"type": "tool", "name": "zapis_fakturu"},
		"messages":    []map[string]any{{"role": "user", "content": []any{block, map[string]any{"type": "text", "text": prompt}}}},
	})
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.Settings.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("Claude API neodpovedá: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var out struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &out)
	if res.StatusCode != http.StatusOK {
		switch res.StatusCode {
		case http.StatusUnauthorized:
			return Result{}, errors.New("Claude API odmietlo kľúč — skontrolujte ho v časti Prístup pre AI")
		case http.StatusTooManyRequests, 529:
			return Result{}, errors.New("Claude API je preťažené alebo je prekročený limit — skúste o chvíľu")
		}
		msg := out.Error.Message
		if msg == "" {
			msg = res.Status
		}
		return Result{}, fmt.Errorf("Claude API: %s", msg)
	}
	for _, b := range out.Content {
		if b.Type == "tool_use" && b.Name == "zapis_fakturu" {
			var r Result
			if err := json.Unmarshal(b.Input, &r); err != nil {
				return Result{}, fmt.Errorf("Claude API vrátilo nečitateľné údaje: %w", err)
			}
			return r, nil
		}
	}
	return Result{}, errors.New("z dokladu sa nepodarilo prečítať údaje")
}

// num prečíta sumu od modelu („1234.50“, „1 234,50“).
func num(s string) (money.Dec, bool) {
	s = strings.NewReplacer(" ", "", " ", "", "€", "", "EUR", "").Replace(strings.TrimSpace(s))
	if c, d := strings.LastIndex(s, ","), strings.LastIndex(s, "."); c > d {
		s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	} else if c >= 0 {
		s = strings.ReplaceAll(s, ",", "")
	}
	if s == "" {
		return money.Zero, false
	}
	v, err := money.Parse(s)
	return v.RoundCents(), err == nil
}

func date(s string) invoice.Date {
	d, err := invoice.ParseDate(strings.TrimSpace(s))
	if err != nil {
		return invoice.Date{}
	}
	return d
}

// Purchase prevedie prečítané údaje na koncept prijatej faktúry a vráti
// upozornenia (čo treba skontrolovať).
func (r Result) Purchase(today invoice.Date) (purchase.Purchase, []string) {
	var warn []string
	country := strings.ToUpper(strings.TrimSpace(r.Country))
	if country == "" {
		country = "SK"
	}
	p := purchase.Purchase{
		SupplierNumber: strings.TrimSpace(r.InvoiceNumber),
		VariableSymbol: strings.Join(strings.Fields(r.VariableSymbol), ""),
		Supplier: invoice.Party{
			Name: strings.TrimSpace(r.SupplierName), Street: strings.TrimSpace(r.Street), City: strings.TrimSpace(r.City),
			Zip: strings.TrimSpace(r.Zip), Country: country, ICO: strings.Join(strings.Fields(r.ICO), ""),
			DIC: strings.Join(strings.Fields(r.DIC), ""), ICDPH: strings.ToUpper(strings.Join(strings.Fields(r.ICDPH), "")),
		},
		SupplierIBAN:  strings.ToUpper(strings.Join(strings.Fields(r.IBAN), "")),
		IssueDate:     date(r.IssueDate),
		DeliveryDate:  date(r.DeliveryDate),
		DueDate:       date(r.DueDate),
		ReceivedDate:  today,
		PaymentMethod: invoice.PayTransfer,
		ReverseCharge: r.ReverseCharge,
		Description:   strings.TrimSpace(r.Description),
	}
	if p.VariableSymbol == "" {
		// VS býva číslo faktúry bez písmen
		if d := strings.Map(func(c rune) rune {
			if c >= '0' && c <= '9' {
				return c
			}
			return -1
		}, p.SupplierNumber); len(d) > 0 && len(d) <= 10 {
			p.VariableSymbol = d
		}
	}
	if p.DeliveryDate.IsZero() {
		p.DeliveryDate = p.IssueDate
	}
	if p.IssueDate.IsZero() {
		warn = append(warn, "Dátum vyhotovenia sa nenašiel")
	}
	if cur := strings.ToUpper(strings.TrimSpace(r.Currency)); cur != "" && cur != "EUR" {
		if fx.Known(cur) {
			p.Currency = cur // kurz ECB doplní program
		} else {
			warn = append(warn, "Doklad je v mene "+cur+", pre ktorú nie je kurz ECB — sumy prepočítajte na eurá")
		}
	}
	for _, x := range r.Rates {
		base, ok1 := num(x.Base)
		tax, ok2 := num(x.VAT)
		rate := vat.Rate(int(x.Rate + 0.5))
		if !ok1 || (!ok2 && x.VAT != "") {
			warn = append(warn, fmt.Sprintf("Nečitateľná suma pri sadzbe %v %%", x.Rate))
			continue
		}
		if !rate.Known() {
			warn = append(warn, fmt.Sprintf("Sadzba %v %% nie je platná sadzba DPH — skontrolujte", x.Rate))
		}
		p.Rates = append(p.Rates, purchase.RateRow{Rate: rate, Base: base, VAT: tax})
	}
	if len(p.Rates) == 0 {
		warn = append(warn, "Rekapitulácia DPH sa nenašla — doplňte základ a DPH")
	}
	for _, w := range p.Normalize() {
		warn = append(warn, w.Message)
	}
	if total, ok := num(r.Total); ok && !total.Equal(p.Summary.Total) && len(p.Rates) > 0 {
		warn = append(warn, fmt.Sprintf("Súčet riadkov (%s €) nesedí s celkovou sumou na doklade (%s €)", p.Summary.Total.Display(), total.Display()))
	}
	return p, warn
}
