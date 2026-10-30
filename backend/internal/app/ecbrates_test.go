package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestUpdateECBRates(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	if err := a.LoadLegal(ctx); err != nil {
		t.Fatal(err)
	}
	local := as(auth.Local)
	if _, err := a.UpdateECBRates(local); !errors.Is(err, ErrECBRatesOff) {
		t.Fatalf("vypnuté: %v", err)
	}
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Write([]byte("TIME_PERIOD,OBS_VALUE\n2026-09-15,2.4\n2026-09-16,2.65\n2026-10-28,2.90\n2026-10-29,2.90\n"))
	}))
	defer srv.Close()
	a.ECBRateURL = srv.URL + "/data?format=csvdata"

	res, err := a.UpdateECBRates(local)
	if err != nil || len(res.Added) != 1 || res.Latest.From.String() != "2026-10-28" {
		t.Fatalf("aktualizácia: %+v %v", res, err)
	}
	if !strings.Contains(query, "format=csvdata") || !strings.Contains(query, "startPeriod=2026-08-17") {
		t.Fatalf("dotaz: %s", query)
	}
	if got := a.Law().ECB.On(invoice.MustDate("2026-11-01")); !got.Equal(m.MustParse("2.90")) {
		t.Fatalf("sadzba pre upomienky: %s", got)
	}
	// druhý beh nič nepridá
	if res, err := a.UpdateECBRates(local); err != nil || len(res.Added) != 0 {
		t.Fatalf("druhý beh: %+v %v", res, err)
	}
	// iba správca programu
	user := as(auth.Actor{UserID: 5, Name: "Jana", Channel: auth.ChannelWeb})
	if _, err := a.UpdateECBRates(user); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bežný používateľ: %v", err)
	}
	// chybný server
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 503) }))
	defer bad.Close()
	a.ECBRateURL = bad.URL
	if _, err := a.UpdateECBRates(local); err == nil {
		t.Fatal("chyba servera prešla")
	}
}
