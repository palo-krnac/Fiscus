package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
)

// pdfHandler vráti doklad v PDF. Predvolene ako prílohu na stiahnutie,
// s ?inline=1 na zobrazenie v prehliadači.
func (s *Server) pdfHandler(get func(*app.App, context.Context, int64, int64) (app.Document, error), onErr func(http.ResponseWriter, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, ok := pathID(w, r, "cid")
		if !ok {
			return
		}
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		doc, err := get(s.App, r.Context(), cid, id)
		if err != nil {
			onErr(w, err)
			return
		}
		disp := "attachment"
		if r.URL.Query().Get("inline") == "1" {
			disp = "inline"
		}
		ct := doc.Type
		if ct == "" {
			ct = "application/pdf"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", disp+`; filename="`+doc.Name+`"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(doc.Data)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(doc.Data)
	}
}
