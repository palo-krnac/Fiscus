package api

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestLogoAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	path := fmt.Sprintf("/api/companies/%d/logo", e.cid)
	e.do("GET", path, nil, 404, nil)
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 60, 20)))
	e.do("PUT", path, b.String(), 204, nil)
	e.do("PUT", path, `<svg><script>x</script></svg>`, 422, nil)

	res, err := e.client.Get(e.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("X-Content-Type-Options") != "nosniff" ||
		!bytes.Equal(body, b.Bytes()) {
		t.Fatalf("stiahnutie: %d %s %d B", res.StatusCode, res.Header.Get("Content-Type"), len(body))
	}
	// zmena bez hlavičky proti CSRF sa odmietne
	req, _ := http.NewRequest("DELETE", e.srv.URL+path, nil)
	if res, _ := e.client.Do(req); res.StatusCode == 204 {
		t.Fatal("zmazanie bez hlavičky X-Fiskus")
	}
	e.do("DELETE", path, nil, 204, nil)
	e.do("GET", path, nil, 404, nil)
}
