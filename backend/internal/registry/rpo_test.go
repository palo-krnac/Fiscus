package registry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Skutočná odpoveď RPO pre Slovnaft (skrátená) — overený formát.
const slovnaftJSON = `{"results":[{
  "id": 1003617,
  "dbModificationDate": "2026-07-09",
  "identifiers": [{"value": "31322832", "validFrom": "1992-04-29"}],
  "fullNames": [
    {"value": "SLOVNAFT a.s.", "validFrom": "1992-04-29", "validTo": "1998-11-29"},
    {"value": "SLOVNAFT , a.s.", "validFrom": "1998-11-30", "validTo": "2008-05-29"},
    {"value": "SLOVNAFT, a.s.", "validFrom": "2008-05-30"}
  ],
  "addresses": [
    {"validFrom": "2006-06-16", "street": "Vlčie hrdlo", "regNumber": 0, "buildingNumber": "1",
     "postalCodes": ["82412"], "municipality": {"value": "Bratislava"},
     "country": {"value": "Slovenská republika", "code": "703", "codelistCode": "CL000086"}},
    {"validFrom": "1992-04-29", "validTo": "2006-06-15", "street": "Vlčie hrdlo", "regNumber": 0,
     "postalCodes": ["82412"], "municipality": {"value": "Bratislava"},
     "country": {"value": "Slovenská republika", "code": "703", "codelistCode": "CL000086"}}
  ],
  "establishment": "1992-05-01",
  "sourceRegister": {"value": {"value": "Obchodný register", "code": "1", "codelistCode": "CL010112"}}
}]}`

func server(t *testing.T, handler http.HandlerFunc) (*RPO, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return NewRPO(srv.URL + "/rpo/v1/"), &calls
}

func TestByICO(t *testing.T) {
	r, calls := server(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/rpo/v1/search" || req.URL.Query().Get("identifier") != "31322832" {
			t.Errorf("zlá požiadavka %s", req.URL)
		}
		w.Write([]byte(slovnaftJSON))
	})
	c, err := r.ByICO(context.Background(), "31 322 832")
	if err != nil {
		t.Fatal(err)
	}
	want := Company{ICO: "31322832", Name: "SLOVNAFT, a.s.", Street: "Vlčie hrdlo 1", Zip: "824 12",
		City: "Bratislava", Country: "SK", Established: "1992-05-01", Source: "Obchodný register"}
	if c != want {
		t.Fatalf("\n got %+v\nwant %+v", c, want)
	}
	// druhé volanie ide z vyrovnávacej pamäte
	if _, err := r.ByICO(context.Background(), "31322832"); err != nil || atomic.LoadInt32(calls) != 1 {
		t.Fatalf("cache: %v, volaní %d", err, atomic.LoadInt32(calls))
	}
}

func TestInvalidICODoesNotCallRegistry(t *testing.T) {
	r, calls := server(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"results":[]}`)) })
	for _, ico := range []string{"", "123", "12345678", "abcdefgh"} {
		if _, err := r.ByICO(context.Background(), ico); !errors.Is(err, ErrInvalidICO) {
			t.Errorf("%q: %v", ico, err)
		}
	}
	if atomic.LoadInt32(calls) != 0 {
		t.Fatalf("register volaný %d×", atomic.LoadInt32(calls))
	}
}

func TestNotFound(t *testing.T) {
	r, _ := server(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"results":[]}`)) })
	if _, err := r.ByICO(context.Background(), "31322832"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestUnavailable(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"HTTP 500":       func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) },
		"neplatný JSON":  func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html>")) },
		"pomalá odpoveď": func(w http.ResponseWriter, _ *http.Request) { time.Sleep(300 * time.Millisecond) },
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			r, calls := server(t, h)
			r.HTTP.Timeout = 100 * time.Millisecond
			for i := 0; i < 2; i++ {
				if _, err := r.ByICO(context.Background(), "31322832"); !errors.Is(err, ErrUnavailable) {
					t.Fatal(err)
				}
			}
			// každé vyhľadanie skúsi RPO dvakrát; výpadok sa neukladá do pamäte
			if atomic.LoadInt32(calls) != 4 {
				t.Fatalf("volaní %d, chcem 4", atomic.LoadInt32(calls))
			}
		})
	}
}

