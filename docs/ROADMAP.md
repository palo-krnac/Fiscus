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
- [x] Cenník: import z Excelu/CSV s náhľadom, uloženie riadku faktúry do cenníka (skladové položky v etape 3)
- [x] Meny a kurzy ECB (faktúra a prijatá faktúra v cudzej mene, DPH v EUR)
- [x] Adresár odberateľov a dodávateľov, výber odberateľa vo faktúre, vlastná splatnosť
- [x] Vyhľadanie firmy v registri (RPO) podľa IČO: názov a adresa
- [ ] DIČ a IČ DPH z registrov finančnej správy (vyžaduje API kľúč)
- [x] Tlač a PDF faktúry (A4) s QR kódom **PAY by square**
- [x] Odoslanie faktúry, ponuky a upomienky e-mailom (PDF zo servera, SMTP), história e-mailov
- [x] Dobropis (opravná faktúra) k vystavenej faktúre: celý aj čiastočný, kontrola limitu, sadzby k pôvodnému dodaniu; zápočet s neuhradenou časťou faktúry (vyrovnaná dobropisom)
- [x] Cenová ponuka: vlastný číselný rad (CP), platnosť, stavy odoslaná / prijatá / odmietnutá / po platnosti, kópia ako nová verzia, tlač, jedným klikom koncept faktúry; nezapočíta sa do DPH
- [x] Pravidelná fakturácia: šablóna faktúry s intervalom (mesačne, štvrťročne, polročne, ročne), koncom, splatnosťou a texty {obdobie} / {mesiac} / {rok} / {minule_obdobie}; Fiškus faktúru vytvorí a vystaví sám (pri štarte a každú hodinu), dobehnuté staršie obdobia ostanú ako koncepty; pozastavenie, faktúra hneď, história
- [x] Pravidelná fakturácia: automatické odoslanie vystavenej faktúry e-mailom
- [x] Pravidelná fakturácia: upozornenie na koncepty na kontrolu (zoznam s dôvodom, odznak v menu, súhrnný e-mail)
- [x] Zálohová faktúra (nie je daňový doklad), po úhrade automaticky daňový doklad k prijatej platbe (dátum dodania = deň prijatia platby, § 19 ods. 4), vyúčtovacia faktúra s presným odpočtom zálohy po sadzbách; do DPH a KV iba zostatok, pri 100 % zálohe sa vyúčtovanie do KV neuvádza
- [x] Zálohy: čiastočná úhrada zálohy (daňový doklad ku každej platbe, sadzby v pomere zálohy), vyúčtovanie s odpočtom všetkých dokladov k platbám aj po čiastočnej úhrade; vyúčtovaná záloha sa nevymáha
- [x] Zálohy: viac zálohových faktúr toho istého odberateľa na jedno vyúčtovanie (pridanie a odobratie v koncepte)
- [x] Zálohy do zahraničia: tovar do EÚ, vývoz a služba mimo EÚ bez dokladu k platbe (záloha znižuje iba úhradu, DPH a súhrnný výkaz z vyúčtovania); služba do EÚ s dokladom k platbe s prenesením (súhrnný výkaz v období platby)
- [x] Zálohy s tuzemským prenesením pri službách (stavebné práce): doklad k platbe bez DPH v KV A.2, vyúčtovanie iba so zostatkom
- [x] Zálohy: dodanie do 15 dní od platby bez dokladu k platbe — kontrola lehoty a obdobia DPH pri vyúčtovaní, dodatočný doklad k platbe jedným klikom
- [ ] Zálohy v cudzej mene, s prenesením pri tovare (množstvo v A.2)
- [x] Cenová ponuka: prijatie alebo odmietnutie zákazníkom cez tajný odkaz (server s verejnou adresou), číslo objednávky zákazníka do faktúry, upozornenie e-mailom
- [x] Prijaté faktúry: evidencia z rekapitulácie DPH dodávateľa, odpočet / bez odpočtu, prenesenie, kontrola duplicít, úhrady
- [x] AI vyťaženie prijatých faktúr z PDF a fotky (Claude API; AI iba navrhuje, používateľ skontroluje)
- [x] Banka: import výpisov camt.053, automatické párovanie podľa VS a sumy, návrhy, ručné párovanie, nepárované pohyby
- [x] Banka: výpisy GPC, CSV a Excel, čiastočné úhrady
- [ ] Banka: platobné príkazy (pain.001), napojenie na banku cez API
- [x] Pokladňa: príjmové a výdavkové doklady (PPD/VPD), kontrola záporného zostatku, úhrada faktúr v hotovosti, storno, tlač so sumou slovom
- [x] Pokladňa: DPH na pokladničných dokladoch — blok ako zjednodušená faktúra (odpočet, KV B.3.1/B.3.2 podľa limitu 3 000 €), tržba z e-kasy (daň na výstupe, KV D.1), rozpis v peňažnom denníku
- [x] Viac pokladní (vlastný zostatok a rad dokladov s predponou, archivácia) a pokladňa v cudzej mene (kurz ECB ku dňu dokladu, v peňažnom denníku v eurách, úhrada faktúr v tej istej mene)
- [ ] Pokladňa: časť B.3 v XML kontrolného výkazu (overiť voči XSD KVDPHv25), načítanie uzávierky e-kasy
- [x] **Priznanie k DPH (DPHv25) a kontrolný výkaz (KVDPHv25)**: výpočet z dokladov, XML pre eDane, mesačne aj štvrťročne
- [x] Súhrnný výkaz (SVDPHv20): tovar do EÚ (kód 0), služby do EÚ (kód 2); r. 13–15 pre dodanie do EÚ a vývoz; nadobudnutie tovaru z EÚ (r. 05–08) a služby zo zahraničia (r. 09–10)
- [x] A.2 a tuzemské prenesenie pri dodaní (§ 69 ods. 12) pri položkách: druhy a) až j), kód colného sadzobníka, hranica 5 000 €, XML
- [ ] Dobropis k A.2 (kód opravy), trojstranný obchod (kód 1), call-off stock, dovoz s DPH na colnici (r. 22–23)
- [x] Dobropisy od tuzemských dodávateľov: oprava odpočítanej dane v r. 28 a riadky C.2 (číslo pôvodnej faktúry)
- [ ] Dobropisy od zahraničných dodávateľov a pri prenesení daňovej povinnosti, čiastočný odpočet (koeficient)
- [x] Opravné a dodatočné priznanie k DPH (dátum zistenia, r. 36 rozdiel oproti poslednej známej povinnosti, r. 37), opravný KV, záznam o podaní (posledná známa povinnosť, návrh druhu podľa lehoty)
- [ ] Dodatočný kontrolný výkaz (iba opravované záznamy)
- [ ] Podanie priamo na portál finančnej správy (vyžaduje autentifikáciu)
- [x] Upomienky: faktúry po splatnosti podľa odberateľov, 1., 2. a posledná upomienka, zákonný úrok z omeškania (ECB + 9 p. b. podnikateľ, + 5 p. b. spotrebiteľ, k prvému dňu omeškania), dohodnutá sadzba, paušálna náhrada 40 € (§ 369c ObZ), tlač listu, číselný rad UP
- [x] Legislatíva v nastaveniach (spoločná pre všetky firmy): obdobia sadzieb DPH, sadzby ECB, prirážky úroku z omeškania, paušálna náhrada; nová inštalácia sa predvyplní, nová verzia programu doplní nové riadky a úpravy používateľa nechá
- [x] Upomienky: základná úroková sadzba ECB pre úrok z omeškania sa dopĺňa automaticky zo servera ECB (ECB Data Portal), aj tlačidlom v Legislatíve
- [ ] Ďalšie zákonné hodnoty do Legislatívy podľa potreby (limity, sadzby pre mzdy a odvody v etape 4)
- [x] Obrazovka Zálohy: stav, ručná záloha, stiahnutie, obnova naplánovaná na ďalšie spustenie, `-restore` zo súboru
- [x] Nastavenie priečinka a hesla záloh priamo v rozhraní, návrh priečinka synchronizovaného do cloudu (OneDrive, Dropbox, Google Disk, iCloud)
- [ ] Záloha priamo do cloudového úložiska (S3, WebDAV)
- [x] Inštalačky: Inno Setup (Windows), DMG s Fiškus.app (macOS), .deb a .rpm (Linux); aplikácia s dátami v priečinku používateľa, otvorí prehliadač, skončí po zatvorení okna
- [ ] Podpísané inštalačky (Authenticode, notarizácia Apple), automatická aktualizácia, AppImage
- [x] Licencia pre serverovú verziu s viacerými firmami (offline overenie podpisu Ed25519)
- [x] Web fiskus.sk (priečinok `site/`): funkcie, cenník, stiahnutie podľa systému, nákup cez Paddle (pripravené, vypnuté), návrhy obchodných podmienok, ochrany údajov a vrátenia platby
- [x] Predaj cez Paddle (pripravené): `fiskus-shop` overí podpis webhooku, vystaví licenciu (obnova nadväzuje), pošle ju e-mailom, evidencia bez zdvojenia
- [ ] Spustiť predaj: overenie domény v Paddle, ostrý kľúč, doplnené právne texty, hosting webu a `fiskus-shop`
- [ ] Ostrý licenčný kľúč (`fiskus-license keygen`) namiesto vývojového
- [x] E-faktúra UBL 2.1 (EN 16931, Peppol BIS Billing 3.0): vytvorenie z vystavenej faktúry a dobropisu (adresa 0245:DIČ), načítanie prijatej e-faktúry ako nákladu
- [x] E-faktúra: všeobecné napojenie na digitálneho poštára — priečinky (synchronizácia aplikáciou poskytovateľa) alebo HTTP API adaptéra; odoslanie, schránka prijatých, zoznam odoslaných a prevzatých
- [ ] E-faktúra: adaptér pre konkrétneho poskytovateľa (po výbere), vyúčtovanie zálohy ako e-faktúra, overenie validátorom Peppol a SK TDD

