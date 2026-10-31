---
title: Nákup
sidebar_position: 4
description: Prijaté faktúry, odpočet DPH, vyťaženie z PDF, bločky a e-faktúry.
tags: [nákup, dph]
---

# Nákup

## Prijaté faktúry

Prijatá faktúra sa eviduje z **rekapitulácie DPH dodávateľa** — základ a daň
po sadzbách, tak ako sú na doklade. Pri každej určíte:

| Voľba | Vplyv |
|---|---|
| s odpočtom / bez odpočtu DPH | riadky 19 a 21 priznania, časť B kontrolného výkazu |
| prenesenie daňovej povinnosti | samozdanenie (riadky 09–10) aj odpočet |
| druh výdavku | stĺpec peňažného denníka, predkontácia v podvojnom účtovníctve |
| vozidlo | odpočet 50 % pri osobnom aute (§ 85n) |

Fiškus upozorní na duplicitu (rovnaký dodávateľ a číslo), eviduje úhrady
a k faktúre sa dá pripojiť súbor (PDF, fotka).

:::tip[Vyťaženie z PDF alebo fotky]
S kľúčom Claude API (Nastavenia → Prístup pre AI) Fiškus prečíta dodávateľa,
čísla a rekapituláciu DPH z PDF alebo fotky. **AI iba navrhuje** —
údaje skontrolujete a uložíte sami.
:::

## Bločky a pokladničné doklady

Bloček z eKasy je zjednodušená faktúra. Najrýchlejšie ho zapíšete
[odfotením QR kódu v mobile](../mobil/index.md): údaje (dodávateľ, položky,
DPH) sa doplnia z Finančnej správy a vznikne uhradený prijatý doklad
s fotkou.

## Zvláštne prípady

- **Pomerné odpočítanie DPH (§ 50)** — predbežný a ročný koeficient,
  vysporiadanie v poslednom období roka.
- **Dovoz z tretej krajiny** s DPH zaplatenou colnému úradu.
- **Nadobudnutie tovaru z EÚ** a služby zo zahraničia.
- **Dobropisy od dodávateľov** — oprava odpočtu, riadky C.2.
- **Platobné príkazy** SEPA (pain.001) z neuhradených faktúr pre banku.
