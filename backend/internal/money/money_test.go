package money

import (
	"encoding/json"
	"testing"
)

func TestParseAndString(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"0", "0.00"},
		{"12", "12.00"},
		{"12.5", "12.50"},
		{"12,5", "12.50"},
		{"1 234,56", "1234.56"},
		{"-0.25", "-0.25"},
		{"0.1234", "0.1234"},
		{"0.12345", "0.1235"}, // polovica od nuly
		{"-0.12345", "-0.1235"},
		{"0.12344", "0.1234"},
	}
	for _, c := range cases {
		d, err := Parse(c.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.in, err)
		}
		if got := d.String(); got != c.want {
			t.Errorf("Parse(%q) = %s, chcem %s", c.in, got, c.want)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	for _, s := range []string{"", "abc", "1/3", "1e5", "1.2.3"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) mal zlyhať", s)
		}
	}
}

func TestFloatTrapDoesNotExist(t *testing.T) {
	// 0.1 + 0.2 == 0.3 — s float by to neplatilo.
	if !MustParse("0.1").Add(MustParse("0.2")).Equal(MustParse("0.3")) {
		t.Fatal("0.1 + 0.2 != 0.3")
	}
}

func TestRoundCents(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1.005", "1.01"},
		{"1.0049", "1.00"},
		{"-1.005", "-1.01"},
		{"2.675", "2.68"}, // klasická float pasca
		{"0.0050", "0.01"},
	}
	for _, c := range cases {
		if got := MustParse(c.in).RoundCents().String(); got != c.want {
			t.Errorf("RoundCents(%s) = %s, chcem %s", c.in, got, c.want)
		}
	}
}

func TestMul(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"3", "19.99", "59.97"},
		{"1.5", "0.3333", "0.50"}, // 0.49995 → 0.5000
		{"-2", "1.25", "-2.50"},
		{"0.001", "0.001", "0.00"},
	}
	for _, c := range cases {
		if got := MustParse(c.a).Mul(MustParse(c.b)).String(); got != c.want {
			t.Errorf("%s * %s = %s, chcem %s", c.a, c.b, got, c.want)
		}
	}
}

func TestMulRatio(t *testing.T) {
	// DPH z ceny 123 € s 23 % DPH: 123 * 23 / 123 = 23
	got := MustParse("123").MulRatio(FromInt(23), FromInt(123))
	if got.String() != "23.00" {
		t.Fatalf("got %s", got)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	type doc struct{ Sum Dec }
	b, err := json.Marshal(doc{MustParse("1234.5")})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"Sum":"1234.50"}` {
		t.Fatalf("json = %s", b)
	}
	var back doc
	if err := json.Unmarshal([]byte(`{"Sum":12.3}`), &back); err != nil {
		t.Fatal(err)
	}
	if back.Sum.String() != "12.30" {
		t.Fatalf("got %s", back.Sum)
	}
}

func TestDisplay(t *testing.T) {
	cases := map[string]string{"0": "0,00", "231": "231,00", "1234.5": "1\u00a0234,50", "-1234567.891": "-1\u00a0234\u00a0567,89"}
	for in, want := range cases {
		if got := MustParse(in).Display(); got != want {
			t.Errorf("%s → %q, chcem %q", in, got, want)
		}
	}
}
