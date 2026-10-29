package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// camt zostaví výpis s pohybmi: smer, suma, EndToEndId, dátum.
func camt(iban string, entries ...[4]string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.02"><BkToCstmrStmt><Stmt>`)
	fmt.Fprintf(&b, `<Acct><Id><IBAN>%s</IBAN></Id></Acct>`, iban)
	for i, e := range entries {
		fmt.Fprintf(&b, `<Ntry><Amt Ccy="EUR">%s</Amt><CdtDbtInd>%s</CdtDbtInd><Sts>BOOK</Sts><BookgDt><Dt>%s</Dt></BookgDt>
			<AcctSvcrRef>R%d</AcctSvcrRef><NtryDtls><TxDtls><Refs><EndToEndId>%s</EndToEndId></Refs>
			<RltdPties><Dbtr><Nm>Odberateľ s.r.o.</Nm></Dbtr></RltdPties></TxDtls></NtryDtls></Ntry>`, e[1], e[0], e[3], i, e[2])
	}
	b.WriteString(`</Stmt></BkToCstmrStmt></Document>`)
	return []byte(b.String())
}

func TestBankImportAndMatching(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.IBAN = "SK31 1200 0000 1987 4263 7541"
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	// dve vystavené faktúry po 492,00 € (VS 20260001, 20260002) a prijatá 100,00 € (VS 20260815)
	var invs []invoice.Invoice
	for i := 0; i < 2; i++ {
		d, _ := a.SaveDraft(ctx, c.ID, validDraft())
		inv, err := a.Issue(ctx, c.ID, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		invs = append(invs, inv)
	}
	p, _, err := a.SavePurchase(ctx, c.ID, validPurchase())
	if err != nil {
		t.Fatal(err)
	}
	if invs[0].VariableSymbol != "20260001" || invs[0].Summary.Total.String() != "492.00" || p.VariableSymbol != "20260815" {
		t.Fatalf("príprava: %s %s %s", invs[0].VariableSymbol, invs[0].Summary.Total, p.VariableSymbol)
	}

	stmt := camt("SK3112000000198742637541",
		[4]string{"CRDT", "492.00", "/VS20260001/SS/KS0308", "2026-10-20"}, // presná zhoda → auto
		[4]string{"DBIT", "100.00", "/VS20260815", "2026-10-21"},           // prijatá → auto
		[4]string{"CRDT", "200.00", "/VS20260002", "2026-10-22"},           // čiastočná → iba návrh
		[4]string{"DBIT", "2.50", "NOTPROVIDED", "2026-10-31"},             // poplatok
	)
	res, err := a.ImportStatement(ctx, c.ID, "vypis.xml", stmt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 4 || res.AutoMatched != 2 || res.Open != 2 || res.Duplicates != 0 || len(res.Warnings) != 0 {
		t.Fatalf("import: %+v", res)
	}
	got, _ := a.Store.GetInvoice(ctx, c.ID, invs[0].ID)
	if got.Status != invoice.StatusPaid || got.PaidDate.String() != "2026-10-20" {
		t.Fatalf("faktúra po importe: %s %s", got.Status, got.PaidDate)
	}
	gp, _ := a.Store.GetPurchase(ctx, c.ID, p.ID)
	if gp.Status != purchase.StatusPaid || gp.PaidDate.String() != "2026-10-21" {
		t.Fatalf("prijatá po importe: %s %s", gp.Status, gp.PaidDate)
	}

	// opakovaný import
	res, _ = a.ImportStatement(ctx, c.ID, "vypis.xml", stmt)
	if res.Imported != 0 || res.Duplicates != 4 {
		t.Fatalf("opakovaný import: %+v", res)
	}
	// iný účet → upozornenie
	res, _ = a.ImportStatement(ctx, c.ID, "iny.xml", camt("SK0211000000002612345678", [4]string{"CRDT", "1", "x", "2026-10-01"}))
	if len(res.Warnings) != 1 {
		t.Fatalf("iný účet: %+v", res)
	}

	open, _ := a.ListBank(ctx, c.ID, store.BankFilter{Status: bank.StatusOpen})
	var partial, fee bank.Transaction
	for _, x := range open {
		switch x.Amount.String() {
		case "200.00":
			partial = x
		case "2.50":
			fee = x
		}
	}
	sug, err := a.Suggestions(ctx, c.ID, partial.ID)
	if err != nil || len(sug) != 1 || sug[0].ID != invs[1].ID || sug[0].Score != bank.ScoreVS {
		t.Fatalf("návrhy: %+v %v", sug, err)
	}
	docs, _ := a.OpenDocuments(ctx, c.ID, partial.ID)
	if len(docs) != 1 || docs[0].Number != invs[1].Number {
		t.Fatalf("otvorené doklady: %+v", docs)
	}
	// príjem nemožno spárovať s prijatou faktúrou
	if _, err := a.MatchTransaction(ctx, c.ID, partial.ID, "purchase", p.ID); !errors.Is(err, ErrWrongDirection) {
		t.Fatalf("zlý smer: %v", err)
	}
	m, err := a.MatchTransaction(ctx, c.ID, partial.ID, "invoice", invs[1].ID)
	if err != nil || m.Status != bank.StatusMatched || m.Match.Auto || m.Match.Number != invs[1].Number {
		t.Fatalf("ručné spárovanie: %+v %v", m, err)
	}
	if _, err := a.MatchTransaction(ctx, c.ID, partial.ID, "invoice", invs[1].ID); !errors.Is(err, ErrAlreadyMatched) {
		t.Fatalf("dvojité spárovanie: %v", err)
	}
	// zrušenie spárovania vráti faktúru medzi neuhradené
	if _, err := a.UnmatchTransaction(ctx, c.ID, partial.ID); err != nil {
		t.Fatal(err)
	}
	if inv, _ := a.Store.GetInvoice(ctx, c.ID, invs[1].ID); inv.Status != invoice.StatusIssued {
		t.Fatalf("po zrušení: %s", inv.Status)
	}
	// poplatok sa nepáruje
	ig, err := a.IgnoreTransaction(ctx, c.ID, fee.ID, true, "poplatok banke")
	if err != nil || ig.Status != bank.StatusIgnored || ig.Note != "poplatok banke" {
		t.Fatalf("ignorovanie: %+v %v", ig, err)
	}

	list, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatBank})
	if len(list) < 7 { // 3 importy, 2 auto, ručné, zrušenie, ignorovanie
		t.Fatalf("audit banky: %d", len(list))
	}
}

