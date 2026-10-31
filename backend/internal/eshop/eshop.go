// Package eshop napája Fiškus na e-shopy cez ich verejné API: načíta
// objednávky (z nich vzniknú faktúry) a pošle do obchodu stav zásob.
//
// Konektory sú napísané podľa verejnej dokumentácie platforiem a neboli
// overené na živom obchode — preto ich rozhranie označuje ako neoverené.
package eshop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Platform je druh e-shopu.
type Platform string

// Podporované platformy.
const (
	WooCommerce Platform = "woocommerce"
	Shopify     Platform = "shopify"
	PrestaShop  Platform = "prestashop"
	Magento     Platform = "magento"
	Drupal      Platform = "drupal"
	Shoptet     Platform = "shoptet"
)

// Info opisuje platformu pre rozhranie: čo treba zadať a kde to v obchode nájsť.
type Info struct {
	Platform  Platform `json:"platform"`
	Name      string   `json:"name"`
	URLHint   string   `json:"urlHint"`
	KeyLabel  string   `json:"keyLabel"`    // "" = platforma kľúč nepotrebuje
	SecretLbl string   `json:"secretLabel"` // názov tajného údaja
	Help      string   `json:"help"`        // kde údaje v obchode vytvoriť
	Docs      string   `json:"docs"`        // odkaz na dokumentáciu API
	Stock     bool     `json:"stock"`       // vie prijať stav zásob
	Catalog   bool     `json:"catalog"`     // vie riadiť cenník a zákazníkov z Fiškusu
	Verified  bool     `json:"verified"`    // overené na živom obchode
	Note      string   `json:"note,omitempty"`
}

// Platforms sú opisy podporovaných platforiem v poradí pre výber.
var Platforms = []Info{
	{Platform: WooCommerce, Name: "WooCommerce", URLHint: "https://mojobchod.sk", KeyLabel: "Consumer key", SecretLbl: "Consumer secret",
		Help: "WooCommerce → Nastavenia → Pokročilé → REST API → Pridať kľúč s oprávnením Čítanie/Zápis.",
		Docs: "https://woocommerce.github.io/woocommerce-rest-api-docs/", Stock: true, Catalog: true},
	{Platform: Shopify, Name: "Shopify", URLHint: "mojobchod.myshopify.com", SecretLbl: "Admin API access token",
		Help: "Shopify admin → Settings → Apps → Develop apps → vlastná aplikácia s právami read_orders, read_products, write_inventory, read_locations.",
		Docs: "https://shopify.dev/docs/api/admin-graphql", Stock: true},
	{Platform: Shoptet, Name: "Shoptet", URLHint: "https://mojobchod.sk", SecretLbl: "Súkromný API token",
		Help: "Shoptet admin → Prepojenia → Súkromné API (balík Premium): vytvorte token s právami na objednávky (čítanie), e-shop a sklady (zápis).",
		Docs: "https://api.docs.shoptet.com/shoptet-api/openapi", Stock: true},
	{Platform: PrestaShop, Name: "PrestaShop", URLHint: "https://mojobchod.sk", SecretLbl: "Kľúč webservice",
		Help: "PrestaShop → Rozšírené parametre → Webservice: zapnite webservice a pridajte kľúč s právami GET na orders, order_details, order_states, addresses, customers, countries, currencies, products a GET+PUT na stock_availables.",
		Docs: "https://devdocs.prestashop-project.org/8/webservice/", Stock: true},
	{Platform: Magento, Name: "Magento / Adobe Commerce", URLHint: "https://mojobchod.sk", SecretLbl: "Access token integrácie",
		Help: "Magento admin → System → Integrations → Add New Integration s prístupom k Sales (Orders) a Catalog (Inventory) → Activate; skopírujte Access Token. V nastaveniach musí byť povolené použitie tokenu integrácie ako bearer tokenu.",
		Docs: "https://developer.adobe.com/commerce/webapi/rest/", Stock: true},
	{Platform: Drupal, Name: "Drupal Commerce", URLHint: "https://mojobchod.sk", KeyLabel: "Používateľ", SecretLbl: "Heslo",
		Help: "Drupal: zapnite moduly JSON:API a HTTP Basic Authentication a vytvorte používateľa iba s právom čítať objednávky a profily (view commerce_order, view commerce_product). Stav zásob Drupal Commerce bez doplnku nevedie, preto sa zásoby neposielajú.",
		Docs: "https://docs.drupalcommerce.org/", Stock: false},
}

