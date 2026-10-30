package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/logbook"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// VehicleInfo je vozidlo so stavom tachometra a počtom záznamov.
type VehicleInfo struct {
	logbook.Vehicle
	Odometer  int64  `json:"lastOdometer"` // stav po poslednej jazde (na predvyplnenie)
	Trips     int    `json:"trips"`
	Fuels     int    `json:"fuels"`
	AssetName string `json:"assetName,omitempty"`
}

func vErr(field, msg string) error {
	return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: field, Message: msg}}}
}

// Vehicles vráti vozidlá firmy.
func (a *App) Vehicles(ctx context.Context, companyID int64) ([]VehicleInfo, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	list, err := a.Store.ListVehicles(ctx, companyID)
	if err != nil {
		return nil, err
	}
	trips, err := a.Store.ListTrips(ctx, companyID, 0, 0)
	if err != nil {
		return nil, err
	}
	fuels, err := a.Store.ListFuel(ctx, companyID, 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]VehicleInfo, 0, len(list))
	for _, v := range list {
		info := VehicleInfo{Vehicle: v}
		var own []logbook.Trip
		for _, t := range trips {
			if t.VehicleID == v.ID {
				own = append(own, t)
			}
		}
		info.Trips = len(own)
		info.Odometer = logbook.NextOdometer(v, own, invoice.Date{})
		for _, f := range fuels {
			if f.VehicleID == v.ID {
				info.Fuels++
			}
		}
		if v.AssetID != 0 {
			if as, err := a.Store.GetAsset(ctx, companyID, v.AssetID); err == nil {
				info.AssetName = as.Number + " " + as.Name
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// SaveVehicle uloží vozidlo (ID 0 = nové).
func (a *App) SaveVehicle(ctx context.Context, companyID int64, v logbook.Vehicle) (logbook.Vehicle, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return v, err
	}
	v.Normalize()
	if err := v.Validate(); err != nil {
		return v, err
	}
	var before *logbook.Vehicle
	if v.ID != 0 {
		old, err := a.Store.GetVehicle(ctx, companyID, v.ID)
		if err != nil {
			return v, err
		}
		before = &old
	}
	if v.AssetID != 0 {
		if _, err := a.Store.GetAsset(ctx, companyID, v.AssetID); err != nil {
			return v, vErr("assetId", "Položka majetku neexistuje")
		}
	}
	list, err := a.Store.ListVehicles(ctx, companyID)
	if err != nil {
		return v, err
	}
	for _, o := range list {
		if o.ID != v.ID && o.Plate == v.Plate {
			return v, vErr("plate", "Vozidlo s EČV "+v.Plate+" už v knihe jázd je")
		}
	}
	if err := a.Store.SaveVehicle(ctx, companyID, &v); err != nil {
		return v, err
	}
	a.record(ctx, companyID, audit.CatLedger, "vehicle", "vehicle", v.ID, "Kniha jázd: vozidlo "+v.Plate+" "+v.Name, before, v)
	return v, nil
}

// DeleteVehicle zmaže vozidlo bez záznamov (s jazdami ho treba archivovať).
func (a *App) DeleteVehicle(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	v, err := a.Store.GetVehicle(ctx, companyID, id)
	if err != nil {
		return err
	}
	trips, err := a.Store.ListTrips(ctx, companyID, id, 0)
	if err != nil {
		return err
	}
	fuels, err := a.Store.ListFuel(ctx, companyID, id, 0)
	if err != nil {
		return err
	}
	if len(trips)+len(fuels) > 0 {
		return vErr("vehicle", "Vozidlo má jazdy alebo tankovania — nemožno ho zmazať, iba archivovať")
	}
	if err := a.Store.DeleteVehicle(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "vehicle", "vehicle", id, "Kniha jázd: zmazané vozidlo "+v.Plate, v, nil)
	return nil
}

// activeVehicle overí, že do vozidla možno zapisovať.
func (a *App) activeVehicle(ctx context.Context, companyID, id int64) (logbook.Vehicle, error) {
	v, err := a.Store.GetVehicle(ctx, companyID, id)
	if errors.Is(err, store.ErrNotFound) {
		return v, vErr("vehicleId", "Vyberte vozidlo")
	}
	if err != nil {
		return v, err
	}
	if v.Archived {
		return v, vErr("vehicleId", "Vozidlo "+v.Plate+" je archivované")
	}
	return v, nil
}

// Trips vráti jazdy vozidla za rok.
func (a *App) Trips(ctx context.Context, companyID, vehicleID int64, year int) ([]logbook.Trip, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	if _, err := a.Store.GetVehicle(ctx, companyID, vehicleID); err != nil {
		return nil, err
	}
	return a.Store.ListTrips(ctx, companyID, vehicleID, year)
}

// SaveTrip uloží jazdu (ID 0 = nová).
func (a *App) SaveTrip(ctx context.Context, companyID int64, t logbook.Trip) (logbook.Trip, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return t, err
	}
	var before *logbook.Trip
	if t.ID != 0 {
		old, err := a.Store.GetTrip(ctx, companyID, t.ID)
		if err != nil {
			return t, err
		}
		before = &old
	}
	v, err := a.activeVehicle(ctx, companyID, t.VehicleID)
	if err != nil {
		return t, err
	}
	if t.Driver == "" {
		t.Driver = v.Driver
	}
	t.Normalize()
	if err := t.Validate(); err != nil {
		return t, err
	}
	if v.Odometer > 0 && t.OdoStart > 0 && t.OdoStart < v.Odometer && !v.OdometerDate.IsZero() && !t.Date.Before(v.OdometerDate) {
		return t, vErr("odoStart", fmt.Sprintf("Stav tachometra je nižší ako pri začatí evidencie (%d km)", v.Odometer))
	}
	if err := a.Store.SaveTrip(ctx, companyID, &t); err != nil {
		return t, err
	}
	a.record(ctx, companyID, audit.CatLedger, "trip", "trip", t.ID,
		fmt.Sprintf("Kniha jázd %s: jazda %s %s – %s, %d km", v.Plate, t.Date.Display(), t.From, t.To, t.Km), before, t)
	return t, nil
}

// DeleteTrip zmaže jazdu.
func (a *App) DeleteTrip(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	t, err := a.Store.GetTrip(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteTrip(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "trip", "trip", id, fmt.Sprintf("Kniha jázd: zmazaná jazda %s %s – %s", t.Date.Display(), t.From, t.To), t, nil)
	return nil
}

// Fuels vráti tankovania vozidla za rok.
func (a *App) Fuels(ctx context.Context, companyID, vehicleID int64, year int) ([]logbook.Fuel, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	if _, err := a.Store.GetVehicle(ctx, companyID, vehicleID); err != nil {
		return nil, err
	}
	return a.Store.ListFuel(ctx, companyID, vehicleID, year)
}

// FuelDoc je doklad, z ktorého možno tankovanie prevziať.
type FuelDoc struct {
	Kind    string       `json:"kind"` // cash | purchase
	ID      int64        `json:"id"`
	Number  string       `json:"number"`
	Date    invoice.Date `json:"date"`
	Partner string       `json:"partner"`
	Text    string       `json:"text"`
	Amount  money.Dec    `json:"amount"`
}

// FuelDocs vráti výdavkové pokladničné doklady a prijaté faktúry v eurách
// za rok, ktoré ešte nie sú priradené k tankovaniu.
func (a *App) FuelDocs(ctx context.Context, companyID int64, year int) ([]FuelDoc, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	used, err := a.usedFuelDocs(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	out := []FuelDoc{}
	docs, err := a.Store.ListCashDocuments(ctx, companyID, year)
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		if d.Kind != cash.Out || d.Cancelled || d.Foreign() || d.LinkKind != "" || used["cash:"+strconv.FormatInt(d.ID, 10)] {
			continue
		}
		out = append(out, FuelDoc{Kind: "cash", ID: d.ID, Number: d.Number, Date: d.Date, Partner: d.Partner, Text: d.Purpose, Amount: d.Amount})
	}
	purs, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{Year: year})
	if err != nil {
		return nil, err
	}
	for _, p := range purs {
		if p.Foreign() || used["purchase:"+strconv.FormatInt(p.ID, 10)] {
			continue
		}
		out = append(out, FuelDoc{Kind: "purchase", ID: p.ID, Number: p.Number, Date: p.DeliveryDate, Partner: p.Supplier.Name, Text: p.Description, Amount: p.Summary.Total})
	}
	return out, nil
}

// usedFuelDocs vráti doklady už priradené k tankovaniam (okrem tankovania except).
func (a *App) usedFuelDocs(ctx context.Context, companyID, except int64) (map[string]bool, error) {
	fuels, err := a.Store.ListFuel(ctx, companyID, 0, 0)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, f := range fuels {
		if f.DocKind != "" && f.ID != except {
			used[f.DocKind+":"+strconv.FormatInt(f.DocID, 10)] = true
		}
	}
	return used, nil
}

// SaveFuel uloží tankovanie (ID 0 = nové); doklad doplní číslo dokladu.
func (a *App) SaveFuel(ctx context.Context, companyID int64, f logbook.Fuel) (logbook.Fuel, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return f, err
	}
	var before *logbook.Fuel
	if f.ID != 0 {
		old, err := a.Store.GetFuel(ctx, companyID, f.ID)
		if err != nil {
			return f, err
		}
		before = &old
	}
	v, err := a.activeVehicle(ctx, companyID, f.VehicleID)
	if err != nil {
		return f, err
	}
	f.DocNumber = ""
	switch f.DocKind {
	case "cash":
		d, err := a.Store.GetCashDocument(ctx, companyID, f.DocID)
		if err != nil || d.Kind != cash.Out || d.Cancelled {
			return f, vErr("docId", "Výdavkový pokladničný doklad neexistuje")
		}
		f.DocNumber = d.Number
		if f.Amount.IsZero() {
			f.Amount = d.Amount
		}
	case "purchase":
		p, err := a.Store.GetPurchase(ctx, companyID, f.DocID)
		if err != nil {
			return f, vErr("docId", "Prijatá faktúra neexistuje")
		}
		f.DocNumber = p.Number
		if f.Amount.IsZero() {
			f.Amount = p.Summary.Total
		}
	default:
		f.DocID = 0
	}
	if f.DocKind != "" {
		used, err := a.usedFuelDocs(ctx, companyID, f.ID)
		if err != nil {
			return f, err
		}
		if used[f.DocKind+":"+strconv.FormatInt(f.DocID, 10)] {
			return f, vErr("docId", "Doklad "+f.DocNumber+" už patrí k inému tankovaniu")
		}
	}
	if err := f.Validate(); err != nil {
		return f, err
	}
	if err := a.Store.SaveFuel(ctx, companyID, &f); err != nil {
		return f, err
	}
	a.record(ctx, companyID, audit.CatLedger, "fuel", "fuel", f.ID,
		fmt.Sprintf("Kniha jázd %s: tankovanie %s, %s %s za %s €", v.Plate, f.Date.Display(), f.Quantity.Display(), v.Fuel.Unit(), f.Amount.Display()), before, f)
	return f, nil
}

