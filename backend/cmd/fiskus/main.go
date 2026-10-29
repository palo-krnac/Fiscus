// Fiskus — účtovný program. Jedna binárka: API + frontend.
//
//	fiskus -demo                          ukážkové dáta v pamäti
//	fiskus -db fiskus.db                  SQLite (zostaviť s -tags sqlite)
//	fiskus -mode server -addr :8080 ...   serverový režim (viac firiem)
//	fiskus                                aplikácia (ako z inštalátora): dáta
//	                                      v priečinku používateľa, otvorí prehliadač
//
// V serverovom režime sa prvý správca vytvorí cez -admin-email a heslo
// v premennej FISKUS_ADMIN_PASSWORD (iba ak ešte nie je žiadny používateľ).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/api"
	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/demo"
	"github.com/palo-krnac/fiscus/backend/internal/desktop"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/fx"
	"github.com/palo-krnac/fiscus/backend/internal/mcp"
	"github.com/palo-krnac/fiscus/backend/internal/registry"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
	"github.com/palo-krnac/fiscus/backend/internal/tray"
	"github.com/palo-krnac/fiscus/backend/internal/web"
)

// version sa nastaví pri zostavení: -ldflags "-X main.version=1.0.0"
var version = "dev"

// openSQLite nastaví súbor sqlite_on.go (iba pri zostavení s -tags sqlite).
var openSQLite func(path string, o backupOpts) (store.Store, *backup.Manager, *app.BackupSetup, func(), error)

// backupOpts sú parametre záloh z príkazového riadka.
type backupOpts struct {
	Dir      string // priečinok záloh (-backup-dir alebo predvolený)
	DirFixed bool   // zadaný parametrom — v programe sa nedá zmeniť
	Restore  string // -restore
	Home     string // domovský priečinok (desktop) pre návrh cloudových priečinkov
}

var errNoPassword = errors.New("obnova zo zálohy vyžaduje heslo záloh (premenná FISKUS_BACKUP_PASSWORD alebo nastavené v programe)")