func TestPickPrefersActiveAndCurrentValues(t *testing.T) {
	results := []entity{
		{ // zaniknutý subjekt s rovnakým IČO
			Identifiers: []timed{{Value: "31322832"}}, FullNames: []timed{{Value: "Stará firma"}},
			Establishment: "1990-01-01", Termination: "2000-01-01",
		},
		{ // iné IČO (vyhľadávanie môže vrátiť viac)
			Identifiers: []timed{{Value: "99999999"}}, FullNames: []timed{{Value: "Cudzia"}},
		},
		{
			Identifiers: []timed{{Value: "31322832"}},
			FullNames:   []timed{{Value: "Nový názov", ValidFrom: "2020-01-01"}, {Value: "Starý názov", ValidFrom: "2010-01-01", ValidTo: "2019-12-31"}},
			Addresses: []address{
				{ValidFrom: "2015-01-01", Street: "", RegNumber: 123, Municipality: codeValue{Value: "Hrochoť"}, PostalCodes: []string{"97637"}},
			},
			Establishment: "2010-01-01",
		},
	}
	c, err := pick(results, "31322832")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Nový názov" || c.Terminated != "" || c.Street != "Hrochoť 123" || c.Zip != "976 37" {
		t.Fatalf("%+v", c)
	}
}

func TestStreetLine(t *testing.T) {
	cases := []struct {
		a    address
		want string
	}{
		{address{Street: "Hlavná", RegNumber: 1234, BuildingNumber: "5"}, "Hlavná 1234/5"},
		{address{Street: "Hlavná", BuildingNumber: "6/A"}, "Hlavná 6/A"},
		{address{Street: "Hlavná", RegNumber: 77}, "Hlavná 77"},
		{address{Street: "Hlavná"}, "Hlavná"},
	}
	for _, c := range cases {
		if got := streetLine(c.a); got != c.want {
			t.Errorf("%+v → %q, chcem %q", c.a, got, c.want)
		}
	}
}

func TestCacheExpires(t *testing.T) {
	r, calls := server(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(slovnaftJSON)) })
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	_, _ = r.ByICO(context.Background(), "31322832")
	now = now.Add(13 * time.Hour)
	_, _ = r.ByICO(context.Background(), "31322832")
	if atomic.LoadInt32(calls) != 2 {
		t.Fatalf("po vypršaní sa má register opýtať znova, volaní %d", atomic.LoadInt32(calls))
	}
}

func TestDefaultBaseURL(t *testing.T) {
	if r := NewRPO(""); !strings.HasPrefix(r.BaseURL, "https://") {
		t.Fatal(r.BaseURL)
	}
}

func TestRetryAndNotFoundTTL(t *testing.T) {
	var n int32
	r, calls := server(t, func(w http.ResponseWriter, _ *http.Request) {
		switch atomic.AddInt32(&n, 1) {
		case 1:
			w.WriteHeader(502) // prvý pokus zlyhá, druhý uspeje
		case 2:
			w.Write([]byte(`{"results":[]}`))
		default:
			w.Write([]byte(slovnaftJSON))
		}
	})
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	if _, err := r.ByICO(context.Background(), "31322832"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("po opakovaní: %v", err)
	}
	now = now.Add(10 * time.Minute)
	if _, err := r.ByICO(context.Background(), "31322832"); !errors.Is(err, ErrNotFound) || atomic.LoadInt32(calls) != 2 {
		t.Fatalf("„nenašla sa“ platí 15 minút: %v %d", err, atomic.LoadInt32(calls))
	}
	now = now.Add(6 * time.Minute)
	if c, err := r.ByICO(context.Background(), "31322832"); err != nil || c.Name != "SLOVNAFT, a.s." {
		t.Fatalf("po 15 minútach sa opýta znova: %+v %v", c, err)
	}
}

