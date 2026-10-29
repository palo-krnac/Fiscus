package mail_test

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/mail/mailtest"
)

func TestValidate(t *testing.T) {
	ok := mail.Settings{Host: "smtp.gmail.com", FromEmail: "faktury@firma.sk"}
	tests := []struct {
		name  string
		mod   func(*mail.Settings)
		field string
	}{
		{"v poriadku", nil, ""},
		{"bez servera", func(s *mail.Settings) { s.Host = "" }, "host"},
		{"server s portom", func(s *mail.Settings) { s.Host = "smtp.gmail.com:587" }, "host"},
		{"zlý port", func(s *mail.Settings) { s.Port = 70000 }, "port"},
		{"zlé zabezpečenie", func(s *mail.Settings) { s.Security = "x" }, "security"},
		{"bez odosielateľa", func(s *mail.Settings) { s.FromEmail = "" }, "fromEmail"},
		{"zlý odosielateľ", func(s *mail.Settings) { s.FromEmail = "faktury" }, "fromEmail"},
		{"zlá odpoveď", func(s *mail.Settings) { s.ReplyTo = "a@b" }, "replyTo"},
		{"meno s koncom riadku", func(s *mail.Settings) { s.FromName = "A\r\nBcc: x@y.sk" }, "fromName"},
		{"zlá adresa upozornení", func(s *mail.Settings) { s.NotifyDrafts = "uctovnik@firma.sk, nie" }, "notifyDrafts"},
		{"dve adresy upozornení", func(s *mail.Settings) { s.NotifyDrafts = "a@firma.sk; b@firma.sk" }, ""},
	}
	for _, tt := range tests {
		s := ok
		if tt.mod != nil {
			tt.mod(&s)
		}
		s.Normalize()
		err := s.Validate()
		var ve *mail.ValidationError
		switch {
		case tt.field == "" && err != nil:
			t.Errorf("%s: %v", tt.name, err)
		case tt.field != "" && (!errors.As(err, &ve) || ve.Fields[0].Field != tt.field):
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		sec  mail.Security
		port int
	}{{"", 587}, {mail.TLS, 465}, {mail.None, 25}}
	for _, tt := range tests {
		s := mail.Settings{Security: tt.sec}
		s.Normalize()
		if s.Port != tt.port {
			t.Errorf("%q: port %d", tt.sec, s.Port)
		}
	}
	def := mail.DefaultTemplates()
	s := mail.Settings{Templates: map[string]mail.Template{
		mail.KindInvoice:  def[mail.KindInvoice],                 // rovnaká ako predvolená → neukladá sa
		mail.KindQuote:    {Subject: "Ponuka {cislo}", Body: ""}, // vlastný predmet
		"iné":             {Subject: "x", Body: "y"},             // neznámy druh
		mail.KindReminder: {Subject: " ", Body: ""},              // prázdna
	}}
	s.Normalize()
	if len(s.Templates) != 1 || s.Template(mail.KindQuote).Subject != "Ponuka {cislo}" ||
		s.Template(mail.KindQuote).Body != def[mail.KindQuote].Body || s.Template(mail.KindInvoice) != def[mail.KindInvoice] {
		t.Fatalf("šablóny: %+v", s.Templates)
	}
}

func TestExpandAndParseList(t *testing.T) {
	vars := map[string]string{"cislo": "FA2026001", "suma": "123,00 €"}
	tests := []struct{ in, want string }{
		{"Faktúra {cislo} na {suma}", "Faktúra FA2026001 na 123,00 €"},
		{"{neznáme} {cislo}", "{neznáme} FA2026001"},
		{"zátvorka { bez konca", "zátvorka { bez konca"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := mail.Expand(tt.in, vars); got != tt.want {
			t.Errorf("Expand(%q) = %q", tt.in, got)
		}
	}
	lists := []struct {
		in   string
		want int
		err  bool
	}{
		{"a@firma.sk", 1, false},
		{"a@firma.sk, b@firma.sk; c@firma.sk", 3, false},
		{" , ", 0, false},
		{"a@firma.sk, zlá", 0, true},
		{"Meno <a@firma.sk>", 0, true},
		{"a@firma.sk\r\nBcc: x@y.sk", 0, true},
	}
	for _, tt := range lists {
		got, err := mail.ParseList(tt.in)
		if (err != nil) != tt.err || len(got) != tt.want {
			t.Errorf("ParseList(%q) = %v %v", tt.in, got, err)
		}
	}
}

func message() mail.Message {
	return mail.Message{
		FromName: "Krnáč – elektroservis", From: "faktury@krnac.sk", ReplyTo: "palo@krnac.sk",
		To: []string{"odberatel@firma.sk"}, Cc: []string{"uctaren@firma.sk"}, Bcc: []string{"faktury@krnac.sk"},
		Subject: "Faktúra FA2026001 – Krnáč", Body: "Dobrý deň,\n\nv prílohe je faktúra. Ďakujeme.\n",
		Attachments: []mail.Attachment{{Name: "Faktura-FA2026001.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.7 obsah")}},
	}
}

// parse prečíta správu ako klient a vráti predmet, text a prílohy.
func parse(t *testing.T, raw string) (*netmail.Message, string, map[string][]byte) {
	t.Helper()
	msg, err := netmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var text string
	files := map[string][]byte{}
	for {
		p, err := mr.NextPart() // dekóduje quoted-printable
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p)
		if name := p.FileName(); name != "" {
			dec, _ := io.ReadAll(base64Reader(string(b)))
			files[name] = dec
		} else {
			text = string(b)
		}
	}
	return msg, text, files
}

