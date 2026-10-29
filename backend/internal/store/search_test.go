package store

import "testing"

func TestMatches(t *testing.T) {
	cases := []struct {
		q      string
		fields []string
		want   bool
	}{
		{"kovac", []string{"Ing. Peter Kováč"}, true},
		{"KOVÁČ", []string{"Ing. Peter Kováč"}, true},
		{"zilina", []string{"Firma", "Žilina"}, true},
		{"3132", []string{"Slovnaft", "31322832"}, true},
		{"", []string{"čokoľvek"}, true},
		{"bratislava", []string{"Košice"}, false},
		{"peter  kovac", []string{"Ing. Peter   Kováč"}, true},
	}
	for _, c := range cases {
		if got := Matches(c.q, c.fields...); got != c.want {
			t.Errorf("Matches(%q, %v) = %v", c.q, c.fields, got)
		}
	}
}