## Používatelia, práva a audit (pre účtovné kancelárie a poskytovateľov služieb)
- [x] Používatelia a prihlasovanie v serverovom režime (heslo PBKDF2, relácia v HttpOnly cookie, limit pokusov, ochrana CSRF)
- [x] Dvojfázové overenie (TOTP z aplikácie v telefóne, 10 záložných kódov, ochrana proti zopakovaniu kódu; zrušenie správcom inštalácie pri strate telefónu)
- [x] Desktop: voliteľný PIN (zamkne okno po spustení, po 30 min nečinnosti aj tlačidlom); server: povinné 2FA pre všetkých používateľov (správca ho musí mať sám)
- [x] Roly pre každú firmu zvlášť: správca, účtovník, fakturant, zákazník (klient kancelárie), len čítanie
- [x] Práva kontroluje backend pri každej požiadavke; rozhranie skrýva nepovolené akcie
- [x] Audit všetkých zmien (kto, kedy, kanál, čo, pred/po), nemenný reťazec s odtlačkami; filter podľa firmy a kategórie
- [x] API kľúče používateľa (len na čítanie / ako používateľ), iba hash v databáze, zrušenie
- [x] MCP server pre AI asistentov (POST /mcp, Streamable HTTP) + most `fiskus-mcp` pre stdio; práva používateľa, každé volanie v audite
- [x] MCP: zoznam cenových ponúk (`list_quotes`) a pravidelných faktúr (`list_recurring_invoices`)
- [x] MCP: zaevidovanie a úhrada prijatej faktúry, návrhy a párovanie bankových platieb, dobropis (celý aj časť položiek), faktúry po splatnosti a upomienky; kontrola povinných argumentov
- [x] MCP: OAuth 2.1 pre vzdialených klientov (Claude na webe a v mobile): metadáta RFC 9728/8414, registrácia RFC 7591 bez stavu, PKCE, súhlas v rozhraní, obnovovacie tokeny s výmenou a odhalením opakovaného použitia