func TestBytes(t *testing.T) {
	raw, err := message().Bytes(time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	msg, text, files := parse(t, string(raw))
	dec := new(mime.WordDecoder)
	subj, _ := dec.DecodeHeader(msg.Header.Get("Subject"))
	from, _ := msg.Header.AddressList("From")
	if subj != "Faktúra FA2026001 – Krnáč" || from[0].Name != "Krnáč – elektroservis" || msg.Header.Get("Cc") != "uctaren@firma.sk" ||
		msg.Header.Get("Reply-To") != "palo@krnac.sk" || msg.Header.Get("Bcc") != "" {
		t.Errorf("hlavička: %q %+v %v", subj, from, msg.Header)
	}
	if text != "Dobrý deň,\r\n\r\nv prílohe je faktúra. Ďakujeme.\r\n" {
		t.Errorf("text: %q", text)
	}
	if string(files["Faktura-FA2026001.pdf"]) != "%PDF-1.7 obsah" {
		t.Errorf("príloha: %q", files)
	}
	for _, l := range strings.Split(string(raw), "\r\n") {
		if len(l) > 998 {
			t.Fatal("riadok dlhší ako 998 znakov")
		}
	}
	bad := []func(*mail.Message){
		func(m *mail.Message) { m.To = nil },
		func(m *mail.Message) { m.Subject = "x\r\nBcc: zly@x.sk" },
		func(m *mail.Message) { m.To = []string{"zlá adresa"} },
	}
	for i, mod := range bad {
		m := message()
		mod(&m)
		if _, err := m.Bytes(time.Now()); err == nil {
			t.Errorf("chybná správa %d prešla", i)
		}
	}
	if r := message().Recipients(); len(r) != 3 {
		t.Errorf("príjemcovia: %v", r)
	}
}

func TestSMTP(t *testing.T) {
	tests := []struct {
		name   string
		srv    func(*mailtest.Server)
		set    func(*mail.Settings)
		err    string
		secure bool
	}{
		{"STARTTLS s prihlásením", func(s *mailtest.Server) { s.User, s.Password = "faktury@krnac.sk", "tajne" }, nil, "", true},
		{"SSL/TLS", func(s *mailtest.Server) { s.ImplicitTLS = true; s.User, s.Password = "faktury@krnac.sk", "tajne" },
			func(s *mail.Settings) { s.Security = mail.TLS }, "", true},
		{"zlé heslo", func(s *mailtest.Server) { s.User, s.Password = "faktury@krnac.sk", "ine" }, nil, "skontrolujte meno a heslo", false},
		{"server bez STARTTLS", func(s *mailtest.Server) { s.NoStartTLS = true }, nil, "nepodporuje šifrovanie", false},
		{"bez šifrovania a bez prihlásenia", func(s *mailtest.Server) { s.NoStartTLS = true },
			func(s *mail.Settings) { s.Security = mail.None; s.Username = "" }, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := mailtest.Start(t, tt.srv)
			set := mail.Settings{Host: srv.Addr, Port: srv.Port, Security: mail.StartTLS, Username: "faktury@krnac.sk", Password: "tajne", FromEmail: "faktury@krnac.sk"}
			if tt.set != nil {
				tt.set(&set)
			}
			err := mail.SMTP{RootCAs: srv.RootCAs, Timeout: 5 * time.Second}.Send(context.Background(), set, message())
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("chyba: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := srv.Mails()
			if len(got) != 1 || got[0].From != "faktury@krnac.sk" || len(got[0].To) != 3 || got[0].TLS != tt.secure {
				t.Fatalf("prijaté: %+v", got)
			}
			if _, _, files := parse(t, got[0].Data); len(files) != 1 {
				t.Fatal("príloha neprišla")
			}
		})
	}
	// nedostupný server
	err := mail.SMTP{Timeout: 2 * time.Second}.Send(context.Background(),
		mail.Settings{Host: "127.0.0.1", Port: 1, Security: mail.StartTLS, FromEmail: "a@firma.sk"}, message())
	if err == nil || !strings.Contains(err.Error(), "pripojenie") {
		t.Fatalf("nedostupný server: %v", err)
	}
}
