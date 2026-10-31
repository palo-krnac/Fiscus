package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/ekasa"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Bloček z mobilu: QR kód pokladničného dokladu eKasa → doklad z Finančnej
// správy → prijatý doklad uhradený v deň nákupu, s fotkou bločku.

// MaxReceiptPhoto je najväčšia fotka bločku (telefón ju pred odoslaním zmenší).
const MaxReceiptPhoto = 3 << 20

// ReceiptScan je vstup: text QR kódu, spôsob úhrady a fotka.
type ReceiptScan struct {
	QR        string
	Payment   invoice.PaymentMethod // cash (predvolené) alebo card
	Expense   purchase.ExpenseKind
	Photo     []byte
	PhotoType string
}

// ReceiptResult je zapísaný bloček.
type ReceiptResult struct {
	Purchase purchase.Purchase  `json:"purchase"`
	Receipt  ekasa.Receipt      `json:"receipt"`
	Warnings []purchase.Warning `json:"warnings"`
}

var photoTypes = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

func (a *App) ekasaClient() ekasa.Client {
	hc := a.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	return ekasa.Client{URL: a.EKasaURL, HTTP: hc}
}

// ScanReceipt zapíše bloček podľa QR kódu.
func (a *App) ScanReceipt(ctx context.Context, companyID int64, in ReceiptScan) (ReceiptResult, error) {
	var out ReceiptResult
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return out, err
	}
	if len(in.Photo) > MaxReceiptPhoto {
		return out, mfaFieldErr("photo", "Fotka je príliš veľká (najviac 3 MB)")
	}
	if len(in.Photo) > 0 && photoTypes[in.PhotoType] == "" {
		return out, mfaFieldErr("photo", "Fotka musí byť JPEG, PNG alebo WebP")
	}
	id, err := ekasa.ParseQR(in.QR)
	if err != nil {
		return out, mfaFieldErr("qr", err.Error())
	}
	existing, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{Search: id})
	if err != nil {
		return out, err
	}
	for _, p := range existing {
		if p.SupplierNumber == id {
			return out, mfaFieldErr("qr", fmt.Sprintf("Bloček je už zapísaný ako %s (%s)", p.Number, p.Supplier.Name))
		}
	}
	r, err := a.ekasaClient().Find(ctx, id)
	switch {
	case errors.Is(err, ekasa.ErrNotFound):
		return out, mfaFieldErr("qr", err.Error())
	case err != nil:
		return out, &EshopError{err} // služba nedostupná → 502 so správou
	}
	out.Receipt = r
	p, err := receiptPurchase(r, in)
	if err != nil {
		return out, mfaFieldErr("qr", err.Error())
	}
	saved, warns, err := a.SavePurchase(ctx, companyID, p)
	if err != nil {
		return out, err
	}
	if saved, err = a.MarkPurchasePaid(ctx, companyID, saved.ID, saved.IssueDate); err != nil {
		return out, err
	}
	if len(in.Photo) > 0 {
		f := store.PurchaseFile{CompanyID: companyID, PurchaseID: saved.ID, Name: "blocek-" + saved.Number + "." + photoTypes[in.PhotoType],
			ContentType: in.PhotoType, Data: in.Photo, Created: a.now()}
		if err := a.Store.SavePurchaseFile(ctx, f); err != nil {
			return out, err
		}
	}
	out.Purchase, out.Warnings = saved, warns
	if out.Warnings == nil {
		out.Warnings = []purchase.Warning{}
	}
	return out, nil
}

// receiptPurchase zostaví prijatý doklad z bločku.
func receiptPurchase(r ekasa.Receipt, in ReceiptScan) (purchase.Purchase, error) {
	if r.Issued.IsZero() {
		return purchase.Purchase{}, errors.New("bloček nemá dátum vystavenia")
	}
	t := r.Issued.In(bratislava())
	day := invoice.NewDate(t.Year(), t.Month(), t.Day())
	pay := in.Payment
	if pay != invoice.PayCard {
		pay = invoice.PayCash
	}
	s := r.Seller
	p := purchase.Purchase{SupplierNumber: r.ID, PaymentMethod: pay, Expense: in.Expense,
		Supplier:  invoice.Party{Name: firstNonBlank(s.Name, "Predajca"), Street: s.Street, City: s.City, Zip: s.Zip, Country: "SK", ICO: s.ICO, DIC: s.DIC, ICDPH: s.ICDPH},
		IssueDate: day, DeliveryDate: day, ReceivedDate: day, DueDate: day,
		Description: receiptDescription(r), Note: "Bloček eKasa " + r.ID}
	for _, row := range r.VAT {
		rate := vat.Rate(row.Rate)
		if !rate.ValidOn(day.String()) {
			return p, fmt.Errorf("bloček má sadzbu DPH %d %%, ktorá k %s neplatí", row.Rate, day.Display())
		}
		p.Rates = append(p.Rates, purchase.RateRow{Rate: rate, Base: row.Base, VAT: row.VAT})
	}
	if len(p.Rates) == 0 {
		return p, errors.New("bloček nemá sumy")
	}
	return p, nil
}

// receiptDescription: „Bloček: položka, položka…“ (najviac 120 znakov).
func receiptDescription(r ekasa.Receipt) string {
	var names []string
	for _, it := range r.Items {
		if it.Name != "" {
			names = append(names, it.Name)
		}
	}
	d := "Bloček"
	if len(names) > 0 {
		d += ": " + strings.Join(names, ", ")
	}
	if utf8.RuneCountInString(d) > 120 {
		d = string([]rune(d)[:119]) + "…"
	}
	return d
}

// PurchaseFile vráti súbor prijatého dokladu (fotku bločku).
func (a *App) PurchaseFile(ctx context.Context, companyID, id int64) (store.PurchaseFile, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesRead); err != nil {
		return store.PurchaseFile{}, err
	}
	return a.Store.GetPurchaseFile(ctx, companyID, id)
}
