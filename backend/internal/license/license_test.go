package license

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"
)

func TestSignParse(t *testing.T) {
	pub, priv, _ := GenerateKey()
	other, _, _ := GenerateKey()
	exp := time.Date(2027, 10, 29, 0, 0, 0, 0, time.UTC)
	l := License{ID: "txn_1", Customer: "Kancelária s.r.o.", Email: "a@b.sk", Plan: PlanMulti, Issued: exp.AddDate(-1, 0, 0), Expires: exp}
	key, err := Sign(l, priv)
	if err != nil {
		t.Fatal(err)
	}
	// zalomený kľúč z e-mailu
	wrapped := key[:30] + "\n  " + key[30:]

	tamper := func(k string) string {
		p := strings.Split(k, ".")
		forged := l
		forged.Expires = exp.AddDate(10, 0, 0)
		f, _ := Sign(forged, priv) // podpísané správne, ale použijeme cudzí podpis
		return p[0] + "." + strings.Split(f, ".")[1] + "." + p[2]
	}

	tests := []struct {
		name string
		key  string
		keys []ed25519.PublicKey
		want error
	}{
		{"platný", key, []ed25519.PublicKey{pub}, nil},
		{"zalomený", wrapped, []ed25519.PublicKey{pub}, nil},
		{"viac kľúčov", key, []ed25519.PublicKey{other, pub}, nil},
		{"cudzí kľúč", key, []ed25519.PublicKey{other}, ErrSignature},
		{"predĺžený podvrhom", tamper(key), []ed25519.PublicKey{pub}, ErrSignature},
		{"nezmysel", "abc", []ed25519.PublicKey{pub}, ErrFormat},
		{"iná predpona", "FISKUS2" + key[7:], []ed25519.PublicKey{pub}, ErrFormat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.key, tt.keys)
			if err != tt.want {
				t.Fatalf("chyba %v, chcem %v", err, tt.want)
			}
			if err == nil && (got.Customer != l.Customer || !got.Expires.Equal(exp)) {
				t.Fatalf("údaje %+v", got)
			}
		})
	}
}

func TestValidity(t *testing.T) {
	exp := time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
	l := License{Plan: PlanMulti, Expires: exp}
	tests := []struct {
		now          time.Time
		valid, grace bool
		warn         string
	}{
		{exp.AddDate(0, -3, 0), true, false, ""},
		{exp.AddDate(0, 0, -10), true, false, "vyprší"},
		{exp.AddDate(0, 0, 10), true, true, "Obnovte"},
		{exp.Add(Grace), false, false, "Nové firmy"},
	}
	for _, tt := range tests {
		s := l.StatusAt(tt.now)
		if l.Valid(tt.now) != tt.valid || l.InGrace(tt.now) != tt.grace || s.Valid != tt.valid || !strings.Contains(s.Warning, tt.warn) || (tt.warn == "" && s.Warning != "") {
			t.Errorf("%s: valid=%v grace=%v %q", tt.now.Format("2006-01-02"), l.Valid(tt.now), l.InGrace(tt.now), s.Warning)
		}
	}
}

func TestTrustedKeyPresent(t *testing.T) {
	if len(TrustedKeys()) == 0 {
		t.Fatal("program musí mať aspoň jeden verejný kľúč")
	}
}
