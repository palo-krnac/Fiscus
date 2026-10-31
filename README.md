# Fiškus

**Slovenský účtovný program určený na študijné účely a na výučbu účtovníctva.**
Celá cesta dokladu — od faktúry cez úhradu v banke, zápis v denníku a priznanie
k DPH až po závierku — v jednom programe, ktorý sa dá vyskúšať na ukážkovej
firme bez rizika, že niečo pokazíte.

Fiškus beží ako **desktopová aplikácia** (Windows, macOS, Linux — jedna firma,
offline) aj ako **webová aplikácia na serveri** (viac používateľov a firiem,
prístup z prehliadača). Obe verzie majú jeden kód aj jedno rozhranie.

> **Prečo na výučbu.** V učebnici je zápis, ale chýbajú súvislosti. Vo Fiškuse
> študent vystaví faktúru a o minútu ju nájde v kontrolnom výkaze, v knihe
> pohľadávok, po úhrade v peňažnom denníku a na konci roka vo výkaze. Každé
> číslo sa dá rozkliknúť až na doklad, z ktorého vzniklo.
>
> Fiškus počíta podľa slovenských predpisov a výpočty sú pokryté testami, no nie
> je certifikovaný softvér. Pri použití na skutočnú firmu si výstupy nechajte
> skontrolovať účtovníkom.

📘 **Dokumentácia:** [docs/](docs/index.md) — návody po oblastiach a
[Fiškus vo výučbe](docs/vyucba/index.md) s úlohami na hodinu.

## Rýchly štart

```bash
make demo        # ukážková firma „Demo elektroservis“, http://127.0.0.1:8080
```

