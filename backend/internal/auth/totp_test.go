package auth

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238, príloha B (SHA-1), posledných 6 číslic.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // "12345678901234567890"

func TestTOTPVectors(t *testing.T) {
	tests := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}
	for _, tt := range tests {
		got, err := TOTPCode(rfcSecret, time.Unix(tt.unix, 0))
		if err != nil || got != tt.want {
			t.Errorf("%d: %s %v, chcem %s", tt.unix, got, err, tt.want)
		}
	}
}

func TestVerifyTOTP(t *testing.T) {
	now := time.Unix(1234567890, 0)
	code, _ := TOTPCode(rfcSecret, now)
	prev, _ := TOTPCode(rfcSecret, now.Add(-30*time.Second))
	old, _ := TOTPCode(rfcSecret, now.Add(-90*time.Second))
	cur := now.Unix() / 30
	tests := []struct {
		name     string
		code     string
		lastStep int64
		ok       bool
	}{
		{"aktuálny", code, 0, true},
		{"s medzerou", code[:3] + " " + code[3:], 0, true},
		{"predchádzajúci krok (posun hodín)", prev, 0, true},
		{"starý kód", old, 0, false},
		{"už použitý krok", code, cur, false},
		{"zlý kód", "000000", 0, false},
		{"krátky", "12345", 0, false},
		{"písmená", "abcdef", 0, false},
	}
	for _, tt := range tests {
		step, ok := VerifyTOTP(rfcSecret, tt.code, now, tt.lastStep)
		if ok != tt.ok {
			t.Errorf("%s: %v", tt.name, ok)
		}
		if ok && step <= tt.lastStep {
			t.Errorf("%s: krok %d", tt.name, step)
		}
	}
	if _, ok := VerifyTOTP("!!!", code, now, 0); ok {
		t.Error("neplatný kľúč")
	}
}

func TestNewTOTPSecretAndURI(t *testing.T) {
	s, err := NewTOTPSecret()
	if err != nil || len(s) != 32 {
		t.Fatalf("%q %v", s, err)
	}
	if c, err := TOTPCode(s, time.Now()); err != nil || len(c) != 6 {
		t.Fatalf("%q %v", c, err)
	}
	u := TOTPURI("Fiškus", "palo@example.sk", s)
	for _, w := range []string{"otpauth://totp/Fi%C5%A1kus:palo@example.sk?", "secret=" + s, "issuer=Fi%C5%A1kus", "digits=6", "period=30"} {
		if !strings.Contains(u, w) {
			t.Errorf("%s: chýba %s", u, w)
		}
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, hashes, err := NewRecoveryCodes(RecoveryCount)
	if err != nil || len(codes) != RecoveryCount || len(hashes) != RecoveryCount {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 11 || c[5] != '-' || seen[c] || strings.ContainsAny(c, "01lio") {
			t.Fatalf("kód %q", c)
		}
		seen[c] = true
		if IsTOTPCode(c) {
			t.Fatalf("%q nie je kód z aplikácie", c)
		}
	}
	rest, ok := UseRecoveryCode(hashes, " "+strings.ToUpper(codes[3])+" ")
	if !ok || len(rest) != RecoveryCount-1 {
		t.Fatal("použitie kódu")
	}
	if _, ok := UseRecoveryCode(rest, codes[3]); ok {
		t.Fatal("kód platí iba raz")
	}
	if _, ok := UseRecoveryCode(rest, strings.ReplaceAll(codes[4], "-", "")); !ok {
		t.Fatal("kód bez pomlčky")
	}
	if len(hashes) != RecoveryCount {
		t.Fatal("pôvodný zoznam sa nemá meniť")
	}
	for c, want := range map[string]bool{"123456": true, "123 456": true, "12345a": false, "1234567": false} {
		if IsTOTPCode(c) != want {
			t.Errorf("IsTOTPCode(%q)", c)
		}
	}
}
