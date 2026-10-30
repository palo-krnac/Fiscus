package app

import (
	"context"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// PurchaseFormatKey je kľúč formátu číselného radu prijatých faktúr v nastaveniach firmy.
const PurchaseFormatKey = "purchase"

// DefaultPurchaseFormat je predvolený formát interného čísla prijatej faktúry.
const DefaultPurchaseFormat = "PF{RRRR}{NNNN}"

// PurchasePreview je prijatá faktúra po prepočte s chybami a upozorneniami.
type PurchasePreview struct {
	purchase.Purchase
	Errors   []invoice.FieldError `json:"errors"`
	Warnings []purchase.Warning   `json:"warnings"`
}

// PreviewPurchase prepočíta doklad bez uloženia.
func (a *App) PreviewPurchase(ctx context.Context, p purchase.Purchase) PurchasePreview {
	normalizePurchase(&p)
	_ = a.applyPurchaseRate(ctx, &p) // bez kurzu ukáže chybu fxRate
	out := PurchasePreview{Purchase: p, Errors: []invoice.FieldError{}, Warnings: p.Normalize()}
	if out.Warnings == nil {
		out.Warnings = []purchase.Warning{}
	}
	if ve, ok := p.Validate().(*invoice.ValidationError); ok && ve != nil {
		out.Errors = ve.Fields
	}
	return out
}

// PreviewPurchaseFor prepočíta doklad firmy (aj odpočet DPH podľa vozidla).
func (a *App) PreviewPurchaseFor(ctx context.Context, companyID int64, p purchase.Purchase) PurchasePreview {
	_ = a.applyVehicleVAT(ctx, companyID, &p)
	return a.PreviewPurchase(ctx, p)
}

// applyVehicleVAT: nákup k vozidlu — percento odpočtu DPH určí vozidlo (§ 85n).
func (a *App) applyVehicleVAT(ctx context.Context, companyID int64, p *purchase.Purchase) error {
	if p.VehicleID == 0 {
		return nil
	}
	date := p.DeliveryDate
	if date.IsZero() {
		date = p.IssueDate
	}
	pct, err := a.vehicleVAT(ctx, companyID, p.VehicleID, date)
	if err != nil {
		return err
	}
	p.DeductPercent = pct
	return nil
}

func normalizePurchase(p *purchase.Purchase) {
	p.SupplierNumber = strings.TrimSpace(p.SupplierNumber)
	p.SupplierIBAN = formatIBAN(p.SupplierIBAN)
	if p.VariableSymbol == "" {
		p.VariableSymbol = invoice.VariableSymbol(p.SupplierNumber)
	}
}

// SavePurchase zaeviduje novú alebo uloží upravenú prijatú faktúru.
// Vráti aj upozornenia (napr. iné zaokrúhlenie DPH na doklade).
func (a *App) SavePurchase(ctx context.Context, companyID int64, p purchase.Purchase) (purchase.Purchase, []purchase.Warning, error) {
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return p, nil, err
	}
	var before any
	if p.ID != 0 {
		old, err := a.Store.GetPurchase(ctx, companyID, p.ID)
		if err != nil {
			return p, nil, err
		}
		before = old
		if old.Status == purchase.StatusPaid {
			return p, nil, purchase.ErrPaid
		}
		// interné číslo a stav sa menia iba vlastnými akciami
		p.Number, p.Status, p.PaidDate = old.Number, old.Status, old.PaidDate
	} else {
		p.Number, p.Status, p.PaidDate = "", purchase.StatusBooked, invoice.Date{}
	}
	normalizePurchase(&p)
	if err := a.applyPurchaseRate(ctx, &p); err != nil {
		return p, nil, err
	}
	if err := a.applyVehicleVAT(ctx, companyID, &p); err != nil {
		return p, nil, err
	}
	warns := p.Normalize()
	if err := p.Validate(); err != nil {
		return p, warns, err
	}
	// rovnakú faktúru toho istého dodávateľa nezaevidovať dvakrát
	same, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{Search: p.SupplierNumber})
	if err != nil {
		return p, warns, err
	}
	for _, o := range same {
		if o.ID != p.ID && purchase.SameDocument(o, p) {
			return p, warns, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "supplierNumber",
				Message: "Táto faktúra dodávateľa je už zaevidovaná ako " + o.Number}}}
		}
	}
	if p.ID == 0 {
		year, month := p.ReceivedDate.Year(), p.ReceivedDate.Month()
		seq, err := a.Store.NextSequence(ctx, companyID, "PF", year)
		if err != nil {
			return p, warns, err
		}
		format := company.NumberFormats[PurchaseFormatKey]
		if format == "" {
			format = DefaultPurchaseFormat
		}
		p.Number = invoice.FormatNumberWith(format, year, month, seq)
	}
	if err := a.Store.SavePurchase(ctx, companyID, &p); err != nil {
		return p, warns, err
	}
	if before == nil {
		a.record(ctx, companyID, audit.CatPurchases, "create", "purchase", p.ID,
			"Zaevidovaná prijatá faktúra "+p.Number+" od "+p.Supplier.Name+", "+p.Summary.Total.Display()+" €", nil, p)
	} else {
		a.record(ctx, companyID, audit.CatPurchases, "update", "purchase", p.ID,
			"Úprava prijatej faktúry "+p.Number, before, p)
	}
	return p, warns, nil
}

// MarkPurchasePaid označí prijatú faktúru ako uhradenú.
func (a *App) MarkPurchasePaid(ctx context.Context, companyID, id int64, on invoice.Date) (purchase.Purchase, error) {
	return a.purchaseTransition(ctx, companyID, id, "pay", "Uhradená prijatá faktúra ", func(p *purchase.Purchase) error {
		if on.IsZero() {
			on = a.Today()
		}
		return p.MarkPaid(on)
	})
}

// UnpayPurchase zruší označenie úhrady.
func (a *App) UnpayPurchase(ctx context.Context, companyID, id int64) (purchase.Purchase, error) {
	return a.purchaseTransition(ctx, companyID, id, "unpay", "Zrušená úhrada prijatej faktúry ", (*purchase.Purchase).Unpay)
}

func (a *App) purchaseTransition(ctx context.Context, companyID, id int64, action, what string, fn func(*purchase.Purchase) error) (purchase.Purchase, error) {
	p, err := a.Store.GetPurchase(ctx, companyID, id)
	if err != nil {
		return p, err
	}
	before := map[string]any{"status": p.Status, "paidDate": p.PaidDate}
	if err := fn(&p); err != nil {
		return p, err
	}
	if err := a.Store.SavePurchase(ctx, companyID, &p); err != nil {
		return p, err
	}
	a.record(ctx, companyID, audit.CatPurchases, action, "purchase", p.ID, what+p.Number, before,
		map[string]any{"status": p.Status, "paidDate": p.PaidDate})
	return p, nil
}

// DeletePurchase zmaže neuhradenú prijatú faktúru (napr. zaevidovanú omylom).
func (a *App) DeletePurchase(ctx context.Context, companyID, id int64) error {
	p, err := a.Store.GetPurchase(ctx, companyID, id)
	if err != nil {
		return err
	}
	if p.Status == purchase.StatusPaid {
		return purchase.ErrPaid
	}
	if err := a.Store.DeletePurchase(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatPurchases, "delete", "purchase", id,
		"Zmazaná prijatá faktúra "+p.Number+" od "+p.Supplier.Name, p, nil)
	return nil
}
