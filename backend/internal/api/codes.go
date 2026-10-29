package api

import (
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
	"github.com/palo-krnac/fiscus/backend/internal/vatreturn"
)

// codes vráti systémové číselníky (rovnaké pre všetky firmy).
func (s *Server) codes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"vatPeriods":      vat.Periods,
		"paymentMethods":  invoice.PaymentMethods,
		"defaultUnits":    store.DefaultUnits,
		"defaultFormats":  defaultFormats(),
		"roles":           auth.Roles,
		"auditCategories": audit.Categories,
		"vatRows":         vatreturn.Rows,
	})
}

func defaultFormats() map[string]string {
	out := map[string]string{app.PurchaseFormatKey: app.DefaultPurchaseFormat}
	for k, v := range invoice.DefaultFormats {
		out[string(k)] = v
	}
	return out
}
