---
title: Skladové doklady
slug: doklady
sidebar_position: 2
description: Príjemka, výdajka, presun medzi skladmi a inventúrny rozdiel.
tags: [sklad, doklady]
---

# Skladové doklady

| Doklad | Množstvo | Cena |
|---|---|---|
| Príjemka | pribudne | zadáte obstarávaciu cenu za jednotku |
| Výdajka | ubudne | vypočíta Fiškus podľa spôsobu ocenenia |
| Presun | ubudne v jednom sklade, pribudne v druhom | ocenenie sa prenesie |
| Inventúrny rozdiel | + prebytok / − manko | prebytok s cenou, manko ocenené ako výdaj |

Doklady sú v Sklade na záložke **Doklady**, s hľadaním, filtrom druhu
a exportom (riadok exportu = riadok dokladu).

!!! warning "Výdaj nad stav"
    Výdajka, ktorá by poslala zásobu do mínusu, sa ukáže ako problém pri
    ocenení. Zásoba v mínuse nemá cenu, ktorou by sa dala oceniť.

Výdajku vytvorenú z faktúry neupravujete priamo — zmení sa spolu s faktúrou.
