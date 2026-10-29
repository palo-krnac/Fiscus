package api

import (
	"github.com/palo-krnac/fiscus/backend/internal/fx"
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
		"vatPeriods":      vat.Periods(),
		"paymentMethods":  invoice.PaymentMethods,
		"defaultUnits":    store.DefaultUnits,
		"defaultFormats":  defaultFormats(),
		"roles":           auth.Roles,
		"auditCategories": audit.Categories,
		"vatRows":         vatreturn.Rows,
		"currencies":      fx.Currencies,
		"domesticRCs":     invoice.DomesticRCs,
	})
}

func defaultFormats() map[string]string {
	out := map[string]string{}
	for k, v := range app.ExtraFormats {
		out[k] = v
	}
	for k, v := range invoice.DefaultFormats {
		out[string(k)] = v
	}
	return out
}
