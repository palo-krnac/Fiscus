// fiskus-license vydáva licenčné kľúče Fiškusa (serverová verzia pre viac firiem).
//
// Súkromný kľúč nikdy nepatrí do repozitára ani k zákazníkom.
//
//	fiskus-license keygen -out licencia.key
//	    vytvorí pár kľúčov; verejný kľúč (hex) treba pridať do internal/license (trusted)
//
//	fiskus-license sign -key licencia.key -customer "Kancelária s.r.o." -email a@b.sk -id txn_123 [-months 12] [-companies 0]
//	    vypíše licenčný kľúč pre zákazníka (po platbe cez Paddle)
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/license"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ExitOnError)
		out := fs.String("out", "licencia.key", "súbor pre súkromný kľúč")
		_ = fs.Parse(os.Args[2:])
		if _, err := os.Stat(*out); err == nil {
			die("súbor %s už existuje — neprepisujem súkromný kľúč", *out)
		}
		pub, priv, err := license.GenerateKey()
		check(err)
		check(os.WriteFile(*out, []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600))
		fmt.Printf("súkromný kľúč: %s (uschovajte bezpečne, napr. v trezore hesiel)\n", *out)
		fmt.Printf("verejný kľúč:  %s\n", hex.EncodeToString(pub))
	case "sign":
		fs := flag.NewFlagSet("sign", flag.ExitOnError)
		keyFile := fs.String("key", "licencia.key", "súbor so súkromným kľúčom")
		customer := fs.String("customer", "", "obchodné meno zákazníka")
		email := fs.String("email", "", "e-mail zákazníka")
		id := fs.String("id", "", "číslo platby (Paddle transaction id)")
		months := fs.Int("months", 12, "platnosť v mesiacoch")
		companies := fs.Int("companies", 0, "max. počet firiem (0 = bez obmedzenia)")
		_ = fs.Parse(os.Args[2:])
		if *customer == "" || *id == "" {
			die("zadajte -customer a -id")
		}
		seed, err := os.ReadFile(*keyFile)
		check(err)
		raw, err := hex.DecodeString(strings.TrimSpace(string(seed)))
		if err != nil || len(raw) != ed25519.SeedSize {
			die("neplatný súkromný kľúč v %s", *keyFile)
		}
		now := time.Now().UTC().Truncate(time.Second)
		key, err := license.Sign(license.License{
			ID: *id, Customer: *customer, Email: *email, Plan: license.PlanMulti,
			Companies: *companies, Issued: now, Expires: now.AddDate(0, *months, 0),
		}, ed25519.NewKeyFromSeed(raw))
		check(err)
		fmt.Println(key)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "použitie: fiskus-license keygen | sign (pozri komentár v cmd/fiskus-license)")
	os.Exit(2)
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, f+"\n", a...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		die("%v", err)
	}
}
