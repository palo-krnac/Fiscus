package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/eshop"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Napojenie na e-shopy: z objednávok vzniknú koncepty faktúr (na kontrolu
// a vystavenie), do obchodu sa pošle stav zásob zo skladu. Prihlasovacie
// údaje obchodu sa nikdy nevracajú do rozhrania ani do auditu.

// MaxEshops je najviac napojených obchodov vo firme.
const MaxEshops = 10

// eshopMemory je počet zapamätaných prenesených objednávok na obchod.
const eshopMemory = 5000

// EshopConn je napojený obchod, ako ho vidí rozhranie (bez tajných údajov).
type EshopConn struct {
	ID       string         `json:"id"`
	Platform eshop.Platform `json:"platform"`
	Name     string         `json:"name"`
	URL      string         `json:"url"`
	// Key a Secret sa iba zadávajú; prázdne pri uložení = ponechať uložené.
	Key       string `json:"key,omitempty"`
	Secret    string `json:"secret,omitempty"`
	HasKey    bool   `json:"hasKey"`
	HasSecret bool   `json:"hasSecret"`
	// OnlyPaid: faktúra vznikne až z uhradenej objednávky.
	OnlyPaid bool `json:"onlyPaid"`
	// Since: objednávky od tohto dňa (pri prvom načítaní).
	Since invoice.Date `json:"since"`
	// LastSync a LastResult opisujú posledné načítanie objednávok.
	LastSync   string `json:"lastSync"`
	LastResult string `json:"lastResult"`
	LastStock  string `json:"lastStock"`
	Imported   int    `json:"imported"`
}

// eshopImport je prenesená objednávka.
type eshopImport struct {
	Order   string `json:"o"`
	Invoice int64  `json:"i"`
}

// eshopStored je obchod v nastaveniach (aj s tajnými údajmi).
type eshopStored struct {
	EshopConn
	StoredKey    string        `json:"storedKey"`
	StoredSecret string        `json:"storedSecret"`
	Cursor       time.Time     `json:"cursor"` // odkiaľ načítať nabudúce
	Done         []eshopImport `json:"done"`
}

func (s eshopStored) view() EshopConn {
	v := s.EshopConn
	v.Key, v.Secret = "", ""
	v.HasKey, v.HasSecret = s.StoredKey != "", s.StoredSecret != ""
	v.Imported = len(s.Done)
	return v
}

// EshopSettings sú napojené obchody a podporované platformy.
type EshopSettings struct {
	Platforms []eshop.Info `json:"platforms"`
	Conns     []EshopConn  `json:"conns"`
}

func eshopKey(companyID int64) string { return fmt.Sprintf("eshops.%d", companyID) }

var eshopMu sync.Mutex

func (a *App) loadEshops(ctx context.Context, companyID int64) ([]eshopStored, error) {
	var out []eshopStored
	raw, err := a.Store.GetSetting(ctx, eshopKey(companyID))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return []eshopStored{}, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *App) saveEshops(ctx context.Context, companyID int64, list []eshopStored) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, eshopKey(companyID), string(b))
}

func eshopViews(list []eshopStored) EshopSettings {
	out := EshopSettings{Platforms: eshop.Platforms, Conns: []EshopConn{}}
	for _, s := range list {
		out.Conns = append(out.Conns, s.view())
	}
	return out
}

// Eshops vráti napojené obchody.
func (a *App) Eshops(ctx context.Context, companyID int64) (EshopSettings, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return EshopSettings{}, err
	}
	list, err := a.loadEshops(ctx, companyID)
	if err != nil {
		return EshopSettings{}, err
	}
	return eshopViews(list), nil
}

