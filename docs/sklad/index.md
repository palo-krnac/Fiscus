---
title: Sklad
sidebar_position: 5
description: Sklady, skladové doklady, ocenenie zásob váženým priemerom alebo FIFO a zaúčtovanie.
tags: [sklad]
---

# Sklad

Stav zásoby sa vo Fiškuse **nezadáva, ale skladá z pohybov**. Každé číslo na
karte sa dá vysvetliť príjemkou, výdajkou, presunom alebo inventúrou — a to je
presne to, čo sa má študent naučiť.

:::info[Prečo nemožno prepísať množstvo]
Lebo budúci mesiac sa niekto opýta, prečo je na sklade jedenásť kusov.
Stav zostavený z pohybov má vždy odpoveď; stav, ktorý niekto prepísal, má
iba človeka, ktorý si nespomína. Na skutočné opravy slúži inventúrny rozdiel
— a ten sa zaúčtuje ako manko alebo prebytok.
:::

```cards
- title: Karta produktu
  href: ./karta-produktu.md
  icon: package
  text: Obrázky, parametre, EAN, dodávateľ, stav po skladoch a všetky pohyby.
- title: Skladové doklady
  href: ./doklady.md
  icon: file-text
  text: Príjemka, výdajka, presun a inventúrny rozdiel.
- title: Ocenenie a účtovanie
  href: ./ocenenie.md
  icon: percent
  text: Vážený priemer alebo FIFO, účty 112, 123, 132 a ich protiúčty.
```

## Sklady

Firma má aspoň jeden sklad, môže ich mať viac (napríklad hlavný sklad
a príručný sklad v aute). Sklad má kód a názov. Sklad, na ktorom sú doklady,
sa nedá zrušiť. Výdajky z faktúr idú z prvého skladu v zozname.

## Skladová položka

Skladová položka je položka **cenníka**, ktorú vediete na sklade. Zaradíte
ju na karte produktu prepínačom **Vediem na sklade** alebo v Sklade →
Nastavenie. Určíte jej druh, od ktorého závisia účty:

| Druh | Účet | Typický príklad |
|---|---|---|
| tovar | 132 | kúpené a ďalej predané meniče, káble |
| materiál | 112 | spotrebovaný pri montáži |
| výrobky | 123 | vlastná výroba |

Položku s pohybmi nemožno zo skladu vyradiť.

## Prepojenie s ostatnými dokladmi

| Udalosť | Čo urobí sklad |
|---|---|
| Vystavená faktúra so skladovou položkou | výdajka zo skladu (upravuje sa s faktúrou) |
| Dobropis | príjem vrátenej zásoby ocenený priemernou cenou |
| Prijatá faktúra | príjemka k nej jedným tlačidlom |
| Objednávka z e-shopu | výdajka |
| Výkaz o majetku a záväzkoch (JÚ) | hodnota zásob |
