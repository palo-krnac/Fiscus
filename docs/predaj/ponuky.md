---
title: Cenové ponuky a pravidelná fakturácia
slug: ponuky
sidebar_position: 3
description: Ponuka s prijatím cez odkaz a faktúry, ktoré sa vystavia samy.
tags: [predaj, ponuky]
---

# Cenové ponuky a pravidelná fakturácia

## Cenová ponuka

Ponuka má vlastný číselný rad (CP), platnosť a stavy *odoslaná*, *prijatá*,
*odmietnutá*, *po platnosti*. Do DPH sa nezapočíta. Na serveri s verejnou
adresou ju zákazník prijme alebo odmietne cez tajný odkaz a môže pripísať
číslo svojej objednávky. Prijatá ponuka sa jedným klikom zmení na koncept
faktúry.

## Pravidelná fakturácia

Šablóna faktúry s intervalom (mesačne, štvrťročne, polročne, ročne) a
koncom. V texte možno použiť `{obdobie}`, `{mesiac}`, `{rok}` a
`{minule_obdobie}`. Fiškus faktúru vytvorí a vystaví sám — pri štarte a
každú hodinu — a ak chcete, aj ju pošle e-mailom.

!!! note "Dobehnuté obdobia"
    Ak program dlhšie nebežal, staršie obdobia sa vytvoria ako koncepty na
    kontrolu, nie ako vystavené faktúry. Odznak v menu ukáže, koľko ich čaká.
