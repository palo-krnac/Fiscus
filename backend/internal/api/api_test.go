package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

type env struct {
	t   *testing.T
	srv *httptest.Server
	cid int64
}

func setup(t *testing.T, mode edition.Mode) *env {
	t.Helper()
	caps, _ := edition.For(mode)
	a := app.New(memstore.New(), caps)
	a.Today = func() invoice.Date { return invoice.MustDate("2026-10-29") }
	c, err := a.CreateCompany(context.Background(), store.Company{Name: "Moja firma", VATPayer: true, ICDPH: "SK2020372640"})
	if err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{
		"index.html":    {Data: []byte("<html>Fiskus</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	srv := httptest.NewServer((&Server{App: a, Web: web}).Handler())
	t.Cleanup(srv.Close)
	return &env{t, srv, c.ID}
}

func (e *env) do(method, path string, body any, wantStatus int, out any) {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if s, ok := body.(string); ok {
			buf.WriteString(s)
		} else {
			_ = json.NewEncoder(&buf).Encode(body)
		}
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &buf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != wantStatus {
		var eb map[string]any
		_ = json.NewDecoder(res.Body).Decode(&eb)
		e.t.Fatalf("%s %s: stav %d, chcem %d (%v)", method, path, res.StatusCode, wantStatus, eb)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			e.t.Fatal(err)
		}
	}
}

const draftJSON = `{
  "customer": {"name": "Odberateľ s.r.o.", "ico": "31322832"},
  "lines": [{"description": "Programovanie", "quantity": "10", "unit": "hod", "unitPrice": "45.50", "vatRate": 23}]
}`

func TestCapabilities(t *testing.T) {
	for _, mode := range []edition.Mode{edition.Desktop, edition.Server} {
		e := setup(t, mode)
		var caps edition.Capabilities
		e.do("GET", "/api/capabilities", nil, 200, &caps)
		if caps.MultiCompany != (mode == edition.Server) {
			t.Errorf("%s: multiCompany = %v", mode, caps.MultiCompany)
		}
	}
}

func TestDesktopRejectsSecondCompany(t *testing.T) {
	e := setup(t, edition.Desktop)
	e.do("POST", "/api/companies", `{"name":"Druhá"}`, 403, nil)
	e2 := setup(t, edition.Server)
	e2.do("POST", "/api/companies", `{"name":"Druhá"}`, 201, nil)
}

func TestInvoiceFlow(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/invoices", e.cid)

	var draft invoice.Invoice
	e.do("POST", base, draftJSON, 201, &draft)
	if draft.Summary.Total.String() != "559.65" || draft.Status != invoice.StatusDraft {
		t.Fatalf("koncept: %+v", draft.Summary)
	}

	// úprava konceptu
	draft.Note = "Ďakujeme"
	var upd invoice.Invoice
	e.do("PUT", fmt.Sprintf("%s/%d", base, draft.ID), draft, 200, &upd)
	if upd.Note != "Ďakujeme" {
		t.Fatal("poznámka sa neuložila")
	}

	var issued invoice.Invoice
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, draft.ID), nil, 200, &issued)
	if issued.Number != "FA20260001" {
		t.Fatalf("číslo %s", issued.Number)
	}
	// vystavená sa nedá meniť
	e.do("PUT", fmt.Sprintf("%s/%d", base, draft.ID), issued, 409, nil)

	var rows []invoiceRow
	e.do("GET", base+"?year=2026", nil, 200, &rows)
	if len(rows) != 1 || rows[0].Overdue {
		t.Fatalf("zoznam: %+v", rows)
	}

	var paid invoice.Invoice
	e.do("POST", fmt.Sprintf("%s/%d/pay", base, draft.ID), `{"date":"2026-11-01"}`, 200, &paid)
	if paid.Status != invoice.StatusPaid || paid.PaidDate.String() != "2026-11-01" {
		t.Fatalf("úhrada: %+v", paid)
	}
}

func TestIssueInvalidReturnsFieldErrors(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	var draft invoice.Invoice
	e.do("POST", base, `{"customer":{"name":""},"lines":[]}`, 201, &draft)
	var body errorBody
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, draft.ID), nil, 422, &body)
	if len(body.Fields) < 2 {
		t.Fatalf("chyby polí: %+v", body)
	}
}

