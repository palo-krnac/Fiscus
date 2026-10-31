package app

import (
	"bytes"
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestInvoicePDF(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	inv := issuedOn(t, a, c.ID, "Odberateľ s.r.o.", "31322832", "2026-10-20")
	tests := []struct {
		name string
		logo []byte
		id   int64
		want string
		err  error
	}{
		{"bez loga", nil, inv.ID, "Faktura-" + inv.Number + ".pdf", nil},
		{"s logom", pngOf(120, 40), inv.ID, "Faktura-" + inv.Number + ".pdf", nil},
		{"neexistuje", nil, 9999, "", store.ErrNotFound},
	}
	for _, tt := range tests {
		if tt.logo != nil {
			if _, err := a.SetLogo(ctx, c.ID, tt.logo); err != nil {
				t.Fatal(err)
			}
		}
		doc, err := a.InvoicePDF(ctx, c.ID, tt.id)
		if !errors.Is(err, tt.err) {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if err != nil {
			continue
		}
		if doc.Name != tt.want || !bytes.HasPrefix(doc.Data, []byte("%PDF-")) {
			t.Errorf("%s: %q %d B", tt.name, doc.Name, len(doc.Data))
		}
		// logo Fiškusa v päte je vždy (obrázok + maska priehľadnosti),
		// logo firmy pribudne iba vtedy, keď je nahraté
		n := bytes.Count(doc.Data, []byte("/Subtype /Image"))
		if n < 2 || (n > 2) != (tt.logo != nil) {
			t.Errorf("%s: obrázkov v PDF %d (logo firmy %v)", tt.name, n, tt.logo != nil)
		}
	}
	if fileKind(invoice.KindCreditNote) != "Dobropis" || fileKind(invoice.KindProforma) != "Zalohova-faktura" {
		t.Error("fileKind")
	}
}

func TestPDFPermissions(t *testing.T) {
	o := newOffice(t)
	d := validDraft()
	dr, err := o.a.SaveDraft(as(o.admin), o.alfa.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.a.InvoicePDF(as(o.jana), o.alfa.ID, dr.ID); err != nil {
		t.Fatalf("účtovníčka: %v", err)
	}
	// klient firmy Beta nevidí faktúru firmy Alfa
	if _, err := o.a.InvoicePDF(as(o.klient), o.alfa.ID, dr.ID); err == nil {
		t.Fatal("cudzia firma")
	}
	if _, err := o.a.InvoicePDF(as(o.klient), o.beta.ID, dr.ID); err == nil {
		t.Fatal("faktúra inej firmy cez ID")
	}
}
