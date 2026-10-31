---
title: E-shopy
sidebar_position: 9
description: Objednávky z e-shopu do faktúr, zásoby do obchodu a riadenie obchodu z Fiškusu.
tags: [e-shop, sklad, predaj]
---

# E-shopy

Napojenie je v **Nastaveniach → E-shopy**. Kľúče a heslá k obchodu sa
uložia na serveri a cez rozhranie sa už nikdy neukážu.

| Obchod | Objednávky → faktúry | Zásoby do obchodu | Riadenie obchodu |
|---|---|---|---|
| WooCommerce | áno | áno | áno |
| Shopify | áno | áno | áno |
| Shoptet | áno | áno | áno |
| PrestaShop | áno | áno | áno |
| Magento / Adobe Commerce | áno | áno | áno |
| Drupal Commerce | áno | áno | — |

:::warning[Neoverené na živom obchode]
Konektory sú napísané podľa oficiálnej dokumentácie jednotlivých obchodov
a pokryté testami, ale zatiaľ neboli vyskúšané na ostrom obchode. Prvé
spustenie urobte na skúšobnom obchode.
:::

## Objednávky

Objednávka sa zmení na **koncept faktúry**: zákazník s IČO, kontaktná osoba,
dodacia adresa, položky prepojené s cenníkom podľa kódu, sadzba DPH podľa
dane v obchode. Voľba „iba uhradené“ prevezme len zaplatené objednávky.
Opakované načítanie nevytvorí duplicitu. Načítava sa automaticky každú
hodinu.

## Riadenie obchodu z Fiškusu

Fiškus môže byť **hlavný zdroj údajov** pre obchod: prevezme z neho produkty
a zákazníkov, potom posiela späť iba zmeny cenníka (názov, cena, trieda DPH,
vyradenie) a stav zásob zo skladu. Nová skladová položka sa v obchode založí
ako skrytý koncept, ktorý doplníte a zverejníte v obchode.

## Odkaz pre vlastný e-shop

Pre obchod bez konektora je v Sklade → Nastavenie tajný odkaz: zásoby a ceny
ako JSON alebo CSV a príjem objednávok, z ktorých vzniknú výdajky.
