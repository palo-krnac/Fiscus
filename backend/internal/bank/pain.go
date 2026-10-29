package bank

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
)

// Hromadný platobný príkaz SEPA (ISO 20022 pain.001.001.03), ktorý sa
// importuje do internet bankingu. Verziu 03 prijímajú všetky slovenské
// banky. Symboly sa podľa zvyklosti SBA zapisujú do EndToEndId v tvare
// „/VS…/SS…/KS…“.

// PainNamespace je menný priestor správy pain.001.001.03.
const PainNamespace = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.03"

// Transfer je jedna úhrada v príkaze.
type Transfer struct {
	Name   string    // príjemca
	IBAN   string    // účet príjemcu
	Amount money.Dec // v eurách
	VS     string
	SS     string
	KS     string
	Info   string // správa pre príjemcu
}

// PaymentOrder je hromadný príkaz z jedného účtu s jedným dátumom splatnosti.
type PaymentOrder struct {
	MsgID      string // jedinečný identifikátor správy (max. 35 znakov)
	Created    time.Time
	Debtor     string // platiteľ (firma)
	DebtorIBAN string // účet platiteľa
	Date       invoice.Date
	Transfers  []Transfer
}

// Total je súčet úhrad v príkaze.
func (o PaymentOrder) Total() money.Dec {
	var s money.Dec
	for _, t := range o.Transfers {
		s = s.Add(t.Amount.RoundCents())
	}
	return s
}

// ErrNoTransfers — príkaz nemá žiadnu úhradu.
var ErrNoTransfers = errors.New("príkaz neobsahuje žiadnu úhradu")

// Check overí príkaz pred zostavením XML.
func (o PaymentOrder) Check() error {
	if len(o.Transfers) == 0 {
		return ErrNoTransfers
	}
	if !party.ValidIBAN(o.DebtorIBAN) {
		return fmt.Errorf("neplatný IBAN platiteľa %q", o.DebtorIBAN)
	}
	if strings.TrimSpace(o.Debtor) == "" {
		return errors.New("chýba názov platiteľa")
	}
	if o.Date.IsZero() {
		return errors.New("chýba dátum splatnosti príkazu")
	}
	for i, t := range o.Transfers {
		switch {
		case strings.TrimSpace(t.Name) == "":
			return fmt.Errorf("úhrada %d: chýba príjemca", i+1)
		case !party.ValidIBAN(t.IBAN):
			return fmt.Errorf("úhrada %d (%s): neplatný IBAN", i+1, t.Name)
		case t.Amount.RoundCents().Sign() <= 0:
			return fmt.Errorf("úhrada %d (%s): suma musí byť kladná", i+1, t.Name)
		}
	}
	return nil
}

// EndToEndID zloží platobné symboly do tvaru „/VS…/SS…/KS…“ (max. 35
// znakov); bez symbolov vráti NOTPROVIDED.
func EndToEndID(vs, ss, ks string) string {
	vs, ss, ks = digits(vs, 10), digits(ss, 10), digits(ks, 4)
	if vs == "" && ss == "" && ks == "" {
		return "NOTPROVIDED"
	}
	return "/VS" + vs + "/SS" + ss + "/KS" + ks
}

func digits(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' && b.Len() < max {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sepaFold prepíše text do znakovej sady SEPA (latinka bez diakritiky,
// číslice a / - ? : ( ) . , ' + medzera) a skráti ho na max znakov.
func sepaFold(s string, max int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		out := string(r)
		if x, ok := sepaMap[r]; ok {
			out = x
		} else if !sepaAllowed(r) {
			out = " "
		}
		for _, c := range out {
			if c == ' ' {
				if space || b.Len() == 0 {
					continue
				}
				space = true
			} else {
				space = false
			}
			b.WriteRune(c)
		}
	}
	res := strings.TrimSpace(b.String())
	if len(res) > max {
		res = strings.TrimSpace(res[:max])
	}
	return res
}

func sepaAllowed(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/-?:().,'+ ", r)
}

