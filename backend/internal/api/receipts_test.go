package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestReceiptAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"returnValue":0,"receipt":{"receiptId":"O-7DBCDA8A56EE426DBCDA8A56EE426D1A","issueDate":"28.10.2026 18:45:00",
 "totalPrice":12.30,"taxBaseBasic":10,"vatAmountBasic":2.30,"vatRateBasic":23,"items":[{"name":"Kábel","quantity":1,"vatRate":23,"price":12.30}],
 "organization":{"name":"ELEKTRO s.r.o.","ico":"31322832","icDph":"SK2021757441","municipality":"Zvolen"}}}`)
	}))
	defer fake.Close()
	e.app.EKasaURL = fake.URL

	post := func(qr string, photo []byte, want int) map[string]any {
		t.Helper()
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		mw.WriteField("qr", qr)
		mw.WriteField("payment", "cash")
		if photo != nil {
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", `form-data; name="photo"; filename="blocek.jpg"`)
			h.Set("Content-Type", "image/jpeg")
			pw, _ := mw.CreatePart(h)
			pw.Write(photo)
		}
		mw.Close()
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/companies/%d/receipts", e.srv.URL, e.cid), &b)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set(CSRFHeader, "1")
		res, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("stav %d, chcem %d: %s", res.StatusCode, want, body)
		}
		var out map[string]any
		_ = jsonDecode(res.Body, &out)
		return out
	}
	out := post("O-7DBCDA8A56EE426DBCDA8A56EE426D1A", []byte{0xff, 0xd8, 0xff}, 201)
	id := int64(out["purchase"].(map[string]any)["id"].(float64))
	res, err := e.client.Get(fmt.Sprintf("%s/api/companies/%d/purchases/%d/file", e.srv.URL, e.cid, id))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" || len(body) != 3 || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("fotka %d %s %d", res.StatusCode, res.Header.Get("Content-Type"), len(body))
	}
	post("O-7DBCDA8A56EE426DBCDA8A56EE426D1A", nil, 422) // už zapísaný
	post("nie je bloček", nil, 422)
	e.do("GET", fmt.Sprintf("/api/companies/%d/purchases/%d/file", e.cid, id+100), nil, 404, nil)
}

func jsonDecode(r io.Reader, v any) error { return json.NewDecoder(r).Decode(v) }