func newEshopID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SaveEshops uloží zoznam obchodov. Prázdny kľúč alebo tajný údaj ponechá
// uložený; obchod, ktorý v zozname chýba, sa odpojí.
func (a *App) SaveEshops(ctx context.Context, companyID int64, in []EshopConn) (EshopSettings, error) {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return EshopSettings{}, err
	}
	if len(in) > MaxEshops {
		return EshopSettings{}, mfaFieldErr("conns", fmt.Sprintf("Napojiť sa dá najviac %d obchodov", MaxEshops))
	}
	eshopMu.Lock()
	defer eshopMu.Unlock()
	old, err := a.loadEshops(ctx, companyID)
	if err != nil {
		return EshopSettings{}, err
	}
	byID := map[string]eshopStored{}
	for _, s := range old {
		byID[s.ID] = s
	}
	var fields []invoice.FieldError
	bad := func(i int, f, msg string) {
		fields = append(fields, invoice.FieldError{Field: fmt.Sprintf("conns[%d].%s", i, f), Message: msg})
	}
	out := make([]eshopStored, 0, len(in))
	for i, c := range in {
		info, ok := eshop.InfoOf(c.Platform)
		if !ok {
			bad(i, "platform", "Vyberte druh e-shopu")
			continue
		}
		s, exists := byID[c.ID]
		if !exists || s.Platform != c.Platform {
			s = eshopStored{Done: []eshopImport{}}
			s.ID = newEshopID()
		}
		u, err := eshop.NormalizeURL(c.Platform, c.URL)
		if err != nil {
			bad(i, "url", strings.ToUpper(err.Error()[:1])+err.Error()[1:])
		}
		s.Platform, s.URL = c.Platform, u
		s.Name = strings.TrimSpace(c.Name)
		if s.Name == "" {
			s.Name = info.Name
		}
		if k := strings.TrimSpace(c.Key); k != "" {
			s.StoredKey = k
		}
		if v := strings.TrimSpace(c.Secret); v != "" {
			s.StoredSecret = v
		}
		if info.KeyLabel != "" && s.StoredKey == "" {
			bad(i, "key", "Zadajte "+info.KeyLabel)
		}
		if s.StoredSecret == "" {
			bad(i, "secret", "Zadajte "+info.SecretLbl)
		}
		s.OnlyPaid = c.OnlyPaid
		if c.Since != s.Since {
			s.Since, s.Cursor = c.Since, time.Time{} // nový začiatok: načítať odznova (prenesené sa nezopakujú)
		}
		if s.Since.IsZero() {
			s.Since = a.Today().AddDays(-30)
		}
		out = append(out, s)
	}
	if len(fields) > 0 {
		return EshopSettings{}, &invoice.ValidationError{Fields: fields}
	}
	if err := a.saveEshops(ctx, companyID, out); err != nil {
		return EshopSettings{}, err
	}
	names := make([]string, 0, len(out))
	for _, s := range out {
		names = append(names, s.Name)
	}
	a.record(ctx, companyID, audit.CatSettings, "update", "eshops", 0, "Napojenie e-shopov: "+firstNonBlank(strings.Join(names, ", "), "žiadne"), nil, nil)
	return eshopViews(out), nil
}

// EshopError je chyba spojenia s obchodom (zobrazí sa používateľovi).
type EshopError struct{ Err error }

func (e *EshopError) Error() string { return e.Err.Error() }
func (e *EshopError) Unwrap() error { return e.Err }

func shopErr(err error) error {
	if err == nil {
		return nil
	}
	return &EshopError{err}
}

// eshopClient je HTTP klient pre obchody: na serveri nesmie siahnuť do vnútornej siete.
func (a *App) eshopClient() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return eshop.Client(a.Capabilities().Mode != edition.Server)
}

func (a *App) eshopConn(ctx context.Context, companyID int64, id string) ([]eshopStored, int, eshop.Connector, error) {
	list, err := a.loadEshops(ctx, companyID)
	if err != nil {
		return nil, 0, nil, err
	}
	for i, s := range list {
		if s.ID == id {
			c, err := eshop.New(eshop.Conn{Platform: s.Platform, URL: s.URL, Key: s.StoredKey, Secret: s.StoredSecret, HTTP: a.eshopClient()})
			return list, i, c, shopErr(err)
		}
	}
	return nil, 0, nil, store.ErrNotFound
}

// TestEshop overí spojenie s obchodom a vráti jeho názov.
func (a *App) TestEshop(ctx context.Context, companyID int64, id string) (string, error) {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return "", err
	}
	_, _, c, err := a.eshopConn(ctx, companyID, id)
	if err != nil {
		return "", err
	}
	name, err := c.Check(ctx)
	return name, shopErr(err)
}

// EshopOrderResult je výsledok jednej objednávky.
type EshopOrderResult struct {
	Order     string    `json:"order"`
	Customer  string    `json:"customer"`
	Total     money.Dec `json:"total"`
	InvoiceID int64     `json:"invoiceId,omitempty"`
	Status    string    `json:"status"` // created, waiting, skipped, error
	Message   string    `json:"message,omitempty"`
}

// EshopSyncReport je výsledok načítania objednávok.
type EshopSyncReport struct {
	Created  int                `json:"created"`
	Waiting  int                `json:"waiting"`
	Skipped  int                `json:"skipped"`
	Failed   int                `json:"failed"`
	Orders   []EshopOrderResult `json:"orders"`
	Warnings []string           `json:"warnings"`
}

