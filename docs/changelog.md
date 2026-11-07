---
title: Prehľad zmien
slug: zmeny
sidebar_position: 90
description: Čo pribudlo v jednotlivých verziách Fiškusa.
tags: [prehľad]
---

# Prehľad zmien

Verzie sú značky v repozitári (`v0.2.0`). Podrobný zoznam funkcií po
etapách je v [pláne vývoja](./ROADMAP.md).

## 0.7.0

- **Fio banka priamo**: pohyby z účtu vo Fio sa stiahnu tlačidlom cez API
  (token iba na čítanie) a spárujú s faktúrami ako z výpisu.
- **Clo pri dovoze**: rozhodnutie colného úradu ako prijatá faktúra s DPH
  a clom, priradená faktúra za tovar, odpočet DPH pri dovoze a zaúčtovanie.
- **Kurzové rozdiely k 31. 12.** aj pre zostatky pokladníc a bankových účtov
  v cudzej mene (analytické účty, zostatok podľa výpisu).
- **Uzávierky e-kasy** v Pokladni: obrat podľa sadzieb a platby kartou sa
  zapíšu ako PPD na tržbu (DPH, KV D.1) a VPD na peniaze na ceste; pripísanie
  platieb z terminálu v banke sa zaúčtuje 221 / 261.
- **Tlačivá**: pokladničné doklady, dodací list, objednávka, potvrdenie
  o prevzatí, splnomocnenie a vlastné tlačivá. Bez firmy sa vyplnia a
  vytlačia, vo firme sa ukladajú, robia sa z nich vzory a pokladničný doklad
  sa zapíše do pokladne ako účtovný doklad.
- **Kalkulačky** aj bez založenej firmy: DPH, úrok z omeškania, úver
  s RPMN a splátkovým kalendárom, sporenie, čistá mzda a kurzy ECB.
- **Kniha jázd zo záznamu GPS**: trasa z mobilu alebo GPS jednotky (GPX,
  CSV) sa rozdelí na jazdy podľa zastávok, s pomenovanými miestami,
  nadväzujúcim tachometrom a kontrolou už zapísaných jázd.
- **Kurzové rozdiely pri čiastočných úhradách** faktúr v cudzej mene.
- **E-faktúra**: vyúčtovanie zálohy.
- **Domácnosť**: plán platieb po mesiacoch, pokuty a pripomienky; detail
  bločku s položkami a údajmi z eKasy.
- **Vyskúšať na ukážkovej firme**, aj vo verzii v prehliadači.
- **Fiškus bez licencií**: žiadny predaj ani limit počtu firiem.
- Návod a web so snímkami obrazovky na fiskus.online a docs.fiskus.online.

## 0.6.0

- **Domácnosť**: účet Hotovosť hneď po založení, bločky v telefóne aj bez
  účtu.

## 0.5.0

- **Mobil**: výber firmy alebo domácnosti pri bločkoch, diagnostika
  pripojenia v sieti a brána firewall.
- **Bločky**: spoľahlivejšie čítanie QR kódu.

## 0.4.0

- **Účtovné odpisy** v podvojnom účtovníctve: účet majetku, doba
  odpisovania v mesiacoch, účtovný odpisový plán s rozdielom k daňovým
  odpisom a zaúčtovanie za rok jedným interným dokladom.

- **Mzdy**: zamestnanci a dohodári, výpočet mzdy, odvodov po fondoch,
  zdravotného poistenia, preddavku na daň a daňového bonusu za roky 2025
  a 2026, výplatná listina so súčtami na úhradu a výplatné pásky.
- **Fiškus v prehliadači**: bez inštalácie, údaje iba v úložisku prehliadača
  na vašom počítači. Funkcie nainštalovaného programu ponúknu stiahnutie;
  záloha a obnova súborom `.fbk` v oboch smeroch.
- **Uzávierka roka** v jednoduchom účtovníctve: kontroly, uzamknutie roka,
  prenos stavov účtov do ďalšieho roka, otvorenie s dôvodom, tlač závierky.
- **Daň z príjmov typ B — rodina a straty**: deti a daňový bonus, manžel/ka,
  straty z minulých rokov, bonus od zamestnávateľa a počet príloh priamo
  vo Fiškuse aj v XML pre eDane.
- **Domácnosť**: domáce účtovníctvo v samostatnej domácnosti — účty
  so zostatkami, výdavky a príjmy podľa kategórií, prevody, mesačný
  rozpočet, import výpisu a prenos výberov zo živnosti.
- **Záväzky domácnosti**: pôžičky, dlhy, exekúcie, pravidelné platby
  a požičané peniaze so splátkovým kalendárom, splátky v kalendári
  a pripomenutie e-mailom.
- **Bločky v domácnosti**: z QR kódu eKasa alebo z fotky (Claude API),
  každá položka v svojej kategórii, fotka pri pohybe.
- Opravy: spárovanie mobilu ponúkne správnu adresu počítača v miestnej
  sieti; zmazanie firmy už neskončí internou chybou servera.

## 0.2 a 0.3

- **Karta produktu**: EAN, kategória, značka, dodávateľ, nákupná cena,
  hmotnosť, technické parametre, popis a až 10 obrázkov; stav po skladoch
  a všetky pohyby na jednom mieste.
- **E-shopy**: WooCommerce, Shopify, Shoptet, PrestaShop, Magento a Drupal
  Commerce — objednávky do faktúr a riadenie obchodu z Fiškusu.
- **Úvodné nastavenie** novej firmy, prenos dokladov z iného programu iba raz,
  zmazanie firmy so zálohou.
- **Mobil**: bločky eKasa odfotením QR kódu, spárovanie telefónu s desktopom
  v miestnej sieti.
- Nastavenia e-mailu oddelené do vlastnej sekcie.

## 0.1

Prvé verejné vydanie: fakturácia, DPH a kontrolný výkaz, banka a pokladňa,
peňažný denník, podvojné účtovníctvo, sklad, majetok, kniha jázd, prenos
dát z POHODY, Money S3, KROS a MRP, zálohy a inštalačky pre Windows, macOS
a Linux.
