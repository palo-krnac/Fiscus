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
- [x] Úvodné nastavenie novej firmy (údaje z IČO, DPH, jednoduché/podvojné účtovníctvo — menu podľa neho, nová firma alebo prechod z iného programu); prenos dokladov z iného programu iba raz
- [x] Bločky z mobilu: odfotenie QR kódu bločku eKasa (čítanie v telefóne), doklad z Finančnej správy → uhradený prijatý doklad s fotkou; stránka /m na plochu telefónu (manifest)
- [ ] Mobil v sieti k desktopu: spárovanie QR kódom, prístup iba pre spárované telefóny
- [x] Nastavenia po sekciách (firma, DPH, doklady a platby, e-faktúry, e-mail, kalendár, bezpečnosť, používatelia, AI, prenos dát, zálohy, kurzy, legislatíva): menu vľavo, na mobile rozbaľovacie, podľa práv
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
- [x] Zálohy v cudzej mene: doklad k platbe v mene zálohy kurzom ECB ku dňu prijatia platby, vyúčtovanie v eurách = celá dodávka kurzom vyúčtovania mínus zálohy v eurách presne podľa dokladov k platbám
- [x] Zálohy s tuzemským prenesením pri tovare (telefóny, obvody, plodiny, kovy): doklad k prvej platbe uvedie v A.2 celé množstvo dodávky, ďalšie doklady a vyúčtovanie iba základ; viac druhov prenesenia na jednej zálohe
- [x] Cenová ponuka: prijatie alebo odmietnutie zákazníkom cez tajný odkaz (server s verejnou adresou), číslo objednávky zákazníka do faktúry, upozornenie e-mailom
- [x] Prijaté faktúry: evidencia z rekapitulácie DPH dodávateľa, odpočet / bez odpočtu, prenesenie, kontrola duplicít, úhrady
- [x] AI vyťaženie prijatých faktúr z PDF a fotky (Claude API; AI iba navrhuje, používateľ skontroluje)
- [x] Banka: import výpisov camt.053, automatické párovanie podľa VS a sumy, návrhy, ručné párovanie, nepárované pohyby
- [x] Banka: výpisy GPC, CSV a Excel, čiastočné úhrady
- [x] Banka: platobné príkazy SEPA (pain.001) z prijatých faktúr
- [ ] Banka: napojenie na banku cez API
- [x] Pokladňa: príjmové a výdavkové doklady (PPD/VPD), kontrola záporného zostatku, úhrada faktúr v hotovosti, storno, tlač so sumou slovom
- [x] Pokladňa: DPH na pokladničných dokladoch — blok ako zjednodušená faktúra (odpočet, KV B.3.1/B.3.2 podľa limitu 3 000 €), tržba z e-kasy (daň na výstupe, KV D.1), rozpis v peňažnom denníku
- [x] Viac pokladní (vlastný zostatok a rad dokladov s predponou, archivácia) a pokladňa v cudzej mene (kurz ECB ku dňu dokladu, v peňažnom denníku v eurách, úhrada faktúr v tej istej mene)
- [ ] Pokladňa: časť B.3 v XML kontrolného výkazu (overiť voči XSD KVDPHv25), načítanie uzávierky e-kasy
- [x] **Priznanie k DPH (DPHv25) a kontrolný výkaz (KVDPHv25)**: výpočet z dokladov, XML pre eDane, mesačne aj štvrťročne
- [x] Súhrnný výkaz (SVDPHv20): tovar do EÚ (kód 0), služby do EÚ (kód 2); r. 13–15 pre dodanie do EÚ a vývoz; nadobudnutie tovaru z EÚ (r. 05–08) a služby zo zahraničia (r. 09–10)
- [x] A.2 a tuzemské prenesenie pri dodaní (§ 69 ods. 12) pri položkách: druhy a) až j), kód colného sadzobníka, hranica 5 000 €, XML
- [x] Dobropis k A.2: C.1 bez dane a sadzby, pri tovare kód SCS / druh a rozdiel množstva so znamienkom mínus
- [x] Trojstranný obchod (§ 45): prvý odberateľ — faktúra s textom, SV kód 1, mimo priznania; druhý odberateľ — r. 11–12, odpočet, KV B.1
- [x] Dovoz tovaru z tretej krajiny s DPH zaplatenou colnému úradu: odpočet r. 18–19, z toho r. 22–23, bez KV
- [ ] Call-off stock, dovoz s uplatnením dane v priznaní (§ 84a ods. 3, r. 11c–12e)
- [x] Dobropisy od tuzemských dodávateľov: oprava odpočítanej dane v r. 28 a riadky C.2 (číslo pôvodnej faktúry)
- [x] Pomerné odpočítanie DPH (§ 50): spoločné nákupy (prijaté faktúry, bloky) s predbežným koeficientom roka, ročný koeficient z plnení vo Fiškuse (alebo ručne), vysporiadanie v poslednom období roka v r. 18–19, prepočet pri zmene koeficientu v nepodaných obdobiach
- [x] Dobropisy pri prenesení daňovej povinnosti (služby zo zahraničia, tuzemské § 69 ods. 12): zníženie samozdanenia v r. 09–10 a odpočtu v r. 18–19, C.2 v období prijatia
- [x] Dobropis k nadobudnutiu tovaru z EÚ: oprava r. 05–08 (trojstranný r. 11–12) a odpočtu v období pôvodného nadobudnutia (podľa čísla pôvodnej faktúry), C.2 v období prijatia, upozornenie na dodatočné priznanie
- [x] Opravné a dodatočné priznanie k DPH (dátum zistenia, r. 36 rozdiel oproti poslednej známej povinnosti, r. 37), opravný KV, záznam o podaní (posledná známa povinnosť, návrh druhu podľa lehoty)
- [ ] Dodatočný kontrolný výkaz (iba opravované záznamy)
- [ ] Podanie priamo na portál finančnej správy (vyžaduje autentifikáciu)
- [x] Upomienky: faktúry po splatnosti podľa odberateľov, 1., 2. a posledná upomienka, zákonný úrok z omeškania (ECB + 9 p. b. podnikateľ, + 5 p. b. spotrebiteľ, k prvému dňu omeškania), dohodnutá sadzba, paušálna náhrada 40 € (§ 369c ObZ), tlač listu, číselný rad UP
- [x] Legislatíva v nastaveniach (spoločná pre všetky firmy): obdobia sadzieb DPH, sadzby ECB, prirážky úroku z omeškania, paušálna náhrada; nová inštalácia sa predvyplní, nová verzia programu doplní nové riadky a úpravy používateľa nechá
- [x] Upomienky: základná úroková sadzba ECB pre úrok z omeškania sa dopĺňa automaticky zo servera ECB (ECB Data Portal), aj tlačidlom v Legislatíve
- [ ] Ďalšie zákonné hodnoty do Legislatívy podľa potreby (limity, sadzby pre mzdy a odvody v etape 4)
- [x] Obrazovka Zálohy: stav, ručná záloha, stiahnutie, obnova naplánovaná na ďalšie spustenie, `-restore` zo súboru
- [x] Nastavenie priečinka a hesla záloh priamo v rozhraní, návrh priečinka synchronizovaného do cloudu (OneDrive, Dropbox, Google Disk, iCloud)
- [x] Záloha priamo do cloudového úložiska (S3, WebDAV) — kópia po každej zálohe s rovnakou rotáciou, overenie spojenia skúšobným súborom, stav poslednej kópie, stiahnutie zálohy z cloudu na novom počítači
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
- [x] Účtový rozvrh: rámcová účtová osnova pre podnikateľov (syntetické účty, druh, nedaňové náklady), analytické účty firmy, skrytie nepoužívaných, strediská a zákazky
- [x] Predkontácie (vzory zaúčtovania) pre faktúry, nákupy, banku a pokladňu — 23 predvolených, vlastné, kontrola vyrovnania MD = D
- [x] Podvojné účtovníctvo: účtovný denník, hlavná kniha (obratová predvaha), automatické účtovanie faktúr, nákupov, banky a pokladne podľa predkontácií, ručné zápisy, interné doklady, počiatočné stavy
- [x] Podvojné účtovníctvo: strediská a zákazky v zápisoch (náklady, výnosy a výsledok po strediskách), kurzové rozdiely pri úhrade faktúry v cudzej mene, automatické účtovanie záloh (DPH zo zálohy, zúčtovanie vo vyúčtovaní), analytické účty bánk a pokladníc
- [x] Podvojné účtovníctvo: dovoz s DPH vyrubenou colným úradom (343 / 379), kurzové rozdiely k 31. 12. (prepočet neuhradených pohľadávok a záväzkov, platí aj pri úhrade v ďalšom roku), pokladničné doklady bez DPH (aj v cudzej mene) podľa predkontácie
- [ ] Podvojné účtovníctvo ďalej: kurzové rozdiely pri čiastočných úhradách, clo a náklady obstarania pri dovoze, zostatky bankových účtov a pokladníc v cudzej mene k 31. 12.
- [x] Závierka mikro účtovnej jednotky z hlavnej knihy: súvaha (Úč MÚJ 1-01) a výkaz ziskov a strát (Úč MÚJ 2-01), počiatočné stavy z konečných stavov minulého roka
- [ ] Závierka ďalej: malá účtovná jednotka (Úč POD), poznámky, uzamknutie obdobia, uloženie do RÚZ (XML)
- [ ] Daňové priznanie k dani z príjmov (typ A/B pre FO, PO)

