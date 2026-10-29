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
		c, err := Chain{Primary: tt.primary, Fallback: tt.second}.ByICO(context.Background(), "31322832")
		if !errors.Is(err, tt.wantErr) || c.Name != tt.wantName || c.DIC != tt.dic {
			t.Errorf("%s: %+v %v", tt.name, c, err)
		}
	}
}

type fakeVAT struct{ valid map[string]bool }

func (f fakeVAT) Check(_ context.Context, id string) (VATCheck, error) {
	return VATCheck{VATID: id, Valid: f.valid[id]}, nil
}

type slow struct {
	fixed
	d time.Duration
}

func (s slow) ByICO(ctx context.Context, ico string) (Company, error) {
	select {
	case <-time.After(s.d):
	case <-ctx.Done():
		return Company{}, ctx.Err()
	}
	return s.fixed.ByICO(ctx, ico)
}

func TestChainVATAndSpeed(t *testing.T) {
	rpo := Company{ICO: "31322832", Name: "Z RPO"}
	ruz := Company{ICO: "31322832", Name: "Z RÚZ", DIC: "2020372640"}
	vat := fakeVAT{valid: map[string]bool{"SK2020372640": true}}
	c, err := Chain{Primary: fixed{c: rpo}, Fallback: fixed{c: ruz}, VAT: vat}.ByICO(context.Background(), "31322832")
	if err != nil || c.DIC != "2020372640" || c.ICDPH != "SK2020372640" || c.Name != "Z RPO" {
		t.Fatalf("IČ DPH z VIES: %+v %v", c, err)
	}
	// neplatiteľ: VIES neplatné → bez IČ DPH
	c, _ = Chain{Primary: fixed{c: rpo}, Fallback: fixed{c: ruz}, VAT: fakeVAT{}}.ByICO(context.Background(), "31322832")
	if c.ICDPH != "" || c.DIC != "2020372640" {
		t.Fatalf("neplatiteľ: %+v", c)
	}
	// pomalý RÚZ nesmie zdržať výsledok z RPO dlhšie ako EnrichWait
	start := time.Now()
	c, err = Chain{Primary: fixed{c: rpo}, Fallback: slow{fixed{c: ruz}, time.Second}, EnrichWait: 50 * time.Millisecond}.ByICO(context.Background(), "31322832")
	if err != nil || c.Name != "Z RPO" || c.DIC != "" || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("čakanie: %+v %v %s", c, err, time.Since(start))
	}
	// RPO a RÚZ bežia súčasne: celkový čas ≈ dlhší z nich, nie súčet
	start = time.Now()
	_, _ = Chain{Primary: slow{fixed{c: rpo}, 200 * time.Millisecond}, Fallback: slow{fixed{c: ruz}, 200 * time.Millisecond}}.ByICO(context.Background(), "31322832")
	if d := time.Since(start); d > 350*time.Millisecond {
		t.Fatalf("hľadanie nie je súbežné: %s", d)
	}
}

func TestVIES(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest-api/ms/CZ/vat/27654321":
			w.Write([]byte(`{"isValid":true,"requestDate":"2026-10-29T18:00:00.000Z","userError":"VALID","name":"BOHEMIA  CONTROL s.r.o.","address":"Purkyňova 99\n61200 Brno"}`))
		case "/rest-api/ms/SK/vat/2020372640":
			w.Write([]byte(`{"isValid":false,"userError":"INVALID","name":"---","address":"---"}`))
		case "/rest-api/ms/DE/vat/123456789":
			w.Write([]byte(`{"isValid":false,"userError":"MS_UNAVAILABLE"}`))
		case "/rest-api/ms/EL/vat/094014201":
			w.Write([]byte(`{"isValid":true,"userError":"VALID","name":"ΕΤΑΙΡΕΙΑ","address":"ΟΔΟΣ 1 10431 ΑΘΗΝΑ"}`))
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	v := NewVIES(srv.URL + "/rest-api/")
	ctx := context.Background()
	c, err := v.Check(ctx, "cz 2765 4321")
	want := VATCheck{VATID: "CZ27654321", Country: "CZ", Valid: true, Name: "BOHEMIA CONTROL s.r.o.", Street: "Purkyňova 99", Zip: "612 00", City: "Brno",
		Address: "Purkyňova 99\n61200 Brno", Checked: "2026-10-29"}
	if err != nil || c.Valid != want.Valid || c.Name != want.Name || c.Street != want.Street || c.Zip != want.Zip || c.City != want.City || c.Checked != want.Checked {
		t.Fatalf("%+v %v", c, err)
	}
	if c, err := v.Check(ctx, "SK2020372640"); err != nil || c.Valid || c.Name != "" {
		t.Fatalf("neplatné: %+v %v", c, err)
	}
	if _, err := v.Check(ctx, "DE123456789"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("štát nedostupný: %v", err)
	}
	if c, err := v.Check(ctx, "GR094014201"); err != nil || c.VATID != "GR094014201" || !c.Valid {
		t.Fatalf("Grécko (EL vo VIES): %+v %v", c, err)
	}
	if _, err := v.Check(ctx, "123"); !errors.Is(err, ErrBadVATID) {
		t.Fatal(err)
	}
}

func TestSplitAddress(t *testing.T) {
	tests := []struct{ in, street, zip, city string }{
		{"Purkyňova 99\n61200 Brno", "Purkyňova 99", "61200", "Brno"},
		{"Hauptstr. 5, 10115 Berlin", "Hauptstr. 5", "10115", "Berlin"},
		{"Ul. Długa 1\n00-950 Warszawa", "Ul. Długa 1", "00-950", "Warszawa"},
		{"Vlčie hrdlo 1\nRužinov\n821 07 Bratislava", "Vlčie hrdlo 1, Ružinov", "821 07", "Bratislava"},
		{"len jeden riadok bez PSČ", "len jeden riadok bez PSČ", "", ""},
		{"---", "", "", ""},
	}
	for _, tt := range tests {
		s, z, c := SplitAddress(tt.in)
		if s != tt.street || z != tt.zip || c != tt.city {
			t.Errorf("%q → %q | %q | %q", tt.in, s, z, c)
		}
	}
}
