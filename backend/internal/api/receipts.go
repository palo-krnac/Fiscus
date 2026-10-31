package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// scanReceipt: POST …/receipts (multipart: qr, payment, expense, photo) —
// bloček eKasa z mobilu zapísaný ako uhradený prijatý doklad.
func (s *Server) scanReceipt(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, app.MaxReceiptPhoto+(64<<10))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Fotka je príliš veľká (najviac 3 MB)", nil)
		return
	}
	in := app.ReceiptScan{QR: r.FormValue("qr"), Payment: invoice.PaymentMethod(r.FormValue("payment")), Expense: purchase.ExpenseKind(r.FormValue("expense"))}
	if f, h, err := r.FormFile("photo"); err == nil {
		in.Photo, err = io.ReadAll(io.LimitReader(f, app.MaxReceiptPhoto+1))
		f.Close()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Fotka sa nedá prečítať", nil)
			return
		}
		in.PhotoType = h.Header.Get("Content-Type")
	}
	out, err := s.App.ScanReceipt(r.Context(), cid, in)
	if err != nil {
		failEshop(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// purchaseFile: GET …/purchases/{id}/file — fotka bločku alebo sken dokladu.
func (s *Server) purchaseFile(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	f, err := s.App.PurchaseFile(r.Context(), cid, id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Doklad nemá súbor", nil)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(f.Data)))
	w.Header().Set("Content-Disposition", `inline; filename="`+f.Name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(f.Data)
}