## Etapa 3 — sklad a majetok
- [x] Sklad: skladové položky z cenníka, viac skladov, príjemky, výdajky, inventúrne rozdiely, ocenenie váženým priemerom, výdajka z vystavenej faktúry a vrátenie k dobropisu, zaúčtovanie (132/131, 504/132…), zásoby vo výkaze o majetku
- [x] Sklad: FIFO alebo vážený priemer v nastaveniach, presun medzi skladmi, príjemka k prijatej faktúre, odkaz pre e-shop (zásoby JSON/CSV, objednávky → výdajky)
- [x] Register dlhodobého majetku s daňovými odpismi (§ 22 – 29 ZDP): skupiny 0 – 6, rovnomerné a zrýchlené odpisovanie, pomerná časť v roku zaradenia, podiel na podnikaní, vyradenie (ZC do výdavkov), odpisový plán, väzba na prijatú faktúru, hľadanie a export
- [x] Odpisy z registra jedným tlačidlom do uzávierky roka (stĺpec e6); zostatkové ceny do výkazu o majetku a záväzkoch (r. 01, 02) a tabuľky 1a DP typ B
- [ ] Účtovné odpisy, technické zhodnotenie, prerušenie odpisovania, prenájom (§ 27 ods. 2)
- [x] Kniha jázd: vozidlá (v obchodnom majetku, prenajaté, súkromné), jazdy so stavom tachometra, účelom a vodičom, tankovanie z pokladničných blokov a prijatých faktúr; vyúčtovanie za rok — PHM podľa knihy jázd s limitom spotreby z TP (nadspotreba, súkromné jazdy), paušál 80 % / 50 %, cestovné náhrady za súkromné auto (základná náhrada za km podľa dňa jazdy + PHM), odpis z registra majetku, náklady na každú jazdu a na km; zaúčtovanie do uzávierky; sadzby za km v Legislatíve
- [x] DPH pri vozidlách od 2026 (§ 85n): kategória a VIN vozidla, odpočet 50 % z nákupov k osobnému autu (M1, L1e, L3e) — vozidlo na prijatej faktúre a pokladničnom bloku nastaví odpočet sám (DPH r. 19/21, KV B.2 a B.3 s odpočítanou daňou, neodpočítaná časť do výdavkov); plný odpočet pri výlučnom podnikaní (VIN, kniha jázd, dátum oznámenia) alebo výnimke; prehľad DPH z dokladov k vozidlu s kontrolou nesúladu
- [x] Tlač knihy jázd (A4 na šírku): vozidlo, jazdy, tankovanie, náklady s DPH, vyúčtovanie
- [ ] Kniha jázd: import z GPS, zdanenie súkromného použitia vozidla s plným odpočtom (§ 9 ods. 2), úprava odpočtu pri predaji do 5 rokov

