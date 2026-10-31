package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

const receiptJSON = `{"returnValue":0,"receipt":{"receiptId":"O-7DBCDA8A56EE426DBCDA8A56EE426D1A","receiptNumber":42,
 "issueDate":"28.10.2026 18:45:00","totalPrice":15.75,"taxBaseBasic":10.00,"vatAmountBasic":2.30,"vatRateBasic":23,
 "taxBaseReduced":3.24,"vatAmountReduced":0.21,"vatRateReduced":5,
 "items":[{"name":"Kábel CYKY","quantity":2,"vatRate":23,"price":12.30},{"name":"Príručka","quantity":1,"vatRate":5,"price":3.45}],
 "organization":{"name":"ELEKTRO s.r.o.","ico":"31322832","dic":"2021757441","icDph":"SK2021757441","streetName":"Hlavná","buildingNumber":"12","municipality":"Banská Bystrica","postalCode":"97401"}}}`

func TestScanReceipt(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "O-7DBCDA8A56EE426DBCDA8A56EE426D1A") {
			io.WriteString(w, receiptJSON)
			return
		}
		io.WriteString(w, `{"returnValue":-1,"receipt":null}`)
	}))
	defer srv.Close()
	a.EKasaURL = srv.URL
	photo := []byte{0xff, 0xd8, 0xff, 0xe0}

	res, err := a.ScanReceipt(ctx, c.ID, ReceiptScan{QR: "O-7DBCDA8A56EE426DBCDA8A56EE426D1A", Payment: invoice.PayCard, Photo: photo, PhotoType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	p := res.Purchase
	if p.Status != purchase.StatusPaid || p.PaidDate.String() != "2026-10-28" || p.IssueDate.String() != "2026-10-28" || p.PaymentMethod != invoice.PayCard ||
		p.Supplier.ICDPH != "SK2021757441" || p.Supplier.Street != "Hlavná 12" || p.SupplierNumber != "O-7DBCDA8A56EE426DBCDA8A56EE426D1A" ||
		!strings.HasPrefix(p.Description, "Bloček: Kábel CYKY, Príručka") {
		t.Errorf("doklad %+v", p)
	}
	if len(p.Rates) != 2 || p.Rates[0].Rate != vat.Rate23 || !p.Rates[0].Base.Equal(m.FromInt(10)) || p.Rates[1].Rate != vat.Rate5 {
		t.Errorf("sadzby %+v", p.Rates)
	}
	f, err := a.PurchaseFile(ctx, c.ID, p.ID)
	if err != nil || f.ContentType != "image/jpeg" || len(f.Data) != 4 || !strings.HasSuffix(f.Name, ".jpg") {
		t.Errorf("fotka %+v %v", f, err)
	}

	tests := []struct {
		name  string
		in    ReceiptScan
		field string
		text  string
	}{
		{"už zapísaný", ReceiptScan{QR: "o-7dbcda8a56ee426dbcda8a56ee426d1a"}, "qr", "už zapísaný"},
		{"offline", ReceiptScan{QR: "OKP:1:2:3"}, "qr", "offline"},
		{"iný QR", ReceiptScan{QR: "https://fiskus.sk"}, "qr", "nie je z bločku"},
		{"nepozná", ReceiptScan{QR: "O-00000000000000000000000000000000"}, "qr", "nepozná"},
		{"zlá fotka", ReceiptScan{QR: "O-00000000000000000000000000000000", Photo: []byte{1}, PhotoType: "application/pdf"}, "photo", "JPEG"},
		{"veľká fotka", ReceiptScan{QR: "O-00000000000000000000000000000000", Photo: make([]byte, MaxReceiptPhoto+1), PhotoType: "image/jpeg"}, "photo", "veľká"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := a.ScanReceipt(ctx, c.ID, tt.in)
			if !fieldErr(err, tt.field) || !strings.Contains(errorText(err), tt.text) {
				t.Errorf("chyba %v", err)
			}
		})
	}
	if calls != 2 {
		t.Errorf("volaní eKasa %d (duplicita a zlé QR sa nemajú pýtať)", calls)
	}
}