// DeleteFuel zmaže tankovanie.
func (a *App) DeleteFuel(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	f, err := a.Store.GetFuel(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteFuel(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "fuel", "fuel", id, "Kniha jázd: zmazané tankovanie "+f.Date.Display(), f, nil)
	return nil
}

// LogbookReport je vyúčtovanie vozidla za rok so stavom zaúčtovania.
type LogbookReport struct {
	logbook.Report
	Vehicle logbook.Vehicle `json:"vehicle"`
	// Posting je suma, ktorá sa zaúčtuje do uzávierky (náhrady +, vylúčenie −),
	// Posted je suma už zaúčtovaná.
	Posting money.Dec `json:"posting"`
	Posted  money.Dec `json:"posted"`
}

func logbookKey(year int, vehicleID int64) string {
	return closingPrefix(year) + "km" + strconv.FormatInt(vehicleID, 10)
}

// VehicleReport vyúčtuje vozidlo za rok.
func (a *App) VehicleReport(ctx context.Context, companyID, vehicleID int64, year int) (LogbookReport, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return LogbookReport{}, err
	}
	v, err := a.Store.GetVehicle(ctx, companyID, vehicleID)
	if err != nil {
		return LogbookReport{}, err
	}
	trips, err := a.Store.ListTrips(ctx, companyID, vehicleID, 0)
	if err != nil {
		return LogbookReport{}, err
	}
	fuels, err := a.Store.ListFuel(ctx, companyID, vehicleID, 0)
	if err != nil {
		return LogbookReport{}, err
	}
	in := logbook.Input{Vehicle: v, Year: year, Trips: trips, Fuel: fuels, Rates: a.legalParams().KmRates}
	if v.Ownership == logbook.Business && v.AssetID != 0 {
		if as, err := a.Store.GetAsset(ctx, companyID, v.AssetID); err == nil {
			in.Depreciation, in.AssetShare = as.InYear(year).Claimed, as.Share
		}
	}
	out := LogbookReport{Report: logbook.Compute(in), Vehicle: v}
	out.Posting = out.Allowance.Sub(out.Exclude)
	overrides, err := a.Store.ListJournalOverrides(ctx, companyID)
	if err != nil {
		return out, err
	}
	if o, ok := overrides[logbookKey(year, vehicleID)]; ok {
		for _, l := range o.Lines {
			out.Posted = out.Posted.Add(l.Amount)
		}
	}
	return out, nil
}