## Etapa 2 — účtovníctvo
- [x] Peňažný denník (opatrenie MF SR č. MF/27076/2007-74, § 4): pohyby z bankových výpisov a pokladne, stĺpce c, d1–d3, e1–e6, f, g; úhrady spárovaných faktúr rozpíše Fiškus (základ podľa druhu plnenia / druhu výdavku, DPH zvlášť, pomerne pri čiastočnej úhrade), ostatné pohyby ručne s rýchlymi voľbami; súčty stĺpcov a rozdiel príjmov a výdavkov; export; MCP `money_journal`
- [x] Stavy pokladnice a účtov v peňažnom denníku (počiatočný stav účtu podľa výpisu, pokladnica z dokladov), kniha pohľadávok a záväzkov k zvolenému dňu (úhrady po dni sa neberú, cudzia mena v eurách, dni po splatnosti)
- [x] Výkazy JÚ v celých eurách: Výkaz o príjmoch a výdavkoch (Úč FO 1-01) z peňažného denníka, Výkaz o majetku a záväzkoch (Úč FO 2-01) z kníh, pokladne a účtov + ručne zadané riadky (dlhodobý majetok, zásoby, úvery…), predchádzajúce obdobie, tlač, export, MCP
- [x] Uzávierkové operácie k 31. 12. (odpisy, zostatková cena predaného majetku, úpravy príjmov a výdavkov) v peňažnom denníku, bez vplyvu na stavy peňazí; prenesú sa do výkazov
- [x] Podklady pre DP FO typ B (DPFOBv25, VI. oddiel): tabuľka č. 1 r. 2, r. 39–42, tabuľka č. 1a na začiatku a na konci obdobia, zaplatené poistné na kontrolu
- [ ] Jednoduché účtovníctvo ďalej: elektronické uloženie závierky do RÚZ, XML daňového priznania typu B pre eDane
- [ ] Číselníky účtovníctva: rámcová účtová osnova, účtový rozvrh firmy, predkontácie, strediská, zákazky
- [ ] Podvojné účtovníctvo: denník, hlavná kniha, automatické účtovanie podľa predkontácií
- [ ] Účtovné obdobia, uzávierky, výkazy, uloženie závierky do RÚZ
- [ ] Daňové priznanie k dani z príjmov (typ A/B pre FO, PO)

