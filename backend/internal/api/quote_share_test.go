package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestQuoteShareAPI(t *testing.T) {
	e := setup(t, edition.Server)
	base := fmt.Sprintf("/api/companies/%d/quotes", e.cid)
	var q struct {
		ID     int64
		Number string
		Share  *struct{ Token, URL string }
	}
	e.do("POST", base, `{"customer":{"name":"Zákazník s.r.o.","country":"SK"},"lines":[{"description":"Montáž","quantity":"2","unit":"hod","unitPrice":"40","vatRate":23}]}`, 201, &q)
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, q.ID), nil, 200, &q)
	e.do("POST", fmt.Sprintf("%s/%d/share", base, q.ID), nil, 409, nil) // bez verejnej adresy
	e.app.PublicURL = "https://fiskus.example.sk"
	e.do("POST", fmt.Sprintf("%s/%d/share", base, q.ID), nil, 200, &q)
	if q.Share == nil || q.Share.URL != "https://fiskus.example.sk/ponuka/"+q.Share.Token {
		t.Fatalf("odkaz: %+v", q.Share)
	}

	// zákazník bez prihlásenia
	c := e.anonymous()
	pub := "/api/public/quotes/" + q.Share.Token
	var v struct {
		Number     string
		CanRespond bool
		Status     string
	}
	c.do("GET", pub, nil, 200, &v)
	if v.Number != q.Number || !v.CanRespond {
		t.Fatalf("ponuka: %+v", v)
	}
	c.do("POST", pub+"/respond", map[string]any{"accepted": true}, 422, nil)
	c.do("POST", pub+"/respond", map[string]any{"accepted": true, "name": "Ján Novák", "order": "OBJ-7"}, 200, &v)
	if v.Status != "accepted" || v.CanRespond {
		t.Fatalf("po prijatí: %+v", v)
	}
	c.do("POST", pub+"/respond", map[string]any{"name": "Ján"}, 409, nil)
	// interné API ponúk zostáva zatvorené
	c.do("GET", base, nil, 401, nil)

	// hádanie tokenov: po 5 neplatných odkazoch 429
	bad := "/api/public/quotes/" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	for i := 0; i < 5; i++ {
		c.do("GET", bad, nil, 404, nil)
	}
	c.do("GET", pub, nil, 429, nil)

	// zrušenie odkazu
	q.Share = nil
	e.do("DELETE", fmt.Sprintf("%s/%d/share", base, q.ID), nil, 200, &q)
	if q.Share != nil {
		t.Fatal("odkaz ostal")
	}
}
