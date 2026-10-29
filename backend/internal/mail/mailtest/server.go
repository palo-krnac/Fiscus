// Package mailtest je falošný server SMTP pre testy (bez siete mimo localhost).
package mailtest

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// Mail je prijatá správa.
type Mail struct {
	From string
	To   []string
	Data string
	User string // prihlásený používateľ (AUTH PLAIN)
	TLS  bool   // správa prišla šifrovane
}

// Server je falošný server SMTP.
type Server struct {
	Addr     string
	Port     int
	RootCAs  *x509.CertPool
	User     string // ak nie je prázdne, vyžaduje sa prihlásenie
	Password string
	// ImplicitTLS: šifrované od začiatku (port 465); inak ponúka STARTTLS.
	ImplicitTLS bool
	NoStartTLS  bool

	mu    sync.Mutex
	mails []Mail
	ln    net.Listener
	conf  *tls.Config
}

// Start spustí server na náhodnom porte; zastaví sa na konci testu.
func Start(t *testing.T, configure func(*Server)) *Server {
	t.Helper()
	s := &Server{}
	if configure != nil {
		configure(s)
	}
	s.conf, s.RootCAs = selfSigned(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if s.ImplicitTLS {
		ln = tls.NewListener(ln, s.conf)
	}
	s.ln = ln
	s.Addr = "127.0.0.1"
	s.Port = ln.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

// Mails vráti prijaté správy.
func (s *Server) Mails() []Mail {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Mail(nil), s.mails...)
}

func (s *Server) serve(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	_, secure := c.(*tls.Conn)
	r := bufio.NewReader(c)
	w := func(l string) { _, _ = c.Write([]byte(l + "\r\n")) }
	w("220 fiskus-test ESMTP")
	var m Mail
	authed := s.User == ""
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			lines := []string{"250-fiskus-test"}
			if !secure && !s.ImplicitTLS && !s.NoStartTLS {
				lines = append(lines, "250-STARTTLS")
			}
			if s.User != "" {
				lines = append(lines, "250-AUTH PLAIN")
			}
			lines = append(lines, "250 8BITMIME")
			for _, l := range lines {
				w(l)
			}
		case cmd == "STARTTLS":
			w("220 začnite TLS")
			tc := tls.Server(c, s.conf)
			if err := tc.Handshake(); err != nil {
				return
			}
			c, secure = tc, true
			r = bufio.NewReader(c)
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN"):]))
			parts := strings.Split(string(raw), "\x00")
			if len(parts) == 3 && parts[1] == s.User && parts[2] == s.Password {
				authed, m.User = true, parts[1]
				w("235 OK")
			} else {
				w("535 5.7.8 Authentication failed")
			}
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			if !authed {
				w("530 prihláste sa")
				continue
			}
			m.From = addr(line[len("MAIL FROM:"):])
			w("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			m.To = append(m.To, addr(line[len("RCPT TO:"):]))
			w("250 OK")
		case cmd == "DATA":
			w("354 pokračujte")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(strings.TrimPrefix(l, "."))
			}
			m.Data, m.TLS = b.String(), secure
			s.mu.Lock()
			s.mails = append(s.mails, m)
			s.mu.Unlock()
			m = Mail{User: m.User}
			w("250 prijaté")
		case cmd == "QUIT":
			w("221 dovidenia")
			return
		case cmd == "RSET", cmd == "NOOP":
			w("250 OK")
		default:
			w("502 neznámy príkaz")
		}
	}
}

// addr vráti adresu z „<a@b> PARAM=…“.
func addr(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '>'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimLeft(s, "<")
}

// selfSigned vytvorí certifikát pre 127.0.0.1 a jeho dôveryhodnú autoritu.
func selfSigned(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, pool
}