// PostLogbook zaúčtuje vyúčtovanie vozidla do uzávierky roka (stĺpec e6):
// cestovné náhrady súkromného vozidla ako výdavok, pri vozidle v majetku
// vylúčenie PHM na súkromné jazdy a nadspotreby (záporná suma). Opakované
// volanie sumu prepíše, nulová suma operáciu zmaže.
func (a *App) PostLogbook(ctx context.Context, companyID, vehicleID int64, year int) (money.Dec, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return money.Zero, err
	}
	r, err := a.VehicleReport(ctx, companyID, vehicleID, year)
	if err != nil {
		return money.Zero, err
	}
	key := logbookKey(year, vehicleID)
	if r.Posting.IsZero() {
		overrides, err := a.Store.ListJournalOverrides(ctx, companyID)
		if err != nil {
			return money.Zero, err
		}
		if old, ok := overrides[key]; ok {
			if err := a.Store.DeleteJournalOverride(ctx, companyID, key); err != nil {
				return money.Zero, err
			}
			a.record(ctx, companyID, audit.CatLedger, "closing", "journal", 0, "Zmazaná uzávierková operácia "+key, old, nil)
		}
		return money.Zero, nil
	}
	text := fmt.Sprintf("Cestovné náhrady %d – súkromné vozidlo %s", year, r.Vehicle.Plate)
	if r.Vehicle.Ownership != logbook.Private {
		text = fmt.Sprintf("Kniha jázd %d – vylúčenie PHM na súkromné jazdy a nadspotreby, %s", year, r.Vehicle.Plate)
	}
	return r.Posting, a.saveClosing(ctx, companyID, key, text, []journal.Line{{Column: journal.ExpOther, Amount: r.Posting}})
}
