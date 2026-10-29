# Plán vývoja

Referenčný prehľad funkcií pochádza z českých systémov MyInvoice (MIT) a MyÚčto ([docs/reference/](reference/)). Kód z nich nepreberáme, slúžia iba ako zoznam funkcií. Proprietárne časti MyÚčto (podvojné účtovníctvo, sklad, majetok, EPO, mzdy) píšeme od nuly podľa slovenskej legislatívy.

## Etapa 0 — kostra ✅
- [x] Presný typ na peniaze, výpočet DPH (23/19/5/0 %, ceny s DPH aj bez DPH, prenesenie daňovej povinnosti)
- [x] Overenie IČO a IČ DPH
- [x] Faktúra: koncept → vystavená → uhradená / stornovaná, číselné rady bez dier
- [x] Edície: desktop (1 firma) a server (viac firiem)
- [x] Úložisko SQLite + spoločná sada testov, šifrované zálohy s rotáciou
- [x] API a rozhranie: zoznam faktúr, editor so živým prepočtom DPH
- [x] CI (testy) a release (binárky pre Win/macOS/Linux)

## Etapa 1 — fakturácia a DPH pre živnostníka
- [x] Nastavenia firmy (údaje, IBAN, platiteľ DPH áno/nie, predvolená splatnosť)
- [ ] Logo a vlastné číselné rady
- [x] Adresár odberateľov a dodávateľov, výber odberateľa vo faktúre, vlastná splatnosť
- [x] Vyhľadanie firmy v registri (RPO) podľa IČO: názov a adresa
- [ ] DIČ a IČ DPH z registrov finančnej správy (vyžaduje API kľúč)
- [ ] PDF faktúry + QR kód **PAY by square**, odoslanie e-mailom
- [ ] Dobropis, zálohová faktúra, cenová ponuka, pravidelná fakturácia
- [ ] Prijaté faktúry (vrátane AI vyťaženia z PDF: AI iba navrhuje, účtuje pevne naprogramovaný automat)
- [ ] Banka: import výpisov (camt.053 / CSV), párovanie platieb
- [ ] Pokladňa
- [ ] **Priznanie DPH, kontrolný výkaz, súhrnný výkaz**: XML pre eDane / finančnú správu
- [ ] Upomienky
- [ ] Obrazovka záloh a obnovy, nastavenie priečinka a hesla
- [ ] Inštalačky: Inno Setup (Windows), DMG (macOS), AppImage (Linux)
- [ ] E-faktúra (UBL / Peppol): pripraviť sa na povinnú e-fakturáciu na Slovensku (termín overiť)

## Etapa 2 — účtovníctvo
- [ ] Jednoduché účtovníctvo (peňažný denník)
- [ ] Podvojné účtovníctvo: účtový rozvrh, denník, hlavná kniha, automatické účtovanie
- [ ] Účtovné obdobia, uzávierky, výkazy, uloženie závierky do RÚZ
- [ ] Daňové priznanie k dani z príjmov (typ A/B pre FO, PO)

## Etapa 3 — sklad a majetok
- [ ] Sklad, prepojenie s e-shopom
- [ ] Evidencia a odpisy majetku
- [ ] Kniha jázd

## Etapa 4 — mzdy (samostatný návrh)
- [ ] Sociálna poisťovňa, zdravotné poisťovne, preddavky na daň, ročné zúčtovanie

## Odlíšenie od konkurencie
Slovenské špecifiká, ktoré MyÚčto nemá: RÚZ, tri identifikátory (IČO / DIČ / IČ DPH), 2 % dane pre neziskovky, slovenská e-fakturácia, PAY by square.
