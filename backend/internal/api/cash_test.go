package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestCashAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/cash", e.cid)
	e.do("POST", base, `{"kind":"out","date":"2026-10-01","amount":"5","purpose":"Známky"}`, 422, nil)
	var d struct {
		ID     int64
		Number string
	}
	e.do("POST", base, `{"kind":"in","date":"2026-10-01","amount":"200","purpose":"Vklad"}`, 201, &d)
	if d.Number != "PPD20260001" {
		t.Fatalf("číslo: %s", d.Number)
	}
	// úhrada faktúry v hotovosti
	var inv struct {
		ID      int64
		Summary struct{ Total string }
	}
	e.do("POST", fmt.Sprintf("/api/companies/%d/invoices", e.cid), draftJSON, 201, &inv)
	e.do("POST", fmt.Sprintf("/api/companies/%d/invoices/%d/issue", e.cid, inv.ID), nil, 200, &inv)
	var cands []struct {
		ID     int64
		Number string
	}
	e.do("GET", base+"/candidates?kind=in", nil, 200, &cands)
	if len(cands) != 1 || cands[0].ID != inv.ID {
		t.Fatalf("kandidáti: %+v", cands)
	}
	var paid struct {
		ID         int64
		LinkNumber string `json:"linkNumber"`
	}
	e.do("POST", base, fmt.Sprintf(`{"kind":"in","date":"2026-10-02","amount":"%s","purpose":"Úhrada","linkKind":"invoice","linkId":%d}`, inv.Summary.Total, inv.ID), 201, &paid)
	var book struct {
		Balance   string
		Documents []any
	}
	e.do("GET", base+"?year=2026", nil, 200, &book)
	if book.Balance != "759.65" || len(book.Documents) != 2 {
		t.Fatalf("kniha: %+v", book)
	}
	e.do("PUT", fmt.Sprintf("%s/%d", base, d.ID), `{"partner":"Majiteľ","purpose":"Vklad hotovosti","note":""}`, 200, nil)
	e.do("POST", fmt.Sprintf("%s/%d/cancel", base, paid.ID), nil, 200, nil)
	e.do("POST", fmt.Sprintf("%s/%d/cancel", base, paid.ID), nil, 409, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, d.ID), nil, 200, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, 999), nil, 404, nil)
}
