# Fiškus

Slovenský účtovný program pre živnostníkov a malé firmy. Beží ako **desktopová aplikácia** (jedna inštalácia, offline, jedna firma) aj ako **webová aplikácia na serveri** (viac firiem, prístup z prehliadača). Obe verzie majú jeden kód aj jeden frontend.

## Edície a ceny

| Edícia | Cena | Pre koho |
|---|---|---|
| Desktop | zadarmo | jedna firma, offline na vlastnom počítači |
| Server – jedna firma | zadarmo | prístup z prehliadača, používatelia a roly |
| Server – viac firiem | 39 € ročne (Paddle) | účtovné kancelárie a poskytovatelia služieb |

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

## AI asistent (MCP)

Fiškus má zabudovaný MCP server na adrese `/mcp`, takže sa k nemu dá pripojiť Claude alebo iný AI asistent. Kľúč si každý používateľ vytvorí v časti **Prístup pre AI**. Asistent má práva svojho používateľa; kľúč môže byť obmedzený len na čítanie. Každé volanie nástroja sa zapíše do auditu.

```
# Claude Code
claude mcp add --transport http fiskus http://127.0.0.1:8080/mcp --header "Authorization: Bearer fk_…"

# aplikácie so stdio (Claude Desktop): most fiskus-mcp
{"mcpServers":{"fiskus":{"command":"fiskus-mcp","args":["-url","http://127.0.0.1:8080/mcp"],"env":{"FISKUS_API_KEY":"fk_…"}}}}
```

Nástroje:
- čítanie: `list_companies`, `company_overview`, `list_invoices`, `get_invoice`, `list_purchases`, `list_contacts`, `list_bank_transactions`, `vat_return`
- zmeny: `create_invoice_draft`, `issue_invoice`, `mark_invoice_paid`, `create_contact`

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
