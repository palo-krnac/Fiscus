---
title: Karta produktu
slug: karta-produktu
sidebar_position: 1
description: Všetko o jednom produkte — obrázky, parametre, EAN, ceny, stav po skladoch a pohyby.
tags: [sklad, cenník, karta]
---

# Karta produktu

Kartu otvoríte z cenníka (dvojklik alebo **Karta produktu**) alebo zo Skladu
klikom na zásobu — vtedy sa pohyby rovno vyfiltrujú na daný sklad.

## Súhrn

Pod názvom je riadok ako v účtovnej knihe: predajná cena bez DPH a s DPH,
posledná nákupná cena, množstvo na sklade a hodnota zásoby (dvojito
podčiarknutá). Ak je zásoba pod minimom, množstvo je červené.

## Údaje

| Skupina | Polia |
|---|---|
| Základné | názov (text na faktúre), kód, **EAN**, kategória, značka/výrobca, merná jednotka, hmotnosť |
| Ceny a nákup | predajná cena bez DPH alebo s DPH, sadzba DPH, nákupná cena, dodávateľ, kód u dodávateľa |
| Parametre | ľubovoľný počet riadkov *parameter – hodnota – jednotka* (napr. Výkon – 4 – kW) |
| Popis | popis pre ponuky a e-shop, interná poznámka, archivácia |

EAN sa kontroluje kontrolnou číslicou (EAN-8, UPC-A, EAN-13, GTIN-14) už
pri písaní. Kategóriu píšte s lomkou, napríklad `Káble / Silové` — v cenníku
sa podľa nej dá hľadať a triediť.

## Obrázky

Až 10 obrázkov JPEG, PNG alebo WebP do 2 MB. Pridáte ich tlačidlom alebo
pretiahnutím súborov na galériu. **Prvý obrázok je hlavný** — ukazuje sa
v zozname cenníka. Šípkami meníte poradie, hviezdičkou nastavíte hlavný.

!!! note "Typ sa overuje podľa obsahu"
    Fiškus neverí prípone súboru. Súbor, ktorý sa iba tvári ako obrázok
    (napríklad SVG so skriptom alebo PDF premenované na .png), odmietne.

## Sklad na karte

Prepínač **Vediem na sklade**, druh zásoby a minimálna zásoba. Pod tým
tabuľka skladov s množstvom, priemernou cenou a hodnotou; klik na sklad
vyfiltruje pohyby. Zmena skladu sa uloží spolu s kartou.

## Pohyby

Všetky príjemky, výdajky, presuny a inventúrne rozdiely, najnovšie hore.
Pri každom pohybe vidíte množstvo so znamienkom, cenu za jednotku, stav po
pohybe a hodnotu zásoby. Hľadá sa podľa čísla dokladu, partnera a druhu;
export do Excelu a CSV obsahuje to, čo je vyfiltrované.

:::tip[Úloha: kde sa vzala priemerná cena]
Na karte frekvenčného meniča v ukážkovej firme sú dve príjemky za 268,00 €
a 279,90 €. Vypočítajte vážený priemer ručne a porovnajte ho s cenou pri
výdajke. Potom v Sklade → Nastavenie prepnite ocenenie na FIFO a sledujte,
ako sa zmení cena výdajky aj hodnota zásoby.
:::
