package api

import (
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// codes vráti systémové číselníky (rovnaké pre všetky firmy).
func (s *Server) codes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"vatPeriods":     vat.Periods,
		"paymentMethods": invoice.PaymentMethods,
		"defaultUnits":   store.DefaultUnits,
		"defaultFormats": invoice.DefaultFormats,
	})
}
