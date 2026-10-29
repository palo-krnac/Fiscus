package journal

import (
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
)

// Kniha pohľadávok a kniha záväzkov (opatrenie MF SR č. MF/27076/2007-74,
// § 5): stav neuhradených dokladov k zvolenému dňu — napr. k 31. 12. pre
// uzávierku a výkaz o majetku a záväzkoch. Úhrady po tomto dni sa neberú.

// OpenItem je pohľadávka alebo záväzok k dňu.
type OpenItem struct {
	Kind      string       `json:"kind"` // invoice, proforma, credit_note, purchase
	ID        int64        `json:"id"`
	Number    string       `json:"number"`
	Partner   string       `json:"partner"`
	ICO       string       `json:"ico,omitempty"`
	IssueDate invoice.Date `json:"issueDate"`
	DueDate   invoice.Date `json:"dueDate"`
	Currency  string       `json:"currency"`
	Total     money.Dec    `json:"total"`     // suma dokladu v mene dokladu
	Paid      money.Dec    `json:"paid"`      // uhradené k dňu
	Remaining money.Dec    `json:"remaining"` // zostáva k dňu
	RemainEUR money.Dec    `json:"remainingEur"`
	DaysLate  int          `json:"daysOverdue"` // po splatnosti k dňu
}

// paidAt vráti úhrady faktúry s dátumom najneskôr at.
func paidAt(inv invoice.Invoice, at invoice.Date) money.Dec {
	if len(inv.Payments) == 0 {
		if inv.Status == invoice.StatusPaid && (inv.PaidDate.IsZero() || !inv.PaidDate.After(at)) {
			return inv.Summary.Total
		}
		return money.Zero
	}
	sum := money.Zero
	for _, p := range inv.Payments {
		if !p.Date.After(at) {
			sum = sum.Add(p.Amount)
		}
	}
	return sum
}

// eur prepočíta zostatok v cudzej mene pomerom súm dokladu (bez kurzu nula).
func eur(remaining, total, totalEUR money.Dec, foreign bool) money.Dec {
	if !foreign {
		return remaining
	}
	if total.IsZero() {
		return money.Zero
	}
	return remaining.MulRatio(totalEUR, total).RoundCents()
}

// Receivables vráti pohľadávky k dňu at: vystavené faktúry a zálohové
// faktúry s neuhradenou časťou; dobropisy so zostatkom na vrátenie idú so
// zápornou sumou. Koncepty, stornované doklady a daňové doklady k platbe nie.
func Receivables(invs []invoice.Invoice, at invoice.Date) []OpenItem {
	out := []OpenItem{}
	for _, inv := range invs {
		if inv.Status == invoice.StatusDraft || inv.Status == invoice.StatusCancelled || inv.Kind == invoice.KindAdvanceReceipt {
			continue
		}
		if inv.IssueDate.After(at) {
			continue
		}
		paid := paidAt(inv, at)
		rem := inv.Summary.Total.Sub(paid)
		if rem.IsZero() || rem.Sign() != inv.Summary.Total.Sign() {
			continue
		}
		sum, ok := inv.TaxSummary()
		totalEUR := inv.Summary.Total
		if ok {
			totalEUR = sum.Total
		}
		it := OpenItem{Kind: string(inv.Kind), ID: inv.ID, Number: inv.Number, Partner: inv.Customer.Name, ICO: inv.Customer.ICO,
			IssueDate: inv.IssueDate, DueDate: inv.DueDate, Currency: currency(inv.Currency), Total: inv.Summary.Total, Paid: paid,
			Remaining: rem, RemainEUR: eur(rem, inv.Summary.Total, totalEUR, inv.Foreign())}
		if !inv.DueDate.IsZero() && inv.DueDate.Before(at) {
			it.DaysLate = inv.DueDate.DaysUntil(at)
		}
		out = append(out, it)
	}
	return out
}

// Payables vráti záväzky k dňu at: prijaté faktúry doručené najneskôr at
// a neuhradené k tomuto dňu.
func Payables(purs []purchase.Purchase, at invoice.Date) []OpenItem {
	out := []OpenItem{}
	for _, p := range purs {
		if p.ReceivedDate.After(at) {
			continue
		}
		if p.Status == purchase.StatusPaid && (p.PaidDate.IsZero() || !p.PaidDate.After(at)) {
			continue
		}
		it := OpenItem{Kind: "purchase", ID: p.ID, Number: p.Number, Partner: p.Supplier.Name, ICO: p.Supplier.ICO,
			IssueDate: p.ReceivedDate, DueDate: p.DueDate, Currency: currency(p.Currency), Total: p.Summary.Total, Paid: money.Zero,
			Remaining: p.Summary.Total, RemainEUR: p.TotalEUR()}
		if !p.DueDate.IsZero() && p.DueDate.Before(at) {
			it.DaysLate = p.DueDate.DaysUntil(at)
		}
		out = append(out, it)
	}
	return out
}

func currency(c string) string {
	if c == "" {
		return "EUR"
	}
	return c
}

// SumEUR spočíta zostatky v eurách.
func SumEUR(items []OpenItem) money.Dec {
	s := money.Zero
	for _, it := range items {
		s = s.Add(it.RemainEUR)
	}
	return s
}
