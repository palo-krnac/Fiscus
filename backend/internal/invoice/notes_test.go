package invoice

import (
	"strings"
	"testing"
)

func TestExemptionNote(t *testing.T) {
	tests := []struct {
		name    string
		reverse bool
		country string
		kind    SupplyKind
		want    string
	}{
		{"bez prenesenia", false, "DE", SupplyGoods, ""},
		{"tuzemské prenesenie", true, "SK", "", "§ 69 ods. 12"},
		{"tovar do EÚ", true, "DE", SupplyGoods, "§ 43"},
		{"vývoz tovaru", true, "US", SupplyGoods, "§ 47"},
		{"služba do EÚ", true, "CZ", SupplyServices, "čl. 196"},
		{"služba mimo EÚ", true, "CH", SupplyServices, "§ 15 ods. 1"},
	}
	for _, tt := range tests {
		got := ExemptionNote(tt.reverse, Party{Country: tt.country}, tt.kind)
		if (tt.want == "") != (got == "") || !strings.Contains(got, tt.want) {
			t.Errorf("%s: %q", tt.name, got)
		}
	}
}

func TestTitleAndPayment(t *testing.T) {
	tests := []struct {
		inv  Invoice
		want string
	}{
		{Invoice{Kind: KindInvoice}, "Faktúra – daňový doklad"},
		{Invoice{Kind: KindInvoice, NonVATPayer: true}, "Faktúra"},
		{Invoice{Kind: KindCreditNote}, "Dobropis – opravný daňový doklad"},
		{Invoice{Kind: KindCreditNote, NonVATPayer: true}, "Dobropis – opravná faktúra"},
		{Invoice{Kind: KindProforma}, "Zálohová faktúra – nie je daňový doklad"},
		{Invoice{Kind: KindAdvanceReceipt}, "Daňový doklad k prijatej platbe"},
	}
	for _, tt := range tests {
		if got := tt.inv.Title(); got != tt.want {
			t.Errorf("%s: %q", tt.inv.Kind, got)
		}
	}
	if PaymentLabel(PayCash) != "V hotovosti" || PaymentLabel("") != "Prevodom na účet" {
		t.Error("PaymentLabel")
	}
}

func TestDomesticReverseNote(t *testing.T) {
	tests := []struct {
		kinds []DomesticRC
		want  string
	}{
		{nil, ""},
		{[]DomesticRC{"", ""}, ""},
		{[]DomesticRC{RCConstruction}, "písm. j) zákona"},
		{[]DomesticRC{RCConstruction, "", RCMetals, RCConstruction}, "písm. g), j) zákona"},
	}
	for _, tt := range tests {
		var lines []Line
		for _, k := range tt.kinds {
			lines = append(lines, Line{Reverse: k})
		}
		got := DomesticReverseNote(lines)
		if (tt.want == "") != (got == "") || !strings.Contains(got, tt.want) {
			t.Errorf("%v: %q", tt.kinds, got)
		}
	}
}
