package reminder

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

const sampleECB = "\xef\xbb\xbfKEY,FREQ,REF_AREA,CURRENCY,PROVIDER_FM,INSTRUMENT_FM,PROVIDER_FM_ID,DATA_TYPE_FM,TIME_PERIOD,OBS_VALUE\n" +
	"FM.B.U2.EUR.4F.KR.MRR_FR.LEV,B,U2,EUR,4F,KR,MRR_FR,LEV,2026-06-15,2.15\n" +
	"FM.B.U2.EUR.4F.KR.MRR_FR.LEV,B,U2,EUR,4F,KR,MRR_FR,LEV,2026-06-17,2.4\n" +
	"FM.B.U2.EUR.4F.KR.MRR_FR.LEV,B,U2,EUR,4F,KR,MRR_FR,LEV,2026-06-16,2.15\n" +
	"FM.B.U2.EUR.4F.KR.MRR_FR.LEV,B,U2,EUR,4F,KR,MRR_FR,LEV,2026-06-18,2.40\n" +
	"FM.B.U2.EUR.4F.KR.MRR_FR.LEV,B,U2,EUR,4F,KR,MRR_FR,LEV,2026-09-16,2.65\n" +
	"FM.B.U2.EUR.4F.KR.MRR_FR.LEV,B,U2,EUR,4F,KR,MRR_FR,LEV,2026-10-28,2.90\n" +
	"FM.B.U2.EUR.4F.KR.MRR_FR.LEV,B,U2,EUR,4F,KR,MRR_FR,LEV,2026-10-29,\n"

func TestParseECBData(t *testing.T) {
	got, err := ParseECBData(strings.NewReader(sampleECB))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-06-15 2.15", "2026-06-17 2.40", "2026-09-16 2.65", "2026-10-28 2.90"}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i, r := range got {
		if s := r.From.String() + " " + r.Rate.String(); s != want[i] {
			t.Errorf("%d: %s, chcem %s", i, s, want[i])
		}
	}
	for _, bad := range []string{"", "A,B\n1,2\n", "TIME_PERIOD,OBS_VALUE\n", "TIME_PERIOD,OBS_VALUE\nx,1\n", "TIME_PERIOD,OBS_VALUE\n2026-01-01,-1\n"} {
		if _, err := ParseECBData(strings.NewReader(bad)); err == nil {
			t.Errorf("prešlo: %q", bad)
		}
	}
}

func TestMergeECB(t *testing.T) {
	fetched, _ := ParseECBData(strings.NewReader(sampleECB))
	table := BuiltinECB() // obsahuje 2026-06-17 a 2026-09-16
	merged, added := MergeECB(table, fetched)
	if len(added) != 1 || added[0].From.String() != "2026-10-28" || !added[0].Rate.Equal(money.MustParse("2.90")) {
		t.Fatalf("pridané: %+v", added)
	}
	if len(merged) != len(table)+1 || !merged.On(invoice.MustDate("2026-11-01")).Equal(money.MustParse("2.90")) {
		t.Fatalf("tabuľka: %+v", merged)
	}
	// ručná oprava v rovnaký deň ostane
	manual := append(BuiltinECB(), Rate{From: invoice.MustDate("2026-10-28"), Rate: money.MustParse("3.00")})
	if _, added := MergeECB(manual, fetched); len(added) != 0 {
		t.Fatalf("prepísaná ručná sadzba: %+v", added)
	}
	// prázdna tabuľka prevezme všetko
	if m, added := MergeECB(nil, fetched); len(added) != 4 || len(m) != 4 {
		t.Fatalf("prázdna: %+v", added)
	}
}
