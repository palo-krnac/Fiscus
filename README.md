# Fiškus

Slovenský účtovný program pre živnostníkov a malé firmy. Beží ako **desktopová aplikácia** (jedna inštalácia, offline, jedna firma) aj ako **webová aplikácia na serveri** (viac firiem, prístup z prehliadača). Obe verzie majú jeden kód aj jeden frontend.

## Edície a ceny

| Edícia | Cena | Pre koho |
|---|---|---|
| Desktop | zadarmo | jedna firma, offline na vlastnom počítači |
| Server – jedna firma | zadarmo | prístup z prehliadača, používatelia a roly |
| Server – viac firiem | 99 € ročne (Paddle) | účtovné kancelárie a poskytovatelia služieb |

Licencia je podpísaný kľúč `FISKUS1.…` (Ed25519). Program ho overí offline,
bez spojenia so serverom. Nahrá ho správca inštalácie v časti **Firmy a licencia**,
prípadne cez súbor `fiskus.license` vedľa databázy alebo premennú `FISKUS_LICENSE`.
Po vypršaní platí ešte 30 dní na obnovenie. Potom sa nedajú zakladať nové firmy,
ale existujúce firmy a ich dáta zostávajú prístupné.

Vydanie licencie (po platbe cez Paddle):

```
go run ./cmd/fiskus-license keygen -out licencia.key        # raz; verejný kľúč pridať do internal/license
go run ./cmd/fiskus-license sign -key licencia.key -customer "Kancelária s.r.o." -email a@b.sk -id txn_…
```

Súkromný kľúč `licencia.key` nepatrí do repozitára.

Po spustení predaja vydáva licencie automaticky služba `fiskus-shop` (webhook Paddle → podpísaný kľúč → e-mail zákazníkovi). Web s cenníkom a nákupom je v priečinku [`site/`](site/README.md).

## AI asistent (MCP)

Fiškus má zabudovaný MCP server na adrese `/mcp`, takže sa k nemu dá pripojiť Claude alebo iný AI asistent. Kľúč si každý používateľ vytvorí v časti **Prístup pre AI**. Asistent má práva svojho používateľa; kľúč môže byť obmedzený len na čítanie. Každé volanie nástroja sa zapíše do auditu.

```
# Claude Code (aplikácia z inštalátora beží na porte 48080)
claude mcp add --transport http fiskus http://127.0.0.1:48080/mcp --header "Authorization: Bearer fk_…"

# Claude Desktop (stdio): most fiskus-mcp z inštalácie, napr. vo Windows
# %LOCALAPPDATA%\Programs\Fiskus\fiskus-mcp.exe
{"mcpServers":{"fiskus":{"command":"C:\\Users\\…\\AppData\\Local\\Programs\\Fiskus\\fiskus-mcp.exe","env":{"FISKUS_API_KEY":"fk_…"}}}}
```

Most `fiskus-mcp` sa predvolene pripája na `http://127.0.0.1:48080/mcp` (iná adresa: `-url`). Ak aplikácia nebeží, most ju spustí na pozadí bez okna prehliadača; kým asistent pracuje, aplikácia sa neukončí.

Nástroje:
- čítanie: `list_companies`, `company_overview`, `list_invoices`, `get_invoice`, `list_purchases`, `list_contacts`, `list_bank_transactions`, `vat_return`
- zmeny: `create_invoice_draft`, `issue_invoice`, `mark_invoice_paid`, `create_contact`

## E-faktúra (od 1. 1. 2027)

Vystavená faktúra a dobropis sa dajú stiahnuť ako e-faktúra UBL 2.1 (EN 16931, Peppol BIS Billing 3.0). Adresa v sieti Peppol je `0245:DIČ`. Prijatá e-faktúra sa načíta ako koncept prijatej faktúry. Pripojenie k digitálnemu poštárovi sa nastavuje v časti **Nastavenia → E-faktúry**:

- **Priečinky**: Fiškus ukladá odosielané e-faktúry do jedného priečinka a číta prijaté z druhého (prevzaté presunie do `prevzate`). Priečinky synchronizuje aplikácia poskytovateľa.
- **API poskytovateľa**: jednoduchá zmluva pre adaptér (`POST /documents`, `GET /inbox`, `GET /inbox/{id}`, `POST /inbox/{id}/ack`, token `Bearer`) — pozri `internal/postman/http.go`.

## Architektúra

```
frontend/   Vue 3 + PrimeVue + AG Grid — rozhranie (hustý ERP štýl)
backend/    Go — API, účtovná logika, SQLite
  cmd/fiskus/          jedna binárka: API + vložený frontend
  internal/money/      presný desatinný typ na peniaze (žiadny float)
  internal/vat/        výpočet DPH podľa SK pravidiel (23 / 19 / 5 / 0 %)
  internal/party/      overenie IČO, DIČ, IČ DPH
  internal/invoice/    doménový model faktúry, stavy, validácia
  internal/app/        aplikačná vrstva (pravidlá edícií, číselné rady)
  internal/edition/    desktop = 1 firma, server = viac firiem
  internal/store/      rozhranie úložiska + memstore + sqlite
  internal/backup/     šifrované zálohy, rotácia, overenie, obnova
  internal/api/        HTTP/JSON API
docs/       plán a referenčné podklady
```

Zásady:
- **Logika je v Go, nie vo frontende.** Frontend si DPH nepočíta, ale pýta sa API (`/api/invoices/preview`). Výsledok je tak vždy rovnaký na desktope, na webe aj v testoch.
- **Peniaze sú `money.Dec`**: 4 desatinné miesta, int64, zaokrúhľovanie polovica od nuly. V JSON-e idú ako reťazec, napr. `"1234.50"`.
- **Viac firiem povoľuje backend** podľa režimu (`-mode server`). Frontend len skryje alebo zobrazí prvky.
- **Každé úložisko musí prejsť spoločnou sadou testov** `storetest.Run`, ktorá overuje aj to, že sa dáta firiem nepremiešajú.

