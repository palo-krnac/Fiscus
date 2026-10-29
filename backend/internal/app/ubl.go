package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/printout"
	"github.com/palo-krnac/fiscus/backend/internal/ubl"
)

// InvoiceUBL vytvorí e-faktúru (UBL 2.1, Peppol BIS Billing 3.0) z vystavenej
// faktúry alebo dobropisu. Chýbajúce údaje vráti ako ubl.IncompleteError.
func (a *App) InvoiceUBL(ctx context.Context, companyID, id int64) (Document, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return Document{}, err
	}
	inv, err := a.Store.GetInvoice(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := ubl.Write(inv, ubl.Seller{Name: co.Name, Street: co.Street, City: co.City, Zip: co.Zip,
		ICO: co.ICO, DIC: co.DIC, ICDPH: co.ICDPH, IBAN: co.IBAN, Email: co.Email, Phone: co.Phone, VATPayer: co.VATPayer})
	if err != nil {
		return Document{}, err
	}
	name := strings.TrimSuffix(printout.FileName(fileKind(inv.Kind), inv.Number), ".pdf") + ".xml"
	return Document{Name: name, Data: data, Type: "application/xml"}, nil
}

// ImportUBL načíta prijatú e-faktúru (UBL) ako koncept prijatej faktúry na
// kontrolu — nič neukladá. Dodávateľa priradí z adresára podľa IČO.
func (a *App) ImportUBL(ctx context.Context, companyID int64, fileName string, data []byte) (PurchaseDraft, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return PurchaseDraft{}, err
	}
	r, err := ubl.Read(data, a.Today())
	if err != nil {
		return PurchaseDraft{}, err
	}
	p := r.Purchase
	normalizePurchase(&p)
	_ = a.applyPurchaseRate(ctx, &p)
	p.Normalize()
	warns := a.completeDraft(ctx, companyID, &p, r.Warnings)
	a.record(ctx, companyID, audit.CatPurchases, "import", "purchase", 0,
		fmt.Sprintf("Načítaná e-faktúra %s: %s %s, %s %s", fileName, p.Supplier.Name, p.SupplierNumber, p.Summary.Total.Display(), currencyOr(p.Currency)), nil, nil)
	return PurchaseDraft{Purchase: p, Warnings: warns}, nil
}

func currencyOr(c string) string {
	if c == "" {
		return "€"
	}
	return c
}