func ruzServer(t *testing.T, units map[string]string, ids string) *RUZ {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cruz-public/api/uctovne-jednotky":
			if r.URL.Query().Get("ico") != "31322832" {
				w.Write([]byte(`{"id":[]}`))
				return
			}
			w.Write([]byte(ids))
		case "/cruz-public/api/uctovna-jednotka":
			w.Write([]byte(units[r.URL.Query().Get("id")]))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return NewRUZ(srv.URL + "/cruz-public/api/")
}

func TestRUZ(t *testing.T) {
	// skutočné ID zo RÚZ pre Slovnaft: 1051838 je zmazaný záznam
	r := ruzServer(t, map[string]string{
		"449752":  `{"id":449752,"nazovUJ":"SLOVNAFT, a.s.","ico":"31322832","dic":"2020372640","ulica":"Vlčie hrdlo 1","mesto":"Bratislava - mestská časť Ružinov","psc":"82107","datumZalozenia":"1992-05-01","datumPoslednejUpravy":"2026-07-03"}`,
		"1051838": `{"id":1051838,"stav":"ZMAZANÉ","datumPoslednejUpravy":"2024-05-31"}`,
	}, `{"id":[1051838,449752],"existujeDalsieId":false}`)
	c, err := r.ByICO(context.Background(), "31322832")
	want := Company{ICO: "31322832", Name: "SLOVNAFT, a.s.", Street: "Vlčie hrdlo 1", Zip: "821 07", City: "Bratislava - mestská časť Ružinov",
		Country: "SK", Established: "1992-05-01", DIC: "2020372640", Source: "Register účtovných závierok"}
	if err != nil || c != want {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := r.ByICO(context.Background(), "35763469"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nenájdená: %v", err)
	}
	if _, err := r.ByICO(context.Background(), "123"); !errors.Is(err, ErrInvalidICO) {
		t.Fatal(err)
	}
}

type fixed struct {
	c     Company
	err   error
	calls *int
}

func (f fixed) ByICO(context.Context, string) (Company, error) {
	if f.calls != nil {
		*f.calls++
	}
	return f.c, f.err
}

func TestChain(t *testing.T) {
	rpo := Company{ICO: "31322832", Name: "Z RPO"}
	ruz := Company{ICO: "31322832", Name: "Z RÚZ", DIC: "2020372640"}
	tests := []struct {
		name            string
		primary, second fixed
		wantName, dic   string
		wantErr         error
	}{
		{"RPO nájde, DIČ z RÚZ", fixed{c: rpo}, fixed{c: ruz}, "Z RPO", "2020372640", nil},
		{"RPO nájde, RÚZ mlčí", fixed{c: rpo}, fixed{err: ErrUnavailable}, "Z RPO", "", nil},
		{"RPO nenájde, RÚZ áno", fixed{err: ErrNotFound}, fixed{c: ruz}, "Z RÚZ", "2020372640", nil},
		{"RPO nedostupné, RÚZ áno", fixed{err: ErrUnavailable}, fixed{c: ruz}, "Z RÚZ", "2020372640", nil},
		{"nikde", fixed{err: ErrNotFound}, fixed{err: ErrNotFound}, "", "", ErrNotFound},
		{"RPO nedostupné, RÚZ nenájde", fixed{err: ErrUnavailable}, fixed{err: ErrNotFound}, "", "", ErrUnavailable},
		{"RPO nenájde, RÚZ nedostupné", fixed{err: ErrNotFound}, fixed{err: ErrUnavailable}, "", "", ErrUnavailable},
		{"neplatné IČO sa nehľadá ďalej", fixed{err: ErrInvalidICO}, fixed{c: ruz}, "", "", ErrInvalidICO},
	}
	for _, tt := range tests {
		c, err := Chain{tt.primary, tt.second}.ByICO(context.Background(), "31322832")
		if !errors.Is(err, tt.wantErr) || c.Name != tt.wantName || c.DIC != tt.dic {
			t.Errorf("%s: %+v %v", tt.name, c, err)
		}
	}
}
