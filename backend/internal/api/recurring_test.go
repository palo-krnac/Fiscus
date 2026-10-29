package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestRecurringAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/recurring", e.cid)
	body := `{"name":"Paušál","active":true,"interval":"monthly","nextDate":"2026-11-03","autoIssue":true,"dueDays":10,"subject":"Paušál {mesiac}",` +
		`"customer":{"name":"Zákazník s.r.o.","country":"SK"},` +
		`"lines":[{"description":"Paušál {obdobie}","quantity":"1","unit":"mes","unitPrice":"200","vatRate":23}]}`
	var pv struct {
		Summary struct{ Total string }
		Sample  struct {
			Period  string
			Subject string
			Lines   []string
		}
		Errors []struct{ Field string }
	}
	e.do("POST", base+"/preview", body, 200, &pv)
	if pv.Summary.Total != "246.00" || pv.Sample.Lines[0] != "Paušál november 2026" || pv.Sample.Subject != "Paušál november" || len(pv.Errors) != 0 {
		t.Fatalf("ukážka: %+v", pv)
	}
	e.do("POST", base, `{"interval":"monthly"}`, 422, nil)
	type tpl struct {
		ID       int64
		Active   bool
		NextDate string `json:"nextDate"`
		History  []struct{ Number string }
	}
	var x tpl
	e.do("POST", base, body, 201, &x)
	e.do("PUT", fmt.Sprintf("%s/%d", base, x.ID), body, 200, &x)
	var inv struct {
		ID      int64
		Number  string
		DueDate string `json:"dueDate"`
	}
	e.do("POST", fmt.Sprintf("%s/%d/generate", base, x.ID), nil, 201, &inv)
	if inv.Number != "FA20260001" || inv.DueDate != "2026-11-08" {
		t.Fatalf("faktúra hneď: %+v", inv)
	}
	e.do("POST", fmt.Sprintf("%s/%d/pause", base, x.ID), nil, 200, &x)
	if x.Active {
		t.Fatal("pozastavenie")
	}
	e.do("POST", fmt.Sprintf("%s/%d/resume", base, x.ID), nil, 200, &x)
	var list []tpl
	e.do("GET", base, nil, 200, &list)
	if len(list) != 1 || !list[0].Active || list[0].NextDate != "2026-12-03" || list[0].History[0].Number != "FA20260001" {
		t.Fatalf("zoznam: %+v", list)
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, x.ID), nil, 204, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, x.ID), nil, 404, nil)
}
