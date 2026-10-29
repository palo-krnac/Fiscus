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
- [x] Logo firmy na faktúre, cenovej ponuke, upomienke a pokladničnom doklade (PNG, JPEG, WebP do 512 kB; SVG nie kvôli bezpečnosti)
- [x] Cenník: položky s kódom, MJ, cenou bez DPH alebo s DPH a sadzbou; výber z cenníka vo faktúre, ponuke aj pravidelnej faktúre; archivácia; MCP `list_catalog_items`
- [ ] Cenník: import z Excelu/CSV, uloženie riadku faktúry do cenníka, skladové položky (etapa 3)
- [ ] Meny a kurzy ECB (faktúra v cudzej mene, DPH v EUR)
- [x] Adresár odberateľov a dodávateľov, výber odberateľa vo faktúre, vlastná splatnosť
- [x] Vyhľadanie firmy v registri (RPO) podľa IČO: názov a adresa
- [ ] DIČ a IČ DPH z registrov finančnej správy (vyžaduje API kľúč)
- [x] Tlač a PDF faktúry (A4) s QR kódom **PAY by square**
- [ ] Odoslanie faktúry e-mailom (PDF na serveri + SMTP)
- [x] Dobropis (opravná faktúra) k vystavenej faktúre: celý aj čiastočný, kontrola limitu, sadzby k pôvodnému dodaniu
- [x] Cenová ponuka: vlastný číselný rad (CP), platnosť, stavy odoslaná / prijatá / odmietnutá / po platnosti, kópia ako nová verzia, tlač, jedným klikom koncept faktúry; nezapočíta sa do DPH
- [x] Pravidelná fakturácia: šablóna faktúry s intervalom (mesačne, štvrťročne, polročne, ročne), koncom, splatnosťou a texty {obdobie} / {mesiac} / {rok} / {minule_obdobie}; Fiškus faktúru vytvorí a vystaví sám (pri štarte a každú hodinu), dobehnuté staršie obdobia ostanú ako koncepty; pozastavenie, faktúra hneď, história
- [ ] Pravidelná fakturácia: automatické odoslanie e-mailom, upozornenie na koncepty na kontrolu
- [x] Zálohová faktúra (nie je daňový doklad), po úhrade automaticky daňový doklad k prijatej platbe (dátum dodania = deň prijatia platby, § 19 ods. 4), vyúčtovacia faktúra s presným odpočtom zálohy po sadzbách; do DPH a KV iba zostatok, pri 100 % zálohe sa vyúčtovanie do KV neuvádza
- [ ] Zálohy: čiastočná úhrada zálohy, viac záloh na jedno vyúčtovanie, zálohy do zahraničia a s prenesením daňovej povinnosti, dodanie do 15 dní od platby bez dokladu k platbe
- [ ] Cenová ponuka: odoslanie e-mailom, prijatie zákazníkom cez odkaz, objednávka
- [x] Prijaté faktúry: evidencia z rekapitulácie DPH dodávateľa, odpočet / bez odpočtu, prenesenie, kontrola duplicít, úhrady
- [ ] AI vyťaženie prijatých faktúr z PDF (AI iba navrhuje, účtuje pevne naprogramovaný automat)
- [x] Banka: import výpisov camt.053, automatické párovanie podľa VS a sumy, návrhy, ručné párovanie, nepárované pohyby
- [ ] Banka: CSV / GPC výpisy, čiastočné úhrady a preplatky, viac účtov, platobné príkazy (pain.001), napojenie na banku cez API
- [x] Pokladňa: príjmové a výdavkové doklady (PPD/VPD), kontrola záporného zostatku, úhrada faktúr v hotovosti, storno, tlač so sumou slovom
- [ ] Pokladňa: DPH na pokladničných dokladoch (B.3), viac pokladní, cudzia mena, ERP / e-kasa (D.1)
- [x] **Priznanie k DPH (DPHv25) a kontrolný výkaz (KVDPHv25)**: výpočet z dokladov, XML pre eDane, mesačne aj štvrťročne
- [x] Súhrnný výkaz (SVDPHv20): tovar do EÚ (kód 0), služby do EÚ (kód 2); r. 13–15 pre dodanie do EÚ a vývoz; nadobudnutie tovaru z EÚ (r. 05–08) a služby zo zahraničia (r. 09–10)
- [x] A.2 a tuzemské prenesenie pri dodaní (§ 69 ods. 12) pri položkách: druhy a) až j), kód colného sadzobníka, hranica 5 000 €, XML
- [ ] Dobropis k A.2 (kód opravy), trojstranný obchod (kód 1), call-off stock, dovoz s DPH na colnici (r. 22–23)
- [ ] Dobropisy od dodávateľov (C.2, r. 28), zjednodušené faktúry a pokladničné doklady (B.3, D.1), čiastočný odpočet (koeficient)
- [ ] Opravné a dodatočné priznanie / KV (KOpr, r. 36–37)
- [ ] Podanie priamo na portál finančnej správy (vyžaduje autentifikáciu)
- [x] Upomienky: faktúry po splatnosti podľa odberateľov, 1., 2. a posledná upomienka, zákonný úrok z omeškania (ECB + 9 p. b. podnikateľ, + 5 p. b. spotrebiteľ, k prvému dňu omeškania), dohodnutá sadzba, paušálna náhrada 40 € (§ 369c ObZ), tlač listu, číselný rad UP
- [x] Legislatíva v nastaveniach (spoločná pre všetky firmy): obdobia sadzieb DPH, sadzby ECB, prirážky úroku z omeškania, paušálna náhrada; nová inštalácia sa predvyplní, nová verzia programu doplní nové riadky a úpravy používateľa nechá
- [ ] Upomienky: odoslanie e-mailom; sadzby ECB načítať automaticky z ECB (zatiaľ ručne v Legislatíve)
- [ ] Ďalšie zákonné hodnoty do Legislatívy podľa potreby (limity, sadzby pre mzdy a odvody v etape 4)
- [x] Obrazovka Zálohy: stav, ručná záloha, stiahnutie, obnova naplánovaná na ďalšie spustenie, `-restore` zo súboru
- [ ] Nastavenie priečinka a hesla záloh priamo v rozhraní (zatiaľ parametre a premenná prostredia), záloha do cloudu
- [x] Inštalačky: Inno Setup (Windows), DMG s Fiškus.app (macOS), .deb a .rpm (Linux); aplikácia s dátami v priečinku používateľa, otvorí prehliadač, skončí po zatvorení okna
- [ ] Podpísané inštalačky (Authenticode, notarizácia Apple), automatická aktualizácia, AppImage
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
- [x] API kľúče používateľa (len na čítanie / ako používateľ), iba hash v databáze, zrušenie
- [x] MCP server pre AI asistentov (POST /mcp, Streamable HTTP) + most `fiskus-mcp` pre stdio; práva používateľa, každé volanie v audite
- [x] MCP: zoznam cenových ponúk (`list_quotes`) a pravidelných faktúr (`list_recurring_invoices`)
- [ ] Ďalšie MCP nástroje: prijaté faktúry (zaevidovanie), párovanie platieb, dobropis, upomienky; OAuth pre vzdialených klientov

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
