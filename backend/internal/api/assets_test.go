package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestAssetsAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/assets", e.cid)
	e.do("POST", base, `{"name":"Notebook","group":1,"acquired":"2026-02-01","cost":"900"}`, 422, nil)
	var x struct {
		ID     int64
		Number string
	}
	e.do("POST", base, `{"name":"Dodávka","group":1,"acquired":"2026-01-15","cost":"24000","share":80}`, 201, &x)
	if x.Number != "M001" {
		t.Fatalf("číslo: %+v", x)
	}
	e.do("PUT", fmt.Sprintf("%s/%d", base, x.ID), `{"name":"Dodávka Ford","group":1,"acquired":"2026-01-15","cost":"24000","share":80}`, 200, nil)
	e.do("PUT", fmt.Sprintf("%s/%d", base, 999), `{"name":"X","group":1,"acquired":"2026-01-15","cost":"24000"}`, 404, nil)
	var reg struct {
		Totals struct{ Claimed, Expense string }
		Items  []struct {
			Name     string
			ThisYear struct{ Claimed string } `json:"thisYear"`
		}
		Posted string
	}
	e.do("GET", base+"?year=2026", nil, 200, &reg)
	// 24 000 / 4 = 6 000, celý rok (január), × 80 %
	if len(reg.Items) != 1 || reg.Items[0].Name != "Dodávka Ford" || reg.Totals.Claimed != "4800.00" || reg.Items[0].ThisYear.Claimed != "4800.00" {
		t.Fatalf("register: %+v", reg)
	}
	var posted struct{ Posted string }
	e.do("POST", base+"/depreciation/2026", nil, 200, &posted)
	e.do("POST", base+"/depreciation/abc", nil, 400, nil)
	if posted.Posted != "4800.00" {
		t.Fatalf("zaúčtované: %+v", posted)
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, x.ID), nil, 204, nil)
	e.do("DELETE", fmt.Sprintf("%s/%d", base, x.ID), nil, 404, nil)
}
