// Fiskus — účtovný program. Jedna binárka: API + frontend.
//
//	fiskus -demo                          ukážkové dáta v pamäti
//	fiskus -db fiskus.db                  SQLite (zostaviť s -tags sqlite)
//	fiskus -mode server -addr :8080 ...   serverový režim (viac firiem)
//
// V serverovom režime sa prvý správca vytvorí cez -admin-email a heslo
// v premennej FISKUS_ADMIN_PASSWORD (iba ak ešte nie je žiadny používateľ).
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/api"
	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/demo"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/mcp"
	"github.com/palo-krnac/fiscus/backend/internal/registry"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
	"github.com/palo-krnac/fiscus/backend/internal/web"
)

// version sa nastaví pri zostavení: -ldflags "-X main.version=1.0.0"
var version = "dev"

// openSQLite nastaví súbor sqlite_on.go (iba pri zostavení s -tags sqlite).
var openSQLite func(path, backupDir, restoreFile string) (store.Store, *backup.Manager, string, func(), error)

var errNoPassword = errors.New("obnova zo zálohy vyžaduje heslo v premennej FISKUS_BACKUP_PASSWORD")

func main() {
	mode := flag.String("mode", "desktop", "režim: desktop | server")
	addr := flag.String("addr", "127.0.0.1:8080", "adresa HTTP servera")
	dbPath := flag.String("db", "", "cesta k databáze SQLite")
	backupDir := flag.String("backup-dir", "", "priečinok na automatické zálohy (heslo v FISKUS_BACKUP_PASSWORD)")
	demoData := flag.Bool("demo", false, "spustiť s ukážkovými dátami v pamäti")
	rpoURL := flag.String("rpo-url", registry.DefaultBaseURL, "adresa API Registra právnických osôb; prázdne = vypnuté")
	restoreFile := flag.String("restore", "", "pred spustením obnoviť databázu zo zálohy (.fbk); pôvodná sa premenuje na .pred-obnovou")
	licensePath := flag.String("license", "", "súbor s licenčným kľúčom (server, viac firiem); predvolene fiskus.license vedľa databázy")
	adminEmail := flag.String("admin-email", "", "e-mail prvého správcu (server); heslo v FISKUS_ADMIN_PASSWORD")
	flag.Parse()

	log.Printf("Fiskus %s", version)
	caps, err := edition.For(edition.Mode(*mode))
	if err != nil {
		log.Fatal(err)
	}

	var st store.Store
	cleanup := func() {}
	var backups *backup.Manager
	var backupsOff string
	switch {
	case *dbPath != "":
		if openSQLite == nil {
			log.Fatal("táto binárka je zostavená bez SQLite — použite -tags sqlite alebo -demo")
		}
		st, backups, backupsOff, cleanup, err = openSQLite(*dbPath, *backupDir, *restoreFile)
		if err != nil {
			log.Fatal(err)
		}
	case *demoData:
		st = memstore.New()
	default:
		log.Fatal("zadajte -db <súbor> alebo -demo")
	}
	defer st.Close()

	a := app.New(st, caps)
	a.Backups, a.BackupsOff = backups, backupsOff
	if err := a.LoadLegal(context.Background()); err != nil {
		log.Fatalf("legislatíva: %v", err)
	}
	if *demoData {
		a.BackupsOff = "Ukážkové dáta sú iba v pamäti a nezálohujú sa. Pre ostrú prácu spustite Fiškus s parametrami -db a -backup-dir."
	}
	if caps.Mode == edition.Server {
		setupLicense(a, *licensePath, *dbPath)
	}
	if *demoData {
		c, err := demo.Seed(context.Background(), a)
		if err != nil {
			log.Fatal(err)
		}
		log.Print("načítané ukážkové dáta")
		if caps.Mode == edition.Server {
			if err := demo.SeedUsers(context.Background(), a, c.ID); err != nil {
				log.Fatal(err)
			}
			log.Printf("prihlásenie: %s / %s (aj uctovnicka@, fakturant@, klient@demo.sk)", demo.DemoAdmin, demo.DemoPassword)
		}
	}
	if caps.Mode == edition.Server && *adminEmail != "" {
		created, err := a.EnsureAdmin(context.Background(), *adminEmail, "Správca", os.Getenv("FISKUS_ADMIN_PASSWORD"))
		if err != nil {
			log.Fatalf("správca: %v", err)
		}
		if created {
			log.Printf("vytvorený správca %s", *adminEmail)
		}
	}

	var lookup registry.Lookup
	if *rpoURL != "" {
		lookup = registry.NewRPO(*rpoURL)
	}

	webFS := web.FS()
	if webFS == nil {
		log.Print("upozornenie: frontend nie je zostavený, beží iba API")
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&api.Server{App: a, Web: webFS, Registry: lookup, MCP: mcp.New(a, version)}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("režim %s, otvorte http://%s", caps.Mode, *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	runCtx, stopRecurring := context.WithCancel(context.Background())
	go recurringLoop(runCtx, a)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Print("ukončujem…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopRecurring()
	_ = srv.Shutdown(ctx)
	cleanup() // pri SQLite: záverečná záloha
}

// setupLicense načíta licenciu (súbor alebo FISKUS_LICENSE) a nastaví, kam sa uloží nová.
func setupLicense(a *app.App, path, dbPath string) {
	if path == "" && dbPath != "" {
		path = filepath.Join(filepath.Dir(dbPath), "fiskus.license")
	}
	if path != "" {
		a.SaveLicense = func(key string) error { return os.WriteFile(path, []byte(key+"\n"), 0o600) }
	}
	key := os.Getenv("FISKUS_LICENSE")
	if key == "" && path != "" {
		if b, err := os.ReadFile(path); err == nil {
			key = string(b)
		}
	}
	if key == "" {
		log.Print("licencia: bezplatná verzia pre jednu firmu")
		return
	}
	if err := a.LoadLicense(key); err != nil {
		log.Printf("licencia: %v — beží bezplatná verzia pre jednu firmu", err)
		return
	}
	if st := a.Capabilities().License; st != nil {
		log.Printf("licencia: %s, platná do %s", st.Customer, st.Expires.Format("2. 1. 2006"))
		if st.Warning != "" {
			log.Printf("licencia: %s", st.Warning)
		}
	}
}

// recurringLoop vytvára pravidelné faktúry: krátko po štarte a potom každú hodinu
// (desktop môže bežať cez polnoc, server nepretržite).
func recurringLoop(ctx context.Context, a *app.App) {
	run := func() {
		res, err := a.RunRecurring(ctx)
		switch {
		case err != nil:
			log.Printf("pravidelná fakturácia: %v", err)
		case res.Created > 0:
			log.Printf("pravidelná fakturácia: vytvorených %d faktúr, z toho vystavených %d", res.Created, res.Issued)
		}
	}
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
			t.Reset(time.Hour)
		}
	}
}
