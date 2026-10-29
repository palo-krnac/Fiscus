# MyInvoice.cz – funkčný inventár (v4.59.3, MIT)

Zdroj: klon `/root/ref/myinvoice` (README, AGENTS.md, CHANGELOG, `manual/01–41`, `web/src/router`, `web/src/pages`, `api/src/Routes.php`, `api/openapi.yaml`, `db/migrations` 0001–0153).
Stack: PHP 8.5 + Slim 4 + Twig + mPDF, Vue 3 + TS + Tailwind + Pinia, MariaDB 10.6+, voliteľne Redis. Self-hosted (IIS / Apache / Docker). Nástupca je MyÚčto (rovnaký základ, git história zdieľaná, migrácie 1000+ patria MyÚčtu).

Značky:
- **[všeobecné]** – funguje bez ohľadu na krajinu.
- **[CZ → SK: …]** – české špecifikum a pravdepodobný slovenský ekvivalent; „?" = neistý ekvivalent alebo v SK nie je priama obdoba.

---

## 0. Navigácia a obrazovky (mapa aplikácie)

Menu je rozdelené do sekcií: **Prehľad · Predaj · Nákup · Financie · Dokumenty · Dane · Systém**.

| Sekcia | Obrazovky (route) |
|---|---|
| Prehľad | `/` dashboard |
| Predaj | `invoices` (zoznam, detail, editor, new), `recurring` (zoznam, detail, formulár), `clients` (zoznam, detail, formulár), `projects` = zákazky, `admin/price-list` (cenník), `admin/export`, `admin/import`, `admin/approvals` (inbox schvaľovania výkazov) |
| Nákup | `purchase-invoices` (zoznam, detail, editor, export), `purchase-invoices/payment-orders` (platobné príkazy) |
| Financie | `crm`, `stats` (Tržby), `purchase-stats` (Náklady), `bank` (výpisy, detail výpisu), `admin/bank-accounts`, `admin/bank-email-notices` |
| Dokumenty | `documents` (prehliadač + detail), `logbook` (kniha jázd – záložky Autá, Jazdy, Tankovanie, Kategórie, Súhrny) |
| Dane | `reports/dph`, `reports/kh`, `reports/shv`, `reports/oss`, `reports/dph-book`, `reports/income-tax`, `tax` (daňový optimalizátor), `reports/monthly-export` (hromadný export), `reports/submissions` (archív podaní) |
| Systém | `admin/settings`, `admin/suppliers`, `admin/codebooks`, `admin/users`, `admin/email-templates`, `admin/emails` (odosielacie profily), `admin/sent-emails`, `admin/activity-log`, `admin/cron-jobs`, `admin/electronic-signatures`, `admin/integrations`, `admin/update`, `admin/upgrade` (prechod na MyÚčto) |
| Profil | `profile/password`, `profile/totp`, `profile/passkeys`, `profile/session-lock`, `profile/api-tokens`, `profile/signing-profiles` |
| Verejné (bez loginu) | `/invoice/:token` (web faktúra), `/approval/:token` (schválenie výkazu), `/work-report/:token` (sledovanie rozpracovaného výkazu), `/setup`, `/login`, `/forgot`, `/reset` |

Globálne prvky rozhrania [všeobecné]:
- **Horná lišta**: globálne vyhľadávanie, prepínač dodávateľa (firmy), tlačidlo **+ Vytvoriť** (rozbaľovacie menu: vydaná faktúra, zálohová faktúra, pravidelná fakturácia, klient, dodávateľ, prijatá faktúra), prepínač jazyka CS/EN, prepínač témy (Systém/Svetlá/Tmavá), používateľské menu (Profil, Zamknúť, Odhlásiť).
- **Bočné menu**: pri položkách sa po prejdení myšou objaví nenápadné „+" (rýchle založenie).
- **Globálne vyhľadávanie** (debounce, od 2 znakov): výsledky zoskupené na *Menu / Klienti / Vydané faktúry / Prijaté faktúry*, ovládanie šípkami, Enter, Esc.
- **PWA** (manifest + service worker), mobilné „karty" namiesto tabuliek, dark mode vrátane grafov.
- Ak chýba `cfg.php` alebo DB, frontend aj API vrátia 503 s inštrukciami namiesto bielej stránky.

---

## 1. Inštalácia, setup wizard, prevádzka

- **Docker** (multi-arch image na GHCR, `php:8.5-apache` + build Vue + composer; služby `app`, `db` MariaDB, `redis`; jeden dátový volume). Skripty `cmd/docker-ghcr.{sh,ps1}` vygenerujú `.env` s náhodnými heslami, `cfg.docker.php`, stiahnu image, spustia migrácie. Šablóny pre Portainer (App Template, Stack) a Dockge. [všeobecné]
- **Natívna inštalácia** (IIS alebo Apache – konfigurácia `.htaccess` aj `web.config`), production bundle bez Composeru/Node. [všeobecné]
- **Setup wizard** (aplikácia je uzamknutá, kým nevznikne prvý admin): 3 kroky – (1) admin + heslo min. 12 znakov s indikátorom sily + voľba „vyžadovať silné MFA", (2) prvý dodávateľ s načítaním z ARES a prvým bankovým účtom (mod-11 kontrola), (3) voliteľné ukážkové dáta (5 klientov CZ/SK/DE, 8 zákaziek, 20 faktúr, 4 dobropisy). Ukážkové dáta sa dajú neskôr odstrániť jedným tlačidlom (sleduje ich tabuľka `sample_data_entries`). Po dokončení automatické prihlásenie. [všeobecné; ARES → CZ → SK: RPO/ORSR]
- **CLI**: `migrate.php` (idempotentné SQL migrácie), `setup.php`, `sample.php`, `reset.php` (aj `--keep-users-supplier`), `reset-2fa.php`, `set-password.php`, `recompute-stats.php`, backfill skripty s predvoleným dry-run. [všeobecné]
- **Cron úlohy** (dvojice `.sh` + `.cmd/.ps1`): záloha DB, záloha PDF dokladov, záloha Dokumentov, čistenie, sken bankových výpisov (30 min), IMAP bankové avíza (30 min), upomienky (Po–Pi 9:00), generovanie pravidelných faktúr (denne), sken inboxu prijatých faktúr, denná kontrola novej verzie. [všeobecné]
- **Systém → Plánované úlohy**: každý cron zapisuje heartbeat do `cron_runs` (štart, koniec, exit code, JSON report); UI ukazuje posledný úspešný beh a varovania *Stáre / Zlyháva / Nebežalo* (prah napr. 36 h). [všeobecné – výborný vzor]
- **Zálohy**: ZIP DB dump, ZIP všetkých PDF + originálov prijatých faktúr (ISDOC/XML/JSON) + príloh, ZIP sekcie Dokumenty; retencia 30 denných + 12 mesačných; voliteľné šifrovanie AES-256 heslom z `cfg.php` (keď PHP nevie šifrovať, záloha zámerne nevznikne). [všeobecné]
- **Aktualizácie z UI**: Docker „update watcher" (systemd / Windows Scheduled Task) alebo natívny updater s fázami preflight → download (iba GitHub HTTPS) → verify SHA-256 → extract → backup → swap → migrate → finish, s automatickým rollbackom pri chybe swapu. Footer ukazuje badge novej verzie. [všeobecné]
- **Online manuál** na `/manual` (HTML + fulltext + PDF), generovaný z Markdownu. [všeobecné]

---

## 2. Prihlásenie, používatelia, roly, bezpečnosť

- **Prihlásenie**: e-mail + heslo (bcrypt + pepper), voliteľne **passkey bez hesla** (WebAuthn), silné MFA = passkey alebo **TOTP**. TOTP kód sa odošle automaticky po 6. číslici (počká na CAPTCHA). Bez záložných recovery kódov (obnova cez ďalšiu passkey, TOTP alebo CLI). [všeobecné]
- **E-mailový OTP** pre účty bez silného faktora + „zapamätať zariadenie na 30 dní". [všeobecné]
- **Brute-force**: po 5 zlyhaniach/5 min CAPTCHA (Cloudflare Turnstile), po 10/15 min blokovanie IP na 15 min, po 30/h blokovanie 24 h + e-mail používateľovi. [všeobecné]
- **Serverový zámok session** (nielen prekrytie obrazovky – zamknutá session nemôže čítať API): manuálne „Zamknúť" alebo automaticky po nečinnosti; admin nastaví limit, používateľ si môže nastaviť prísnejší; odomknutie passkey bez opätovného loginu. Absolútna platnosť session 30 dní. [všeobecné]
- **Roly (RBAC)**: `admin` (všetko vrátane konfigurácie, force-edit, mazania), `accountant` (doklady, klienti, banka, exporty, výkazy; bez konfigurácie a používateľov), `readonly` (vidí a exportuje to isté ako účtovník, nič nemení – backend povolí iba GET). UI skrýva zápisové tlačidlá podľa roly. [všeobecné]
- **Prístup k firmám per používateľ** (nič nezaškrtnuté = všetky firmy). [všeobecné]
- IP allowlist s `trusted_proxies`, CSRF + Origin check, AES-256-GCM šifrovanie citlivých polí (API kľúče, OAuth secrets), path-traversal guardy, bezpečný upload podľa reálneho MIME, DKIM. [všeobecné]
- **Activity log** všetkých mutácií (entity_type, entity_id, user, IP, user-agent, zoznam zmenených polí bez hodnôt PII), filtrovateľný, per dodávateľ. [všeobecné]

