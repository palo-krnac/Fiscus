package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func cashRegistersKey(companyID int64) string { return fmt.Sprintf("cash.registers.%d", companyID) }

// cashRegisters vráti pokladne firmy; hlavná (ID 0) je vždy prvá.
func (a *App) cashRegisters(ctx context.Context, companyID int64) ([]cash.Register, error) {
	out := []cash.Register{cash.Main()}
	v, err := a.Store.GetSetting(ctx, cashRegistersKey(companyID))
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var saved []cash.Register
	if err := json.Unmarshal([]byte(v), &saved); err != nil {
		return nil, err
	}
	for _, r := range saved {
		if r.ID == 0 {
			r.Normalize()
			out[0] = r
		} else {
			out = append(out, r)
		}
	}
	return out, nil
}

// cashRegister nájde pokladňu podľa ID.
func (a *App) cashRegister(ctx context.Context, companyID, id int64) (cash.Register, error) {
	list, err := a.cashRegisters(ctx, companyID)
	if err != nil {
		return cash.Register{}, err
	}
	for _, r := range list {
		if r.ID == id {
			return r, nil
		}
	}
	return cash.Register{}, store.ErrNotFound
}

// CashRegisterInfo je pokladňa so stavom.
type CashRegisterInfo struct {
	cash.Register
	Balance    money.Dec `json:"balance"`    // zostatok v mene pokladne
	BalanceEUR money.Dec `json:"balanceEur"` // v eurách podľa kurzov dokladov (peňažný denník)
	Documents  int       `json:"documents"`
}

// CashRegisters vráti pokladne firmy so zostatkami.
func (a *App) CashRegisters(ctx context.Context, companyID int64) ([]CashRegisterInfo, error) {
	if err := a.Require(ctx, companyID, auth.PermCashRead); err != nil {
		return nil, err
	}
	list, err := a.cashRegisters(ctx, companyID)
	if err != nil {
		return nil, err
	}
	docs, err := a.Store.ListCashDocuments(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	out := make([]CashRegisterInfo, 0, len(list))
	for _, r := range list {
		info := CashRegisterInfo{Register: r}
		for _, d := range docs {
			if d.RegisterID != r.ID {
				continue
			}
			info.Documents++
			if !d.Cancelled {
				info.Balance = info.Balance.Add(d.Signed())
				info.BalanceEUR = info.BalanceEUR.Add(d.SignedEUR())
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// SaveCashRegisters uloží zoznam pokladní (hlavná má ID 0, nové záporné ID
// a dostanú nové). Pokladňu s dokladmi nemožno zmazať ani jej
// zmeniť menu či predponu — dá sa archivovať.
func (a *App) SaveCashRegisters(ctx context.Context, companyID int64, list []cash.Register) ([]CashRegisterInfo, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return nil, err
	}
	old, err := a.CashRegisters(ctx, companyID)
	if err != nil {
		return nil, err
	}
	byID := map[int64]CashRegisterInfo{}
	var maxID int64
	for _, r := range old {
		byID[r.ID] = r
		maxID = max(maxID, r.ID)
	}
	var clean []cash.Register
	seen := map[int64]bool{}
	hasMain := false
	for i, r := range list {
		isNew := r.ID < 0
		if isNew {
			maxID++
			r.ID = maxID
		}
		r.Normalize()
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if r.ID == 0 {
			hasMain = true
		}
		if prev, ok := byID[r.ID]; ok && prev.Documents > 0 && (prev.Currency != r.Currency || prev.Prefix != r.Prefix) {
			return nil, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: fmt.Sprintf("registers[%d].currency", i), Message: cash.ErrRegisterUsed.Error()}}}
		} else if !ok && !isNew {
			return nil, store.ErrNotFound
		}
		clean = append(clean, r)
	}
	if !hasMain {
		clean = append([]cash.Register{cash.Main()}, clean...)
	}
	for _, r := range old {
		if !seen[r.ID] && r.ID != 0 && r.Documents > 0 {
			return nil, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "registers", Message: "Pokladňa " + r.Name + " má doklady — nedá sa zmazať, iba archivovať"}}}
		}
	}
	if err := cash.ValidateRegisters(clean); err != nil {
		return nil, err
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return nil, err
	}
	if err := a.Store.SetSetting(ctx, cashRegistersKey(companyID), string(b)); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(clean))
	for _, r := range clean {
		names = append(names, r.Name+" ("+r.Currency+")")
	}
	a.record(ctx, companyID, audit.CatCash, "registers", "cash", 0, "Pokladne: "+strings.Join(names, ", "), old, clean)
	return a.CashRegisters(ctx, companyID)
}
