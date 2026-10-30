// Package mail posiela doklady e-mailom cez SMTP firmy: nastavenie servera,
// šablóny textov a zostavenie správy MIME s prílohou PDF.
package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

// Security je zabezpečenie spojenia so serverom SMTP.
type Security string

const (
	StartTLS Security = "starttls" // port 587: šifrovanie po pripojení
	TLS      Security = "tls"      // port 465: šifrované od začiatku
	None     Security = "none"     // bez šifrovania (iba lokálny server)
)

// Settings je nastavenie odosielania e-mailov firmy.
type Settings struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Security Security `json:"security"`
	Username string   `json:"username"`
	// Password sa do API nikdy nevracia (PasswordSet hovorí, či je uložené).
	Password    string `json:"password,omitempty"`
	PasswordSet bool   `json:"passwordSet"`
	FromName    string `json:"fromName"`
	FromEmail   string `json:"fromEmail"`
	ReplyTo     string `json:"replyTo"`
	// BccSelf: kópia každej správy na adresu odosielateľa (do vlastnej pošty).
	BccSelf bool `json:"bccSelf"`
	// NotifyDrafts: adresa pre upozornenia na koncepty z pravidelnej fakturácie (prázdna = neposielať).
	NotifyDrafts string              `json:"notifyDrafts"`
	Templates    map[string]Template `json:"templates"`
}

// Template je predmet a text správy; obsahuje zástupné znaky {cislo} a pod.
type Template struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Druhy šablón.
const (
	KindInvoice  = "invoice"
	KindQuote    = "quote"
	KindReminder = "reminder"
)

// Kinds sú druhy šablón v poradí v nastaveniach.
var Kinds = []string{KindInvoice, KindQuote, KindReminder}

// DefaultTemplates sú predvolené texty správ.
func DefaultTemplates() map[string]Template {
	return map[string]Template{
		KindInvoice: {
			Subject: "{Doklad} {cislo} – {firma}",
			Body: "Dobrý deň,\n\nv prílohe Vám posielame {doklad} {cislo} na sumu {suma}.\n{platba}\n" +
				"Ďakujeme za spoluprácu.\n\nS pozdravom\n{firma}",
		},
		KindQuote: {
			Subject: "Cenová ponuka {cislo} – {firma}",
			Body: "Dobrý deň,\n\nv prílohe Vám posielame cenovú ponuku {cislo} na sumu {suma}. Ponuka platí do {platna_do}.\n\n" +
				"{prijatie}V prípade otázok nás neváhajte kontaktovať.\n\nS pozdravom\n{firma}",
		},
		KindReminder: {
			Subject: "{Doklad} {cislo} – {firma}",
			Body: "Dobrý deň,\n\nv prílohe Vám posielame upomienku k neuhradeným faktúram. Dlžnú sumu {suma} prosíme uhradiť " +
				"najneskôr do {splatnost} na účet {iban}.\n\nAk ste medzičasom zaplatili, považujte túto správu za bezpredmetnú.\n\n" +
				"S pozdravom\n{firma}",
		},
	}
}

// Template vráti šablónu druhu (vlastnú, inak predvolenú).
func (s Settings) Template(kind string) Template {
	t := DefaultTemplates()[kind]
	if own, ok := s.Templates[kind]; ok {
		if strings.TrimSpace(own.Subject) != "" {
			t.Subject = own.Subject
		}
		if strings.TrimSpace(own.Body) != "" {
			t.Body = own.Body
		}
	}
	return t
}

// Configured vráti true, ak je nastavený server a odosielateľ.
func (s Settings) Configured() bool { return s.Host != "" && s.FromEmail != "" }

// FieldError je chyba poľa nastavenia.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError sú chyby nastavenia.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = f.Message
	}
	return strings.Join(msgs, "; ")
}

// Normalize doplní predvolené hodnoty a orezá medzery.
func (s *Settings) Normalize() {
	s.Host = strings.TrimSpace(s.Host)
	s.Username = strings.TrimSpace(s.Username)
	s.FromName = strings.TrimSpace(s.FromName)
	s.FromEmail = strings.TrimSpace(s.FromEmail)
	s.ReplyTo = strings.TrimSpace(s.ReplyTo)
	s.NotifyDrafts = strings.TrimSpace(s.NotifyDrafts)
	if s.Security == "" {
		s.Security = StartTLS
	}
	if s.Port == 0 {
		switch s.Security {
		case TLS:
			s.Port = 465
		case None:
			s.Port = 25
		default:
			s.Port = 587
		}
	}
	// šablóny zhodné s predvolenými sa neukladajú — zmena predvolených sa prejaví
	def := DefaultTemplates()
	for k, t := range s.Templates {
		if _, known := def[k]; !known || (t.Subject == def[k].Subject && t.Body == def[k].Body) ||
			(strings.TrimSpace(t.Subject) == "" && strings.TrimSpace(t.Body) == "") {
			delete(s.Templates, k)
		}
	}
}