// InfoOf vráti opis platformy.
func InfoOf(p Platform) (Info, bool) {
	for _, i := range Platforms {
		if i.Platform == p {
			return i, true
		}
	}
	return Info{}, false
}

// Address je adresa a kontakt z objednávky.
type Address struct {
	Name    string `json:"name"`
	Company string `json:"company"`
	Street  string `json:"street"`
	City    string `json:"city"`
	Zip     string `json:"zip"`
	Country string `json:"country"` // ISO kód
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	ICO     string `json:"ico"`
	DIC     string `json:"dic"`
	ICDPH   string `json:"icdph"`
}

// IsZero: adresa bez ulice a mesta.
func (a Address) IsZero() bool {
	return strings.TrimSpace(a.Street) == "" && strings.TrimSpace(a.City) == ""
}

// Line je riadok objednávky (tovar, doprava, poplatok). Sumy sú po zľave,
// Net bez DPH a Tax samotná daň.
type Line struct {
	SKU      string    `json:"sku"`
	Name     string    `json:"name"`
	Quantity money.Dec `json:"quantity"`
	Net      money.Dec `json:"net"`
	Tax      money.Dec `json:"tax"`
	Shipping bool      `json:"shipping"`
}

// Order je objednávka z e-shopu.
type Order struct {
	ID        string    `json:"id"`     // interné ID v obchode
	Number    string    `json:"number"` // číslo, ktoré vidí zákazník
	Created   time.Time `json:"created"`
	Status    string    `json:"status"`
	Paid      bool      `json:"paid"`
	PaidAt    time.Time `json:"paidAt"`
	Cancelled bool      `json:"cancelled"`
	Currency  string    `json:"currency"`
	Billing   Address   `json:"billing"`
	Shipping  Address   `json:"shipping"`
	Lines     []Line    `json:"lines"`
	Total     money.Dec `json:"total"`   // s DPH
	Payment   string    `json:"payment"` // transfer, card, cod, cash alebo "" (neznáma)
	Note      string    `json:"note"`
}

// Conn sú údaje na pripojenie k obchodu.
type Conn struct {
	Platform Platform
	URL      string
	Key      string
	Secret   string
	HTTP     *http.Client
}

// Connector je pripojený obchod.
type Connector interface {
	// Check overí spojenie a vráti názov obchodu.
	Check(ctx context.Context) (string, error)
	// Orders vráti objednávky vytvorené od since (najstaršie prvé).
	Orders(ctx context.Context, since time.Time) ([]Order, error)
	// SetStock nastaví dostupné množstvo produktu s kódom sku.
	SetStock(ctx context.Context, sku string, qty int64) error
}

// Chyby konektorov.
var (
	ErrNoSKU    = errors.New("produkt s týmto kódom v obchode nie je")
	ErrPlatform = errors.New("neznámy druh e-shopu")
	// ErrNoStock — platforma stav zásob cez API neprijíma.
	ErrNoStock = errors.New("tento druh e-shopu neprijíma stav zásob")
	ErrAuth    = errors.New("obchod odmietol prihlasovacie údaje — skontrolujte kľúč a jeho oprávnenia")
)

// paymentOf odhadne spôsob úhrady z názvu platobnej metódy obchodu.
func paymentOf(method string) string {
	m := strings.ToLower(strings.TrimSpace(method))
	switch {
	case m == "":
		return ""
	case m == "cod" || strings.Contains(m, "dobierk") || strings.Contains(m, "cash on delivery"):
		return "cod"
	case m == "bacs" || m == "cheque" || m == "manual" || strings.Contains(m, "bank") || strings.Contains(m, "prevod"):
		return "transfer"
	case strings.Contains(m, "hotovos") || m == "cash":
		return "cash"
	}
	return "card"
}

// MaxOrders je najviac objednávok načítaných naraz.
const MaxOrders = 2000

// New vytvorí konektor.
func New(c Conn) (Connector, error) {
	base, err := NormalizeURL(c.Platform, c.URL)
	if err != nil {
		return nil, err
	}
	c.URL = base
	if c.HTTP == nil {
		c.HTTP = Client(false)
	}
	switch c.Platform {
	case WooCommerce:
		return &woo{c: c}, nil
	case Shopify:
		return &shopify{c}, nil
	case PrestaShop:
		return &presta{c}, nil
	case Magento:
		return &magento{c}, nil
	case Drupal:
		return &drupal{c}, nil
	case Shoptet:
		return &shoptet{c}, nil
	}
	return nil, ErrPlatform
}