func TestOverdueFlag(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	var d invoice.Invoice
	e.do("POST", base, `{"customer":{"name":"X"},"issueDate":"2026-10-01","dueDate":"2026-10-15",
		"lines":[{"description":"a","quantity":"1","unitPrice":"1","vatRate":23}]}`, 201, &d)
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, d.ID), nil, 200, nil)
	var rows []invoiceRow
	e.do("GET", base, nil, 200, &rows)
	if !rows[0].Overdue || rows[0].DaysOverdue != 14 {
		t.Fatalf("omeškanie: %v %d", rows[0].Overdue, rows[0].DaysOverdue)
	}
}

func TestPreview(t *testing.T) {
	e := setup(t, edition.Desktop)
	var out struct {
		Summary struct{ Total string }
		Errors  []invoice.FieldError
	}
	e.do("POST", fmt.Sprintf("/api/companies/%d/invoices/preview", e.cid), draftJSON, 200, &out)
	if out.Summary.Total != "559.65" {
		t.Fatalf("náhľad: %+v", out)
	}
	e.do("POST", fmt.Sprintf("/api/companies/%d/invoices/preview", e.cid), `{"lines":[{"description":"x","quantity":"1","unitPrice":"1","vatRate":20}]}`, 422, nil)
}

func TestNotFoundAndIsolation(t *testing.T) {
	e := setup(t, edition.Desktop)
	e.do("GET", "/api/companies/999/invoices", nil, 404, nil)
	e.do("GET", fmt.Sprintf("/api/companies/%d/invoices/12345", e.cid), nil, 404, nil)
	e.do("GET", "/api/companies/abc/invoices", nil, 400, nil)
	e.do("GET", "/api/nic", nil, 404, nil)
}

func TestBadJSON(t *testing.T) {
	e := setup(t, edition.Desktop)
	e.do("POST", fmt.Sprintf("/api/companies/%d/invoices", e.cid), `{"lines":[{"quantity":"abc"}]}`, 400, nil)
	e.do("POST", "/api/companies", `{"name":"x","neznamePole":1}`, 400, nil)
}

func TestSPAFallback(t *testing.T) {
	e := setup(t, edition.Desktop)
	for _, p := range []string{"/", "/faktury/12", "/assets/app.js"} {
		res, err := http.Get(e.srv.URL + p)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("%s: %v %d", p, err, res.StatusCode)
		}
		res.Body.Close()
	}
}

func TestContactsAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/contacts", e.cid)
	var c store.Contact
	e.do("POST", base, `{"name":"Kováč Peter","city":"Banská Bystrica"}`, 201, &c)
	e.do("POST", base, `{"name":""}`, 422, nil)
	var list []store.Contact
	e.do("GET", base+"?q=kovac", nil, 200, &list)
	if len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("hľadanie: %+v", list)
	}
	c.Email = "peter@kovac.sk"
	e.do("PUT", fmt.Sprintf("%s/%d", base, c.ID), c, 200, &c)
	e.do("GET", fmt.Sprintf("%s/%d", base, c.ID), nil, 200, &c)
	if c.Email != "peter@kovac.sk" {
		t.Fatal("e-mail sa neuložil")
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, c.ID), nil, 204, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, c.ID), nil, 404, nil)
	e.do("GET", base, nil, 200, &list)
	if len(list) != 0 {
		t.Fatal("prázdny adresár má byť [] a nie null")
	}
}

func TestUpdateCompanyAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	path := fmt.Sprintf("/api/companies/%d", e.cid)
	var c store.Company
	e.do("PUT", path, `{"name":"Nový názov","vatPayer":false,"defaultDueDays":30}`, 200, &c)
	if c.Name != "Nový názov" || c.VATPayer || c.DefaultDueDays != 30 || c.ID != e.cid {
		t.Fatalf("%+v", c)
	}
	e.do("PUT", path, `{"name":""}`, 422, nil)
	e.do("PUT", "/api/companies/999", `{"name":"x"}`, 404, nil)
}