// SyncEshopOrders načíta nové objednávky a vytvorí z nich koncepty faktúr.
// Prenesená objednávka sa už znova neprenesie; neuhradená pri voľbe
// „iba uhradené“ počká na ďalšie načítanie.
func (a *App) SyncEshopOrders(ctx context.Context, companyID int64, id string) (EshopSyncReport, error) {
	rep := EshopSyncReport{Orders: []EshopOrderResult{}, Warnings: []string{}}
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return rep, err
	}
	eshopMu.Lock()
	defer eshopMu.Unlock()
	list, i, conn, err := a.eshopConn(ctx, companyID, id)
	if err != nil {
		return rep, err
	}
	s := &list[i]
	since := s.Cursor
	if since.IsZero() {
		since = time.Date(s.Since.Year(), time.Month(s.Since.Month()), s.Since.Day(), 0, 0, 0, 0, bratislava())
	}
	orders, err := conn.Orders(ctx, since)
	if err != nil {
		return rep, shopErr(err)
	}
	sort.SliceStable(orders, func(x, y int) bool { return orders[x].Created.Before(orders[y].Created) })
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return rep, err
	}
	items, err := a.Store.ListItems(ctx, companyID)
	if err != nil {
		return rep, err
	}
	byCode := map[string]int{}
	for k, it := range items {
		if it.Code != "" && !it.Archived {
			byCode[strings.ToUpper(it.Code)] = k
		}
	}
	done := map[string]bool{}
	for _, d := range s.Done {
		done[d.Order] = true
	}
	// kurzor: najstaršia objednávka, ktorá ešte čaká, inak najnovšia
	var waitFrom, newest time.Time
	for _, o := range orders {
		if o.Created.After(newest) {
			newest = o.Created
		}
		if done[o.ID] {
			continue
		}
		r := EshopOrderResult{Order: o.Number, Customer: firstNonBlank(o.Billing.Company, o.Billing.Name), Total: o.Total}
		switch {
		case o.Cancelled:
			r.Status, r.Message = "skipped", "zrušená"
			rep.Skipped++
		case o.Currency != "" && o.Currency != "EUR":
			r.Status, r.Message = "skipped", "v mene "+o.Currency+" — zadajte ju ručne"
			rep.Skipped++
		case s.OnlyPaid && !o.Paid:
			r.Status = "waiting"
			rep.Waiting++
			if waitFrom.IsZero() || o.Created.Before(waitFrom) {
				waitFrom = o.Created
			}
		default:
			inv, warns, err := orderInvoice(o, co, items, byCode, a.Today())
			if err == nil {
				inv, err = a.SaveDraft(ctx, companyID, inv)
			}
			if err != nil {
				r.Status, r.Message = "error", errorText(err)
				rep.Failed++
				break
			}
			r.Status, r.InvoiceID = "created", inv.ID
			if len(warns) > 0 {
				r.Message = strings.Join(warns, "; ")
			}
			rep.Created++
			s.Done = append(s.Done, eshopImport{Order: o.ID, Invoice: inv.ID})
			done[o.ID] = true
		}
		if r.Status == "skipped" {
			s.Done = append(s.Done, eshopImport{Order: o.ID})
			done[o.ID] = true
		}
		rep.Orders = append(rep.Orders, r)
	}
	if n := len(s.Done); n > eshopMemory {
		s.Done = s.Done[n-eshopMemory:]
	}
	switch {
	case !waitFrom.IsZero():
		s.Cursor = waitFrom.Add(-time.Minute)
	case !newest.IsZero():
		s.Cursor = newest.Add(-24 * time.Hour) // prekrytie: neskoré zápisy v obchode
	}
	if len(orders) >= eshop.MaxOrders {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("Načítalo sa prvých %d objednávok — zvyšok príde pri ďalšom načítaní", eshop.MaxOrders))
	}
	s.LastSync = a.now().Format(time.RFC3339)
	s.LastResult = fmt.Sprintf("%d nových faktúr", rep.Created)
	if rep.Waiting > 0 {
		s.LastResult += fmt.Sprintf(", %d čaká na úhradu", rep.Waiting)
	}
	if rep.Failed > 0 {
		s.LastResult += fmt.Sprintf(", %d s chybou", rep.Failed)
	}
	if err := a.saveEshops(ctx, companyID, list); err != nil {
		return rep, err
	}
	if rep.Created > 0 {
		a.record(ctx, companyID, audit.CatInvoices, "create", "eshop_orders", 0,
			fmt.Sprintf("Z e-shopu %s: %d konceptov faktúr z objednávok", s.Name, rep.Created), nil, nil)
	}
	return rep, nil
}

