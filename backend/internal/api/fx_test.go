package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestRatesAPI(t *testing.T) {
	e := setup(t, edition.Desktop)                   // dnes 29. 10. 2026
	e.do("POST", "/api/rates/update", nil, 409, nil) // sťahovanie vypnuté
	e.app.ECBURL = fx.ECB90
	e.app.FetchRates = func(context.Context, string) ([]fx.Rate, error) {
		return []fx.Rate{{Currency: "CZK", Date: "2026-10-23", Rate: money.MustParse("25.123"), Source: fx.SourceECB}}, nil
	}
	var up struct{ Count int }
	e.do("POST", "/api/rates/update", nil, 200, &up)
	e.do("PUT", "/api/rates", `{"currency":"CZK","date":"2026-10-26","rate":"0"}`, 422, nil)
	e.do("PUT", "/api/rates", `{"currency":"USD","date":"2026-10-26","rate":"1.08"}`, 200, nil)
	var list []struct{ Currency, Date, Rate, Source string }
	e.do("GET", "/api/rates?year=2026", nil, 200, &list)
	if up.Count != 1 || len(list) != 2 || list[0].Source != "manual" {
		t.Fatalf("kurzy: %d %+v", up.Count, list)
	}
	var codes struct{ Currencies []struct{ Code string } }
	e.do("GET", "/api/codes", nil, 200, &codes)
	if len(codes.Currencies) < 20 || codes.Currencies[0].Code != "CZK" {
		t.Fatalf("číselník mien: %+v", codes.Currencies)
	}

	// faktúra v korunách: kurz z predchádzajúceho dňa, sumy v eurách
	inv := fmt.Sprintf("/api/companies/%d/invoices", e.cid)
	var pv struct {
		FXRate string `json:"fxRate"`
		FXDate string `json:"fxDate"`
		Local  struct{ Total string }
	}
	e.do("POST", inv+"/preview", `{"customer":{"name":"Odberateľ s.r.o.","country":"SK"},"currency":"CZK","issueDate":"2026-10-26","deliveryDate":"2026-10-24",`+
		`"dueDate":"2026-11-10","lines":[{"description":"Práca","quantity":"10","unit":"hod","unitPrice":"1000","vatRate":23}]}`, 200, &pv)
	// 12 300 Kč / 25,123 = 489,59 €
	if pv.FXRate != "25.123" || pv.FXDate != "2026-10-23" || pv.Local.Total != "489.59" {
		t.Fatalf("náhľad: %+v", pv)
	}
	e.do("DELETE", "/api/rates/USD/2026-10-26", nil, 204, nil)
	e.do("DELETE", "/api/rates/USD/2026-10-26", nil, 404, nil)
}
