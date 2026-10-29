package printout

import (
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/payqr"
	"github.com/palo-krnac/fiscus/backend/internal/pdf"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// payCode vráti reťazec PAY by square pre faktúru, alebo "" ak QR nedáva
// zmysel — rovnaké pravidlá ako payModel vo frontende (lib/payqr.js).
func payCode(inv invoice.Invoice, co store.Company) string {
	if co.IBAN == "" || inv.Status != invoice.StatusIssued || inv.Summary.Total.Sign() <= 0 {
		return ""
	}
	if inv.PaymentMethod != "" && inv.PaymentMethod != invoice.PayTransfer {
		return ""
	}
	amount := inv.Summary.Total.String() // „269.12“, „100.00“ → „100“
	if strings.Contains(amount, ".") {
		amount = strings.TrimSuffix(strings.TrimRight(amount, "0"), ".")
	}
	currency := inv.Currency
	if currency == "" {
		currency = "EUR"
	}
	code, err := payqr.Encode(clip(inv.Number, 10), payqr.Payment{
		Amount: amount, Currency: currency, DueDate: strings.ReplaceAll(inv.DueDate.String(), "-", ""),
		VariableSymbol: inv.VariableSymbol, Note: clip("Faktura "+inv.Number, 140), IBAN: co.IBAN,
		Name: clip(co.Name, 70), Street: clip(co.Street, 70), City: clip(joinNonEmpty(" ", co.Zip, co.City), 70),
	})
	if err != nil {
		return ""
	}
	return code
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// qr nakreslí QR kód (bez tichej zóny) do štvorca so stranou size.
func (s *sheet) qr(text string, x, y, size float64) bool {
	q, err := payqr.NewQR(text)
	if err != nil {
		return false
	}
	m := size / float64(q.Size)
	black := pdf.Color{}
	for row, mods := range q.Modules {
		for col := 0; col < q.Size; {
			if !mods[col] {
				col++
				continue
			}
			start := col
			for col < q.Size && mods[col] {
				col++
			}
			// mierne presahy, aby medzi susednými modulmi neostali svetlé čiary
			s.p.Rect(x+float64(start)*m, y+float64(row)*m, float64(col-start)*m+0.05, m+0.05, black)
		}
	}
	return true
}