func main() {
	mode := flag.String("mode", "desktop", "režim: desktop | server")
	addr := flag.String("addr", "127.0.0.1:8080", "adresa HTTP servera")
	dbPath := flag.String("db", "", "cesta k databáze SQLite")
	backupDir := flag.String("backup-dir", "", "priečinok na automatické zálohy (inak sa nastaví v programe); heslo v FISKUS_BACKUP_PASSWORD alebo v programe")
	demoData := flag.Bool("demo", false, "spustiť s ukážkovými dátami v pamäti")
	rpoURL := flag.String("rpo-url", registry.DefaultBaseURL, "adresa API Registra právnických osôb; prázdne = vypnuté")
	viesURL := flag.String("vies-url", registry.VIESBaseURL, "adresa VIES na overenie IČ DPH; prázdne = vypnuté")
	ruzURL := flag.String("ruz-url", registry.RUZBaseURL, "adresa API Registra účtovných závierok (záloha RPO, DIČ); prázdne = vypnuté")
	ecbURL := flag.String("ecb-url", fx.ECB90, "adresa kurzov ECB (posledných 90 dní); prázdne = vypnuté, kurzy sa zadávajú ručne")
	claudeURL := flag.String("claude-url", "", "adresa Claude API na čítanie dokladov (prázdne = api.anthropic.com)")
	restoreFile := flag.String("restore", "", "pred spustením obnoviť databázu zo zálohy (.fbk); pôvodná sa premenuje na .pred-obnovou")
	licensePath := flag.String("license", "", "súbor s licenčným kľúčom (server, viac firiem); predvolene fiskus.license vedľa databázy")
	publicURL := flag.String("public-url", "", "server: verejná adresa https://… (pripojenie Claude cez OAuth za reverznou proxy); prázdne = podľa požiadavky")
	adminEmail := flag.String("admin-email", "", "e-mail prvého správcu (server); heslo v FISKUS_ADMIN_PASSWORD")
	showVersion := flag.Bool("version", false, "vypíše verziu a skončí")
	openBrowser := flag.Bool("open", true, "aplikácia: otvoriť prehliadač po spustení (most fiskus-mcp spúšťa bez neho)")
	appMode := flag.Bool("app", false, "aplikácia (spustenie z inštalátora): dáta v priečinku používateľa, otvorí prehliadač, skončí po zatvorení okna")
	flag.Parse()
	dirFixed := false
	flag.Visit(func(f *flag.Flag) { dirFixed = dirFixed || f.Name == "backup-dir" })
	if *showVersion {
		fmt.Println("Fiskus", version)
		return
	}
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// desktop bez -db a -demo (dvojklik, ikona z inštalátora) = aplikácia
	if *mode == string(edition.Desktop) && *dbPath == "" && !*demoData {
		*appMode = true
	}
	if *appMode {
		if !set["addr"] {
			*addr = desktop.Addr
		}
		if desktop.Running(context.Background(), *addr) {
			// druhé spustenie: iba otvoriť okno bežiaceho programu
			if *openBrowser {
				_ = desktop.Open("http://" + *addr)
			}
			return
		}
		dir, err := desktop.DataDir()
		if err != nil {
			log.Fatalf("priečinok dát: %v", err)
		}
		if f, err := desktop.LogFile(dir); err == nil {
			log.SetOutput(io.MultiWriter(f, quietStderr{}))
			defer f.Close()
		}
		if *dbPath == "" && !*demoData {
			*dbPath = filepath.Join(dir, "fiskus.db")
		}
		if *backupDir == "" {
			*backupDir = filepath.Join(dir, "zalohy")
		}
		log.Printf("aplikácia: dáta v %s", dir)
	}

	log.Printf("Fiskus %s", version)
	caps, err := edition.For(edition.Mode(*mode))
	if err != nil {
		log.Fatal(err)
	}

	var st store.Store
	cleanup := func() {}
	var backups *backup.Manager
	var backupsOff string
	var backupSetup *app.BackupSetup
	switch {
	case *dbPath != "":
		if openSQLite == nil {
			log.Fatal("táto binárka je zostavená bez SQLite — použite -tags sqlite alebo -demo")
		}
		o := backupOpts{Dir: *backupDir, DirFixed: dirFixed, Restore: *restoreFile}
		if caps.Mode == edition.Desktop {
			o.Home, _ = os.UserHomeDir()
		}
		st, backups, backupSetup, cleanup, err = openSQLite(*dbPath, o)
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
	a.ECBURL = *ecbURL
	a.AIBaseURL = *claudeURL
	a.Backups, a.BackupsOff, a.BackupSetup = backups, backupsOff, backupSetup
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

	var vies registry.VATChecker
	if *viesURL != "" {
		vies = registry.NewVIES(*viesURL)
	}
	var lookup registry.Lookup
	if *rpoURL != "" {
		lookup = registry.NewRPO(*rpoURL)
		if *ruzURL != "" {
			lookup = registry.Chain{Primary: lookup, Fallback: registry.NewRUZ(*ruzURL), VAT: vies}
		}
	}

	webFS := web.FS()
	if webFS == nil {
		log.Print("upozornenie: frontend nie je zostavený, beží iba API")
	}
	apiSrv := &api.Server{App: a, Web: webFS, Registry: lookup, VAT: vies, MCP: mcp.New(a, version), Version: version, PublicURL: *publicURL}
	var watchdog *desktop.Watchdog
	if *appMode {
		watchdog = desktop.NewWatchdog()
		apiSrv.Desktop = watchdog
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		if *appMode {
			log.Fatalf("%v: %s — ukončite program, ktorý ho používa, alebo spustite Fiškus s -addr 127.0.0.1:<iný port>", desktop.ErrBusy, *addr)
		}
		log.Fatal(err)
	}
	go func() {
		log.Printf("režim %s, otvorte http://%s", caps.Mode, *addr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	if *appMode && tray.Supported() {
		// ikona vpravo dole: program beží, verzia, otvoriť a ukončiť
		name := "Fiškus " + strings.TrimPrefix(version, "v")
		stopTray, err := tray.Run(tray.Menu{Title: name, Tooltip: name + " – beží",
			Open: func() { _ = desktop.Open("http://" + *addr) }, Quit: watchdog.Quit})
		if err != nil {
			log.Printf("ikona v oblasti oznámení: %v", err)
		} else {
			watchdog.Resident = true
			defer stopTray()
		}
	}
	if *appMode && *openBrowser {
		if err := desktop.Open("http://" + *addr); err != nil {
			log.Printf("prehliadač sa neotvoril (%v) — otvorte http://%s", err, *addr)
		}
	}

	runCtx, stopRecurring := context.WithCancel(context.Background())
	go recurringLoop(runCtx, a)
	go ratesLoop(runCtx, a)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	if watchdog != nil {
		// aplikácia skončí aj po zatvorení posledného okna alebo cez „Ukončiť“
		sigCtx, cancelSig := context.WithCancel(context.Background())
		go func() { <-stop; cancelSig() }()
		log.Printf("ukončujem (%s)…", watchdog.Wait(sigCtx, 5*time.Second))
	} else {
		<-stop
		log.Print("ukončujem…")
	}
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

// ratesLoop sťahuje kurzy ECB: krátko po štarte a potom každých 6 hodín
// (ECB ich vyhlasuje v pracovné dni okolo 16:00 SEČ).
func ratesLoop(ctx context.Context, a *app.App) {
	if a.ECBURL == "" {
		return
	}
	t := time.NewTimer(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if res, err := a.UpdateRates(ctx); err != nil {
				log.Printf("kurzy ECB: %v", err)
			} else {
				log.Printf("kurzy ECB: %d kurzov, posledné k %s", res.Count, res.Latest)
			}
			t.Reset(6 * time.Hour)
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

// quietStderr píše denník aj do konzoly, ak existuje; aplikácia na Windows
// (bez konzoly) chybu zápisu ignoruje, aby sa denník do súboru nezastavil.
type quietStderr struct{}

func (quietStderr) Write(p []byte) (int, error) {
	_, _ = os.Stderr.Write(p)
	return len(p), nil
}
