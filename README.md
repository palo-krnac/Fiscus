# Fiskus

Slovenský účtovný program pre živnostníkov a malé firmy. Beží ako **desktopová aplikácia** (jedna inštalácia, offline, jedna firma) aj ako **webová aplikácia na serveri** (viac firiem, prístup z prehliadača). Obe verzie majú jeden kód aj jeden frontend.

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

bin/fiskus -db fiskus.db                                   # desktop
bin/fiskus -mode server -addr :8080 -db /data/fiskus.db    # server
FISKUS_BACKUP_PASSWORD=… bin/fiskus -db fiskus.db -backup-dir ~/Fiskus-zalohy
```

Vývoj s okamžitým obnovovaním: `make dev-api` v jednom termináli a `make dev-web` v druhom (http://localhost:5173).

## Register firiem

Pri zadaní IČO (adresár, faktúra, nastavenia) sa názov a adresa doplnia z verejného [Registra právnických osôb](https://api.statistics.sk/rpo/v1) Štatistického úradu SR (dáta pod licenciou CC BY 4.0). Do registra sa posiela iba IČO. Výsledky sa 12 hodín držia v pamäti. Bez internetu program funguje ďalej, len sa údaje nedoplnia. Vypnutie: `-rpo-url ""`.

## Zálohy

Zálohy sa robia automaticky pri štarte, každých 24 hodín a pri ukončení programu. Postup:
1. `VACUUM INTO` vytvorí konzistentnú kópiu aj počas behu programu.
2. `PRAGMA integrity_check` kópiu overí.
3. Kópia sa zašifruje (AES-256-GCM, heslo cez PBKDF2), znova sa dešifruje a porovná s originálom.
4. Záloha sa atomicky zapíše a staré zálohy sa zrotujú: 7 denných, 4 týždenné, 12 mesačných.

Pri obnove sa pôvodná databáza uloží ako `.pred-obnovou`, takže sa nič nestratí.

## Vydanie verzie

`git tag v0.1.0 && git push --tags`. GitHub Actions potom postaví binárky pre Windows, macOS (Intel aj Apple Silicon) a Linux a pripojí ich k vydaniu. Inštalačky (Inno Setup, DMG) pribudnú neskôr.

Plán funkcií: [docs/ROADMAP.md](docs/ROADMAP.md).