// NormalizeURL skontroluje adresu obchodu (iba https) a odstráni koncové lomky.
// Pri Shopify stačí názov obchodu („mojobchod“ → mojobchod.myshopify.com).
func NormalizeURL(p Platform, raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if p == Shopify && s != "" && !strings.Contains(s, ".") && !strings.Contains(s, "/") {
		s += ".myshopify.com"
	}
	if s != "" && !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("zadajte adresu obchodu, napr. %s", hintOf(p))
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLocalHost(u.Hostname())) {
		return "", errors.New("adresa obchodu musí začínať https:// (prihlasovacie údaje sa nesmú posielať nešifrovane)")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("adresa obchodu nesmie obsahovať meno, heslo ani parametre")
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.EscapedPath(), "/"), nil
}

func hintOf(p Platform) string {
	if i, ok := InfoOf(p); ok {
		return i.URLHint
	}
	return "https://mojobchod.sk"
}

func isLocalHost(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

// Client vráti HTTP klienta pre volania obchodu. Bez allowPrivate odmietne
// spojenie na adresy vnútornej siete (server nesmie slúžiť ako most do nej).
func Client(allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: 15 * time.Second}
	if !allowPrivate {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || blockedIP(ip) {
				return fmt.Errorf("adresa %s je vo vnútornej sieti — e-shop musí byť dostupný z internetu", host)
			}
			return nil
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = d.DialContext
	if !allowPrivate {
		tr.Proxy = nil // cez proxy by kontrola adresy neplatila
	}
	return &http.Client{Timeout: 60 * time.Second, Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("priveľa presmerovaní")
			}
			if req.URL.Scheme != "https" && !isLocalHost(req.URL.Hostname()) {
				return errors.New("obchod presmeroval na nešifrovanú adresu")
			}
			return nil
		}}
}

func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast()
}

// call pošle požiadavku s telom JSON a odpoveď (JSON) dekóduje do out.
func call(ctx context.Context, hc *http.Client, method, u string, body any, out any, auth func(*http.Request)) (http.Header, error) {
	var raw []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return send(ctx, hc, method, u, "application/json", raw, out, auth)
}

// send pošle požiadavku s ľubovoľným telom (nil = bez tela).
func send(ctx context.Context, hc *http.Client, method, u, contentType string, body []byte, out any, auth func(*http.Request)) (http.Header, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", "Fiskus")
	auth(req)
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("obchod nedostupný: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return nil, ErrAuth
	case res.StatusCode == http.StatusNotFound:
		return nil, &notFound{req.URL.Path}
	case res.StatusCode >= 300:
		return nil, fmt.Errorf("obchod vrátil chybu %d: %s", res.StatusCode, snippet(data))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return nil, fmt.Errorf("obchod neodpovedal očakávaným spôsobom (%s): %s", req.URL.Path, snippet(data))
		}
	}
	return res.Header, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if r := []rune(s); len(r) > 160 {
		s = string(r[:160]) + "…"
	}
	return s
}

// num je suma alebo množstvo z JSON — číslo aj text ("12.30"), prázdne = 0.
type num money.Dec

// UnmarshalJSON prijme číslo, text aj null.
func (n *num) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*n = num(money.Zero)
		return nil
	}
	d, err := money.Parse(s)
	if err != nil {
		return fmt.Errorf("neplatná suma %q", s)
	}
	*n = num(d)
	return nil
}

func (n num) dec() money.Dec { return money.Dec(n) }

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// jsonRaw odloží časť odpovede na neskoršie dekódovanie.
type jsonRaw []byte

func (r *jsonRaw) UnmarshalJSON(b []byte) error {
	*r = append((*r)[:0], b...)
	return nil
}

func (r jsonRaw) decode(out any) error {
	if out == nil || len(r) == 0 || string(r) == "null" {
		return nil
	}
	return json.Unmarshal(r, out)
}

// errNotFound — obchod odpovedal 404 (errors.Is na *notFound).
var errNotFound = errors.New("nenájdené")

type notFound struct{ path string }

func (e *notFound) Error() string {
	return "obchod odpovedal „nenájdené“ (" + e.path + ") — skontrolujte adresu obchodu"
}
func (e *notFound) Is(t error) bool { return t == errNotFound }
