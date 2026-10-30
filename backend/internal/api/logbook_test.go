package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestLogbookAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	var v struct{ ID int64 }
	e.do("POST", base+"/vehicles", `{"name":"Octavia","plate":"BB123AB"}`, 422, nil) // bez spotreby
	e.do("POST", base+"/vehicles", `{"name":"Octavia","plate":"BB123AB","consumption":"5","ownership":"private"}`, 201, &v)
	var tr struct{ ID, Km int64 }
	e.do("POST", base+"/trips", fmt.Sprintf(`{"vehicleId":%d,"date":"2026-10-01","from":"BB","to":"ZV","purpose":"Montáž","odoStart":100,"odoEnd":125}`, v.ID), 201, &tr)
	if tr.Km != 25 {
		t.Fatalf("jazda: %+v", tr)
	}
	e.do("PUT", fmt.Sprintf("%s/trips/%d", base, tr.ID), fmt.Sprintf(`{"vehicleId":%d,"date":"2026-10-01","from":"BB","to":"ZV","purpose":"Montáž","odoStart":100,"odoEnd":130}`, v.ID), 200, &tr)
	e.do("POST", base+"/fuel", fmt.Sprintf(`{"vehicleId":%d,"date":"2026-10-01","quantity":"10","amount":"16.00"}`, v.ID), 201, nil)
	var trips []struct{ Km int64 }
	e.do("GET", fmt.Sprintf("%s/vehicles/%d/trips?year=2026", base, v.ID), nil, 200, &trips)
	if len(trips) != 1 || trips[0].Km != 30 {
		t.Fatalf("jazdy: %+v", trips)
	}
	var rep struct {
		BaseAllowance string
		FuelExpense   string
		Posting       string
	}
	e.do("GET", fmt.Sprintf("%s/vehicles/%d/report?year=2026", base, v.ID), nil, 200, &rep)
	// 30 × 0,313 = 9,39; 30 × 5/100 × 1,6 = 2,40
	if rep.BaseAllowance != "9.39" || rep.FuelExpense != "2.40" || rep.Posting != "11.79" {
		t.Fatalf("vyúčtovanie: %+v", rep)
	}
	e.do("POST", fmt.Sprintf("%s/vehicles/%d/report/2026", base, v.ID), nil, 200, nil)
	e.do("GET", base+"/fuel-docs?year=2026", nil, 200, nil)
	e.do("DELETE", fmt.Sprintf("%s/vehicles/%d", base, v.ID), nil, 422, nil)
	e.do("DELETE", fmt.Sprintf("%s/trips/%d", base, tr.ID), nil, 204, nil)
	e.do("GET", fmt.Sprintf("%s/vehicles/%d/trips", base, 999), nil, 404, nil)
}
