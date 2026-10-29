package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

type purchaseRow struct {
	purchase.Purchase
	Overdue     bool `json:"overdue"`
	DaysOverdue int  `json:"daysOverdue"`
}

func (s *Server) listPurchases(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	f := store.PurchaseFilter{Search: r.URL.Query().Get("q")}
	if y := r.URL.Query().Get("year"); y != "" {
		var err error
		if f.Year, err = strconv.Atoi(y); err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
			return
		}
	}
	list, err := s.App.Store.ListPurchases(r.Context(), cid, f)
	if err != nil {
		fail(w, err)
		return
	}
	today := s.App.Today()
	rows := make([]purchaseRow, len(list))
	for i, p := range list {
		rows[i] = purchaseRow{Purchase: p, Overdue: p.Overdue(today)}
		if rows[i].Overdue {
			rows[i].DaysOverdue = p.DueDate.DaysUntil(today)
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getPurchase(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, err := s.App.Store.GetPurchase(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) previewPurchase(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.company(w, r); !ok {
		return
	}
	var p purchase.Purchase
	if !decodeLoose(w, r, &p) {
		return
	}
	writeJSON(w, http.StatusOK, s.App.PreviewPurchase(r.Context(), p))
}

func (s *Server) savePurchase(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var p purchase.Purchase
	if !decodeLoose(w, r, &p) {
		return
	}
	status := http.StatusCreated
	if r.Method == http.MethodPut {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		p.ID, status = id, http.StatusOK
	} else {
		p.ID = 0
	}
	saved, warns, err := s.App.SavePurchase(r.Context(), cid, p)
	if err != nil {
		failPurchase(w, err)
		return
	}
	if warns == nil {
		warns = []purchase.Warning{}
	}
	writeJSON(w, status, app.PurchasePreview{Purchase: saved, Errors: []invoice.FieldError{}, Warnings: warns})
}

func (s *Server) payPurchase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date invoice.Date `json:"date"`
	}
	if r.ContentLength > 0 && !decode(w, r, &body) {
		return
	}
	s.purchaseAction(w, r, func(cid, id int64) (purchase.Purchase, error) {
		return s.App.MarkPurchasePaid(r.Context(), cid, id, body.Date)
	})
}

func (s *Server) unpayPurchase(w http.ResponseWriter, r *http.Request) {
	s.purchaseAction(w, r, func(cid, id int64) (purchase.Purchase, error) {
		return s.App.UnpayPurchase(r.Context(), cid, id)
	})
}

func (s *Server) deletePurchase(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeletePurchase(r.Context(), cid, id); err != nil {
		failPurchase(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) purchaseAction(w http.ResponseWriter, r *http.Request, fn func(cid, id int64) (purchase.Purchase, error)) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, err := fn(cid, id)
	if err != nil {
		failPurchase(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func failPurchase(w http.ResponseWriter, err error) {
	if errors.Is(err, purchase.ErrPaid) {
		writeErr(w, http.StatusConflict, err.Error(), nil)
		return
	}
	fail(w, err)
}

// paymentCandidates: GET …/purchases/payment-order — neuhradené faktúry pre príkaz.
func (s *Server) paymentCandidates(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	list, err := s.App.PaymentCandidates(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// paymentOrder: POST …/purchases/payment-order — XML pain.001 na stiahnutie.
func (s *Server) paymentOrder(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs  []int64      `json:"ids"`
		Date invoice.Date `json:"date"`
	}
	if !decode(w, r, &body) {
		return
	}
	name, data, err := s.App.PaymentOrder(r.Context(), cid, body.IDs, body.Date)
	var poe *app.PaymentOrderError
	if errors.As(err, &poe) {
		writeErr(w, http.StatusUnprocessableEntity, poe.Msg, nil)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(data)
}