## Prenos dát z iných programov
- [x] POHODA (XML export, aj Windows-1250): adresár, zásoby do cenníka, vydané faktúry a dobropisy s pôvodnými číslami a úhradami, prijaté faktúry; náhľad pred prenosom, opakovaný prenos nevytvorí duplicity, číselný rad pokračuje za prenesenými číslami
- [x] Money S3 (XML export, slovenská aj česká verzia, Windows-1250): adresár, zásoby, vydané faktúry a dobropisy, prijaté faktúry; položky so zľavou alebo zmiešanými cenami podľa rekapitulácie DPH; voľba „faktúry so splatnosťou do … sú uhradené“ (Money úhrady neexportuje, platí aj pre POHODU)
- [x] Všeobecná tabuľka Excel/CSV z ľubovoľného programu: adresár, cenník, vydané alebo prijaté faktúry (riadok = faktúra, sumy po sadzbách, iba základ a DPH alebo iba suma so sadzbou); stĺpce podľa hlavičky (SK/CZ/EN), dátumy aj z Excelu, úhrada (áno/dátum), dobropisy; vzory na stiahnutie
- [x] KROS Omega (textový export podľa špecifikácie KROS: partneri T04, skladové karty T03, fakturácia T01, doklady EUD T00 s úhradami a dobropismi)
- [x] iDoklad: export pre POHODU (ZIP s XML po druhoch dokladov, aj jednotlivé XML); kontakty sa založia z odberateľov a dodávateľov na faktúrach
- [x] MRP-K/S podľa dokumentácie importu faktúr (5.53): vydané FAKVY/FAKVYPOL/FAKVYUHR, prijaté FAKPR/FAKPRPOL/FAKPRUHR, adresy FV_ADRES/FP_ADRES s IBAN; dobropisy (TYP_DOKL=D, ORIGCIS2), úhrady, zľavy, textové riadky, vlastné čísla drobných odberateľov (X0001)
- [x] Adresár: kontaktné osoby (funkcia, e-mail, telefón, predvolená) a ďalšie adresy (dodacia, korešpondenčná, prevádzka) — na faktúre kontaktná osoba a dodacia adresa z adresára alebo ručne, na tlači aj v PDF; importy ukladajú osobu ako osobu
- [x] Import kontaktov priamo v Adresári: vizitky vCard (.vcf, verzie 2.1–4.0) a tabuľky vrátane exportov z Outlooku a Google Kontaktov (meno a priezvisko, pracovná alebo domáca adresa, IBAN); kontaktná osoba do poznámky
- [x] Počiatočné stavy účtov z tabuľky (konečná súvaha alebo predvaha z iného programu: Účet; Názov; MD; D alebo jeden stĺpec so zostatkom) — návrh do editora, 211000 → 211, upozornenie na chýbajúce účty a nevyrovnané strany
- [x] SuperFaktúra a Fakturoid: ich export pre účtovníka (POHODA, Money S3, KROS Omega alebo MRP) — formát sa rozpozná sám, aj v ZIP; kontakty z faktúr
- [ ] KROS Alfa; priame napojenie cez API (Fakturoid, iDoklad, SuperFaktúra)
- [x] Napojenie e-shopov WooCommerce a Shopify (neoverené na živom obchode): objednávky → koncepty faktúr (zákazník, IČO, kontaktná osoba, dodacia adresa, väzba na cenník podľa kódu, sadzba DPH z dane), „iba uhradené“, bez duplicít; stav zásob zo skladu do obchodu; na serveri zákaz volaní do vnútornej siete
- [x] PrestaShop, Magento / Adobe Commerce, Drupal Commerce (neoverené); automatické načítanie objednávok každú hodinu s chybou pri obchode
- [x] Shoptet (súkromné API): objednávky → koncepty faktúr, zásoby (realStock) — neoverené
- [x] Riadenie obchodu z Fiškusu (WooCommerce): prevzatie produktov a zákazníkov, odoslanie cenníka (názov, cena, trieda DPH, vyradenie) iba zmien, nové skladové položky ako koncept; každú hodinu aj so zásobami
- [x] Riadenie obchodu pre Shopify (zdaniteľný = 23 %, nezdaniteľný = 0 %) a Magento (triedy dane z daňových pravidiel, ceny bez DPH)
- [x] Riadenie obchodu pre Shoptet podľa oficiálnej schémy openapi.yaml: produkty a varianty, zákazníci, cena v predvolenom cenníku (sadzba + cena bez DPH), názov a skrytie, nový produkt skrytý v prvej kategórii
- [x] Riadenie obchodu pre PrestaShop: skupiny daňových pravidiel (SK), produkty aj kombinácie, zákazníci; úprava celého produktu v XML (iba cena, DPH, aktívnosť, jednojazyčný názov), nový produkt neaktívny v domovskej kategórii
- [ ] Upgates (čaká na oficiálnu dokumentáciu)

