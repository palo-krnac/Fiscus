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
- [x] Číselníky: sadzby DPH s obdobím platnosti (kontrola podľa dátumu dodania), číselné rady s vlastným formátom, merné jednotky, spôsoby úhrady, krajiny
- [ ] Logo na faktúre
- [ ] Číselník cenníka / skladových položiek (opakované položky faktúr)
- [ ] Meny a kurzy ECB (faktúra v cudzej mene, DPH v EUR)
- [x] Adresár odberateľov a dodávateľov, výber odberateľa vo faktúre, vlastná splatnosť
- [x] Vyhľadanie firmy v registri (RPO) podľa IČO: názov a adresa
- [ ] DIČ a IČ DPH z registrov finančnej správy (vyžaduje API kľúč)
- [x] Tlač a PDF faktúry (A4) s QR kódom **PAY by square**
- [ ] Odoslanie faktúry e-mailom (PDF na serveri + SMTP)
- [x] Dobropis (opravná faktúra) k vystavenej faktúre: celý aj čiastočný, kontrola limitu, sadzby k pôvodnému dodaniu
- [ ] Zálohová faktúra a daňový doklad k prijatej platbe, cenová ponuka, pravidelná fakturácia
- [x] Prijaté faktúry: evidencia z rekapitulácie DPH dodávateľa, odpočet / bez odpočtu, prenesenie, kontrola duplicít, úhrady
- [ ] AI vyťaženie prijatých faktúr z PDF (AI iba navrhuje, účtuje pevne naprogramovaný automat)
- [ ] Banka: import výpisov (camt.053 / CSV), párovanie platieb
- [ ] Pokladňa
- [ ] **Priznanie DPH, kontrolný výkaz, súhrnný výkaz**: XML pre eDane / finančnú správu
- [ ] Upomienky
- [ ] Obrazovka záloh a obnovy, nastavenie priečinka a hesla
- [ ] Inštalačky: Inno Setup (Windows), DMG (macOS), AppImage (Linux)
- [x] Licencia pre serverovú verziu s viacerými firmami (offline overenie podpisu Ed25519)
- [ ] Web fiskus.sk s nákupom licencie
- [ ] Predaj cez Paddle: webhook po platbe → automatické vydanie a odoslanie licenčného kľúča e-mailom
- [ ] Ostrý licenčný kľúč (`fiskus-license keygen`) namiesto vývojového
- [ ] E-faktúra (UBL / Peppol): pripraviť sa na povinnú e-fakturáciu na Slovensku (termín overiť)

## Používatelia, práva a audit (pre účtovné kancelárie a poskytovateľov služieb)
- [x] Používatelia a prihlasovanie v serverovom režime (heslo PBKDF2, relácia v HttpOnly cookie, limit pokusov, ochrana CSRF)
- [ ] 2FA; desktop voliteľne PIN
- [x] Roly pre každú firmu zvlášť: správca, účtovník, fakturant, zákazník (klient kancelárie), len čítanie
- [x] Práva kontroluje backend pri každej požiadavke; rozhranie skrýva nepovolené akcie
- [x] Audit všetkých zmien (kto, kedy, kanál, čo, pred/po), nemenný reťazec s odtlačkami; filter podľa firmy a kategórie
- [ ] Audit API kľúčov a MCP (kanály sú pripravené, chýbajú samotné API kľúče a MCP server)
- [ ] MCP server pre AI asistentov: čítanie dát a kontrolované akcie v rámci práv používateľa, každé volanie auditované

## Etapa 2 — účtovníctvo
- [ ] Jednoduché účtovníctvo (peňažný denník)
- [ ] Číselníky účtovníctva: rámcová účtová osnova, účtový rozvrh firmy, predkontácie, strediská, zákazky
- [ ] Podvojné účtovníctvo: denník, hlavná kniha, automatické účtovanie podľa predkontácií
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