func bratislava() *time.Location {
	if loc, err := time.LoadLocation("Europe/Bratislava"); err == nil {
		return loc
	}
	return time.UTC
}

func errorText(err error) string {
	var ve *invoice.ValidationError
	if errors.As(err, &ve) && len(ve.Fields) > 0 {
		return ve.Fields[0].Message
	}
	return err.Error()
}

// rateTolerance je odchýlka percenta DPH z obchodu (zaokrúhlenie dane v riadku).
// centTolerance je odchýlka dane zaokrúhlenej po kusoch (malé sumy).
var (
	rateTolerance = money.MustParse("0.6")
	centTolerance = money.MustParse("0.02")
)

// shopRate určí sadzbu DPH z dane a základu riadku (sadzby platné k dátumu).
func shopRate(net, tax money.Dec, date invoice.Date) (vat.Rate, bool) {
	if tax.IsZero() {
		return vat.Rate0, true
	}
	if net.Sign() <= 0 {
		return vat.Rate0, false
	}
	pct := tax.MulRatio(money.FromInt(100), net)
	for _, r := range vat.RatesOn(date.String()) {
		expected := net.MulRatio(money.FromInt(int64(r)), money.FromInt(100))
		if absDec(pct.Sub(money.FromInt(int64(r)))).Cmp(rateTolerance) <= 0 || absDec(expected.Sub(tax)).Cmp(centTolerance) <= 0 {
			return r, true
		}
	}
	return vat.Rate0, false
}

var shopPayments = map[string]invoice.PaymentMethod{"transfer": invoice.PayTransfer, "card": invoice.PayCard, "cod": invoice.PayCOD, "cash": invoice.PayCash}

func shopParty(a eshop.Address) invoice.Party {
	country := party.CountryCode(a.Country)
	if country == "" {
		country = "SK"
	}
	return invoice.Party{Name: firstNonBlank(a.Company, a.Name), Street: a.Street, City: a.City, Zip: strings.ReplaceAll(a.Zip, " ", ""),
		Country: country, ICO: strings.ReplaceAll(a.ICO, " ", ""), DIC: strings.TrimSpace(a.DIC), ICDPH: strings.ReplaceAll(a.ICDPH, " ", ""), Email: a.Email}
}

