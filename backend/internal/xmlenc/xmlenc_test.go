package xmlenc

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestCharsetReader(t *testing.T) {
	tests := []struct {
		name, enc string
		raw       []byte
		want      string
	}{
		{"cp1250", "windows-1250", []byte{'K', 'o', 'v', 0xE1, 0xE8}, "Kováč"},
		{"utf8", "UTF-8", []byte("Kováč"), "Kováč"},
		{"š ž ľ", "cp1250", []byte{0x9A, 0x9E, 0xBE}, "šžľ"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := append([]byte(`<?xml version="1.0" encoding="`+tc.enc+`"?><a>`), append(tc.raw, []byte("</a>")...)...)
			var v struct {
				Text string `xml:",chardata"`
			}
			d := xml.NewDecoder(strings.NewReader(string(doc)))
			d.CharsetReader = CharsetReader
			if err := d.Decode(&v); err != nil || v.Text != tc.want {
				t.Errorf("%q %v", v.Text, err)
			}
		})
	}
	if _, err := CharsetReader("iso-8859-5", strings.NewReader("")); err == nil {
		t.Error("neznáme kódovanie má byť chyba")
	}
}
