package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// PaymentCandidate je neuhradená prijatá faktúra ponúknutá do platobného
// príkazu. Problem vysvetľuje, prečo ju do príkazu zaradiť nemožno.
type PaymentCandidate struct {
	ID             int64        `json:"id"`
	Number         string       `json:"number"`
	SupplierNumber string       `json:"supplierNumber"`
	Supplier       string       `json:"supplier"`
	IBAN           string       `json:"iban"`
	VariableSymbol string       `json:"variableSymbol"`
	Amount         money.Dec    `json:"amount"`
	Currency       string       `json:"currency"`
	DueDate        invoice.Date `json:"dueDate"`
	Overdue        bool         `json:"overdue"`
	Problem        string       `json:"problem,omitempty"`
}

// PaymentOrderError je dôvod, pre ktorý sa príkaz nedá zostaviť.
type PaymentOrderError struct{ Msg string }

func (e *PaymentOrderError) Error() string { return e.Msg }

// PaymentCandidates vráti neuhradené prijaté faktúry zoradené podľa splatnosti.
func (a *App) PaymentCandidates(ctx context.Context, companyID int64) ([]PaymentCandidate, error) {
	list, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
	if err != nil {
		return nil, err
	}
	today := a.Today()
	ibans := map[int64]string{} // IBAN z adresára, ak ho doklad nemá
	out := []PaymentCandidate{}
	for _, p := range list {
		if p.Status != purchase.StatusBooked {
			continue
		}
		iban := p.SupplierIBAN
		if strings.TrimSpace(iban) == "" && p.ContactID != 0 {
			if _, ok := ibans[p.ContactID]; !ok {
				if c, err := a.Store.GetContact(ctx, companyID, p.ContactID); err == nil {
					ibans[p.ContactID] = formatIBAN(c.IBAN)
				}
			}
			iban = ibans[p.ContactID]
		}
		c := PaymentCandidate{
			ID: p.ID, Number: p.Number, SupplierNumber: p.SupplierNumber, Supplier: p.Supplier.Name,
			IBAN: formatIBAN(iban), VariableSymbol: p.VariableSymbol, Amount: p.Summary.Total,
			Currency: p.Currency, DueDate: p.DueDate, Overdue: p.Overdue(today),
		}
		c.Problem = paymentProblem(c, p.PaymentMethod)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := out[i].DueDate, out[j].DueDate
		if di.IsZero() != dj.IsZero() {
			return dj.IsZero()
		}
		return di.Before(dj)
	})
	return out, nil
}

func paymentProblem(c PaymentCandidate, method invoice.PaymentMethod) string {
	switch {
	case c.Currency != "" && c.Currency != "EUR":
		return "Doklad je v mene " + c.Currency + " — príkaz SEPA je iba v eurách"
	case c.Amount.Sign() <= 0:
		return "Nie je čo uhradiť (dobropis alebo nulová suma)"
	case method == invoice.PayCash || method == invoice.PayCard:
		return "Platí sa v hotovosti alebo kartou"
	case strings.TrimSpace(c.IBAN) == "":
		return "Chýba IBAN dodávateľa"
	case !party.ValidIBAN(c.IBAN):
		return "Neplatný IBAN dodávateľa"
	}
	return ""
}

// PaymentOrder zostaví hromadný platobný príkaz (pain.001) na vybrané
// prijaté faktúry s dátumom splatnosti on. Vráti názov súboru a XML.
func (a *App) PaymentOrder(ctx context.Context, companyID int64, ids []int64, on invoice.Date) (string, []byte, error) {
	if len(ids) == 0 {
		return "", nil, &PaymentOrderError{"Vyberte aspoň jednu prijatú faktúru."}
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return "", nil, err
	}
	if !party.ValidIBAN(company.IBAN) {
		return "", nil, &PaymentOrderError{"Doplňte platný IBAN firmy v nastaveniach — z neho sa platí."}
	}
	today := a.Today()
	if on.IsZero() {
		on = today
	}
	if on.Before(today) {
		return "", nil, &PaymentOrderError{"Dátum splatnosti príkazu nemôže byť v minulosti."}
	}
	cands, err := a.PaymentCandidates(ctx, companyID)
	if err != nil {
		return "", nil, err
	}
	byID := map[int64]PaymentCandidate{}
	for _, c := range cands {
		byID[c.ID] = c
	}
	now := a.now()
	order := bank.PaymentOrder{
		MsgID:   fmt.Sprintf("FISKUS-%d-%s", companyID, now.Format("20060102150405")),
		Created: now, Debtor: company.Name, DebtorIBAN: company.IBAN, Date: on,
	}
	var numbers []string
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		c, ok := byID[id]
		if !ok {
			return "", nil, &PaymentOrderError{fmt.Sprintf("Prijatá faktúra %d neexistuje alebo je už uhradená.", id)}
		}
		if c.Problem != "" {
			return "", nil, &PaymentOrderError{c.Number + ": " + c.Problem}
		}
		info := "Faktura " + c.SupplierNumber
		if c.SupplierNumber == "" {
			info = "Faktura " + c.Number
		}
		order.Transfers = append(order.Transfers, bank.Transfer{
			Name: c.Supplier, IBAN: c.IBAN, Amount: c.Amount, VS: c.VariableSymbol, Info: info,
		})
		numbers = append(numbers, c.Number)
	}
	data, err := order.XML()
	if err != nil {
		return "", nil, &PaymentOrderError{err.Error()}
	}
	a.record(ctx, companyID, audit.CatPurchases, "payment_order", "purchase", 0,
		fmt.Sprintf("Platobný príkaz na %s (%d úhrad, %s €, splatnosť %s)", strings.Join(numbers, ", "),
			len(order.Transfers), order.Total().Display(), on.Display()), nil, map[string]any{"ids": ids, "date": on})
	return "prikaz-" + on.String() + ".xml", data, nil
}
