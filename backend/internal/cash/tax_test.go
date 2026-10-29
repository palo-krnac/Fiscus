package cash

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

func receipt(rows ...VATRow) Document {
	d := doc(Out, "2026-10-12", "0")
	var sum money.Dec
	for _, r := range rows {
		sum = sum.Add(r.Gross)
	}
	d.Amount = sum
	d.Tax = &Tax{Rows: rows, SupplierICDPH: "sk 2020 372 640", Expense: purchase.ExpenseStock}
	return d
}

func row(rate vat.Rate, gross string) VATRow {
	return VATRow{Rate: rate, Gross: money.MustParse(gross)}
}

func TestNormalizeTax(t *testing.T) {
	d := receipt(row(23, "61.50"), row(5, "10.50"), row(23, "12.30"), row(19, "0"))
	d.NormalizeTax()
	tx := d.Tax
	if len(tx.Rows) != 2 || tx.SupplierICDPH != "SK2020372640" {
		t.Fatalf("%+v", tx)
	}
	if r := tx.Rows[0]; r.Rate != 23 || r.Gross.String() != "73.80" || r.VAT.String() != "13.80" || r.Base.String() != "60.00" {
		t.Fatalf("23 %%: %+v", r)
	}
	if r := tx.Rows[1]; r.VAT.String() != "0.50" || r.Base.String() != "10.00" {
		t.Fatalf("5 %%: %+v", r)
	}
	if b, v, g := tx.Totals(); b.String() != "70.00" || v.String() != "14.30" || g.String() != "84.30" {
		t.Fatalf("súčty %s %s %s", b, v, g)
	}
	if d.Deductible().String() != "14.30" {
		t.Fatal(d.Deductible())
	}
	// príjem: údaje o predajcovi a odpočte sa zahodia
	in := d
	in.Kind = In
	in.Tax = &Tax{Rows: []VATRow{row(23, "12.30")}, SupplierICDPH: "SK1", NoDeduction: true, Expense: purchase.ExpenseOther, Goods: true}
	in.NormalizeTax()
	if in.Tax.SupplierICDPH != "" || in.Tax.NoDeduction || in.Tax.Expense != "" || !in.Tax.Goods || !in.Deductible().IsZero() {
		t.Fatalf("príjem: %+v", in.Tax)
	}
	empty := doc(Out, "2026-10-12", "5")
	empty.Tax = &Tax{}
	empty.NormalizeTax()
	if empty.Tax != nil {
		t.Fatal("prázdny rozpis ostal")
	}
}

func TestValidateTax(t *testing.T) {
	tests := []struct {
		name  string
		mod   func(*Document)
		field string
	}{
		{"platný blok", func(*Document) {}, ""},
		{"súčet nesedí", func(d *Document) { d.Amount = money.MustParse("80") }, "tax.rows"},
		{"bez IČ DPH", func(d *Document) { d.Tax.SupplierICDPH = "" }, "tax.supplierIcdph"},
		{"bez odpočtu nepotrebuje IČ DPH", func(d *Document) { d.Tax.SupplierICDPH, d.Tax.NoDeduction = "", true }, ""},
		{"stará sadzba", func(d *Document) { d.Tax.Rows[0].Rate = 20; d.NormalizeTax() }, "tax.rows.0.vatRate"},
		{"uhrádza faktúru", func(d *Document) { d.LinkKind, d.LinkID = "purchase", 3 }, "tax"},
		{"zlý druh výdavku", func(d *Document) { d.Tax.Expense = "x" }, "tax.expense"},
	}
	for _, tt := range tests {
		d := receipt(row(23, "61.50"), row(5, "10.50"))
		d.NormalizeTax()
		tt.mod(&d)
		err := d.Validate()
		got := ""
		if ve, ok := err.(*invoice.ValidationError); ok && ve != nil {
			for _, f := range ve.Fields {
				got += f.Field + ";"
			}
		}
		if tt.field == "" && got != "" || tt.field != "" && !strings.Contains(got, tt.field+";") {
			t.Errorf("%s: %q", tt.name, got)
		}
	}
}
