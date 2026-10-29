package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Sender odošle správu. V testoch sa nahradí falošným.
type Sender interface {
	Send(ctx context.Context, s Settings, m Message) error
}

// SMTP posiela správy cez server SMTP z nastavenia firmy.
type SMTP struct {
	// RootCAs: certifikáty dôveryhodných autorít (nil = systémové). Pre testy.
	RootCAs *x509.CertPool
	// Timeout celého odoslania; 0 = 60 s.
	Timeout time.Duration
}

// Send pripojí sa na server, prihlási a odošle správu.
func (x SMTP) Send(ctx context.Context, s Settings, m Message) error {
	data, err := m.Bytes(time.Now())
	if err != nil {
		return err
	}
	timeout := x.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	tlsConf := &tls.Config{ServerName: s.Host, RootCAs: x.RootCAs, MinVersion: tls.VersionTLS12}
	d := &net.Dialer{}
	var conn net.Conn
	if s.Security == TLS {
		conn, err = (&tls.Dialer{NetDialer: d, Config: tlsConf}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return friendly(fmt.Errorf("pripojenie na %s: %w", addr, err))
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return friendly(err)
	}
	defer c.Close()
	if err := c.Hello(helloName(s.FromEmail)); err != nil {
		return friendly(err)
	}
	if s.Security == StartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("server nepodporuje šifrovanie STARTTLS — skúste zabezpečenie SSL/TLS (port 465)")
		}
		if err := c.StartTLS(tlsConf); err != nil {
			return friendly(fmt.Errorf("šifrovanie: %w", err))
		}
	}
	if s.Username != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("server nepodporuje prihlásenie (AUTH)")
		}
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return friendly(fmt.Errorf("prihlásenie: %w", err))
		}
	}
	if err := c.Mail(m.From); err != nil {
		return friendly(fmt.Errorf("odosielateľ: %w", err))
	}
	for _, r := range m.Recipients() {
		if err := c.Rcpt(r); err != nil {
			return friendly(fmt.Errorf("príjemca %s: %w", r, err))
		}
	}
	w, err := c.Data()
	if err != nil {
		return friendly(err)
	}
	if _, err := w.Write(data); err != nil {
		return friendly(err)
	}
	if err := w.Close(); err != nil {
		return friendly(err)
	}
	return c.Quit()
}

// helloName je doména odosielateľa pre EHLO (niektoré servery odmietnu „localhost“).
func helloName(from string) string {
	if i := strings.LastIndex(from, "@"); i >= 0 && i < len(from)-1 {
		return from[i+1:]
	}
	return "localhost"
}

// friendly doplní k častým chybám radu, čo skontrolovať.
func friendly(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "535") || strings.Contains(strings.ToLower(msg), "authentication failed"):
		return fmt.Errorf("%w — skontrolujte meno a heslo (pri Gmaile heslo aplikácie)", err)
	case strings.Contains(msg, "unencrypted connection"):
		return errors.New("prihlásenie bez šifrovania nie je dovolené — zapnite STARTTLS alebo SSL/TLS")
	case strings.Contains(msg, "certificate"):
		return fmt.Errorf("%w — certifikát servera nie je platný pre tento názov", err)
	case strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "deadline exceeded"):
		return fmt.Errorf("%w — server neodpovedá, skontrolujte názov a port", err)
	}
	return err
}