## Spustenie

```bash
make demo                 # ukážkové dáta, http://127.0.0.1:8080
make test                 # všetky testy (Go + frontend)
make build                # bin/fiskus so SQLite a vloženým frontendom

bin/fiskus                                                 # aplikácia ako z inštalátora
bin/fiskus -db fiskus.db                                   # desktop, vlastná databáza
bin/fiskus -mode server -addr :8080 -db /data/fiskus.db    # server
FISKUS_BACKUP_PASSWORD=… bin/fiskus -db fiskus.db -backup-dir ~/Fiskus-zalohy
```

Vývoj s okamžitým obnovovaním: `make dev-api` v jednom termináli a `make dev-web` v druhom (http://localhost:5173).

### Aplikácia z inštalátora

Ikona z inštalátora spustí `fiskus -app` (rovnako sa správa `fiskus` bez `-db` a `-demo`):

- dáta sú v priečinku používateľa — Windows `%APPDATA%\Fiskus`, macOS `~/Library/Application Support/Fiskus`, Linux `~/.local/share/fiskus` (databáza `fiskus.db`, zálohy v `zalohy`, denník `fiskus.log`);
- program beží na `http://127.0.0.1:48080` a sám otvorí prehliadač; druhé spustenie iba otvorí okno bežiaceho programu;
- vo Windows je vpravo dole na paneli úloh ikona Fiškusa s verziou: klik otvorí program, pravé tlačidlo ponúkne **Otvoriť** a **Ukončiť** — program beží, kým ho takto neukončíte;
- na macOS a Linuxe (bez ikony) sa otvorená stránka programu ozýva a po zatvorení posledného okna sa program asi o 1,5 minúty sám ukončí; v menu je aj tlačidlo **Ukončiť**.

## Register firiem

Pri zadaní IČO (adresár, faktúra, nastavenia) sa názov a adresa doplnia z verejného [Registra právnických osôb](https://api.statistics.sk/rpo/v1) Štatistického úradu SR (dáta pod licenciou CC BY 4.0). Do registra sa posiela iba IČO. Výsledky sa 12 hodín držia v pamäti. Bez internetu program funguje ďalej, len sa údaje nedoplnia. Vypnutie: `-rpo-url ""`.

## Legislatíva

Zákonné hodnoty, ktoré sa menia so zákonom, nie sú natvrdo v kóde. Sú v **Nastaveniach → Legislatíva** a platia pre všetky firmy:
- obdobia sadzieb DPH,
- základná úroková sadzba ECB,
- prirážky úroku z omeškania pre podnikateľov a spotrebiteľov,
- paušálna náhrada za faktúru.

Nová inštalácia sa predvyplní hodnotami z programu (`vat.DefaultPeriods`, `reminder/ecb.go`). Keď nová verzia programu prinesie nový riadok, napríklad novú sadzbu ECB, pri štarte sa doplní sám. Riadky, ktoré používateľ upravil alebo zmazal, ostanú tak, ako ich nechal. Meniť ich môže iba správca a každá zmena je v audite.

## Zálohy

Zálohy sa robia automaticky pri štarte, každých 24 hodín a pri ukončení programu. Postup:
1. `VACUUM INTO` vytvorí konzistentnú kópiu aj počas behu programu.
2. `PRAGMA integrity_check` kópiu overí.
3. Kópia sa zašifruje (AES-256-GCM, heslo cez PBKDF2), znova sa dešifruje a porovná s originálom.
4. Záloha sa atomicky zapíše a staré zálohy sa zrotujú: 7 denných, 4 týždenné, 12 mesačných.

Na obrazovke **Zálohy** (iba správca) je zoznam záloh, tlačidlo „Zálohovať teraz“, stiahnutie zálohy a obnova. Obnova sa nespraví za behu: naplánuje sa a prebehne pri ďalšom spustení, ešte pred otvorením databázy. Zálohu z iného počítača obnovíte parametrom `-restore subor.fbk`. Každá obnova aj záloha sa zapíše do auditu.

Pri obnove sa pôvodná databáza uloží ako `.pred-obnovou`, takže sa nič nestratí.

## Vydanie verzie

`git tag v0.1.0 && git push --tags`. GitHub Actions (`release.yml`) potom postaví a pripojí k vydaniu:

| Systém | Súbor | Poznámka |
|---|---|---|
| Windows | `fiskus-0.1.0-windows-setup.exe` | Inno Setup, inštalácia pre používateľa bez práv správcu, ikona v ponuke Štart |
| macOS | `fiskus-0.1.0-macos.dmg` | `Fiškus.app` pre Apple Silicon aj Intel; bez podpisu Apple — prvé spustenie cez pravé tlačidlo → Otvoriť |
| Linux | `fiskus_0.1.0_amd64.deb`, `.rpm` (aj arm64) | ikona v ponuke aplikácií |
| všetky | `fiskus-v0.1.0-<os>-<arch>` | samostatné binárky pre server a príkazový riadok |

Balenie je v priečinku `packaging/`. Pri zmene balenia v pull requeste sa inštalátory zostavia a overia (inštalácia na Windows, obsah balíka .deb) bez zverejnenia. Podpisovanie (Windows Authenticode, Apple notarizácia) zatiaľ chýba — vyžaduje certifikáty.

Plán funkcií: [docs/ROADMAP.md](docs/ROADMAP.md).
