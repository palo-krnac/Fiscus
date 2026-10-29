package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/extract"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

const aiKey = "ai:anthropic"

// loadAI vráti nastavenie Claude API vrátane kľúča. Kľúč môže prísť aj
// z premennej prostredia FISKUS_ANTHROPIC_API_KEY (server).
func (a *App) loadAI(ctx context.Context) (extract.Settings, error) {
	var s extract.Settings
	raw, err := a.Store.GetSetting(ctx, aiKey)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return s, err
	default:
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return s, err
		}
	}
	if s.APIKey == "" {
		s.APIKey = strings.TrimSpace(os.Getenv("FISKUS_ANTHROPIC_API_KEY"))
	}
	if s.Model == "" {
		s.Model = extract.DefaultModel
	}
	s.KeySet = s.APIKey != ""
	return s, nil
}

// AISettings vráti nastavenie čítania dokladov (bez kľúča). Čítať ho môže
// každý prihlásený — podľa neho sa ukáže tlačidlo „Načítať z PDF“.
func (a *App) AISettings(ctx context.Context) (extract.Settings, error) {
	s, err := a.loadAI(ctx)
	s.APIKey = ""
	return s, err
}

// SetAISettings uloží kľúč Claude API a model (iba správca programu).
// Prázdny kľúč ponechá uložený, clearKey ho zmaže.
func (a *App) SetAISettings(ctx context.Context, s extract.Settings, clearKey bool) (extract.Settings, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return s, err
	}
	var old extract.Settings
	if raw, err := a.Store.GetSetting(ctx, aiKey); err == nil {
		_ = json.Unmarshal([]byte(raw), &old)
	}
	s.APIKey = strings.TrimSpace(s.APIKey)
	if s.APIKey == "" && !clearKey {
		s.APIKey = old.APIKey
	}
	s.Model = strings.TrimSpace(s.Model)
	s.KeySet = false
	b, _ := json.Marshal(s)
	if err := a.Store.SetSetting(ctx, aiKey, string(b)); err != nil {
		return s, err
	}
	a.record(ctx, 0, audit.CatAI, "update", "ai_settings", 0, "Nastavenie čítania dokladov cez Claude API (model "+s.Model+")", nil,
		map[string]any{"model": s.Model, "keySet": s.APIKey != ""})
	return a.AISettings(ctx)
}

// PurchaseDraft je koncept prijatej faktúry prečítaný z dokladu.
type PurchaseDraft struct {
	Purchase purchase.Purchase `json:"purchase"`
	Warnings []string          `json:"warnings"`
}

// ExtractPurchase prečíta PDF alebo fotku prijatej faktúry cez Claude API
// a vráti koncept na kontrolu (nič neukladá). Dodávateľa priradí z adresára
// podľa IČO a upozorní na faktúru, ktorá už je zaevidovaná.
func (a *App) ExtractPurchase(ctx context.Context, companyID int64, fileName string, data []byte) (PurchaseDraft, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return PurchaseDraft{}, err
	}
	s, err := a.loadAI(ctx)
	if err != nil {
		return PurchaseDraft{}, err
	}
	r, err := extract.Client{Settings: s, BaseURL: a.AIBaseURL}.Extract(ctx, data)
	if err != nil {
		return PurchaseDraft{}, err
	}
	p, warns := r.Purchase(a.Today())
	warns = a.completeDraft(ctx, companyID, &p, warns)
	a.record(ctx, companyID, audit.CatAI, "extract", "purchase", 0,
		fmt.Sprintf("Prečítaný doklad %s cez Claude API: %s %s, %s €", fileName, p.Supplier.Name, p.SupplierNumber, p.Summary.Total.Display()), nil, nil)
	return PurchaseDraft{Purchase: p, Warnings: warns}, nil
}

// completeDraft priradí dodávateľa z adresára podľa IČO (aj IBAN) a upozorní
// na faktúru, ktorá už je zaevidovaná.
func (a *App) completeDraft(ctx context.Context, companyID int64, p *purchase.Purchase, warns []string) []string {
	if p.Supplier.ICO != "" {
		if list, err := a.Store.ListContacts(ctx, companyID, p.Supplier.ICO); err == nil {
			for _, c := range list {
				if c.ICO == p.Supplier.ICO {
					p.ContactID = c.ID
					if p.SupplierIBAN == "" {
						p.SupplierIBAN = c.IBAN
					}
					break
				}
			}
		}
	}
	if p.SupplierNumber != "" {
		if all, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{Search: p.SupplierNumber}); err == nil {
			for _, x := range all {
				if x.SupplierNumber == p.SupplierNumber && (p.Supplier.ICO == "" || x.Supplier.ICO == p.Supplier.ICO) {
					warns = append(warns, fmt.Sprintf("Faktúra %s od tohto dodávateľa už je zaevidovaná ako %s", p.SupplierNumber, x.Number))
				}
			}
		}
	}
	if warns == nil {
		warns = []string{}
	}
	return warns
}