// orderInvoice zostaví koncept faktúry z objednávky. Riadky s kódom
// z cenníka sa naviažu na položku (sklad sa odpíše pri vystavení).
func orderInvoice(o eshop.Order, co store.Company, items []catalog.Item, byCode map[string]int, today invoice.Date) (invoice.Invoice, []string, error) {
	var warns []string
	cust := shopParty(o.Billing)
	if strings.TrimSpace(cust.Name) == "" {
		return invoice.Invoice{}, nil, errors.New("objednávka nemá meno zákazníka")
	}
	delivery := today
	if !o.Created.IsZero() {
		t := o.Created.In(bratislava())
		delivery = invoice.NewDate(t.Year(), t.Month(), t.Day())
	}
	due := today.AddDays(co.DefaultDueDays)
	if o.Paid {
		due = today
	}
	inv := invoice.Invoice{Customer: cust, IssueDate: today, DeliveryDate: delivery, DueDate: due, PriceMode: vat.PriceNet,
		Subject: "Objednávka č. " + o.Number, PaymentMethod: shopPayments[o.Payment]}
	if o.Billing.Company != "" && o.Billing.Name != "" {
		inv.Attention = &invoice.Person{Name: o.Billing.Name, Email: o.Billing.Email, Phone: o.Billing.Phone}
	}
	if sh := o.Shipping; !sh.IsZero() && (sh.Street != o.Billing.Street || sh.City != o.Billing.City) {
		inv.Delivery = &invoice.Address{Name: firstNonBlank(sh.Company, sh.Name), Street: sh.Street, City: sh.City,
			Zip: strings.ReplaceAll(sh.Zip, " ", ""), Country: firstNonBlank(party.CountryCode(sh.Country), "SK")}
	}
	if o.Paid {
		inv.Intro = "Objednávka bola uhradená " + invoice.NewDate(o.PaidAt.In(bratislava()).Date()).Display() + "."
	}
	for _, l := range o.Lines {
		if l.Net.IsZero() && l.Tax.IsZero() {
			continue // doprava zadarmo a pod.
		}
		price, rate := l.Net, vat.Rate0
		if co.VATPayer {
			r, ok := shopRate(l.Net, l.Tax, delivery)
			if !ok {
				return invoice.Invoice{}, nil, fmt.Errorf("riadok „%s“: daň %s € zo základu %s € nezodpovedá žiadnej sadzbe DPH", l.Name, l.Tax.Display(), l.Net.Display())
			}
			rate = r
		} else {
			price = l.Net.Add(l.Tax) // neplatiteľ: cena tak, ako ju zákazník zaplatil
		}
		line := invoice.Line{Description: firstNonBlank(l.Name, "Položka"), Quantity: money.FromInt(1), Unit: "ks", UnitPrice: price, Rate: rate}
		if q := l.Quantity; q.Sign() > 0 && !q.Equal(money.FromInt(1)) {
			unit := price.MulRatio(money.FromInt(1), q)
			if unit.Mul(q).RoundCents().Equal(price.RoundCents()) {
				line.Quantity, line.UnitPrice = q, unit
			} else {
				line.Description += " (" + strings.TrimRight(strings.TrimRight(q.String(), "0"), ".") + " ks)"
			}
		}
		if k, ok := byCode[strings.ToUpper(l.SKU)]; ok && l.SKU != "" {
			line.ItemID = items[k].ID
			line.Unit = firstNonBlank(items[k].Unit, "ks")
		}
		inv.Lines = append(inv.Lines, line)
	}
	if len(inv.Lines) == 0 {
		return invoice.Invoice{}, nil, errors.New("objednávka nemá žiadne položky")
	}
	if err := inv.Recalculate(); err == nil && !o.Total.IsZero() &&
		absDec(inv.Summary.Total.Sub(o.Total)).Cmp(migTolerance) > 0 {
		warns = append(warns, fmt.Sprintf("suma %s € sa líši od objednávky %s € — skontrolujte", inv.Summary.Total.Display(), o.Total.Display()))
	}
	return inv, warns, nil
}

// EshopStockReport je výsledok odoslania zásob.
type EshopStockReport struct {
	Updated int      `json:"updated"`
	Missing []string `json:"missing"` // kódy, ktoré obchod nepozná
	Errors  []string `json:"errors"`
}

// MaxStockPush je najviac položiek odoslaných naraz.
const MaxStockPush = 1000

// PushEshopStock pošle do obchodu stav zásob skladových položiek (podľa kódu).
func (a *App) PushEshopStock(ctx context.Context, companyID int64, id string) (EshopStockReport, error) {
	rep := EshopStockReport{Missing: []string{}, Errors: []string{}}
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return rep, err
	}
	eshopMu.Lock()
	defer eshopMu.Unlock()
	list, i, conn, err := a.eshopConn(ctx, companyID, id)
	if err != nil {
		return rep, err
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return rep, err
	}
	feed, err := a.feedItems(ctx, companyID, cfg)
	if err != nil {
		return rep, err
	}
	if len(feed) == 0 {
		return rep, mfaFieldErr("stock", "Na sklade nie sú žiadne položky — pridajte ich v module Sklad")
	}
	sent := 0
	for _, f := range feed {
		if f.Code == "" {
			continue
		}
		if sent >= MaxStockPush {
			rep.Errors = append(rep.Errors, fmt.Sprintf("Odoslaných prvých %d položiek", MaxStockPush))
			break
		}
		sent++
		qty := f.Quantity.Raw() / money.FromInt(1).Raw() // celé kusy nadol
		switch err := conn.SetStock(ctx, f.Code, qty); {
		case err == nil:
			rep.Updated++
		case errors.Is(err, eshop.ErrNoSKU):
			rep.Missing = append(rep.Missing, f.Code)
		case errors.Is(err, eshop.ErrAuth):
			return rep, shopErr(err)
		default:
			if len(rep.Errors) < 20 {
				rep.Errors = append(rep.Errors, f.Code+": "+err.Error())
			}
		}
	}
	list[i].LastStock = a.now().Format(time.RFC3339)
	if err := a.saveEshops(ctx, companyID, list); err != nil {
		return rep, err
	}
	a.record(ctx, companyID, audit.CatSettings, "update", "eshop_stock", 0,
		fmt.Sprintf("Zásoby odoslané do e-shopu %s: %d položiek", list[i].Name, rep.Updated), nil, nil)
	return rep, nil
}