func TestBankPermissions(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	member := func(email string, role auth.Role) auth.Actor {
		mb, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: email, Name: email, Password: "heslo-1234567", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
		return actorOf(u, auth.Actor{Channel: auth.ChannelWeb})
	}
	stmt := camt("SK3112000000198742637541", [4]string{"CRDT", "1", "x", "2026-10-01"})
	tests := []struct {
		name               string
		actor              auth.Actor
		importErr, listErr error
	}{
		{"účtovníčka", o.jana, nil, nil},
		{"klient", member("k@alfa.sk", auth.RoleClient), ErrForbidden, nil},
		{"fakturant", member("f@alfa.sk", auth.RoleInvoicer), ErrForbidden, ErrForbidden},
	}
	for _, tt := range tests {
		if _, err := o.a.ImportStatement(as(tt.actor), o.alfa.ID, "v.xml", stmt); !errors.Is(err, tt.importErr) {
			t.Errorf("%s import: %v", tt.name, err)
		}
		if _, err := o.a.ListBank(as(tt.actor), o.alfa.ID, store.BankFilter{}); !errors.Is(err, tt.listErr) {
			t.Errorf("%s zoznam: %v", tt.name, err)
		}
	}
	if _, err := o.a.ImportStatement(as(o.jana), o.alfa.ID, "v.xml", []byte("nie je výpis")); !errors.Is(err, bank.ErrFormat) {
		t.Errorf("zlý súbor: %v", err)
	}
}
