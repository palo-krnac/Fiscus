package ekasa

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestParseQR(t *testing.T) {
	tests := []struct {
		in, want string
		err      error
	}{
		{"O-7DBCDA8A56EE426DBCDA8A56EE426D1A", "O-7DBCDA8A56EE426DBCDA8A56EE426D1A", nil},
		{" o-7dbcda8a56ee426dbcda8a56ee426d1a\n", "O-7DBCDA8A56EE426DBCDA8A56EE426D1A", nil},
		{"https://monitoring.ekasa.financnasprava.sk/?id=O-7DBCDA8A56EE426DBCDA8A56EE426D1A", "O-7DBCDA8A56EE426DBCDA8A56EE426D1A", nil},
		{"OKP:88812345678901234:1C2D3E4F:2026-10-31T10:00:00:42:12.50", "", ErrOffline},
		{"SPD*1.0*ACC:SK31", "", ErrNotReceipt},
		{"O-123", "", ErrNotReceipt},
	}
	for _, tt := range tests {
		got, err := ParseQR(tt.in)
		if got != tt.want || err != tt.err {
			t.Errorf("ParseQR(%q) = %q, %v", tt.in, got, err)
		}
	}
}

const sample = `{"returnValue":0,"receipt":{"receiptId":"O-7DBCDA8A56EE426DBCDA8A56EE426D1A","receiptNumber":42,
 "issueDate":"31.10.2026 10:15:30","createDate":"31.10.2026 10:15:31","customerId":null,
 "totalPrice":15.75,"taxBaseBasic":10.00,"vatAmountBasic":2.30,"vatRateBasic":23,"taxBaseReduced":3.24,"vatAmountReduced":0.21,"vatRateReduced":5,"freeTaxAmount":0,
 "items":[{"name":"Kábel","itemType":"K","quantity":2,"vatRate":23,"price":12.30},{"name":"Kniha","itemType":"K","quantity":1,"vatRate":5,"price":3.45}],
 "organization":{"name":"ELEKTRO s.r.o.","ico":"36000001","dic":"2020000001","icDph":"SK2020000001","streetName":"Hlavná","buildingNumber":"12","municipality":"Banská Bystrica","postalCode":"974 01"}},
 "searchIdentifier":{"createDate":1}}`

func TestFind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("požiadavka %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if b["receiptId"] != "O-7DBCDA8A56EE426DBCDA8A56EE426D1A" {
			io.WriteString(w, `{"returnValue":-1,"receipt":null}`)
			return
		}
		io.WriteString(w, sample)
	}))
	defer srv.Close()
	c := Client{URL: srv.URL}
	r, err := c.Find(context.Background(), "O-7DBCDA8A56EE426DBCDA8A56EE426D1A")
	if err != nil {
		t.Fatal(err)
	}
	if r.Number != "42" || r.Seller.Name != "ELEKTRO s.r.o." || r.Seller.Street != "Hlavná 12" || r.Seller.Zip != "97401" || r.Seller.ICDPH != "SK2020000001" ||
		r.Issued.Format("2006-01-02 15:04") != "2026-10-31 10:15" || !r.Total.Equal(money.MustParse("15.75")) || len(r.Items) != 2 {
		t.Errorf("doklad %+v", r)
	}
	if len(r.VAT) != 2 || r.VAT[0].Rate != 23 || !r.VAT[0].Base.Equal(money.FromInt(10)) || r.VAT[1].Rate != 5 || !r.VAT[1].VAT.Equal(money.MustParse("0.21")) {
		t.Errorf("DPH %+v", r.VAT)
	}
	if _, err := c.Find(context.Background(), "O-00000000000000000000000000000000"); err != ErrNotFound {
		t.Errorf("neznámy: %v", err)
	}
}

func TestVATFromItems(t *testing.T) {
	// hlavička bez rekapitulácie: DPH sa dopočíta z položiek
	var raw struct {
		Receipt rawReceipt `json:"receipt"`
	}
	json.Unmarshal([]byte(`{"receipt":{"receiptId":"O-1","totalPrice":"24.60","items":[{"name":"A","quantity":1,"vatRate":23,"price":"24.60"}],"organization":{}}}`), &raw)
	r := raw.Receipt.receipt()
	if len(r.VAT) != 1 || !r.VAT[0].Base.Equal(money.FromInt(20)) || !r.VAT[0].VAT.Equal(money.MustParse("4.60")) {
		t.Errorf("DPH %+v", r.VAT)
	}
}
