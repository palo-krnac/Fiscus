package api

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestBankAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	var inv struct {
		ID             int64
		VariableSymbol string `json:"variableSymbol"`
	}
	e.do("POST", base+"/invoices", draftJSON, 201, &inv)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, inv.ID), nil, 200, &inv)

	stmt := fmt.Sprintf(`<Document><BkToCstmrStmt><Stmt><Acct><Id><IBAN>SK3112000000198742637541</IBAN></Id></Acct>
	<Ntry><Amt Ccy="EUR">559.65</Amt><CdtDbtInd>CRDT</CdtDbtInd><BookgDt><Dt>2026-10-20</Dt></BookgDt><AcctSvcrRef>A1</AcctSvcrRef>
	<NtryDtls><TxDtls><Refs><EndToEndId>/VS%s</EndToEndId></Refs></TxDtls></NtryDtls></Ntry>
	<Ntry><Amt Ccy="EUR">3.00</Amt><CdtDbtInd>DBIT</CdtDbtInd><BookgDt><Dt>2026-10-31</Dt></BookgDt><AcctSvcrRef>A2</AcctSvcrRef></Ntry>
	</Stmt></BkToCstmrStmt></Document>`, inv.VariableSymbol)

	post := func(body string, want int) {
		t.Helper()
		req, _ := http.NewRequest("POST", e.srv.URL+base+"/bank/import?name=vypis.xml", bytes.NewBufferString(body))
		req.Header.Set(CSRFHeader, "1")
		res, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("import: stav %d, chcem %d", res.StatusCode, want)
		}
	}
	post(stmt, 200)
	post("nie je výpis", 422)

	var list []struct {
		ID     int64
		Status string
		Amount string
		Match  *struct{ Number string }
	}
	e.do("GET", base+"/bank", nil, 200, &list)
	if len(list) != 2 || list[1].Status != "matched" || list[1].Match == nil || list[0].Status != "open" {
		t.Fatalf("pohyby: %+v", list)
	}
	var invAfter struct{ Status string }
	e.do("GET", fmt.Sprintf("%s/invoices/%d", base, inv.ID), nil, 200, &invAfter)
	if invAfter.Status != "paid" {
		t.Fatalf("faktúra: %s", invAfter.Status)
	}
	fee := list[0].ID
	var sug struct {
		Suggestions []any `json:"suggestions"`
		Documents   []any `json:"documents"`
	}
	e.do("GET", fmt.Sprintf("%s/bank/%d/suggestions", base, fee), nil, 200, &sug)
	if len(sug.Suggestions) != 0 {
		t.Fatalf("návrhy pre poplatok: %+v", sug)
	}
	e.do("POST", fmt.Sprintf("%s/bank/%d/ignore", base, fee), `{"ignore":true,"note":"poplatok"}`, 200, nil)
	e.do("GET", base+"/bank?status=ignored", nil, 200, &list)
	if len(list) != 1 {
		t.Fatalf("ignorované: %+v", list)
	}
	e.do("POST", fmt.Sprintf("%s/bank/%d/match", base, fee), fmt.Sprintf(`{"kind":"invoice","id":%d}`, inv.ID), 409, nil)
	e.do("GET", base+"/bank?status=matched", nil, 200, &list)
	matched := list[0].ID
	e.do("POST", fmt.Sprintf("%s/bank/%d/unmatch", base, matched), nil, 200, nil)
	e.do("GET", fmt.Sprintf("%s/invoices/%d", base, inv.ID), nil, 200, &invAfter)
	if invAfter.Status != "issued" {
		t.Fatalf("po zrušení spárovania: %s", invAfter.Status)
	}
}
