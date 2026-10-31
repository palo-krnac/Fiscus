---
title: Peniaze
sidebar_position: 6
description: Bankové výpisy s automatickým párovaním a pokladňa s DPH.
tags: [banka, pokladňa]
---

# Peniaze

## Banka

| Formát výpisu | Poznámka |
|---|---|
| camt.053 (XML) | štandard slovenských bánk |
| GPC | starší formát (ABO) |
| CSV, Excel | stĺpce podľa hlavičky |

Po načítaní Fiškus **spáruje platby s faktúrami** podľa variabilného symbolu
a sumy. Čo si nie je istý, ponúkne ako návrh; zvyšok spárujete ručne alebo
necháte ako nepárovaný pohyb (poplatky, vklady), ktorý sa v peňažnom
denníku zapíše ručne. Čiastočné úhrady sa sčítajú.

:::info[Prečo párovanie, a nie odškrtnutie „uhradené“]
Úhrada je udalosť s dátumom a sumou. Z nej sa počíta peňažný denník,
kurzový rozdiel aj stav pohľadávok k ľubovoľnému dňu. Faktúra s odškrtnutým
políčkom by nevedela povedať, kedy a koľko prišlo.
:::

## Pokladňa

Príjmové a výdavkové pokladničné doklady (PPD, VPD) s tlačou a sumou slovom.
Fiškus nedovolí, aby zostatok pokladne klesol pod nulu. Pokladničný doklad
môže uhradiť faktúru v hotovosti.

- **DPH na pokladničných dokladoch** — bloček ako zjednodušená faktúra
  (odpočet, KV B.3.1/B.3.2 podľa limitu 3 000 €), tržba z e-kasy (KV D.1).
- **Viac pokladní** — každá má vlastný zostatok a rad dokladov.
- **Pokladňa v cudzej mene** — kurz ECB ku dňu dokladu.
