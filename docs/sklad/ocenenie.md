---
title: Ocenenie a účtovanie zásob
slug: ocenenie
sidebar_position: 3
description: Vážený aritmetický priemer, FIFO a predkontácie pre účtovanie zásob spôsobom A.
tags: [sklad, účtovanie, ocenenie]
---

# Ocenenie a účtovanie zásob

## Spôsob ocenenia úbytku

| Spôsob | Ako sa ocení výdaj |
|---|---|
| Vážený aritmetický priemer | po každom príjme sa prepočíta priemerná cena zásoby; výdaj ide za ňu |
| FIFO | vydáva sa najstaršia zásoba za jej obstarávaciu cenu |

Spôsob sa nastavuje v Sklade → Nastavenie. Zmena prepočíta ocenenie všetkých
výdajov od začiatku.

:::warning[Spôsob ocenenia sa nemení len tak]
Zákon o účtovníctve dovoľuje zmeniť spôsob oceňovania iba výnimočne a od
začiatku účtovného obdobia. Vo výučbe je prepínanie užitočné na porovnanie,
v ostrej firme sa poraďte s účtovníkom.
:::

## Zaúčtovanie (spôsob A)

V podvojnom účtovníctve sa skladové doklady zaúčtujú podľa predkontácií:

| Udalosť | Tovar | Materiál | Výrobky |
|---|---|---|---|
| Príjem | 132 / 131 | 112 / 111 | 123 / 613 |
| Výdaj (predaj, spotreba) | 504 / 132 | 501 / 112 | 613 / 123 |
| Vrátenie (dobropis) | 132 / 504 | 112 / 501 | 123 / 613 |
| Inventúrne manko | 549 / 132 | 549 / 112 | 549 / 123 |
| Inventúrny prebytok | 132 / 648 | 112 / 648 | 123 / 648 |

Predkontácie sa dajú upraviť v Účtovom rozvrhu na záložke **Predkontácie**. V jednoduchom
účtovníctve sa hodnota zásob prenesie do výkazu o majetku a záväzkoch.
