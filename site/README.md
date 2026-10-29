# fiskus.sk

Statický web (HTML, CSS a trochu JavaScriptu, bez zostavovania). Písmo Source Sans 3 je uložené v `fonts/`, takže web nenačítava nič od tretích strán. Výnimkou je Paddle.js, ktorý sa načíta až po kliknutí na platbu.

- `config.js` — verzia na stiahnutie, adresa súborov a nastavenie Paddle. Predaj je vypnutý, kým sú `paddle.token` a `paddle.priceId` prázdne.
- `obchodne-podmienky.html`, `ochrana-osobnych-udajov.html`, `vratenie-platby.html` — **návrhy**. Pred spustením predaja doplňte údaje v hranatých zátvorkách a dajte ich skontrolovať. Paddle ich pri schvaľovaní domény vyžaduje.
- `node site/check.mjs` — kontrola odkazov (beží v CI).

## Nasadenie

Celý priečinok nahrajte na ľubovoľný statický hosting (napr. Cloudflare Pages, Netlify alebo vlastný server). Inštalátory z GitHub Release patria do `stiahnut/<verzia>/` (adresa je v `config.js`).

## Predaj cez Paddle (príprava)

1. V Paddle (najprv sandbox) vytvorte produkt „Fiškus – Server pre viac firiem“ a ročnú cenu 99 €. Jej `pri_…` patrí do `config.js` aj do `PADDLE_PRICE_ID`.
2. V časti Developer tools vytvorte client-side token a vložte ho do `config.js`. Do Paddle pridajte doménu fiskus.sk.
3. V časti Notifications vytvorte cieľ `https://fiskus.sk/paddle/webhook` s udalosťou `transaction.completed`. Tajomstvo patrí do `PADDLE_WEBHOOK_SECRET`.
4. Na serveri spustite licenčnú službu `fiskus-shop` za reverznou proxy s HTTPS (pozri komentár v `backend/cmd/fiskus-shop`). Na podpis použite **ostrý** súkromný kľúč z `fiskus-license keygen`.

Po zaplatení Paddle pošle udalosť. `fiskus-shop` overí podpis, vystaví licenčný kľúč, zapíše ho do evidencie a pošle zákazníkovi e-mailom. Ak doručenie opakuje, licencia sa nevystaví druhýkrát. Obnova nadväzuje na koniec platnosti predchádzajúcej licencie.