## Etapa 3 — sklad a majetok
- [ ] Sklad, prepojenie s e-shopom
- [x] Register dlhodobého majetku s daňovými odpismi (§ 22 – 29 ZDP): skupiny 0 – 6, rovnomerné a zrýchlené odpisovanie, pomerná časť v roku zaradenia, podiel na podnikaní, vyradenie (ZC do výdavkov), odpisový plán, väzba na prijatú faktúru, hľadanie a export
- [x] Odpisy z registra jedným tlačidlom do uzávierky roka (stĺpec e6); zostatkové ceny do výkazu o majetku a záväzkoch (r. 01, 02) a tabuľky 1a DP typ B
- [ ] Účtovné odpisy, technické zhodnotenie, prerušenie odpisovania, prenájom (§ 27 ods. 2)
- [x] Kniha jázd: vozidlá (v obchodnom majetku, prenajaté, súkromné), jazdy so stavom tachometra, účelom a vodičom, tankovanie z pokladničných blokov a prijatých faktúr; vyúčtovanie za rok — PHM podľa knihy jázd s limitom spotreby z TP (nadspotreba, súkromné jazdy), paušál 80 % / 50 %, cestovné náhrady za súkromné auto (základná náhrada za km podľa dňa jazdy + PHM), odpis z registra majetku, náklady na každú jazdu a na km; zaúčtovanie do uzávierky; sadzby za km v Legislatíve
- [ ] Kniha jázd: pomerný odpočet DPH z PHM a nájomného podľa služobných km, tlač knihy jázd, import z GPS

## Etapa 4 — mzdy (samostatný návrh)
- [ ] Sociálna poisťovňa, zdravotné poisťovne, preddavky na daň, ročné zúčtovanie

## Odlíšenie od konkurencie
Slovenské špecifiká, ktoré MyÚčto nemá: RÚZ, tri identifikátory (IČO / DIČ / IČ DPH), 2 % dane pre neziskovky, slovenská e-fakturácia, PAY by square.
