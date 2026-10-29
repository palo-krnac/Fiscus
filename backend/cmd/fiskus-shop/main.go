// fiskus-shop prijíma notifikácie Paddle a po platbe pošle zákazníkovi
// licenčný kľúč (serverová verzia pre viac firiem). Beží oddelene od
// Fiškusa, napr. na fiskus.sk za reverznou proxy s HTTPS.
//
//	fiskus-shop -addr 127.0.0.1:8090 -key licencia.key -store licencie.jsonl
//
// Premenné prostredia (tajomstvá nepatria do parametrov ani repozitára):
//
//	PADDLE_WEBHOOK_SECRET  tajomstvo cieľa notifikácií (Paddle → Developer tools → Notifications)
//	PADDLE_PRICE_ID        cena „Server – viac firiem“ (pri_…)
//	PADDLE_API_KEY         nepovinné: dočítanie e-mailu zákazníka (customer.read)
//	PADDLE_SANDBOX=1       sandbox namiesto ostrého Paddle
//	SMTP_HOST, SMTP_PORT, SMTP_SECURITY (starttls|tls|none), SMTP_USER, SMTP_PASSWORD,
//	SMTP_FROM, SMTP_FROM_NAME   odosielanie licencie e-mailom (bez nich sa kľúč pošle ručne)
//
// Webhook: POST /paddle/webhook (udalosť transaction.completed). Kontrola: GET /healthz.
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/shop"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "adresa HTTP servera (za reverznou proxy s HTTPS)")
	keyFile := flag.String("key", "licencia.key", "súkromný kľúč na podpis licencií (fiskus-license keygen)")
	storePath := flag.String("store", "licencie.jsonl", "evidencia vydaných licencií")
	months := flag.Int("months", 12, "platnosť jednej zakúpenej licencie v mesiacoch")
	flag.Parse()

	secret := os.Getenv("PADDLE_WEBHOOK_SECRET")
	price := os.Getenv("PADDLE_PRICE_ID")
	if secret == "" || price == "" {
		log.Fatal("nastavte PADDLE_WEBHOOK_SECRET a PADDLE_PRICE_ID")
	}
	seed, err := os.ReadFile(*keyFile)
	if err != nil {
		log.Fatalf("súkromný kľúč: %v", err)
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(seed)))
	if err != nil || len(raw) != ed25519.SeedSize {
		log.Fatalf("neplatný súkromný kľúč v %s", *keyFile)
	}

	port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
	smtp := mail.Settings{Host: os.Getenv("SMTP_HOST"), Port: port, Security: mail.Security(os.Getenv("SMTP_SECURITY")),
		Username: os.Getenv("SMTP_USER"), Password: os.Getenv("SMTP_PASSWORD"),
		FromEmail: os.Getenv("SMTP_FROM"), FromName: os.Getenv("SMTP_FROM_NAME")}
	smtp.Normalize()
	if !smtp.Configured() {
		log.Print("upozornenie: SMTP nie je nastavené — licencie sa vystavia, kľúč treba poslať ručne (súbor evidencie)")
	}

	is := &shop.Issuer{Key: ed25519.NewKeyFromSeed(raw), PriceID: price, Months: *months,
		Store: &shop.FileStore{Path: *storePath}, Mail: mail.SMTP{}, SMTP: smtp}
	if k := os.Getenv("PADDLE_API_KEY"); k != "" {
		base := shop.LiveAPI
		if os.Getenv("PADDLE_SANDBOX") == "1" {
			base = shop.SandboxAPI
		}
		is.Lookup = shop.API{BaseURL: base, Key: k}
	}

	mux := http.NewServeMux()
	mux.Handle("/paddle/webhook", &shop.Webhook{Secret: secret, Issuer: is})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second}
	log.Printf("fiskus-shop: webhook na http://%s/paddle/webhook, evidencia %s", *addr, *storePath)
	log.Fatal(srv.ListenAndServe())
}
