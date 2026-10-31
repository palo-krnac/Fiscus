// Package ekasa načíta pokladničný doklad (bloček) z verejnej služby eKasa
// Finančnej správy SR podľa jeho identifikátora z QR kódu.
//
// Služba „Overenie dokladu“ (https://ekasa.financnasprava.sk) nemá
// zverejnenú formálnu dokumentáciu; formát odpovede je podľa skutočných
// odpovedí služby. Konektor je preto označený ako neoverený.
package ekasa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// DefaultURL je adresa služby vyhľadania dokladu.
const DefaultURL = "https://ekasa.financnasprava.sk/mdu/api/v1/opd/receipt/find"

// Chyby.
var (
	ErrNotReceipt = errors.New("QR kód nie je z bločku eKasa")
	ErrOffline    = errors.New("bloček bol vystavený v režime offline — Finančná správa ho ešte nemusí poznať, zadajte ho ručne alebo to skúste neskôr")
	ErrNotFound   = errors.New("Finančná správa bloček s týmto kódom nepozná")
)

// receiptID je identifikátor online dokladu: O- a 32 hexadecimálnych znakov.
var receiptID = regexp.MustCompile(`(?i)\bO-[0-9A-F]{32}\b`)

// ParseQR vráti identifikátor dokladu z textu QR kódu.
func ParseQR(text string) (string, error) {
	t := strings.TrimSpace(text)
	if m := receiptID.FindString(t); m != "" {
		return "O-" + strings.ToUpper(m[2:]), nil
	}
	if strings.HasPrefix(strings.ToUpper(t), "OKP:") || regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{8}-`).MatchString(t) {
		return "", ErrOffline
	}
	return "", ErrNotReceipt
}

// Seller je predávajúci z dokladu.
type Seller struct {
	Name   string `json:"name"`
	ICO    string `json:"ico"`
	DIC    string `json:"dic"`
	ICDPH  string `json:"icDph"`
	Street string `json:"street"`
	City   string `json:"city"`
	Zip    string `json:"zip"`
}

// Item je položka dokladu (cena s DPH za celý riadok).
type Item struct {
	Name     string    `json:"name"`
	Quantity money.Dec `json:"quantity"`
	Price    money.Dec `json:"price"`
	VATRate  int       `json:"vatRate"`
}

// VATRow je základ a daň pre jednu sadzbu.
type VATRow struct {
	Rate int       `json:"rate"`
	Base money.Dec `json:"base"`
	VAT  money.Dec `json:"vat"`
}

// Receipt je pokladničný doklad.
type Receipt struct {
	ID       string    `json:"id"`
	Number   string    `json:"number"`
	Issued   time.Time `json:"issued"`
	Seller   Seller    `json:"seller"`
	Items    []Item    `json:"items"`
	Total    money.Dec `json:"total"`
	VAT      []VATRow  `json:"vat"`
	Customer string    `json:"customer"` // IČO/IČ DPH kupujúceho, ak ho pokladník zadal
}

// Client volá službu eKasa.
type Client struct {
	URL  string
	HTTP *http.Client
}

// num prijme číslo alebo text (aj null).
type num struct{ money.Dec }

func (n *num) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		n.Dec = money.Zero
		return nil
	}
	d, err := money.Parse(s)
	if err != nil {
		return fmt.Errorf("neplatná suma %q", s)
	}
	n.Dec = d
	return nil
}

type rawReceipt struct {
	ReceiptID        string `json:"receiptId"`
	ReceiptNumber    any    `json:"receiptNumber"`
	IssueDate        string `json:"issueDate"`
	CreateDate       string `json:"createDate"`
	CustomerID       string `json:"customerId"`
	TotalPrice       num    `json:"totalPrice"`
	TaxBaseBasic     num    `json:"taxBaseBasic"`
	TaxBaseReduced   num    `json:"taxBaseReduced"`
	VATAmountBasic   num    `json:"vatAmountBasic"`
	VATAmountReduced num    `json:"vatAmountReduced"`
	VATRateBasic     num    `json:"vatRateBasic"`
	VATRateReduced   num    `json:"vatRateReduced"`
	FreeTaxAmount    num    `json:"freeTaxAmount"`
	Items            []struct {
		Name     string `json:"name"`
		ItemType string `json:"itemType"`
		Quantity num    `json:"quantity"`
		VATRate  num    `json:"vatRate"`
		Price    num    `json:"price"`
	} `json:"items"`
	Organization struct {
		Name                   string `json:"name"`
		ICO                    string `json:"ico"`
		DIC                    string `json:"dic"`
		ICDPH                  string `json:"icDph"`
		StreetName             string `json:"streetName"`
		BuildingNumber         string `json:"buildingNumber"`
		PropertyRegistrationNo string `json:"propertyRegistrationNumber"`
		Municipality           string `json:"municipality"`
		PostalCode             string `json:"postalCode"`
	} `json:"organization"`
	Unit *struct {
		StreetName     string `json:"streetName"`
		BuildingNumber string `json:"buildingNumber"`
		Municipality   string `json:"municipality"`
		PostalCode     string `json:"postalCode"`
	} `json:"unit"`
}

// Find načíta doklad podľa identifikátora (O-…).
func (c Client) Find(ctx context.Context, id string) (Receipt, error) {
	u := c.URL
	if u == "" {
		u = DefaultURL
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	body, _ := json.Marshal(map[string]string{"receiptId": id})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return Receipt{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := hc.Do(req)
	if err != nil {
		return Receipt{}, fmt.Errorf("služba eKasa nie je dostupná: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return Receipt{}, err
	}
	if res.StatusCode == http.StatusNotFound {
		return Receipt{}, ErrNotFound
	}
	if res.StatusCode >= 300 {
		return Receipt{}, fmt.Errorf("služba eKasa vrátila chybu %d", res.StatusCode)
	}
	var out struct {
		ReturnValue int         `json:"returnValue"`
		Receipt     *rawReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Receipt{}, fmt.Errorf("služba eKasa neodpovedala očakávaným spôsobom: %w", err)
	}
	if out.Receipt == nil || out.ReturnValue != 0 {
		return Receipt{}, ErrNotFound
	}
	return out.Receipt.receipt(), nil
}

func pct(n num) int { return int(n.RoundTo(0).Raw() / money.FromInt(1).Raw()) }

func (r rawReceipt) receipt() Receipt {
	o := r.Organization
	street := strings.TrimSpace(o.StreetName + " " + firstOf(o.BuildingNumber, o.PropertyRegistrationNo))
	out := Receipt{ID: r.ReceiptID, Number: strings.TrimSpace(fmt.Sprint(r.ReceiptNumber)), Total: r.TotalPrice.Dec, Customer: strings.TrimSpace(r.CustomerID),
		Seller: Seller{Name: strings.TrimSpace(o.Name), ICO: strings.TrimSpace(o.ICO), DIC: strings.TrimSpace(o.DIC), ICDPH: strings.TrimSpace(o.ICDPH),
			Street: street, City: strings.TrimSpace(o.Municipality), Zip: strings.ReplaceAll(strings.TrimSpace(o.PostalCode), " ", "")}}
	if out.Number == "<nil>" {
		out.Number = ""
	}
	loc, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		loc = time.UTC
	}
	for _, s := range []string{r.IssueDate, r.CreateDate} {
		if t, err := time.ParseInLocation("02.01.2006 15:04:05", s, loc); err == nil {
			out.Issued = t
			break
		}
	}
	for _, it := range r.Items {
		out.Items = append(out.Items, Item{Name: strings.TrimSpace(it.Name), Quantity: it.Quantity.Dec, Price: it.Price.Dec, VATRate: pct(it.VATRate)})
	}
	// rekapitulácia DPH z hlavičky dokladu; ak chýba, z položiek
	add := func(rate num, base, vat num) {
		if !base.IsZero() || !vat.IsZero() {
			out.VAT = append(out.VAT, VATRow{Rate: pct(rate), Base: base.Dec, VAT: vat.Dec})
		}
	}
	add(r.VATRateBasic, r.TaxBaseBasic, r.VATAmountBasic)
	add(r.VATRateReduced, r.TaxBaseReduced, r.VATAmountReduced)
	if !r.FreeTaxAmount.IsZero() {
		out.VAT = append(out.VAT, VATRow{Rate: 0, Base: r.FreeTaxAmount.Dec})
	}
	if covered := sumVAT(out.VAT); !covered.Equal(out.Total) && len(out.Items) > 0 {
		out.VAT = fromItems(out.Items)
	}
	return out
}

func sumVAT(rows []VATRow) money.Dec {
	s := money.Zero
	for _, r := range rows {
		s = s.Add(r.Base).Add(r.VAT)
	}
	return s
}

// fromItems zostaví rekapituláciu DPH z položiek (ceny s DPH).
func fromItems(items []Item) []VATRow {
	gross := map[int]money.Dec{}
	var order []int
	for _, it := range items {
		if _, ok := gross[it.VATRate]; !ok {
			order = append(order, it.VATRate)
		}
		gross[it.VATRate] = gross[it.VATRate].Add(it.Price)
	}
	var out []VATRow
	for _, r := range order {
		g := gross[r]
		base := g.MulRatio(money.FromInt(100), money.FromInt(int64(100+r))).RoundCents()
		out = append(out, VATRow{Rate: r, Base: base, VAT: g.Sub(base)})
	}
	return out
}

func firstOf(v ...string) string {
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}
