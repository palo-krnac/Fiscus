# Pokyny pre vývoj Fiskus

Komunikácia a texty v rozhraní sú po slovensky. Kód (identifikátory) je po anglicky, komentáre po slovensky.

## Pravidlá
- Peniaze vždy cez `money.Dec`, nikdy `float64`. Vo frontende sú sumy reťazce z API a frontend nikdy nepočíta DPH ani súčty dokladu, len zobrazuje výsledok `/api/invoices/preview`. Výnimkou sú súčty v zozname (súčet už vypočítaných súm).
- Biznis pravidlá patria do `internal/invoice` (doména) a `internal/app` (aplikácia), nie do API ani do frontendu.
- Každá zmena logiky musí mať test. Používaj table-driven testy.
- Každá nová implementácia `store.Store` musí prejsť `storetest.Run`.
- Všetko s dátami firmy je filtrované cez `companyID`.
- Vystavená faktúra sa nesmie meniť, opravuje sa dobropisom.
- SQLite kód je za build tagom `sqlite`: `go test -tags sqlite ./...`.

## Príkazy
- `make test`: všetky testy
- `make demo`: spustenie s ukážkovými dátami
- `cd frontend && npm test`: testy frontendu (vitest)

## Dizajn rozhrania
Hustý ERP štýl: font Source Sans 3 (tabuľkové číslice), atramentová modrá `#1f3a5c` / `#2f5d8c`, mosadzný akcent `#d9a441` iba pre aktívnu položku menu, červený atrament `#b03a26` pre omeškanie. Konečná suma dokladu má dvojité podčiarknutie ako v účtovnej knihe. Tokeny sú v `frontend/src/styles.css`. Každú obrazovku treba skontrolovať aj na mobile (390 px).
