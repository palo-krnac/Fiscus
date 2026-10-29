package invoice

import (
	"errors"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Zdroj úhrady.
const (
	PaymentManual = "manual" // označená ručne
	PaymentBank   = "bank"   // spárovaný bankový pohyb
	PaymentCash   = "cash"   // pokladničný doklad
)

// Payment je úhrada faktúry (aj čiastočná). Suma má rovnaké znamienko ako
// faktúra — pri dobropise je záporná (vrátené peniaze).
type Payment struct {
	Date   Date      `json:"date"`
	Amount money.Dec `json:"amount"`
	Source string    `json:"source"`
	Ref    int64     `json:"ref,omitempty"` // ID bankového pohybu alebo pokladničného dokladu
}

var (
	// ErrBadPayment — nulová suma alebo suma s opačným znamienkom.
	ErrBadPayment = errors.New("suma úhrady musí byť kladná (pri dobropise vrátená suma)")
	// ErrPartialAdvance — zálohová faktúra sa uhrádza celá (daňový doklad k platbe).
	ErrPartialAdvance = errors.New("zálohovú faktúru zatiaľ označte ako uhradenú až po prijatí celej sumy")
)

// Paid vráti uhradenú sumu. Faktúra uhradená pred evidenciou úhrad je uhradená celá.
func (inv *Invoice) Paid() money.Dec {
	if len(inv.Payments) == 0 && inv.Status == StatusPaid {
		return inv.Summary.Total
	}
	return inv.paymentsSum()
}

func (inv *Invoice) paymentsSum() money.Dec {
	sum := money.Zero
	for _, p := range inv.Payments {
		sum = sum.Add(p.Amount)
	}
	return sum
}

// Remaining vráti zostatok na úhradu (pri dobropise záporný = zostáva vrátiť).
func (inv *Invoice) Remaining() money.Dec { return inv.Summary.Total.Sub(inv.Paid()) }

// PartlyPaid vráti true pre vystavenú faktúru s časťou úhrady.
func (inv *Invoice) PartlyPaid() bool { return inv.Status == StatusIssued && !inv.Paid().IsZero() }

// SyncPaid prepočíta uhradenú sumu (po načítaní z úložiska).
func (inv *Invoice) SyncPaid() {
	inv.PaidAmount = inv.Paid()
	inv.Balance = inv.Remaining()
	inv.BalanceEUR = inv.Balance
	if inv.Foreign() {
		inv.BalanceEUR = money.Zero
		if inv.FXRate.Sign() > 0 {
			inv.BalanceEUR = inv.Balance.DivCents(inv.FXRate)
		}
	}
}

// AddPayment pripíše úhradu. Keď je faktúra uhradená celá (alebo preplatená),
// prejde do stavu uhradená s dátumom poslednej úhrady.
func (inv *Invoice) AddPayment(p Payment) error {
	if inv.Status != StatusIssued {
		return ErrBadTransition
	}
	total := inv.Summary.Total
	if !total.IsZero() && (p.Amount.IsZero() || p.Amount.Sign() != total.Sign()) {
		return ErrBadPayment
	}
	if inv.Kind == KindProforma && p.Amount.Cmp(inv.Remaining()) < 0 {
		return ErrPartialAdvance
	}
	if !p.Amount.IsZero() {
		inv.Payments = append(inv.Payments, p)
	}
	if rem := inv.Remaining(); rem.IsZero() || rem.Sign() != total.Sign() {
		inv.Status, inv.PaidDate = StatusPaid, p.Date
	}
	inv.SyncPaid()
	return nil
}

// RemovePayment zruší úhradu z daného zdroja (napr. zrušené spárovanie pohybu).
// Vráti false, ak taká úhrada nie je.
func (inv *Invoice) RemovePayment(source string, ref int64) bool {
	for i, p := range inv.Payments {
		if p.Source == source && p.Ref == ref {
			inv.Payments = append(inv.Payments[:i:i], inv.Payments[i+1:]...)
			rem := inv.Summary.Total.Sub(inv.paymentsSum())
			if inv.Status == StatusPaid && !rem.IsZero() && rem.Sign() == inv.Summary.Total.Sign() {
				inv.Status, inv.PaidDate = StatusIssued, Date{}
			}
			inv.SyncPaid()
			return true
		}
	}
	return false
}
