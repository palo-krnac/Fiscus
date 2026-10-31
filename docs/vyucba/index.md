---
title: Fiškus vo výučbe
sidebar_position: 3
sidebar_label: Výučba
description: Ako s Fiškusom učiť účtovníctvo — ukážková firma, úlohy podľa tém a čo si na hodine všímať.
tags: [výučba, prehľad]
---

# Fiškus vo výučbe

Fiškus je určený na **štúdium a výučbu účtovníctva** — na stredných
odborných školách, vysokých školách, v kurzoch pre začínajúcich
podnikateľov aj na samoštúdium. Je zadarmo, beží bez internetu a nič
neposiela von.

:::info[Prečo skutočný program namiesto cvičných tabuliek]
Učebnica ukáže zápis, ale neukáže, **odkiaľ sa vzal a kam ide ďalej**.
Fiškus prepája doklad, úhradu, daň a knihu tak, ako sa to deje vo firme.
Študent vystaví faktúru a o minútu ju nájde v kontrolnom výkaze, v knihe
pohľadávok a po úhrade v peňažnom denníku. Chyba v jednom kroku sa ukáže
v ďalšom — presne ako v praxi.
:::

## Príprava hodiny

| Situácia | Odporúčanie |
|---|---|
| ukážka učiteľa | `fiskus -demo` — ukážková firma, po reštarte vždy rovnaká |
| každý študent vlastnú firmu | desktopová inštalácia, úvodné nastavenie od nuly |
| trieda na jednom serveri | serverová verzia, každý študent ako používateľ s vlastnou firmou alebo rolou *Len čítanie* na ukážkovej |
| domáca úloha | export zo zoznamov do Excelu — študent odovzdá súbor |

Ukážková firma **Demo elektroservis** je platiteľ DPH a má vystavené aj
prijaté faktúry, zálohy s vyúčtovaním, banku, pokladňu, sklad s dvoma skladmi
a kartami produktov, majetok a knihu jázd.

## Úlohy podľa tém

| Téma | Úloha | Čo si všímať |
|---|---|---|
| DPH | vystavte faktúru s dvoma sadzbami | rekapitulácia po sadzbách, riadky priznania |
| Opravy dokladov | uhraďte faktúru a skúste ju upraviť | prečo sa dá opraviť iba dobropisom |
| Zálohy | záloha 50 %, úhrada, vyúčtovanie | daň v dvoch obdobiach |
| Banka | načítajte výpis a nechajte ho spárovať | variabilný symbol, čiastočná úhrada |
| Jednoduché účtovníctvo | nájdite úhradu faktúry v peňažnom denníku | rozpis do stĺpcov, DPH mimo základu dane |
| Podvojné účtovníctvo | sledujte faktúru od zápisu po súvahu | 311, 343, 602/604, obratová predvaha |
| Sklad | príjemka, výdajka, inventúra | vážený priemer vs. FIFO, manko na 549 |
| Majetok | zaraďte auto a pozrite odpisový plán | odpisové skupiny, pomerná časť v prvom roku |
| Závierka | vytvorte výkaz o príjmoch a výdavkoch | odkiaľ sa berie každý riadok |

Podrobné úlohy sú pri jednotlivých témach (napríklad
[karta produktu](../sklad/karta-produktu.md) a [zálohy](../predaj/zalohy.md)).

## Čo Fiškus naučí navyše

- **Audit** — každá zmena má autora a čas. Dobrá téma o tom, prečo sa
  v účtovníctve nič „nezmaže“.
- **Legislatíva ako dáta** — sadzby DPH a úroky sa menia; študent vidí, že
  program na to musí byť pripravený.
- **Kontrolné väzby** — faktúra, ktorá je v DPH, nejde len tak prepísať;
  sklad nejde do mínusu; predkontácia musí byť vyrovnaná.

!!! note "Ostré účtovníctvo"
    Ak program použijete na skutočnú firmu, výstupy si nechajte skontrolovať
    účtovníkom — Fiškus nie je certifikovaný softvér.
