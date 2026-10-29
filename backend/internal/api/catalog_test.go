package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestCatalogAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/items", e.cid)
	var it struct {
		ID         int64
		Name       string
		PriceGross string `json:"priceGross"`
	}
	e.do("POST", base, `{"code":"M1","name":"Montáž","unit":"hod","price":"40","vatRate":23}`, 201, &it)
	if it.PriceGross != "49.20" {
		t.Fatalf("cena s DPH: %+v", it)
	}
	e.do("POST", base, `{"code":"m1","name":"Iné","vatRate":23}`, 422, nil)
	e.do("PUT", fmt.Sprintf("%s/%d", base, it.ID), `{"code":"M1","name":"Montáž rozvádzača","unit":"hod","price":"45","priceMode":"gross","vatRate":23}`, 200, &it)
	var list []struct {
		Name     string
		PriceNet string `json:"priceNet"`
	}
	e.do("GET", base, nil, 200, &list)
	if len(list) != 1 || list[0].Name != "Montáž rozvádzača" || list[0].PriceNet != "36.59" {
		t.Fatalf("zoznam: %+v", list)
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, it.ID), nil, 204, nil)
	e.do("DELETE", fmt.Sprintf("%s/%d", base, it.ID), nil, 404, nil)
}