var sepaMap = func() map[rune]string {
	m := map[rune]string{'&': "+", 'ß': "ss", 'Æ': "AE", 'æ': "ae", 'Ø': "O", 'ø': "o", 'Ł': "L", 'ł': "l", '„': "'", '“': "'", '”': "'", '‚': "'", '‘': "'", '’': "'", '–': "-", '—': "-", '"': "'", '_': "-", '\t': " "}
	pairs := []string{
		"ÁÀÂÄÃÅĂĄ", "A", "áàâäãåăą", "a", "ČĆÇ", "C", "čćç", "c", "ĎĐ", "D", "ďđ", "d",
		"ÉÈÊËĚĘ", "E", "éèêëěę", "e", "ÍÌÎÏ", "I", "íìîï", "i", "ĹĽĻ", "L", "ĺľļ", "l",
		"ŇŃÑ", "N", "ňńñ", "n", "ÓÒÔÖÕŐ", "O", "óòôöõő", "o", "ŔŘ", "R", "ŕř", "r",
		"ŠŚŞ", "S", "šśş", "s", "ŤŢ", "T", "ťţ", "t", "ÚÙÛÜŮŰ", "U", "úùûüůű", "u",
		"ÝŸ", "Y", "ýÿ", "y", "ŽŹŻ", "Z", "žźż", "z",
	}
	for i := 0; i < len(pairs); i += 2 {
		for _, r := range pairs[i] {
			m[r] = pairs[i+1]
		}
	}
	return m
}()

// XML zostaví správu pain.001.001.03.
func (o PaymentOrder) XML() ([]byte, error) {
	if err := o.Check(); err != nil {
		return nil, err
	}
	type amt struct {
		Ccy   string `xml:"Ccy,attr"`
		Value string `xml:",chardata"`
	}
	type acct struct {
		IBAN string `xml:"Id>IBAN"`
	}
	type agent struct {
		Othr string `xml:"FinInstnId>Othr>Id"`
	}
	type tx struct {
		EndToEndID string  `xml:"PmtId>EndToEndId"`
		Amount     amt     `xml:"Amt>InstdAmt"`
		Cdtr       string  `xml:"Cdtr>Nm"`
		CdtrAcct   acct    `xml:"CdtrAcct"`
		Ustrd      *string `xml:"RmtInf>Ustrd,omitempty"`
	}
	type pmtInf struct {
		ID       string `xml:"PmtInfId"`
		Method   string `xml:"PmtMtd"`
		NbOfTxs  int    `xml:"NbOfTxs"`
		CtrlSum  string `xml:"CtrlSum"`
		Svc      string `xml:"PmtTpInf>SvcLvl>Cd"`
		Date     string `xml:"ReqdExctnDt"`
		Dbtr     string `xml:"Dbtr>Nm"`
		DbtrAcct acct   `xml:"DbtrAcct"`
		DbtrAgt  agent  `xml:"DbtrAgt"`
		ChrgBr   string `xml:"ChrgBr"`
		Txs      []tx   `xml:"CdtTrfTxInf"`
	}
	type doc struct {
		XMLName xml.Name `xml:"Document"`
		NS      string   `xml:"xmlns,attr"`
		MsgID   string   `xml:"CstmrCdtTrfInitn>GrpHdr>MsgId"`
		Created string   `xml:"CstmrCdtTrfInitn>GrpHdr>CreDtTm"`
		NbOfTxs int      `xml:"CstmrCdtTrfInitn>GrpHdr>NbOfTxs"`
		CtrlSum string   `xml:"CstmrCdtTrfInitn>GrpHdr>CtrlSum"`
		Init    string   `xml:"CstmrCdtTrfInitn>GrpHdr>InitgPty>Nm"`
		Pmt     pmtInf   `xml:"CstmrCdtTrfInitn>PmtInf"`
	}
	msgID := sepaFold(o.MsgID, 35)
	total := o.Total().String()
	d := doc{
		NS: PainNamespace, MsgID: msgID, Created: o.Created.Format("2006-01-02T15:04:05"),
		NbOfTxs: len(o.Transfers), CtrlSum: total, Init: sepaFold(o.Debtor, 70),
		Pmt: pmtInf{
			ID: msgID, Method: "TRF", NbOfTxs: len(o.Transfers), CtrlSum: total, Svc: "SEPA",
			Date: o.Date.String(), Dbtr: sepaFold(o.Debtor, 70),
			DbtrAcct: acct{party.NormalizeID(o.DebtorIBAN)}, DbtrAgt: agent{"NOTPROVIDED"}, ChrgBr: "SLEV",
		},
	}
	for _, t := range o.Transfers {
		x := tx{
			EndToEndID: EndToEndID(t.VS, t.SS, t.KS),
			Amount:     amt{"EUR", t.Amount.RoundCents().String()},
			Cdtr:       sepaFold(t.Name, 70),
			CdtrAcct:   acct{party.NormalizeID(t.IBAN)},
		}
		if info := sepaFold(t.Info, 140); info != "" {
			x.Ustrd = &info
		}
		d.Pmt.Txs = append(d.Pmt.Txs, x)
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(d); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
