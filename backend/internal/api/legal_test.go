package api

import (
	"encoding/json"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func TestLegalAPI(t *testing.T) {
	t.Cleanup(func() { vat.SetPeriods(vat.DefaultPeriods) })
	e := setup(t, edition.Desktop)
	var p map[string]any
	e.do("GET", "/api/legal", nil, 200, &p)
	if p["flatFee"] != "40.00" || len(p["ecb"].([]any)) == 0 || len(p["vatPeriods"].([]any)) == 0 {
		t.Fatalf("legislatíva: %v", p)
	}
	p["flatFee"] = "50"
	b, _ := json.Marshal(p)
	var out map[string]any
	e.do("PUT", "/api/legal", string(b), 200, &out)
	if out["flatFee"] != "50.00" {
		t.Fatalf("uloženie: %v", out["flatFee"])
	}
	p["businessMargin"] = "-3"
	b, _ = json.Marshal(p)
	e.do("PUT", "/api/legal", string(b), 422, nil)
	e.do("POST", "/api/legal/reset", nil, 200, &out)
	if out["flatFee"] != "40.00" {
		t.Fatal("obnovenie")
	}
	var codes struct {
		VATPeriods []vat.Period `json:"vatPeriods"`
	}
	e.do("GET", "/api/codes", nil, 200, &codes)
	if len(codes.VATPeriods) != len(vat.DefaultPeriods) {
		t.Fatal("číselník DPH v /api/codes")
	}
}
