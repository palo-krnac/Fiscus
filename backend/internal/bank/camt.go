package bank

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// ErrFormat — súbor nie je výpis camt.053.
var ErrFormat = errors.New("súbor nie je bankový výpis vo formáte camt.053 (XML ISO 20022)")

// Štruktúra camt.053 (verzie 001.02 až 001.08). Menný priestor sa ignoruje,
// porovnávajú sa iba lokálne názvy prvkov.
type camtDoc struct {
	Stmts []camtStmt `xml:"BkToCstmrStmt>Stmt"`
}

type camtStmt struct {
	IBAN    string      `xml:"Acct>Id>IBAN"`
	Other   string      `xml:"Acct>Id>Othr>Id"`
	Entries []camtEntry `xml:"Ntry"`
}

type camtAmt struct {
	Value string `xml:",chardata"`
	Ccy   string `xml:"Ccy,attr"`
}

type camtDate struct {
	Dt   string `xml:"Dt"`
	DtTm string `xml:"DtTm"`
}

func (d camtDate) date() (invoice.Date, error) {
	s := d.Dt
	if s == "" && len(d.DtTm) >= 10 {
		s = d.DtTm[:10]
	}
	return invoice.ParseDate(s)
}

type camtParty struct {
	Nm    string `xml:"Nm"`
	PtyNm string `xml:"Pty>Nm"` // camt.053.001.08
}

func (p camtParty) name() string {
	if p.Nm != "" {
		return p.Nm
	}
	return p.PtyNm
}

type camtTx struct {
	Amt       *camtAmt  `xml:"Amt"`
	TxAmt     *camtAmt  `xml:"AmtDtls>TxAmt>Amt"`
	EndToEnd  string    `xml:"Refs>EndToEndId"`
	AcctRef   string    `xml:"Refs>AcctSvcrRef"`
	Ustrd     []string  `xml:"RmtInf>Ustrd"`
	CdtrRef   []string  `xml:"RmtInf>Strd>CdtrRefInf>Ref"`
	Dbtr      camtParty `xml:"RltdPties>Dbtr"`
	DbtrIBAN  string    `xml:"RltdPties>DbtrAcct>Id>IBAN"`
	Cdtr      camtParty `xml:"RltdPties>Cdtr"`
	CdtrIBAN  string    `xml:"RltdPties>CdtrAcct>Id>IBAN"`
	AddtlInfo string    `xml:"AddtlTxInf"`
}

type camtEntry struct {
	Amt    camtAmt `xml:"Amt"`
	CdtDbt string  `xml:"CdtDbtInd"`
	Sts    struct {
		Text string `xml:",chardata"`
		Cd   string `xml:"Cd"`
	} `xml:"Sts"`
	Booking   camtDate `xml:"BookgDt"`
	Value     camtDate `xml:"ValDt"`
	AcctRef   string   `xml:"AcctSvcrRef"`
	NtryRef   string   `xml:"NtryRef"`
	AddtlInfo string   `xml:"AddtlNtryInf"`
	Txs       []camtTx `xml:"NtryDtls>TxDtls"`
}

// ParseCAMT053 načíta pohyby z výpisu. Pohyb s viacerými transakciami
// (hromadná platba) sa rozdelí na jednotlivé platby.
func ParseCAMT053(data []byte) ([]Transaction, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !bytes.Contains(data, []byte("BkToCstmrStmt")) {
		return nil, ErrFormat
	}
	var doc struct {
		XMLName xml.Name
		camtDoc
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	var out []Transaction
	seen := map[string]int{}
	for _, st := range doc.Stmts {
		account := strings.ToUpper(strings.ReplaceAll(st.IBAN, " ", ""))
		if account == "" {
			account = st.Other
		}
		for _, e := range st.Entries {
			if s := strings.ToUpper(strings.TrimSpace(e.Sts.Cd + e.Sts.Text)); s != "" && !strings.Contains(s, "BOOK") {
				continue // iba zaúčtované pohyby (nie blokácie)
			}
			date, err := e.Booking.date()
			if err != nil {
				if date, err = e.Value.date(); err != nil {
					return nil, fmt.Errorf("%w: pohyb bez dátumu", ErrFormat)
				}
			}
			dir := In
			switch strings.ToUpper(e.CdtDbt) {
			case "DBIT":
				dir = Out
			case "CRDT":
			default:
				return nil, fmt.Errorf("%w: neznámy smer platby %q", ErrFormat, e.CdtDbt)
			}
			txs := e.Txs
			if len(txs) == 0 {
				txs = []camtTx{{}}
			}
			for i, tx := range txs {
				amt := e.Amt
				if len(txs) > 1 {
					switch {
					case tx.Amt != nil:
						amt = *tx.Amt
					case tx.TxAmt != nil:
						amt = *tx.TxAmt
					}
				}
				amount, err := money.Parse(amt.Value)
				if err != nil {
					return nil, fmt.Errorf("%w: suma %q", ErrFormat, amt.Value)
				}
				t := Transaction{
					Account: account, Date: date, Amount: amount, Currency: amt.Ccy, Direction: dir,
					BankRef: firstNonEmpty(tx.AcctRef, e.AcctRef, e.NtryRef),
					Status:  StatusOpen,
				}
				if len(txs) > 1 && t.BankRef != "" && tx.AcctRef == "" {
					t.BankRef = fmt.Sprintf("%s#%d", t.BankRef, i+1)
				}
				msg := strings.TrimSpace(strings.Join(append(tx.Ustrd, tx.AddtlInfo), " "))
				if msg == "" {
					msg = strings.TrimSpace(e.AddtlInfo)
				}
				t.Message = strings.Join(strings.Fields(msg), " ")
				if dir == In {
					t.CounterName, t.CounterIBAN = tx.Dbtr.name(), tx.DbtrIBAN
				} else {
					t.CounterName, t.CounterIBAN = tx.Cdtr.name(), tx.CdtrIBAN
				}
				t.CounterIBAN = strings.ToUpper(strings.ReplaceAll(t.CounterIBAN, " ", ""))
				e2e := tx.EndToEnd
				if strings.EqualFold(e2e, "NOTPROVIDED") {
					e2e = ""
				}
				texts := append([]string{e2e}, tx.CdtrRef...)
				texts = append(texts, t.Message)
				t.VS, t.SS, t.KS = ParseSymbols(texts...)
				if t.VS == "" {
					// SEPA referencia „RF“ alebo čisté číslo v EndToEndId ako VS
					for _, ref := range append(tx.CdtrRef, e2e) {
						if d := strings.TrimLeft(ref, "0"); d != "" && len(d) <= 10 && isDigits(d) {
							t.VS = d
							break
						}
					}
				}
				key := fingerprint(t, 0)
				seen[key]++
				t.Fingerprint = fingerprint(t, seen[key]-1)
				out = append(out, t)
			}
		}
	}
	if len(out) == 0 && len(doc.Stmts) == 0 {
		return nil, ErrFormat
	}
	return out, nil
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