---

## 3. Dashboard (Prehľad)

- **KPI dlaždice** (počet sa prispôsobuje počtu aktívnych mien, 4–6): Obrat roku v CZK (+ porovnanie s minulým rokom k rovnakému dňu), Obrat v EUR, Počet vystavených faktúr, **Po splatnosti** (suma, červené, klik → filtrovaný zoznam), **Ø doba úhrady**, Pred splatnosťou, Čaká na schválenie (výkazy, s počtom „5+ dní bez reakcie"). [všeobecné]
- **„Akce pro tebe"** (action items): po splatnosti pohľadávky, po splatnosti záväzky, koncepty prijatých faktúr, nespárované bankové pohyby, blížiaca sa pravidelná fakturácia, riziko odchodu klienta (churn), blížiaci sa termín súhrnného hlásenia. Každú položku možno **skryť na deň / týždeň / „pre historické dáta" (skryť súčasné, ukazovať iba nové výskyty) / navždy**, s možnosťou „Obnoviť skryté (n)". [všeobecné – výborný vzor]
- Koláče: top 3 klienti letos vs. loni, stav faktúr (zaplatené/odoslané/vystavené/upomienka/storno). [všeobecné]
- Grafy obratu po mesiacoch (tento rok plná farba, minulý obrys), oddelene CZK a EUR. [všeobecné]
- Tabuľka po splatnosti (s tlačidlom **Upomienka** priamo v riadku) a nezaplatené pred splatnosťou. [všeobecné]
- Widget **Výkazy práce** – rozpracované koncepty s výkazom, tlačidlo „Výkaz". [všeobecné]
- Sekcia prijatých faktúr: počet YTD, náklady roka, nezaplatené, po splatnosti, trend nákladov 12 mesiacov. [všeobecné]
- Widget **Dane roku** – odhad čistého príjmu (príjem − odvody), odkaz na optimalizátor. [CZ → SK: výpočet DPFO + SP/ZP podľa SK sadzieb]
- Štatistiky z agregačnej cache (`client_revenue_cache`, `project_revenue_cache`, `crm_monthly_summary`), prepočet pri vystavení/storne/úhrade, CLI `recompute-stats`. [všeobecné]

---

## 4. Vydané faktúry – zoznam

- **Zoskupenie po mesiacoch vystavenia** so sticky hlavičkou mesiaca; v skupine zoradené od najnovšej. [všeobecné – vzor]
- Stĺpce: checkbox, číslo (VS), typ (ikona/farba: faktúra, zálohová, dobropis, storno, daňový doklad k platbe), klient (klikateľný), vystavené, splatnosť (červená po splatnosti), suma v mene faktúry, stavový badge, akcie (PDF priamo, detail).
- **Filtre**: stav, typ, klient, zákazka (závislá od klienta), mena, obdobie (tento/minulý mesiac, tento/minulý rok, vlastný rozsah), fulltext (VS presne aj prefix, popis položiek, klient, číslo projektu/zmluvy). Filtre sa **ukladajú do URL query** (zdieľateľné, prekliky z CRM s predvyplneným filtrom); klik na položku menu filtre resetuje. [všeobecné]
- **Legenda stavov s počtami** nad zoznamom (zaplatené tento mesiac, odoslané, vystavené neodoslané, po splatnosti, upomínané) – klik prepne filter. [všeobecné – vzor]
- **Hromadné akcie** (lišta sa objaví po zaškrtnutí; checkbox v hlavičke = všetky v danom mesiaci):
  - **Vystaviť znovu (N)** – klony ako nové koncepty s auto-inkrementom mesiaca v popisoch (`3/2026 → 4/2026`),
  - **Odoslať klientovi (N)** – iba `issued`,
  - **Označiť zaplatené (N)** dnešným dátumom,
  - **Upomienka (N)** – s cooldownom 14 dní, výsledok „odoslané / preskočené (cooldown) / chyby",
  - **PDF export (N)** – spojí max. 100 vystavených dokladov do jedného PDF, voliteľne elektronicky podpísaného; ak je vo výbere koncept/storno, vypíše ktoré vyradiť. [všeobecné]
- **Workflow mesačný retainer** (z manuálu): filter „minulý mesiac" → označiť → Vystaviť znovu → upraviť koncepty → hromadne Vystaviť → hromadne Odoslať („hotovo za 5 minút"). [všeobecné – vzor]
- **Klávesové skratky**: `Ctrl+N` nová faktúra (aj v prijatých), `Ctrl+S` uložiť v editore, `Esc` zatvára modaly/detaily; Ctrl/⌘+klik alebo stredné tlačidlo na riadku otvorí detail v novom paneli (riadky sa správajú ako odkazy). [všeobecné]
- **Hranica splatnosti**: faktúra splatná dnes je v splatnosti do konca dňa (v časovej zóne aplikácie), voliteľne konfigurovateľné; rovnaké pravidlo pre zoznamy, dashboard aj súhrny (`OverduePolicy`). [všeobecné]

### 4.1 Stavy vydanej faktúry
`draft` (koncept) → `issued` (vystavené, číslo pridelené, nemenné PDF) → `sent` (odoslané e-mailom) → `reminded` (upomienka) → `paid`; ďalej odvodené **Čiastočne uhradené** a **Preplatené** (z evidencie platieb), terminálne `cancellation` (interné storno) a `credit_note` (dobropis). [všeobecné]

---

## 5. Editor faktúry (workflow)

Rozloženie: hlavička (typ, klient, zákazka, dátumy) vľavo, položky v strede, súhrn a akcie vpravo/dole.

