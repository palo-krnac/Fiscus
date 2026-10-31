package api

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestItemCardAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/items", e.cid)
	var it struct{ ID int64 }
	e.do("POST", base, `{"code":"R1","name":"Relé","unit":"ks","price":"10","vatRate":23,"ean":"5901234123457",
		"params":[{"name":"Napätie","value":"24","unit":"V"}]}`, 201, &it)
	e.do("POST", base, `{"code":"R2","name":"Zlý EAN","vatRate":23,"ean":"5901234123458"}`, 422, nil)
	item := fmt.Sprintf("%s/%d", base, it.ID)

	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 40, 40)))
	var img struct {
		ID          int64
		ContentType string `json:"contentType"`
	}
	e.do("POST", item+"/images?name=predok.png", b.String(), 201, &img)
	e.do("POST", item+"/images?name=x.svg", `<svg onload="alert(1)"/>`, 422, nil)

	var card struct {
		Item struct {
			EAN    string `json:"ean"`
			Params []struct{ Name, Value, Unit string }
		}
		Images []struct{ ID int64 }
	}
	e.do("GET", item+"/card", nil, 200, &card)
	if card.Item.EAN != "5901234123457" || len(card.Item.Params) != 1 || len(card.Images) != 1 || card.Images[0].ID != img.ID {
		t.Fatalf("karta %+v", card)
	}

	res, err := e.client.Get(fmt.Sprintf("%s%s/images/%d", e.srv.URL, item, img.ID))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("X-Content-Type-Options") != "nosniff" ||
		!bytes.Equal(body, b.Bytes()) {
		t.Fatalf("obrázok: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	// obrázok inej položky cez cestu tejto položky sa nevydá
	var other struct{ ID int64 }
	e.do("POST", base, `{"code":"R3","name":"Iné","vatRate":23}`, 201, &other)
	e.do("GET", fmt.Sprintf("%s/%d/images/%d", base, other.ID, img.ID), nil, 404, nil)
	e.do("PUT", item+"/images/order", fmt.Sprintf(`{"ids":[%d]}`, img.ID), 200, nil)
	e.do("DELETE", fmt.Sprintf("%s/images/%d", item, img.ID), nil, 204, nil)
	e.do("GET", fmt.Sprintf("%s/images/%d", item, img.ID), nil, 404, nil)
}
