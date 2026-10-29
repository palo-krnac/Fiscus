package fx

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
	<gesmes:subject>Reference rates</gesmes:subject>
	<Cube>
		<Cube time="2026-10-23">
			<Cube currency="USD" rate="1.0823"/>
			<Cube currency="CZK" rate="25.123"/>
			<Cube currency="XYZ" rate="1"/>
		</Cube>
		<Cube time="2026-10-22">
			<Cube currency="USD" rate="1.0811"/>
			<Cube currency="CZK" rate="25.2"/>
		</Cube>
	</Cube>
</gesmes:Envelope>`

func TestParseECB(t *testing.T) {
	rates, err := ParseECB(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(rates) != 4 || rates[0] != (Rate{"CZK", "2026-10-23", money.MustParse("25.123"), SourceECB}) || rates[3].Date != "2026-10-22" {
		t.Fatalf("kurzy: %+v", rates)
	}
	bad := []string{
		"nie je xml",
		`<Envelope><Cube></Cube></Envelope>`,
		`<Envelope><Cube><Cube time="23.10.2026"><Cube currency="USD" rate="1"/></Cube></Cube></Envelope>`,
		`<Envelope><Cube><Cube time="2026-10-23"><Cube currency="USD" rate="-1"/></Cube></Cube></Envelope>`,
	}
	for _, b := range bad {
		if _, err := ParseECB(strings.NewReader(b)); err == nil {
			t.Errorf("prešlo: %q", b)
		}
	}
}

func TestBefore(t *testing.T) {
	rates, _ := ParseECB(strings.NewReader(sample))
	tests := []struct {
		name, cur, date, want string
	}{
		{"deň po vyhlásení", "USD", "2026-10-24", "2026-10-23"}, // sobota → piatkový kurz
		{"nedeľa", "USD", "2026-10-25", "2026-10-23"},           // víkend: posledný vyhlásený
		{"v deň vyhlásenia platí predošlý", "USD", "2026-10-23", "2026-10-22"},
		{"pred históriou", "USD", "2026-10-22", ""},
		{"iná mena", "GBP", "2026-10-24", ""},
		{"zlý dátum", "USD", "x", ""},
	}
	for _, tt := range tests {
		r, ok := Before(rates, tt.cur, tt.date)
		if got := map[bool]string{true: r.Date, false: ""}[ok]; got != tt.want {
			t.Errorf("%s: %q", tt.name, got)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		r  Rate
		ok bool
	}{
		{Rate{"CZK", "2026-10-23", money.MustParse("25.1"), SourceManual}, true},
		{Rate{"EUR", "2026-10-23", money.MustParse("1"), SourceManual}, false},
		{Rate{"CZK", "23.10.2026", money.MustParse("25.1"), SourceManual}, false},
		{Rate{"CZK", "2026-10-23", money.Zero, SourceManual}, false},
	}
	for _, tt := range tests {
		if (tt.r.Validate() == nil) != tt.ok {
			t.Errorf("%+v: %v", tt.r, tt.r.Validate())
		}
	}
}