## Etapa 4 — mzdy (samostatný návrh)
- [ ] Sociálna poisťovňa, zdravotné poisťovne, preddavky na daň, ročné zúčtovanie

## Odlíšenie od konkurencie
Slovenské špecifiká, ktoré MyÚčto nemá: RÚZ, tri identifikátory (IČO / DIČ / IČ DPH), 2 % dane pre neziskovky, slovenská e-fakturácia, PAY by square.

## Kalendár termínov
- [x] Kalendár (mesiac aj zoznam): priznanie k DPH a KV, SV (ak sú dodania do EÚ), daň z príjmov, daň z motorových vozidiel — lehoty posunuté na pracovný deň podľa dní pracovného pokoja (upraviteľné v Legislatíve); splatnosti vydaných a prijatých faktúr, pravidelné faktúry, koniec platnosti ponúk; zmeškané termíny; podané priznanie označené
- [x] Vlastné opakované termíny (odvody, preddavky, STK, poistky) s rýchlym výberom a označením „splnené“
- [x] Odber do mobilu a kalendára (iCalendar, tajný odkaz iba na čítanie, pripomienky 3 dni a 1 deň vopred), súbor .ics, odznak v menu
- [x] Prepojenie kalendára termínov s Google Kalendárom a Outlookom (Microsoft 365, Outlook.com) cez ich API — funguje aj v desktopovej verzii
- [ ] Upozornenie e-mailom pred lehotou, termíny miezd (etapa 4)