Ukážkové dáta sú iba v pamäti, po reštarte sú znova pôvodné. Inštalačky pre
Windows, macOS a Linux sú vo [vydaniach](https://github.com/palo-krnac/Fiscus/releases).

## Funkcie

### Predaj
- **Faktúry** od konceptu po úhradu, číselné rady bez dier, DPH 23 / 19 / 5 / 0 %, ceny s DPH aj bez DPH, cudzia mena s kurzom ECB
- **Úprava vystavenej faktúry** s auditom, kým nie je uhradená, priznaná ani opravená — potom iba **dobropis** (celý aj čiastočný, kontrola limitu)
- **Zálohové faktúry**, daňový doklad k prijatej platbe, vyúčtovanie s odpočtom po sadzbách (aj čiastočné úhrady, cudzia mena, zahraničie, prenesenie)
- **Cenové ponuky** s prijatím cez odkaz, **pravidelná fakturácia**, **upomienky** s úrokom z omeškania a paušálnou náhradou
- Tlač a PDF s **PAY by square**, odoslanie e-mailom, **e-faktúra** UBL 2.1 / Peppol
- Osobitné režimy: tuzemské prenesenie (§ 69 ods. 12), dodanie do EÚ, vývoz, trojstranný obchod

### Nákup
- **Prijaté faktúry** z rekapitulácie DPH, odpočet / bez odpočtu, prenesenie, pomerné odpočítanie (§ 50), dovoz, DPH pri vozidlách (§ 85n)
- Vyťaženie údajov z PDF a fotky (Claude API — AI iba navrhuje)
- **Bločky eKasa z mobilu**: odfotenie QR kódu, doklad z Finančnej správy, uhradený nákup s fotkou

### Sklad
- **Karta produktu**: EAN s kontrolou, kategória, značka, dodávateľ a jeho kód, nákupná cena, hmotnosť, technické parametre, popis a až **10 obrázkov**
- **Viac skladov**, stav a hodnota po skladoch, všetky pohyby s hľadaním a exportom
- Príjemky, výdajky, **presuny**, inventúrne rozdiely; ocenenie **váženým priemerom alebo FIFO**
- Výdajka z vystavenej faktúry, vrátenie k dobropisu, príjemka k prijatej faktúre
- Zaúčtovanie spôsobom A (112, 123, 132 a protiúčty), zásoby vo výkaze o majetku

### Peniaze
- **Banka**: výpisy camt.053, GPC, CSV, Excel; automatické párovanie podľa VS a sumy; platobné príkazy SEPA
- **Pokladňa**: PPD / VPD, viac pokladní, cudzia mena, DPH na blokoch a tržbách z e-kasy

### Dane
- **Priznanie k DPH** (DPHv25), **kontrolný výkaz** (KVDPHv25), **súhrnný výkaz** (SVDPHv20) ako XML pre eDane; opravné a dodatočné priznanie
- **Kalendár termínov** s lehotami posunutými na pracovný deň, odber do mobilu, Google Kalendár a Outlook

### Účtovníctvo
- **Jednoduché**: peňažný denník podľa opatrenia MF SR, knihy pohľadávok a záväzkov, výkazy Úč FO 1-01 a 2-01, uzávierka, podklady pre DP typ B
- **Podvojné**: účtový rozvrh, predkontácie, automatické účtovanie, účtovný denník, hlavná kniha, strediská a zákazky, kurzové rozdiely, závierka MÚJ
- **Majetok** s daňovými odpismi (§ 22 – 29 ZDP), **kniha jázd** s ročným vyúčtovaním

### E-shopy
- WooCommerce, Shopify, Shoptet, PrestaShop, Magento, Drupal Commerce: objednávky → koncepty faktúr, zásoby do obchodu
- **Riadenie obchodu z Fiškusu**: cenník, ceny, DPH a zásoby sa posielajú do obchodu (iba zmeny)

### Prenos dát
- POHODA, Money S3, KROS Omega, MRP, iDoklad, SuperFaktúra, Fakturoid, ľubovoľná tabuľka Excel/CSV
- Adresár z vCard, Outlooku a Google Kontaktov; počiatočné stavy účtov z tabuľky

### Bezpečnosť a správa
- Používatelia a **roly** pre každú firmu, povinné **2FA** na serveri, PIN na desktope
- **Audit** všetkých zmien s nemenným reťazcom odtlačkov
- **Šifrované zálohy** s rotáciou a kópiou do cloudu (WebDAV, S3)
- Legislatíva ako dáta (sadzby DPH, ECB, úroky, sviatky), **AI asistent cez MCP**
- **Mobil v sieti**: telefón spárovaný s desktopom QR kódom

Podrobný stav po etapách: [plán vývoja](docs/ROADMAP.md).

## Dokumentácia

Dokumentácia je v priečinku [`docs/`](docs/index.md), napísaná v dialekte
**Sourcebook** podľa pravidiel fabrikaERP: obyčajný Markdown s front matter,
admonície v zápise MkDocs (`!!! note`) aj Docusaurus (`:::note`) a bloky
`cards` a `table`.

### Ako to funguje

- **`main` je pravda.** Zverejňuje sa iba to, čo je v `main`.
- **Verzia je značka**, nie vetva: `git tag docs/0.2 && git push origin docs/0.2`
  zmrazí dokumentáciu k danému dňu.
- **Úprava v platforme** sa vráti ako vetva a pull request; do `main` nič
  nepíše potichu.
- Každý súbor sa musí zostaviť v **MkDocs Material aj Docusaure bez úprav**.
  `mkdocs.yml` je tu ako dôkaz (kontroluje ho CI), nie ako spôsob zverejnenia:

```sh
pip install mkdocs-material && mkdocs build --strict
```

### Štruktúra

```
docs/
  index.md          prehľad, na čo je Fiškus
  zaciname/         inštalácia, úvodné nastavenie, prenos dát
  vyucba/           Fiškus na hodine — úlohy podľa tém
  predaj/           faktúry, zálohy, ponuky, upomienky, cenník
  nakup/            prijaté faktúry, bločky
  sklad/            karta produktu, doklady, ocenenie a účtovanie
  peniaze/          banka, pokladňa
  dane/             DPH, kontrolný a súhrnný výkaz, kalendár
  uctovnictvo/      jednoduché, podvojné, majetok, kniha jázd
  eshopy/  mobil/  nastavenia/
  ROADMAP.md        plán vývoja
  changelog.md      prehľad zmien
  reference/        podklady (zoznamy funkcií iných systémov), nezverejňujú sa
```

## Edície

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

### Claude na webe a v mobile (serverová verzia)

Serverová verzia je zároveň autorizačný server OAuth 2.1 (dynamická registrácia klienta, PKCE S256, obnovovacie tokeny s výmenou). V Claude stačí pridať vlastný konektor s adresou `https://vas-server/mcp`. Claude otvorí Fiškus, používateľ sa prihlási (aj s 2FA) a povolí prístup ako on alebo iba na čítanie. Pripojenie je v *Prístup pre AI* a dá sa zrušiť.

Fiškus musí byť dostupný z internetu cez HTTPS. Za reverznou proxy ho spustite s `-public-url https://vas-server`, aby adresy v metadátach OAuth sedeli.

Nástroje:
- čítanie: `list_companies`, `company_overview`, `list_invoices`, `get_invoice`, `list_quotes`, `list_recurring_invoices`, `list_catalog_items`, `list_purchases`, `list_contacts`, `list_bank_transactions`, `bank_match_suggestions`, `cash_book`, `list_overdue`, `vat_return`
- zmeny: `create_invoice_draft`, `issue_invoice`, `mark_invoice_paid`, `create_credit_note`, `create_purchase`, `mark_purchase_paid`, `match_bank_transaction`, `create_reminder`, `create_contact`

DPH, súčty, úroky z omeškania aj limity dobropisu počíta Fiškus; asistent iba zadáva údaje. Vystavenie faktúry a upomienka idú iba na výslovný pokyn používateľa, každé volanie je v audite.

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
docs/       dokumentácia (dialekt Sourcebook, mkdocs.yml ako dôkaz), plán vývoja
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

Zálohy: priečinok a heslo sa dajú nastaviť v programe (Zálohy) — uložia sa do súboru `fiskus-zalohy.json` vedľa databázy (práva 0600). Parameter `-backup-dir` a premenná `FISKUS_BACKUP_PASSWORD` majú prednosť a v programe sa potom nedajú zmeniť. Bez parametra je priečinok `zalohy` vedľa databázy.

Kópia v cloude: v časti Zálohy sa dá nastaviť WebDAV (Nextcloud, ownCloud, Synology…) alebo úložisko kompatibilné s S3 (AWS, Wasabi, Backblaze B2, Hetzner, MinIO). Po každej zálohe sa zašifrovaný súbor skopíruje aj tam a staré kópie sa mažú podľa rovnakého pravidla ako na disku. Výpadok cloudu lokálnu zálohu nezastaví — chyba sa ukáže v stave záloh a v denníku. Prístupové údaje sú v `fiskus-zalohy.json` a API ich nikdy nevracia. Na novom počítači stačí nastaviť rovnaké heslo záloh a cloud, v zozname „Zálohy v cloude“ zálohu stiahnuť a obnoviť.

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

Plán funkcií: [docs/ROADMAP.md](docs/ROADMAP.md). Dokumentácia: [docs/](docs/index.md).
