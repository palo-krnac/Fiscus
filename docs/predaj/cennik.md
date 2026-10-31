---
title: Cenník
slug: cennik
sidebar_position: 5
description: Položky faktúr s cenou bez DPH alebo s DPH; skladové položky majú kartu produktu.
tags: [predaj, cenník, sklad]
---

# Cenník

Cenník sú položky, ktoré fakturujete opakovane: kód, názov, merná jednotka,
cena **bez DPH alebo s DPH** a sadzba. Druhú cenu dopočíta server. Položku
vyberiete vo faktúre, ponuke aj v pravidelnej faktúre; riadok faktúry sa dá
naopak uložiť do cenníka.

V zozname vidíte náhľad hlavného obrázka, kategóriu, EAN a stav skladu.
Hľadá sa aj podľa EAN, kategórie, značky a dodávateľa. Klik otvorí rýchly
panel, dvojklik alebo tlačidlo **Karta produktu** celú kartu — pozri
[Karta produktu](../sklad/karta-produktu.md).

## Import a export

Export do Excelu obsahuje všetky údaje karty vrátane parametrov
(`Napätie: 230 V; Krytie: IP20`). Rovnaký súbor sa dá načítať späť:

- položka s rovnakým kódom (bez kódu s rovnakým názvom) sa zmení, ostatné
  pribudnú, nič sa nemaže;
- **stĺpce, ktoré v súbore nie sú, ostanú na karte bez zmeny** — súbor iba
  s kódom a cenou teda nezmaže EAN, parametre ani popis.

:::tip[Archivácia namiesto mazania]
Položku, ktorú už nepredávate, archivujte. Zmizne z ponuky vo faktúrach,
ale ostane v histórii a na starých dokladoch.
:::