- **Typy dokladov a číslovanie VS**: faktúra `YYMMNNN`, zálohová (proforma) `9`+YYMMNNN, dobropis (opravný daňový doklad) `7`+YYMMNNN, interné storno, daňový doklad k prijatej platbe. Šablóna čísla per dodávateľ a typ (`{YYYY}{YY}{MM}{C…}`), reset ročne/mesačne/nikdy. [všeobecné; VS je CZ/SK zvyklosť]
- **Ručné číslo** – pole je voliteľné; placeholder ukazuje **živý náhľad čísla**, ktoré doklad dostane; ručne zadané číslo sa kontroluje na unikátnosť per dodávateľ. Po vystavení je číslo nemenné. [všeobecné – vzor]
- **Klient + zákazka**: vyhľadávací select (meno/IČO); zákazka filtrovaná podľa klienta; výber zákazky predvyplní hodinovú sadzbu a splatnosť; zmena klienta resetuje zákazku. Varovanie pri pozastavenej zákazke a pri prekročení rozpočtu zákazky. [všeobecné]
- **Dátumy**: vystavenie (dnes), DUZP (= vystavenie), splatnosť (vystavenie + splatnosť zo zákazky → klient → dodávateľ; predvoľby 7/14 dní/kalendárny mesiac/vlastná), dátum úhrady (automaticky). Prepínač **„Posielať automatické upomienky"** priamo pod splatnosťou. [všeobecné]
- **Mena a reverse charge**: predvyplnené z klienta/zákazky; RC nastaví položky na sadzbu „RC 0 %" a do PDF text „Daň přiznává odběratel". [všeobecné EU; text § 92a → SK: „prenesenie daňovej povinnosti" § 69 ods. 12]
- **Režim cien s DPH / bez DPH** (brutto/netto) per doklad, predvolený per dodávateľ: v brutto režime sa DPH počíta „zhora" koeficientom a celková suma sedí na halier; haliérové reziduum sa dorovná na najsilnejšom riadku sadzby, aby detail, PDF a DPH výkazy ukazovali rovnaké číslo. Zadanie do stĺpca „Celkom s DPH" v netto režime spätne dopočíta jednotkovú cenu. [všeobecné; „§37 koeficient" CZ → SK: výpočet DPH z ceny vrátane dane]
- **Položky**: popis (multiline), množstvo, jednotka z číselníka, cena/MJ, sadzba DPH (21/12/0 osloboden./RC), celkom, celkom s DPH; **drag & drop poradie** (úchyt ☰); **„Pridať z cenníka"** s vyhľadávaním podľa kódu/názvu – ukazuje výslednú cenu a jej zdroj (individuálna cena zákazníka → pevná cena v mene → prepočet kurzom); vložený riadok je snapshot (neprecení sa pri zmene cenníka). [všeobecné]
- **Číselné polia akceptujú výrazy** (`400-100`, `12*1.21`, `(100+50)*2`, desatinná čiarka) – vyhodnotí sa bezpečným parserom pri Enter/Tab. [všeobecné – vzor]
- **Zľava z celej faktúry v %** sa pri uložení materializuje ako záporná položka „Zľava X %" – pri viacerých sadzbách DPH jedna záporná položka na každú sadzbu (správne DPH). Pevná zľava = záporná položka. [všeobecné]
- **Súhrn**: medzisúčet, zľava, základ, DPH po sadzbách, celkom k úhrade; pri cudzej mene po uložení automaticky stiahne **kurz ČNB** (až 7 dní dozadu, cache, fallback na posledný známy kurz s toastom) a zobrazí prepočet základov a DPH do CZK. [CZ → SK: kurz ECB/NBS, prepočet do EUR]
- **Tlačidlá**: Uložiť koncept, **Vystaviť** (nevratné), **Vystaviť a odoslať**, Náhľad PDF (vodoznak „NÁHLED"), Zmazať koncept, Klonovať. `Ctrl+S` kedykoľvek uloží. [všeobecné]
- **Poznámka nad položkami / pod položkami** (tlačí sa v PDF). [všeobecné]
- **Kategória tržby** na faktúre (predvolená zo zákazky/zákazníka) pre analytiku. [všeobecné]
- **Zálohová → daňový doklad**: po úhrade zálohy tlačidlo „Vystaviť daňový doklad" vytvorí faktúru so zápornou položkou „Odpočet zálohy …"; spätné spárovanie existujúcej zálohy a faktúry z oboch strán (ponúka sa iba ak existuje vhodný protějšok). [všeobecné; daňová logika CZ → SK: § 71/§ 73 ZDPH SK]
- **Daňový doklad k prijatej platbe** pri čiastočnej úhrade zálohy (platiteľ DPH): koncept automaticky z párovania banky alebo na klik; DPH zhora, rozdelenie platby pomerom sadzieb; finálne vyúčtovanie pridá záporné odpočtové riadky; ochrana proti dvojitému zdaneniu. [CZ § 28 → SK: faktúra k prijatej platbe do 15 dní]
- **Storno vs. dobropis**: storno = interné, klientovi sa nič neposiela; dobropis = opravný doklad so zápornými položkami. Modal „Storno / Dobropis" ponúka aj admin **force-delete** s podrobným varovaním podľa stavu. [všeobecné]
- **Uzamknutie vystaveného dokladu**: editor zobrazí varovný pruh; admin môže „Odomknúť na editáciu" (modal s vymenovanými dôsledkami + checkbox; platí do obnovenia stránky; audit so starým/novým snapshotom). Ľahšia akcia „Obnoviť údaje klienta" prepíše iba snapshoty a pregeneruje PDF (stará verzia sa archivuje). „Nezaplatené" vráti `paid` späť (blokované, ak je spárovaná banková transakcia – návod na zrušenie párovania). [všeobecné – vzor]
- **Upozornenie pri zmene sadzby DPH štátom**: klon aj šablóna s expirovanou sadzbou sa zastavia s jasnou chybou, nič sa ticho nevystaví so starou sadzbou. [všeobecné]

### 5.1 Výkaz víceprací (prác) a materiálu
- Ku hodinovej položke sa otvorí modal s riadkami (dátum, popis, hodiny, sadzba, celkom); suma sa prenesie do položky faktúry; výkaz sa tlačí na **2. stranu PDF**, položka vo faktúre je **klikateľný odkaz** na stranu výkazu. [všeobecné]
- **Výkaz materiálu** (popis, množstvo, MJ, cena/MJ) → druhá súhrnná položka „Materiál"; každý výkaz má vlastnú sadzbu DPH (práca 21 %, materiál 12 % predvolene). [všeobecné]
- **Schvaľovanie zákazníkom** (zapína sa na zákazke): faktúra s výkazom sa nedá vystaviť, kým zákazník neschváli. „Odoslať na schválenie" vygeneruje jednorazový token + PDF výkazu + e-mail s veľkým tlačidlom „✓ Schváliť"; verejná stránka bez loginu s CAPTCHA, Schváliť / Zamietnuť s povinným dôvodom; po schválení sa faktúra **automaticky vystaví a odošle**. Badge stavu (Neurčené/Vyžiadané/Schválené/Zamietnuté), „Test schválenia" na vlastný e-mail, admin manuálna zmena stavu (napr. schválené telefonicky), upomienky schvaľovania, globálny **inbox schvaľovania** s filtrom „5+ dní bez reakcie". [všeobecné – výborný vzor]
- **Sledovací odkaz na rozpracovaný výkaz** (z detailu klienta alebo zákazky): trvalý verejný odkaz so živým stavom nevyfakturovaných hodín a sumy; prístup overený jednorazovým kódom na e-mail klienta (cookie 180 dní), maskované adresy, zneplatnenie odkazu. [všeobecné – vzor]

---

## 6. Detail faktúry, PDF, QR, e-mail, web faktúra

- **Detail**: hlavička, read-only položky, **vložený náhľad PDF** (iframe, fullscreen), zdrojové PDF z importu (oddelené od nášho), box **Platby**, box Súvisiace bankové operácie, Prílohy e-mailu, Dokumenty, História PDF, Activity log. Akčné tlačidlá podľa stavu (Stiahnuť PDF, Odoslať, Web faktúra, Označiť zaplatené, Čiastočná úhrada, Upomienka, Storno, Dobropis, Test odoslania, Test upomienky, Vytvoriť šablónu z tejto faktúry, Klonovať). [všeobecné]
- **Evidencia platieb**: viac platieb na doklad (splátky), zdroj (ručne/banka/označené), referencia, mazanie platby vráti doklad medzi pohľadávky; zostatok sa premieta do PDF aj QR (QR vždy na zostávajúcu sumu). [všeobecné]
- **PDF**: logo, dodávateľ, odberateľ (vrátane národných daňových čísel – SK IČO → DIČ → IČ DPH, DE Steuernummer, PL NIP, HU Adószám), číslo, dátumy, bankové spojenie + VS, položky, súhrn, prepočet do domácej meny, **QR platba**, päta, voliteľne 2. strana výkazu. **Branding**: logo + akcentová farba (čiara, nadpisy, hlavička tabuľky), voliteľne názov firmy vedľa loga; sémantické farby (dobropis červená, storno sivá) zostávajú. Viac **brandingových profilov**. [všeobecné]
- **PDF/A-3b** (overené veraPDF), vložené fonty, sRGB (CMYK logo sa konvertuje), **ISDOC XML vložený do PDF** (badge „ISDOC" pod VS; vypínateľné per dodávateľ). [PDF/A všeobecné; ISDOC CZ → SK: UBL 2.1 / Peppol BIS (napr. ako Factur-X/ZUGFeRD príloha)?]
- **QR platby**: CZK → **SPAYD** (iba s VS, teda nie v koncepte), EUR → **SEPA EPC QR** (aj v koncepte). QR len pri platnom účte (mod-11 / IBAN checksum). Pri spôsobe úhrady karta/hotovosť sa QR ani banka netlačí. [SPAYD CZ → SK: PAY by square; EPC všeobecné]
- **Odoslanie e-mailom**: príjemcovia z kontaktov klienta podľa účelu + e-maily zákazky; v modale je pri každom príjemcovi **zdroj** (kontakt/zákazka/hlavný e-mail) a zoznam sa dá upraviť (to/cc/bcc). Šablóna podľa jazyka klienta (CS/EN), stav → `sent`. Každá faktúra = samostatný e-mail. [všeobecné]
- **Prílohy e-mailu** k faktúre (aj ku konceptu): drag & drop, 10 MiB/súbor, 20 MiB/faktúra, kontrola reálneho obsahu; nepribaľujú sa k upomienkam. [všeobecné]
- **Kópia dodávateľovi** (neposielať/CC/BCC) zvlášť pre odoslanie dokladu, upomienky a schvaľovanie. [všeobecné]
- **Poďakovanie za úhradu**: automaticky pri spárovaní z banky, predzaškrtnuté pri ručnom označení, voliteľne s PDF opečiatkovaným „Uhradené". [všeobecné – vzor]
- **S/MIME podpis e-mailov** a **PAdES podpis PDF** (viď kap. 18). [všeobecné]
- **Web faktúra** (`/invoice/{48-znakový token}`): trvalý verejný odkaz, lazy generovaný, kopírovať/otvoriť, automaticky v e-maile („Zobraziť faktúru online"), live stav úhrady, stiahnutie PDF aj príloh, **badge „👁 Zobrazené klientom"** s dátumom posledného zobrazenia, „Vygenerovať nový odkaz" = revokácia. [všeobecné – výborný vzor]
- **História PDF**: všetky archivované verzie s dôvodom (Odoslané – nikdy sa nemaže, Vystavenie, Úprava, Zmena výkazu, Zmena bank. údajov), pri odoslaných aj zoznam príjemcov; Zobraziť/Stiahnuť. [všeobecné – vzor]
- **Zmena bankového účtu** v číselníku mien invaliduje PDF iba konceptov; vystavené majú immutable `bank_snapshot`. [všeobecné]

---

## 7. Pravidelné fakturácie (recurring)

- Šablóna: periodicita mesačne/štvrťročne/polročne/ročne, deň 1–28 alebo **posledný deň mesiaca**, dátum prvého vystavenia, voliteľný dátum ukončenia (stav *Vypršala*). Založenie aj **„Vytvoriť šablónu z tejto faktúry"**. [všeobecné]
- Metadáta: typ (faktúra/zálohová), mena, jazyk, spôsob úhrady, splatnosť, % zľava, pevná kategória tržby, režim s/bez DPH, **režim DUZP** („rovnaké ako vystavenie" alebo „posledný deň predchádzajúceho mesiaca" – fakturujem 1.6. za máj), poznámky nad/pod položkami. [všeobecné]
- Položky ručné alebo napojené na cenník s politikou **Pevná cena / Vždy aktuálna cena / Pri zmene vyžadovať kontrolu** (generovanie sa zastaví, v editore „Prevziať aktuálne údaje"). [všeobecné]
- **Placeholdery obdobia** v popisoch a poznámkach (vyhodnotené voči DUZP): `{YYYY}`, `{YY±n}`, `{M}`, `{MM±n}`, `{MMMM}` (názov mesiaca podľa jazyka dokladu), `{Q}`, `{D}`, `{DATE}`, `{DATE+1Y-1D}` (dátumová aritmetika s orezaním na koniec mesiaca), `{BOM}`, `{EOM±n}`; neznáme tokeny zostanú nezmenené; nápoveda je inline v editore. Starší režim „Synchronizovať mesiac v popisoch s DUZP" (detekcia `M/YYYY`, `YYYY-MM`…). [všeobecné – výborný vzor]
- Automatizácia: **po vygenerovaní rovno vystaviť**, **po vystavení rovno odoslať** (predvolene obe zapnuté). [všeobecné]
- **Otvorený koncept na začiatku obdobia** (fixné SLA + priebežné viacpráce): 1. dňa mesiaca cron vytvorí koncept s dátumom vystavenia/DUZP = koniec obdobia, počas mesiaca sa doň píše výkaz, X dní pred koncom príde pripomienka e-mailom, deň po konci obdobia cron koncept uzavrie, prepočíta, vystaví, odošle a hneď otvorí nový. [všeobecné – výborný vzor]
- Životný cyklus Active / Paused / Expired; akcie **Pozastaviť/Obnoviť**, **Vygenerovať teraz** (date picker, žlté varovanie pri budúcom dátume; voľba „nahradiť plánovaný termín a posunúť plán" vs. mimoriadna faktúra; dialóg ukáže výsledný budúci termín), **Vygenerovať koncept**, **Zmeniť príští termín** (s kontrolou existujúcich faktúr a potvrdzovacím checkboxom). [všeobecné]
- **Banner „Generovanie zlyhalo"** na detaile + odznak v zozname (posledná chyba cronu). Catch-up po výpadku cronu = iba 1 faktúra za cyklus a deň. Kill-switch per dodávateľ. Badge **↻ Pravidelná** na vygenerovanej faktúre s odkazom na šablónu. Spoločný zámok proti súbehu cron/ručné generovanie. [všeobecné]

---

## 8. Klienti a dodávatelia (protistrany)

- Jedna tabuľka `clients` s príznakmi `is_customer` / `is_vendor` (firma môže byť oboje). [všeobecné]
- **Zoznam**: meno, IČO, štát, mena, hlavný e-mail, badge **Platiteľ DPH áno/nie**, obrat letos; vyhľadávanie meno/IČO, filter Aktívni/Archivovaní; tlačidlo Zmazať iba ak nemá doklady (inak 409 a skryté). [všeobecné]
- **Nový klient (modal)**: IČO → **Načítať z ARES** (názov, DIČ, adresa, právna forma, platcovstvo); **Overiť DIČ (VIES)** s cache 24 h a badge „VIES OK"; pre zahraničné DIČ „Detaily platiteľa DPH" cez VIES; pre CZ DIČ registr plátců (zverejnené účty, nespoľahlivý platiteľ). [ARES CZ → SK: RPO/ORSR/ŽRSR, FinStat?; CRPDPH CZ → SK: zoznamy FS SR (platitelia DPH, dôvody na zrušenie registrácie §69 ods. 14) ?; VIES všeobecné]
- **SK klient**: po výbere štátu Slovensko sa pole DIČ premenuje na **IČ DPH**, pribudne pole **DIČ** (bez prefixu), VIES ho predvyplní; PDF tlačí IČO → DIČ → IČ DPH. Podobne DE/AT Steuernummer, PL NIP, HU Adószám. [už SK-ready]
- Polia: jazyk (CS/EN), predvolená mena, predvolená DPH, reverse charge, splatnosť (predvoľby), interná poznámka, prepínač automatických upomienok, predvolená kategória tržby.
- **E-mailové kontakty podľa účelu** (max. 10): e-mail, meno, popisok, účely (**Doklady / Upomienky / Schvaľovanie / Komunikácia**), rola to/cc/bcc, aktívny; pravidlá výberu príjemcov s fallbackmi (upomienky → doklady → hlavný e-mail), deduplikácia, tlačidlo „Prevziať hlavný e-mail". [všeobecné – výborný vzor]
- **Detail klienta** – záložky Prehľad (kontakt, predvoľby, obraty letos/loni, počty faktúr podľa stavu), Zákazky, Faktúry (filter + stránkovanie), Aktivita; panel Dokumenty; tlačidlo „Poslať odkaz na sledovanie výkazu práce". Archivácia/obnova. [všeobecné]

---

## 9. Zákazky (projekty)

- 1:N pod klientom; zoznam s filtrom klienta (stav Aktívne/Pozastavené/Uzavreté, hodinová sadzba, mena, splatnosť, obrat letos). [všeobecné]
- Polia: názov, **číslo projektu** a **číslo zmluvy** (tlačia sa v rámčeku na PDF a exportujú do ISDOC OrderReference/ContractReference), hodinová sadzba, mena, splatnosť (zákazka prebíja klienta aj dodávateľa), **rozpočet celkom/ročne/mesačne** (varovanie v editore), stav, poznámka. [všeobecné]
- **Fakturačné e-maily zákazky** (až 3, s účelmi) + režim kombinácie s kontaktmi klienta (Predvolený / Vždy pridať / Vždy nahradiť). [všeobecné]
- Prepínač „Vyžaduje schválenie výkazu zákazníkom" (kap. 5.1). [všeobecné]
- Detail: prehľad + obrat letos/loni/celkom, faktúry, výkazy víceprací. Mazanie iba bez faktúr. [všeobecné]

---

## 10. Cenník

- Iba admin; položka: kód (unikátny per dodávateľ), názov, fakturačný popis, jednotka, sadzba DPH, režim s/bez DPH, povinná základná cena v jednej mene; pevné ceny v ďalších menách; voľba **„Povoliť prepočet kurzom"** s náhľadom (zdrojová cena, krížový kurz, dátum lístka); **individuálne ceny zákazníkov** per mena; poradie vyhodnotenia ceny v 4 krokoch; mazanie používanej položky = archivácia. Zobrazuje počet napojených šablón zákazníka. [všeobecné; kurz ČNB → SK: ECB]

---

## 11. Prijaté faktúry (nákup)

- **Stavy**: draft → received (prijatá) → booked (zaúčtovaná) → paid; cancelled. Mazať iba koncept. [všeobecné]
- **Zoznam**: filtre stav, typ dokladu, dodávateľ, iba neuhradené, po splatnosti, **„Ke kontrole"** (AI varovanie), **predané k úhrade áno/nie**, **importná dávka**; žlté zvýraznenie riadkov s varovaním. **Hromadné akcie**: označiť prijaté / zaúčtované / zaplatené, stornovať, zmazať koncepty, **Do príkazu k úhrade**, nastaviť typ dokladu; výsledok „provedeno X, chyby Y". [všeobecné]
- **Editor**: drag & drop zóna nad formulárom (PDF, fotka JPG/PNG/WEBP/HEIC, ISDOC/ISDOCX; 20 MiB; SHA-256 deduplikácia); karta „súbor pripravený, nahrá sa po uložení"; PDF s vloženým ISDOC predvyplní formulár, inak AI extrakcia alebo ručne. Originál (aj strojový ISDOC) sa archivuje ako dôkaz. [ISDOC CZ → SK: UBL/Peppol?; inak všeobecné]
- Polia: dodávateľ (autocomplete + „+ nový dodávateľ" s ARES), číslo dokladu dodávateľa (unikátne per dodávateľ+dátum), **interné číslo** podľa šablóny `{PP}{YY}{MM}{CCC}` kde prefix kóduje daňový typ (PF/PN plný nárok uznateľný/neuznateľný, KU/KN krátený, NU/NN bez nároku) – pri zmene typu sa prepíše iba prefix; typ dokladu (faktúra, doklad o úhrade/účtenka, dobropis, záloha), dátum vystavenia, DUZP, splatnosť, dátum prijatia, mena, **kurz k DUZP** (tlačidlo „Načítať z ČNB"), reverse charge, prepínač „Dodávateľ je platiteľ DPH" (z ARES/VIES, pri neplatiteľovi vynúti „bez nároku" a sadzby 0 %), platobný účet dodávateľa, kategória nákladu. [CZ → SK: kurz ECB; ostatné všeobecné]
- **Klasifikácia**: nárok na odpočet Plný / Bez nároku / Krátený (%), daňovo uznateľný náklad áno/nie; DPH klasifikačný kód na riadku (auto-default). [CZ → SK: riadky DPH priznania SK, § 49–51 ZDPH SK]
- **Rekapitulácia DPH podľa dokladu** – prepis základu/DPH per sadzba podľa originálu (zvýraznené ručne upravené pole, odkaz „Spočítať automaticky"). [všeobecné – vzor]
- **Platba v inej mene** (faktúra USD platená z CZK účtu) → ekvivalent + kurzový rozdiel. [všeobecné]
- **Spárovanie zálohy s vyúčtovacou faktúrou** z oboch strán, ponuka zoradená podľa meny a najbližšej sumy; **AI návrh prepojenia** (text „zaplatené zálohou č. X") na potvrdenie/zamietnutie. Pravidlá, aby sa náklad nepočítal 2×. [všeobecné]
- **Zaplatiť pomocou QR** – QR pre dodávateľa (SPAYD/EPC) z účtu získaného z ISDOC → AI rozpoznanie (jednorazovo) → ručne → fallback zobrazenie obrázka QR nájdeného v PDF. [CZ → SK: PAY by square]
- **Sken inbox adresára** (tlačidlo „📥 Naskenovať inbox" aj cron): rekurzívne, SHA-256 dedupe, PDF s ISDOC a samostatné .isdoc/.isdocx sa importujú, plain PDF sa preskočí; modal s výsledkom per súbor; max 500 súborov/beh. [všeobecné]
- Exporty per doklad: **naše PDF (rekonštrukcia)**, ISDOC (inverzia rolí, interné číslo v extension), Pohoda XML (`pur:purchase`); hromadne za mesiac/štvrťrok podľa DUZP/vystavenia/prijatia (PDF ZIP preferuje originál, inak rekonštrukcia s príponou `-rekonstrukce`). [ISDOC/Pohoda CZ → SK: UBL, Pohoda SK XML, Money S3 SK, Omega (KROS) import?]
- Úpravy po prijatí: admin odomknutie s auditom (ako pri vydaných). [všeobecné]

---

## 12. AI extrakcia (Anthropic Claude, BYOK)

- API kľúč per dodávateľ (šifrovaný); default lacnejší model (Haiku), **automatický upgrade na silnejší model** (Sonnet), ak výsledok vyzerá slabo (dodávateľ = vlastná firma, nesedia súčty). [všeobecné]
- **Sanity check**: súčet riadkov vs. AI prečítaný základ; rozdiel >2 % → flag „Ke kontrole" (žltý riadok, ⚠ v zozname, filter, banner s diagnostikou, tlačidlo „Beriem na vedomie"; flag zmizne aj posunom stavu). [všeobecné – vzor]
- Katastrofálny nesúlad (>50 %): zachová popisy, vynuluje množstvá/ceny a pridá riadok **KOREKCIA** s AI totalom – používateľ doplní a korekciu zmaže. [všeobecné – vzor]
- Overenie platcovstva dodávateľa (ARES/VIES + signál „Neplátce DPH") → vynúti bez nároku. [CZ → SK: RPO/zoznam platiteľov DPH FS SR]
- **Reverse charge zo zahraničia**: AI klasifikuje tovar vs. služba, nastaví tuzemskú sadzbu a klasifikačný kód, dopočíta zákonné DUZP (15. deň mesiaca po dodaní) a kurz; zapíše informačné varovanie. [CZ → SK: § 19/§ 69 ZDPH SK, riadky SK priznania]
- AI import účteniek nastaví brutto režim; AI rozpoznanie platobného účtu z PDF; AI/heuristika aj pre **faktúry čerpacích staníc** v knihe jázd. CLI `recheck-ai-extracted-invoices` (dry-run / `--apply`). [všeobecné]
- Princíp „AI vyťažuje, človek rozhoduje" – nič sa nepotvrdí automaticky. [všeobecné]

---

## 13. Platobné príkazy

- Výber **účtu platiteľa** (z bankových účtov dodávateľa per mena), dátum splatnosti (minulý sa posunie na dnes), KS, poznámka; príkaz je vždy jednomenový. [všeobecné]
- Dve tabuľky: faktúry v CZK (ABO) a ostatné meny (CSV/PDF); vyberateľné iba s účtom a v mene účtu; zobrazuje zdroj účtu (ISDOC/AI/QR/ručne), stav overenia, VS, sumu; prepínač „Skryť už zaradené"; inline „Doplniť účet" / tužka; rýchle akcie **QR / náhľad PDF / detail**. [všeobecné]
- **Overenie účtu príjemcu** proti registru platiteľov DPH (zverejnený / nezverejnený / nespoľahlivý platiteľ). [CZ → SK: ? (SK nemá povinne zverejňované účty; zoznamy rizikových platiteľov FS SR)]
- Akcie: Iba označiť, **Export CSV**, **Export PDF** (na šírku), **Export ABO (KPC)**; voliteľne rovno označiť zaplatené. Príznak „Predané k úhrade" je samostatná dimenzia, nie stav. **História príkazov** so snapshotom a opätovným stiahnutím. [ABO/KPC CZ → SK: SEPA XML pain.001]

---

## 14. Banka

- **Import výpisov GPC/ABO** (KB, Fio, ČSOB, RB, ČS, mBank) a **PDF výpisov** bez AI (Creditas, ČSOB, KB, RB) s kontrolou súčtov na počiatočný a konečný zostatok; mix súborov naraz; SHA-256 dedupe; validácia, že účet patrí dodávateľovi. [GPC/ABO CZ → SK: camt.053 / MT940 / CSV (Tatra, SLSP, VÚB)?]
- **Auto-párovanie** podľa VS + sumy (tolerancia), čiastočné platby, cudzia mena s devízovou toleranciou, odchádzajúce platby na prijaté faktúry podľa platobného VS / interného čísla / čísla dodávateľa (normalizácia núl a oddeľovačov), kartové platby podľa sumy + podobnosti názvu. Hláška „Importované X, spárované Y, na ručné párovanie Z". [všeobecné]
- **Zoznam výpisov** (dátum, číslo, účet, mena, príjmy, výdaje, spárované `12/14`, importoval kto/kedy); **detail výpisu** s filtrom stavu (Spárované / Bez zhody / Ignorované). [všeobecné]
- **Detail transakcie** (ikona oka): všetky údaje + akcie podľa stavu: Vytvoriť faktúru (z odchádzajúcej platby vznikne koncept prijatej faktúry s placeholderom `BANK-{id}`), Spárovať, Ignorovať (s poznámkou do 1000 znakov), Zrušiť spárovanie, Zrušiť ignorovanie; po akcii sa aktualizuje iba riadok, filter ostáva. [všeobecné – vzor]
- **Ručné párovanie** – modal s vyhľadávaním; **Zlúčená úhrada**: aplikácia sama hľadá kombinácie faktúr jedného klienta so súčtom = platba v okne ±7 dní (rozšíriteľné na ±14), „vyber jednu faktúru a dohľadaj zvyšok". [všeobecné – výborný vzor]
- **Bankové e-mailové avíza cez IMAP**: mapovanie účtov, systémové parsery (RB, UniCredit, ČSOB, ČS, Fio, Creditas, MONETA, Air Bank) + vlastné **regex parsery** s testom v UI; prenos ignorovaní z avíz do neskoršieho výpisu (výber, „Vybrať všetko"). Import bankových pohybov aj z iDokladu. [parsery CZ → SK: parsery SK bánk (Tatra, SLSP, VÚB, ČSOB SK)]
- **Cron sken adresára** výpisov (napr. SFTP/e-mail export banky). Stavy/zostatky na účtoch, graf vývoja zostatku. [všeobecné]

---

## 15. Upomienky

- 3 cesty: ručne z detailu, hromadne zo zoznamu, **automaticky cronom** (prah dní po splatnosti per dodávateľ – predvoľby 3 dni/týždeň/mesiac/vlastné, cooldown, `--dry-run`). [všeobecné]
- Povolenie na 3 úrovniach: dodávateľ ∧ klient ∧ faktúra. Ručné a hromadné odoslanie funguje vždy. [všeobecné – vzor]
- Šablóna `invoice_reminder` s placeholdermi (VS, suma, mena, splatnosť, dni po splatnosti, `reminder_count` pre eskaláciu tónu Twig podmienkou), CS/EN, HTML + text. Samostatná `proforma_reminder` pre nezaplatené zálohy. **Test upomienky** na vlastný e-mail. [všeobecné]
- **Odoslané e-maily** – prehľad všetkých odoslaní vrátane **neúspešných** (červený riadok, text chyby, filter, skratka „Neodoslané: N"). **SMTP log analýza** (hMailServer/MailEnable logy: doručené/odložené/odmietnuté, problémové cieľové servery, väzba na faktúru). [všeobecné; SMTP log parser je skôr niche]
- Pozn.: žiadne úroky z omeškania / penále ani viacstupňové „1./2./3. upomienka" ako samostatné doklady – iba e-mail. [všeobecné – medzera]

---

## 16. CRM, Tržby, Náklady (analytika)

- **CRM**: KPI Tržby/Náklady/Zisk tento mesiac s trendom, 12 mesiacov klzavo a YTD s medziročnou zmenou %; tabuľka **porovnania období** (tento/minulý mesiac, 12 m, YTD, minulý rok) s maržou a preklikmi; grafy zisku (12 m stĺpce + linka minulého roka; kumulatívny YTD vs. loni); mesačný trend tržby vs. náklady; top klienti a dodávatelia (Pareto, podiel %); **aging** pohľadávok a záväzkov (v termíne/1–30/31–60/61–90/90+); **DSO, DPO, platobná morálka, koncentračné riziko** (top 1 klient %, low/medium/high, koľko klientov tvorí 80 %), cyklus pracovného kapitálu; náklady a tržby podľa kategórií; **churn risk** (60+ dní bez objednávky); **cash-flow forecast na 4 týždne**; tabuľky po rokoch/mesiacoch s preklikmi do filtrovaných zoznamov; filter obdobia 3/6/12/24 m a meny („Všetko v CZK"). Pre platiteľa DPH bez DPH, cash metriky s DPH. [všeobecné]
- **Tržby**: klzavý 12-mesačný obrat, **obrat pre registráciu DPH** s progress barom (limit 2 000 000 / 2 536 500 Kč), naplnenie pásma paušálnej dane, predikcia roka (medián 3 metód + rozsah), top klienti/zákazky, stav zákaziek, histogram doby úhrady, rozpad podľa sadzby DPH, cash-flow YTD, distribúcia veľkosti faktúr. [limit CZ → SK: 50 000 € / 62 500 € (od 2025); paušálna daň → SK nemá]
- **Náklady**: zrkadlovo – odhad nákladov roka, nezaplatené záväzky, odhad budúcich platieb podľa splatností, nárok na odpočet podľa sadzby. [všeobecné]

---

## 17. Dokumenty (DMS)

- Úložisko per dodávateľ: **virtuálny strom priečinkov** (presun okamžitý), **obojstranné väzby** na vydanú/prijatú faktúru, klienta, zákazku (panel „Dokumenty" v detailoch + našepkávač „Súvisí s" podľa čísla, firmy, e-mailu, IČ/DIČ, projektu), **tagy**. [všeobecné]
- Nahrávanie: súbory, **celý priečinok** (štruktúra sa zrekonštruuje), drag & drop priečinkov kamkoľvek; ZIP v režime „Rozbaliť a kategorizovať" alebo „Ako jeden súbor"; ochrana zip-bomb / Zip Slip / XXE; spustiteľné, HTML, SVG odmietnuté. [všeobecné]
- **ZFO datové zprávy** – automatické rozbalenie metadát a príloh, napárovanie P7S podpisov. [CZ → SK: správy zo schránky slovensko.sk (ÚPVS) – ASiC-E/XAdES kontajnery?]
- Náhľady (thumbnail + inline) pre PDF a obrázky, ostatné len stiahnuť; **fulltext v obsahu** (PDF s textom, Office, XML), mriežka/zoznam, hromadné akcie nad súbormi aj priečinkami (presunúť cez stromový výber, otagovať, stiahnuť ZIP so štruktúrou, zmazať), veľkosť priečinka v dlaždici, mobil: prvé ťuknutie odkryje akcie, druhé spustí. **Kôš** s obnovou a vysypaním (fyzické zmazanie iba ak súbor nik iný nereferencuje – dedupe podľa hashu). Samostatná záloha. [všeobecné]

---

## 18. Elektronické podpisy

- Podpisové profily (P12/PFX), politika hesla (šifrovane v DB / passphrase súbor), TSA časová pečiatka (PAdES-T), dôvod podpisu; konfigurácia per výstup (vydaná faktúra, výkaz práce, hromadný export, e-mail s faktúrou → PDF alebo S/MIME), stratégia profil dodávateľa vs. prihlásený používateľ, fallback a politika pri chybe (vrátiť nepodpísané / zastaviť / preskočiť), override na konkrétnom doklade, audit udalostí. [všeobecné EU – eIDAS]

---

## 19. Kniha jázd

- Autá (typ vrátane elektro, ŠPZ, počiatočný tachometer), jazdy (dátum, čas, odkiaľ→kam, účel, kategória služobná/súkromná, tachometer predvyplnený posledným stavom), **import CSV/XLSX** (dopočet km, auto-založenie kategórií), export, tankovanie/nabíjanie ručne alebo **načítanie z prijatých faktúr čerpacích staníc** (detailné výpisy napr. Axigon), súhrny a grafy km. [CZ → SK: kniha jázd pre daňové výdavky SK – väčšinou všeobecné]

---

## 20. Dane a výkazy

- **Daňový sprievodca** (manuál) – platiteľ/neplatiteľ, identifikovaná osoba, RC, OSS/IOSS obmedzenia. [CZ → SK: osoba registrovaná podľa § 7/7a]
- **Priznanie k DPH (DPHDP3)**: prepínač mesačne/štvrťročne, výber obdobia, riadne/opravné/dodatočné, 4 KPI (DPH na výstupe, na vstupe, daň/nadmerný odpočet, **termín podania s odpočtom dní**), trend graf, tabuľky riadkov, **XML pre EPO** s XSD validáciou; povinné polia (FÚ, ÚzP, NACE našepkávačom z číselníka) – náhľad sa nikdy neblokuje, blokuje sa iba stiahnutie XML. [CZ → SK: DPH priznanie XML pre eDane/PFS]
- **Kontrolné hlásenie (DPHKH1)** sekcie A.1–A.5, B.1–B.3, prah 10 000 Kč. [CZ → SK: kontrolný výkaz DPH (A.1, A.2, B.1–B.3, C.1, C.2, D.1, D.2) – iná štruktúra]
- **Súhrnné hlásenie (DPHSHV)** – kódy plnenia 0/1/2/3, mesačne/štvrťročne. [CZ → SK: súhrnný výkaz]
- **OSS priznanie** (OSSEI1, štvrťročne, ručne označené riadky). [všeobecné EU, SK cez PFS]
- **Kniha DPH** – mesačný žurnál v PDF/A so stĺpcom sekcie KH. [CZ → SK: evidencia DPH § 70 SK]
- **Daň z príjmov** DPFO (DPFDP5) / DPPO (DPPDP9) – iba orientačný základ (tržby − náklady), XML kostra, CSV, termíny. Príjmy mimo základu (oslobodený predaj, prefakturácia). [CZ → SK: DPFO typ B / DPPO]
- **Daňový optimalizátor (OSVČ)** – retrospektíva a predikcia: paušálna daň vs. paušálne výdavky 40/60/80 % vs. skutočné výdavky, odvody SP/ZP, ročné daňové konštanty v číselníku. [CZ → SK: paušálne výdavky 60 % (max. limit), 15 % / 19 % / 25 % sadzby, odvody SP/ZP SK; SK nemá paušálnu daň]
- **Archív podaní** (`tax_submissions`) – snapshoty vygenerovaných XML. [všeobecné]
- **Hromadný export pre účtovníka (ZIP)** za mesiac/štvrťrok, beh na pozadí: vydané (PDF/ISDOC), prijaté (PDF s preferenciou originálu/ISDOC), výpisy (PDF/GPC), Kniha DPH, s daňovo korektným zaradením do obdobia a priečinkovou štruktúrou. [všeobecné; formáty CZ → SK]

---

## 21. Exporty a importy (vydané faktúry)

- **Systém → Exporty**: formát PDF ZIP / jedno spojené PDF (voliteľne podpísané) / **ISDOC 6.0.2** (validované proti XSD, ZIP) / **Pohoda XML** dataPack (kódy per dodávateľ) / **Stereo XML**; obdobie mesiac alebo Q1–Q4; filter podľa dátumu vystavenia alebo DUZP; typ. [ISDOC → SK: UBL/Peppol; Pohoda → Pohoda SK / Money S3 / Omega; Stereo → ?]
- **Importy**: Pohoda XML, ISDOC, ISDOCX, PDF/A-3 s vloženým ISDOC – klient podľa IČO (doplnenie z ARES), zákazka podľa čísla, zachovaný pôvodný VS, pravidlo „splatnosť staršia ako 30 dní → zaplatená", preskočenie cudzieho dodávateľa a duplicít, report. [CZ → SK: import Pohoda SK / UBL]
- **API sync z iDokladu a Fakturoidu** (OAuth2 client credentials, šifrované): kontakty, faktúry, dobropisy, prijaté faktúry, účtenky, bankové pohyby; voľba rokov a sekcií, **dry-run predvolene**, beh na pozadí s progress barom (polling), detailný log, tlačidlo Zrušiť; otvorené doklady sa zakladajú ako vystavené s vypnutými automatickými upomienkami. [CZ → SK: SuperFaktúra, Faktúrka.sk, iKros/Omega?]

---

## 22. Multi-supplier (viac firiem)

- Neobmedzený počet dodávateľov (OSVČ + s.r.o., holding, účtovná kancelária); **prepínač v hornej lište**; izolované: klienti, zákazky, faktúry, meny a bankové účty, číselné rady a šablóny čísel, predvoľby (splatnosť, sadzba, režim cien), e-mailové šablóny, Pohoda kódy, From/Reply-To, odosielacie e-mailové profily (vlastný SMTP), branding, AI kľúč, štatistiky, dokumenty, kill-switch recurring, prah upomienok, poďakovanie za úhradu. Zdieľané: používatelia (s obmedzením firiem), systémové číselníky, bezpečnosť, log. API cez hlavičku `X-Supplier-Id` / token viazaný na firmu. [všeobecné]

---

## 23. Nastavenia a číselníky

- **Môj dodávateľ**: identifikácia, adresa (ulica, č. popisné, č. orientačné), logo, podpis, typ poplatníka FO/PO, periodicita DPH, identifikovaná osoba, OSS, kódy FÚ/ÚzP/NACE, oprávnená osoba a zostavovateľ (pre EPO), číslovanie, predvoľby, ISDOC v PDF, recurring kill-switch, upomienky, poďakovanie, kópie e-mailov. [CZ špecifiká polí → SK: kód daňového úradu, SK NACE]
- **Číselníky**: meny + bankové účty (CZ číslo/kód banky alebo IBAN/BIC), sadzby DPH s platnosťou od–do (zmena sadzby = nový riadok), DPH klasifikačné kódy (mapovanie na riadky priznania/KH), krajiny, jednotky, kategórie nákladov a tržieb, daňové konštanty per rok. [všeobecné; obsah CZ → SK]
- **E-mailové šablóny** (Twig, CS/EN × HTML/text): `invoice_send`, `invoice_reminder`, `proforma_reminder`, `invoice_payment_thanks`, `invoice_approval`, `recurring_draft_reminder`, šablóny pre sledovací odkaz výkazu a overovací kód, `password_reset`, `login_otp`, `welcome`, `test`; editor s náhľadom a **Test odoslania**. [všeobecné]
- **Odosielacie e-mailové profily** (From, transport globálny / vlastný SMTP), DKIM. [všeobecné]
- **Integrácie** (iDoklad, Fakturoid, AI kľúč). **API tokeny**. **Upgrade** na MyÚčto. [všeobecné]

---

## 24. REST API

- `/api/v1/*` – **189 ciest** v `openapi.yaml` (README uvádza 101 endpointov), kurátorovaná podmnožina: invoices (38), purchase-invoices (26), crm (24), reports (18), clients (12), settings, logbook, codebooks, projects, documents, recurring, bank-statements, bank-transactions, kategórie, search, health, version. [všeobecné]
- **Osobné prístupové tokeny (PAT)** so scope `read` / `read_write`, viazateľné na firmu, `last_used_at`, okamžitá revokácia; rola používateľa sa kontroluje pred scope; rate limit 600 req/min/token (`Retry-After`). [všeobecné]
- Dokumentácia: **Swagger UI** (`/api/docs`, Try it out), **Redoc** (`/api/reference`), **Scalar** (`/api/scalar`), surový OpenAPI 3.1 (import do Postmanu, Make, Zapier). [všeobecné]
- Zaujímavé endpointy: nastavenie dodávateľa a **počítadla číselnej rady** s náhľadom ďalšieho čísla, upload loga, export faktúr (`format=pdf-zip|isdoc|pohoda|stereo&month=`), spojené PDF podľa `ids`, ISDOC jednej faktúry, recurring pause/resume/run-now. [všeobecné]
- Chýba: webhooks, OAuth2, Idempotency-Key, admin/settings mutácie. [medzera]

---

## UX vzory hodné prevzatia

1. **Zoznam faktúr zoskupený po mesiacoch so sticky hlavičkou** + checkbox „označiť celý mesiac" – prirodzené pre mesačný rytmus fakturácie.
2. **„Vystaviť znovu (N)"** – hromadné klonovanie s automatickým posunom mesiaca v popisoch; v kombinácii s hromadným Vystaviť a Odoslať = mesačná fakturácia za 5 minút.
3. **Legenda stavov s počtami nad zoznamom ako klikateľné filtre** (zaplatené, odoslané, neodoslané, po splatnosti, upomínané).
4. **Filtre v URL** – každý KPI/graf/tabuľka v CRM je preklik na predfiltrovaný zoznam; menu link filtre resetuje.
5. **Placeholder s náhľadom ďalšieho čísla dokladu** v prázdnom poli čísla.
6. **Matematické výrazy v číselných poliach** (`12*1.21`, `400-100`).
7. **Ctrl+S / Ctrl+N / Esc** a riadky správajúce sa ako odkazy (Ctrl+klik, stredné tlačidlo → nový panel).
8. **Globálne vyhľadávanie** cez menu, klientov, vydané aj prijaté faktúry s klávesovou navigáciou.
9. **„+ Vytvoriť" v hornej lište** a nenápadné „+" pri položkách bočného menu.
10. **Action items na dashboarde s granulárnym skrytím** (deň / týždeň / „iba nové výskyty" / navždy) a „Obnoviť skryté (n)".
11. **Varovný pruh + explicitný modal s vymenovanými dôsledkami + checkbox** pri odomknutí vystaveného dokladu; odomknutie platí iba do refreshu; audit so starým/novým snapshotom.
12. **História verzií PDF** s dôvodom a pri odoslaných verziách so zoznamom príjemcov (dôkaz, čo klient dostal).
13. **Web faktúra s „Zobrazené klientom"** a revokáciou odkazu; live stav úhrady na verejnej stránke.
14. **Schválenie výkazu zákazníkom jedným klikom** → automatické vystavenie a odoslanie; globálny inbox schvaľovania s „5+ dní bez reakcie".
15. **Živý sledovací odkaz na rozpracované hodiny** s overením cez e-mailový kód a maskovanými adresami.
16. **Otvorený koncept počas obdobia** pri pravidelnej fakturácii (paušál + priebežné viacpráce, pripomienka pred uzávierkou, automatické uzavretie).
17. **Placeholdery obdobia** (`{MMMM} {YYYY}`, `{DATE}–{DATE+1Y-1D}`, `{BOM}–{EOM}`) s inline nápovedou.
18. **Dialóg „Vygenerovať teraz"** ukáže výsledný budúci termín a rozlíši nahradenie plánovaného termínu vs. mimoriadnu faktúru.
19. **E-mailové kontakty podľa účelu** (doklady/upomienky/schvaľovanie, to/cc/bcc) a v modale odoslania **zdroj** každého príjemcu.
20. **Trojúrovňový vypínač automatických upomienok** (dodávateľ ∧ klient ∧ faktúra) a **Test odoslania/Test upomienky** na vlastný e-mail.
21. **Prehľad odoslaných e-mailov vrátane zlyhaní** so skratkou „Neodoslané: N".
22. **Zlúčená úhrada** – automatické hľadanie kombinácií faktúr klienta, ktorých súčet = platba, s rozšírením okna a „vyber jednu, dohľadaj zvyšok".
23. **Detail bankovej transakcie s akciami podľa stavu** (vrátane „Vytvoriť faktúru" z odchádzajúcej platby), aktualizácia iba riadku bez straty filtra, poznámka k ignorovaniu.
24. **AI flag „Na kontrolu"** so žltým riadkom, filtrom a bannerom s diagnostikou; riadok **KOREKCIA** pri zlyhaní extrakcie; AI návrh prepojenia zálohy na potvrdenie.
25. **Rekapitulácia DPH s ručným prepisom per sadzba** (zvýraznené pole + „Spočítať automaticky").
26. **Interné číslo prijatej faktúry s prefixom daňového typu** (rýchla vizuálna orientácia v nároku na odpočet).
27. **Drag & drop zóna na prijatú faktúru už pri zakladaní** (karta „nahrá sa po uložení"), podpora fotky z mobilu.
28. **Platobné príkazy**: zdroj účtu (ISDOC/AI/QR/ručne) ako štítok, „Predané k úhrade" ako samostatná dimenzia, opakované stiahnutie zo snapshotu.
29. **Dokumenty**: obojstranné väzby + našepkávač, upload celých priečinkov, fulltext v obsahu, kôš, bezpečné mobilné akcie (dve ťuknutia).
30. **Plánované úlohy s heartbeatom** a varovaniami „Nebežalo/Zlyháva" – odhalí nenastavený cron bez ohľadu na OS.
31. **Setup wizard s ARES predvyplnením a odstrániteľnými ukážkovými dátami**.
32. **DPH výkazy: náhľad sa nikdy neblokuje**, blokuje sa iba stiahnutie XML s vymenovaním chýbajúcich polí; KPI „termín podania" s odpočtom.
33. **Tri stavy témy (Systém/Svetlá/Tmavá)** vrátane grafov; mobilné karty namiesto tabuliek.

---

## Dátový model (hlavné tabuľky)

**Firma, používatelia, bezpečnosť**
- `supplier` – vlastné firmy (dodávatelia) s identifikáciou, daňovým nastavením, brandingom, predvoľbami.
- `users` – používatelia, rola, jazyk, MFA stav. `user_suppliers` – obmedzenie prístupu k firmám.
- `sessions`, `login_attempts`, `login_otps`, `trusted_devices`, `password_resets` – session, brute-force, e-mailové OTP, zapamätané zariadenia.
- `webauthn_credentials`, `webauthn_ceremonies`, `mfa_step_up_proofs`, `mfa_recovery_codes` – passkeys a step-up overenie.
- `api_tokens` – PAT so scope, firmou, `last_used_at`.
- `activity_log` – audit všetkých mutácií. `app_meta`, `cron_runs` – metadáta a heartbeat cronov.

**Protistrany a zákazky**
- `clients` – odberatelia aj dodávatelia (`is_customer`/`is_vendor`), IČO/DIČ/národné DIČ, platcovstvo, predvoľby.
- `client_email_contacts` – kontakty podľa účelu (to/cc/bcc).
- `projects` – zákazky (sadzba, rozpočty, čísla projektu/zmluvy, schvaľovanie). `project_billing_emails` – fakturačné e-maily zákazky.
- `ares_cache`, `vies_cache`, `crpdph_cache` – cache registrov.

**Vydané faktúry**
- `invoices` – hlavička dokladu (typ, stav, VS, dátumy, mena, kurz, snapshoty klienta/dodávateľa/banky, `parent_invoice_id`, `recurring_template_id`, web token, zaokrúhlenie, ceny s DPH).
- `invoice_items` – riadky (sadzba, klasifikačný kód, väzba na cenník).
- `invoice_payments` – evidované platby/čiastočné úhrady. `payment_matches` – väzby bankových transakcií na doklady.
- `invoice_pdfs` – história verzií PDF (dôvod, odoslané, príjemcovia). `invoice_attachments` – prílohy e-mailu.
- `invoice_counters` – počítadlá číselných rád per dodávateľ/typ/obdobie.
- `work_reports`, `work_report_items`, `work_report_materials` – výkaz prác a materiálu k faktúre + schvaľovací token/stav.
- `work_report_links`, `work_report_link_codes`, `work_report_link_sessions` – verejné sledovacie odkazy a ich overovanie.
- `recurring_invoice_templates`, `recurring_invoice_template_items` – šablóny pravidelnej fakturácie.
- `price_list_items`, `price_list_item_prices`, `price_list_customer_overrides` – cenník, ceny v menách, individuálne ceny.

**Prijaté faktúry a platby**
- `purchase_invoices` – prijaté doklady (typ dokladu, stav, interné číslo, nárok na odpočet, uznateľnosť, kurz, platobný účet dodávateľa, AI varovanie, väzba na zálohu, príznak predania k úhrade).
- `purchase_invoice_items` – riadky. `purchase_invoice_counters` – počítadlá interných čísel.
- `payment_orders`, `payment_order_items` – dávky platobných príkazov so snapshotom.
- `import_jobs` – úlohy importu na pozadí (iDoklad/Fakturoid, hromadné exporty).

**Banka**
- `bank_statements` – importované výpisy (hash, účet, zostatky, originál).
- `bank_transactions` – pohyby (symboly, protistrana, stav párovania, ignorovanie + poznámka).
- `bank_email_imap_settings`, `bank_email_account_mappings`, `bank_email_notice_providers`, `bank_email_processed_messages` – IMAP avíza, mapovanie účtov, regex parsery.
- `bank_notice_ignore_transfers` – audit prenosu ignorovania z avíz do výpisov. `external_bank_account_mappings` – mapovanie účtov z externých systémov.

**Číselníky a dane**
- `currencies` (aj bankové účty per mena), `exchange_rates` (kurzy ČNB), `countries`, `units`, `vat_rates` (s platnosťou), `vat_classifications` (mapovanie na riadky výkazov).
- `expense_categories`, `revenue_categories` – kategórie nákladov/tržieb.
- `tax_constants`, `tax_profiles` – ročné daňové konštanty a profil poplatníka pre optimalizátor.
- `tax_submissions` – archív vygenerovaných podaní (XML snapshoty).
- `client_revenue_cache`, `project_revenue_cache`, `crm_monthly_summary`, `crm_action_item_dismissals` – agregácie pre dashboard/CRM a skryté action items.

**E-mail a podpisy**
- `email_templates` – Twig šablóny per dodávateľ/jazyk. `email_profiles` – odosielacie profily (From, SMTP).
- `branding_profiles` – logo/farba pre PDF a e-maily.
- `signing_profiles`, `signing_credentials`, `signing_settings`, `pdf_signature_output_settings`, `signature_user_profiles`, `signature_role_profiles`, `signature_document_overrides` – elektronické podpisy.

**Dokumenty a kniha jázd**
- `documents`, `document_folders`, `document_links`, `document_tags`, `document_tag_map`, `document_dms_messages` – DMS, väzby na entity, tagy, metadáta ZFO.
- `cars`, `trips`, `trip_categories`, `fuelings`, `logbook_fuel_scans` – kniha jázd a tankovanie.
- `sample_data_entries` – evidencia ukážkových dát na ich čisté odstránenie.