// Validate overí nastavenie.
func (s Settings) Validate() error {
	var fs []FieldError
	add := func(field, msg string) { fs = append(fs, FieldError{field, msg}) }
	if s.Host == "" {
		add("host", "Zadajte server SMTP")
	} else if strings.ContainsAny(s.Host, " /:\r\n") {
		add("host", "Zadajte iba názov servera, napr. smtp.gmail.com")
	}
	if s.Port < 1 || s.Port > 65535 {
		add("port", "Port musí byť 1 až 65535")
	}
	switch s.Security {
	case StartTLS, TLS, None:
	default:
		add("security", "Neznáme zabezpečenie")
	}
	if s.FromEmail == "" {
		add("fromEmail", "Zadajte e-mail odosielateľa")
	} else if !ValidAddress(s.FromEmail) {
		add("fromEmail", "Neplatná e-mailová adresa")
	}
	if s.ReplyTo != "" && !ValidAddress(s.ReplyTo) {
		add("replyTo", "Neplatná e-mailová adresa")
	}
	if strings.TrimSpace(s.NotifyDrafts) != "" {
		if _, err := ParseList(s.NotifyDrafts); err != nil {
			add("notifyDrafts", "Neplatná e-mailová adresa")
		}
	}
	if hasNewline(s.FromName) || hasNewline(s.Username) {
		add("fromName", "Meno nesmie obsahovať koniec riadku")
	}
	if len(fs) > 0 {
		return &ValidationError{fs}
	}
	return nil
}

func hasNewline(s string) bool { return strings.ContainsAny(s, "\r\n") }

// ValidAddress vráti true pre jednu holú e-mailovú adresu (bez mena).
func ValidAddress(s string) bool {
	if hasNewline(s) || strings.ContainsAny(s, " <>,;\"") {
		return false
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && strings.Contains(s[strings.LastIndex(s, "@"):], ".")
}

// ParseList rozdelí zoznam adries oddelených čiarkou alebo bodkočiarkou.
// Vráti chybu s neplatnou adresou.
func ParseList(s string) ([]string, error) {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !ValidAddress(p) {
			return nil, fmt.Errorf("neplatná e-mailová adresa „%s“", p)
		}
		out = append(out, p)
	}
	return out, nil
}

// Expand nahradí zástupné znaky {nazov} hodnotami; neznáme ponechá.
func Expand(tpl string, vars map[string]string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(tpl, '{')
		if i < 0 {
			b.WriteString(tpl)
			return b.String()
		}
		j := strings.IndexByte(tpl[i:], '}')
		if j < 0 {
			b.WriteString(tpl)
			return b.String()
		}
		b.WriteString(tpl[:i])
		key := tpl[i+1 : i+j]
		if v, ok := vars[key]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(tpl[i : i+j+1])
		}
		tpl = tpl[i+j+1:]
	}
}

// Attachment je príloha správy.
type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

// Message je správa na odoslanie.
type Message struct {
	FromName, From string
	ReplyTo        string
	To, Cc, Bcc    []string
	Subject        string
	Body           string // čistý text
	Attachments    []Attachment
}

// ErrNoRecipient — správa nemá príjemcu.
var ErrNoRecipient = errors.New("zadajte e-mail príjemcu")

// Recipients vráti všetkých príjemcov (aj skrytú kópiu) bez opakovania.
func (m Message) Recipients() []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{m.To, m.Cc, m.Bcc} {
		for _, a := range list {
			k := strings.ToLower(a)
			if !seen[k] {
				seen[k] = true
				out = append(out, a)
			}
		}
	}
	return out
}

// Bytes zostaví správu vo formáte MIME (riadky CRLF). Hlavička Bcc sa nepíše.
func (m Message) Bytes(date time.Time) ([]byte, error) {
	if len(m.To) == 0 {
		return nil, ErrNoRecipient
	}
	for _, s := range []string{m.FromName, m.Subject, m.ReplyTo} {
		if hasNewline(s) {
			return nil, errors.New("hlavička správy nesmie obsahovať koniec riadku")
		}
	}
	for _, a := range append(append(append([]string{m.From}, m.To...), m.Cc...), m.Bcc...) {
		if !ValidAddress(a) {
			return nil, fmt.Errorf("neplatná e-mailová adresa „%s“", a)
		}
	}
	var buf bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	h("From", (&mail.Address{Name: m.FromName, Address: m.From}).String())
	h("To", strings.Join(m.To, ", "))
	if len(m.Cc) > 0 {
		h("Cc", strings.Join(m.Cc, ", "))
	}
	if m.ReplyTo != "" {
		h("Reply-To", m.ReplyTo)
	}
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", date.Format(time.RFC1123Z))
	h("Message-ID", "<"+randomID()+"@"+m.From[strings.LastIndex(m.From, "@")+1:]+">")
	h("MIME-Version", "1.0")

	mw := multipart.NewWriter(&buf)
	h("Content-Type", `multipart/mixed; boundary="`+mw.Boundary()+`"`)
	buf.WriteString("\r\n")

	tw, _ := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/plain; charset=utf-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	qp := quotedprintable.NewWriter(tw)
	body := strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n")
	if _, err := qp.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	for _, a := range m.Attachments {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		aw, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {mime.FormatMediaType(ct, map[string]string{"name": a.Name})},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": a.Name})},
			"Content-Transfer-Encoding": {"base64"},
		})
		enc := base64.StdEncoding.EncodeToString(a.Data)
		for len(enc) > 76 {
			_, _ = aw.Write([]byte(enc[:76] + "\r\n"))
			enc = enc[76:]
		}
		_, _ = aw.Write([]byte(enc + "\r\n"))
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
