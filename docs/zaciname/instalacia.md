---
title: Inštalácia
slug: instalacia
sidebar_position: 1
description: Inštalačky pre Windows, macOS a Linux, ukážková firma a spustenie na serveri.
tags: [začíname, inštalácia]
---

# Inštalácia

## Na vlastnom počítači

| Systém | Súbor | Poznámka |
|---|---|---|
| Windows | `fiskus-…-windows-setup.exe` | inštalácia bez práv správcu, ikona v ponuke Štart a pri hodinách |
| macOS | `fiskus-…-macos.dmg` | Apple Silicon aj Intel; prvé spustenie cez pravé tlačidlo → Otvoriť |
| Linux | `.deb`, `.rpm` | ikona v ponuke aplikácií |

Po spustení Fiškus otvorí prehliadač na adrese `http://127.0.0.1:48080`.
Dáta sú v priečinku používateľa:

| Systém | Priečinok |
|---|---|
| Windows | `%APPDATA%\Fiskus` |
| macOS | `~/Library/Application Support/Fiskus` |
| Linux | `~/.local/share/fiskus` |

:::tip[Druhé spustenie]
Ak Fiškus už beží, ikona iba otvorí jeho okno. Vo Windows ho ukončíte
pravým tlačidlom na ikone pri hodinách → **Ukončiť**.
:::

## Ukážková firma

Na výučbu a skúšanie je najlepšia ukážková firma **Demo elektroservis**:
faktúry, nákupy, banka, pokladňa, sklad s kartami produktov, majetok aj
kniha jázd.

```sh
make demo                      # zo zdrojového kódu, http://127.0.0.1:8080
fiskus -demo -addr 127.0.0.1:8080
```

Ukážkové dáta sú iba v pamäti — po ukončení programu zmiznú a pri ďalšom
spustení sú znova pôvodné. Študent tak nič nepokazí.

## Na serveri

```sh
fiskus -mode server -addr :8080 -db /data/fiskus.db
```

Serverová verzia má prihlasovanie, povinné dvojfázové overenie, roly
používateľov a pri licencii aj viac firiem. Za reverznou proxy ju spustite
s `-public-url https://vas-server`, aby odkazy (ponuky pre zákazníkov,
pripojenie AI asistenta) mali správnu adresu.

!!! note "Jedna firma je zadarmo"
    Desktop aj server s jednou firmou sú zadarmo. Licencia je potrebná iba
    pre server s viacerými firmami (účtovné kancelárie).
