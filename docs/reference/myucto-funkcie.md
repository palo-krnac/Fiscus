# Funkčný inventár: MyÚčto.cz (referencia pre nový SK účtovný systém)

Zdroj: repozitár `radekhulan/myucto` (klon v `/root/ref/myucto`), verzia podľa `VERSION`, licencia v1.4 (22. 8. 2026).
Prečítané: README.md, AGENTS.md, LICENCE.txt, celý manuál (121 kapitol; úvodná mapa funkcií prečítaná celá, ostatné kapitoly cez nadpisy, súhrny a vybrané sekcie), definície API rout (`api/src/Routes.php` + `api/src/Routes/*`), `api/openapi.yaml`, DB snapshot (`db/schema.snapshot.json`), router a stránky frontendu (`web/src/router`, `web/src/pages`, `web/src/views`), MCP server (`MCP/`), zoznam cron skriptov (`api/bin/cron-*`).

Dokument opisuje **ČO** systém robí, nie ako je napísaný. Žiadny kód sa nepreberá.

## Legenda značiek

- **[CZ-špecifické]**: viazané na českú legislatívu alebo české systémy. V zátvorke je pravdepodobný slovenský ekvivalent: `(SK: …)`, pri neistote `(SK: …?)`.
- **[všeobecné]**: dá sa prebrať koncepčne bez ohľadu na krajinu.
- **[proprietárne]**: funkcia patrí do platenej „Komerčnej funkčnej nadstavby“ podľa LICENCE.txt čl. 1.4 písm. a)–f). Ide o: (a) podvojné účtovníctvo (denník, hlavná kniha, výkazy, automatické účtovanie, účtovné nastavenia), (b) účtovné nástroje, obdobia, uzávierky a závierkové balíčky, (c) sklad a napojenie e-shopu, (d) evidencia a účtovné spracovanie majetku, (e) EPO podania, priame EPO napojenie a archív podaní, (f) rozšírené opravy DPH podľa § 74b, § 43, § 46 a § 79.

> **Poznámka k licencii.** Čl. 1.4 hovorí, že proprietárne je *všetko*, čo nepochádza z MIT projektu MyInvoice. Zoznam a)–f) je len „najmä“, teda funkčný výpočet platenej vrstvy. Manuál (kap. 104) pridáva do platenej časti aj **mzdy** (samostatný doplnok so samostatnou cenou), **režim OSS**, **automatizáciu**, **prehľad firiem** a **daňovú evidenciu**. LICENCE 1.12 však „základnú daňovú evidenciu“ uvádza medzi bezplatnými funkciami, takže dokumenty sa tu rozchádzajú. Bezplatné (z MyInvoice) sú: fakturácia, prijaté doklady, kontakty, banka a pokladnica, základná daňová evidencia, bežné výkazy DPH (priznanie, KH, SH) a nastavenia firmy. Značka [proprietárne] nižšie sleduje čl. 1.4 a)–f) a doplnky z manuálu. Pre clean-room reimplementáciu na tom nezáleží, pretože opisujeme iba funkcie.

---

## 0. Architektúra a prierezové princípy (pre kontext)

- Self-hosted (Docker multi-arch, IIS, Apache, nginx), bez telemetrie; PHP 8.5/Slim + Vue 3/TS + MariaDB, voliteľne Redis. [všeobecné]
- Multi-firma („multi-supplier“): ľubovoľný počet izolovaných firemných agend v jednej inštalácii; izolácia je vynútená v API, zostavách, cronoch aj cestách k súborom. [všeobecné]
- Dve rozhrania nad tými istými dátami: administrácia pre účtovníčku a samostatný **klientsky portál** pre podnikateľa. [všeobecné]
- „Doklad sa zadá raz“: z dokladu sa odvodí platba, zaúčtovanie, DPH, daň z príjmov aj závierka. [všeobecné]
- AI iba navrhuje. Účtuje deterministický engine s pravidlami a limitmi; AI návrhy sa nedajú schváliť hromadne. [všeobecné]
- Účtovná a daňová vrstva je cez API a MCP **iba na čítanie**: zaúčtovanie, storno, uzavretie obdobia ani podanie sa cez token urobiť nedajú. [všeobecné]
- Nemenné PDF dokladu vznikne pri vystavení; daňové XML sa ukladajú ako nemenné snapshoty s hashom. [všeobecné]
- Storno namiesto mazania; zaúčtované doklady sú zamknuté; uzavreté obdobie môže znovu otvoriť len admin. Platí aj zámok účtovania k dátumu. [všeobecné]
- Activity log všetkých mutácií s reťazením hashov (tabuľka `activity_log_chain_head`) a maskovaním citlivých hodnôt. [všeobecné]
- Lokalizácia CS/EN (vue-i18n), PWA, svetlý a tmavý režim, viac pracovných panelov naraz, klávesové skratky. [všeobecné]
- Moduly sa zapínajú po firmách: Vést účetnictví, Vést mzdy, Vést sklad, Režim OSS; k tomu režim podvojné účtovníctvo / daňová evidencia a účtovný rámec 500 / 504. [všeobecné]

---

## 1. Fakturácia / predaj (vydané doklady)

1. Typy dokladov: faktúra – daňový doklad (platiteľ), faktúra (neplatiteľ), zálohová faktúra (proforma), opravný daňový doklad (dobropis) s väzbou na originál, interné storno. [všeobecné]
2. Editor faktúry: hlavička (typ, klient, zákazka, dátumy, mena, DPH, zaokrúhlenie úhrady, ručné číslo, VS), položky a textové riadky, poznámky nad a pod položkami. [všeobecné]
3. Ceny s DPH aj bez DPH (brutto/netto) a prenos príznaku pri každom odvodení dokladu (proforma → faktúra, dobropis, klon, opakovaná). [všeobecné]
4. Sadzby DPH po položkách, zmiešané sadzby, rekapitulácia po sadzbách. [všeobecné]
5. Zľava z celej faktúry. [všeobecné]
6. Faktúra v cudzej mene s prepočtom do domácej meny (kurz ČNB). [CZ-špecifické] (SK: kurz ECB/NBS z predchádzajúceho dňa, domáca mena EUR)
7. Spôsob úhrady vrátane hotovosti a automatického vyrovnania cez pokladnicu (vznikne PPD). [všeobecné]
8. Položka „predaj majetku“ s väzbou na kartu majetku. [všeobecné] [proprietárne]
9. Časové rozlíšenie výnosu priamo z položky faktúry. [všeobecné] [proprietárne]
10. Vyúčtovanie so zápornou sumou na vyplatenie (preplatok klientovi). [všeobecné]
11. Zálohový cyklus: proforma → daňový doklad k prijatej platbe (DDKP) → vyúčtovacia faktúra, párovanie existujúcich záloh, postupné čerpanie jednej zálohy viacerými faktúrami (míľniky). [CZ-špecifické] (SK: zálohová faktúra + faktúra k prijatej platbe podľa § 71 ods. 1 písm. b) ZDPH + vyúčtovacia faktúra)
12. Číselné rady s nastaviteľným formátom VS (`YYMM###`, `YY####`, vlastné šablóny) a náhľad ďalšieho čísla. [všeobecné]
13. Klonovanie faktúry s automatickým posunom mesiaca v popisoch (`3/2026 → 4/2026`). [všeobecné]
14. Hromadné akcie: znovu vystaviť, odoslať, upomenúť, označiť za zaplatené, zaúčtovať, zlúčené PDF, hromadné nastavenie OSS. [všeobecné]
15. Zoznam faktúr: stĺpce, filtre (kategória tržby, neistý OSS…), zoskupenie, uložené filtre a pohľady, „kto dlžil k dňu X“. [všeobecné]
16. Stavy faktúry (koncept, vystavená, odoslaná, po splatnosti, zaplatená, stornovaná…) s ikonovou legendou. [všeobecné]
17. Platby a čiastočné úhrady na faktúre; daňový doklad k prijatej platbe zo zálohy. [všeobecné]
18. Admin zásahy nad vystavenou faktúrou: editácia (force), vrátenie zo stavu zaplatená, force-delete, zrušenie storna, zmena bankového účtu po vystavení. [všeobecné]
19. Activity log na faktúre (kto vytvoril, vystavil, odoslal, kedy bola zaplatená). [všeobecné]
20. Výkaz víceprací (timesheet) ako druhá strana PDF; suma sa prenesie do položky a odoslaný výkaz sa archivuje ako snapshot. [všeobecné]
21. Výkaz materiálu k faktúre. [všeobecné]
22. Schvaľovanie výkazu zákazníkom cez verejný odkaz (token + CAPTCHA, bez účtu); po schválení sa faktúra sama vystaví a odošle. Obsahuje test schválenia, ručnú zmenu stavu a pripomienky. [všeobecné]
23. Sledovací verejný odkaz na výkaz práce s overením kódom. [všeobecné]
24. Penalizačná faktúra (úrok z omeškania) vrátane náhľadu a nadväzných penalizácií. [CZ-špecifické] (SK: úrok z omeškania = základná sadzba ECB + 8 p. b., § 369a ObZ / NV 21/2013)
25. Prepojenie faktúry so skladovými výdajkami (automatický výdaj pri vystavení). [všeobecné] [proprietárne]
26. Kontrola príloh faktúry (attachment check) s potvrdením. [všeobecné]
27. Vratka odberateľovi: platobný príkaz z faktúry (refund order). [všeobecné]
28. Zápočet dobropisu s faktúrou. [všeobecné]
29. Prepojenie s EET (platba prijatá pri faktúre). [CZ-špecifické] (SK: eKasa)
30. Pravidlo účtovania priamo z detailu faktúry. [všeobecné] [proprietárne]
31. Opakované (pravidelné) fakturácie: šablóna s periodicitou, položkami, placeholdermi obdobia, automatizáciou (vystaviť a odoslať), pauzou a obnovením, opravou termínu, ručným generovaním a kill-switchom. [všeobecné]
32. „Otvorený koncept“: paušál, ku ktorému sa v priebehu mesiaca pripisujú viacpráce. [všeobecné]
33. Upomienky: ručná, hromadná, automatická cez cron (X dní po splatnosti, cooldown 14 dní), šablóny pre 1./2./3. úroveň, test a dry-run, tri úrovne vypnutia. [všeobecné]
34. E-mail s poďakovaním za úhradu (cron payment-thanks). [všeobecné]
35. Ceník služieb: položky, ceny v menách, individuálne ceny zákazníkov (len pre firmy bez skladu). [všeobecné]
36. Zákazky (projekty) 1:N pod klientom: fakturačné e-maily, šablóny podmienok, ekonomika zákazky (výnosy, náklady, marža), pauza a uzavretie, fronta schvaľovania. [všeobecné]
37. Kategórie tržieb a nákladov (číselník) pre reporting. [všeobecné]
38. Zjednodušený daňový doklad (§ 30 ZDPH). [CZ-špecifické] (SK: zjednodušená faktúra § 74 ods. 3 ZDPH / doklad z eKasy)
39. Rýchla účtenka pre mobilné aplikácie a pokladnice (cez API). [všeobecné]

### 1a. PDF, QR platba, e-mail, zdieľanie

40. Nemenné PDF (mPDF + Twig šablóny), história verzií PDF, prestavba snapshotov. [všeobecné]
41. PDF/A-3b (archívny formát) s možnosťou vloženého ISDOC. [CZ-špecifické] (SK: PDF/A-3 s vloženým UBL / ZUGFeRD-Factur-X?)
42. QR platba SPAYD pre CZK. [CZ-špecifické] (SK: PAY by square)
43. QR SEPA EPC pre EUR. [všeobecné]
44. Odoslanie e-mailom cez vlastné SMTP s DKIM, výber príjemcov podľa účelu kontaktu, prílohy a test odoslania na vlastnú adresu. [všeobecné]
45. E-mailové šablóny s placeholdermi (`{varsymbol}`, `{amount}`, `{due_date}`), viacjazyčné, per firma. [všeobecné]
46. S/MIME podpis e-mailov a elektronický podpis PDF. [všeobecné]
47. „Web faktúra“: trvalý verejný odkaz na faktúru (PDF, ISDOC, prílohy), regenerácia odkazu, vypnutie na inštalácii. [všeobecné]
48. Verejný odkaz na účtenku (receipt link). [všeobecné]
49. Branding per firma: logo, farby, hlavička a pätička, brandingové profily PDF a e-mailov. [všeobecné]

### 1b. Daňový sprievodca fakturácie

50. Prepínač platiteľ/neplatiteľ DPH s históriou zmien statusu (dátum vzniku a zániku registrácie); mení hlavičku, stĺpce aj povinné poznámky. [CZ-špecifické] (SK: platiteľ podľa § 4/§ 4b; registrácia podľa § 7/§ 7a; tri identifikátory IČO/DIČ/IČ DPH)
51. Stráženie limitu pre povinnú registráciu (obrat). [CZ-špecifické] (SK: obrat 50 000 € s hranicou 62 500 € od 2025)
52. Identifikovaná osoba (§ 6g–6l ZDPH). [CZ-špecifické] (SK: registrácia podľa § 7/§ 7a)
53. Číselník sadzieb DPH (`CZ-21`, `CZ-12`, `CZ-0`, `CZ-RC`) vrátane sadzby s budúcou platnosťou. [CZ-špecifické] (SK: 23 %, 19 %, 5 %, 0 %)
54. Klasifikačné kódy DPH na riadkoch (automaticky podľa sadzby), ktoré sa nesú až do kontrolného hlásenia. [CZ-špecifické] (SK: mapovanie riadkov na oddiely kontrolného výkazu A1/A2/B1/B2/B3/C1/C2/D1/D2)
55. Reverse charge: tuzemský podľa § 92a–g aj EU B2B, automatická zákonná doložka. [CZ-špecifické] (SK: § 69 ods. 12 tuzemský RC, § 69 ods. 2/3 EU)
56. Overenie EU VAT ID cez VIES (s cache). [všeobecné]
57. Export mimo EU (nie RC), registrácia k DPH vo viacerých krajinách. [všeobecné]
58. Rozhodujúci je DUZP a zaradenie dokladu do obdobia podľa neho. [CZ-špecifické] (SK: dátum dodania tovaru/služby, DVD)

---

## 2. Adresár / klienti / CRM a štatistiky

1. Klienti v roli odberateľ aj dodávateľ (jeden kontakt môže mať obe roly), archivácia. [všeobecné]
2. Lookup podľa IČO v ARES (názov, adresa, DIČ, právna forma). [CZ-špecifické] (SK: RPO ŠÚ SR / ORSR / ŽRSR / FinStat API)
3. Lookup v registri platiteľov DPH (zverejnené bankové účty, nespoľahlivý platiteľ). [CZ-špecifické] (SK: zoznam platiteľov DPH FS SR + zoznam platiteľov s dôvodmi na zrušenie registrácie / rizikoví platitelia)
4. Slovenský klient a národné daňové čísla (podpora SK IČO/DIČ/IČ DPH už existuje). [všeobecné]
5. E-mailové kontakty klienta podľa účelu (faktúry, upomienky, výkazy), predmet e-mailu a názov PDF per klient. [všeobecné]
6. Bankové účty protistrany (aj naučené z bankových pohybov). [všeobecné]
7. Detail klienta s históriou dokladov a predvolenými dimenziami. [všeobecné]
8. Dashboard „Zisk“: KPI, porovnanie období, grafy, TOP klienti a dodávatelia, aging pohľadávok a záväzkov, ukazovatele zdravia firmy, kategórie, churn. [všeobecné]
9. Dashboard „Tržby“: obrat, predikcia roka, limit registrácie k DPH, spoločný prehľad všetkých firiem (meny, spojené osoby, riziká). [všeobecné]
10. Dashboard „Náklady“: náklady, záväzky, kategórie. [všeobecné]
11. CRM analytika: DSO, DPO, platobná disciplína, histogram doby úhrady, efektivita upomienok, riziko oneskorenia, churn risk, koncentrácia odberateľov a dodávateľov, cash-flow forecast, daňový kalendár. [všeobecné]
12. Hlavný prehľad (dashboard): KPI dlaždice, top klienti, stav faktúr, obrat po mesiacoch, po splatnosti, „Akcie pre teba“ (fronta úloh so skrývaním), daňový kalendár, sprievodca prvým nastavením, rýchle „+ nový doklad“. [všeobecné]
13. Prehľad skupiny firiem (group stats / portfolio) so súčtami a prepočtom meny. [všeobecné]
14. Globálne vyhľadávanie naprieč odberateľmi a dokladmi. [všeobecné]

---

## 3. Nákup / prijaté doklady

1. Prijaté faktúry a účtenky: koncept → kontrola → prijatý, stavy, zoznam so stĺpcami úhrad, filtre a sestavy. [všeobecné]
2. Nahranie dokladu drag & drop; import ISDOC/ISDOCX alebo PDF s vloženým ISDOC. [CZ-špecifické] (SK: UBL 2.1 / Peppol BIS e-faktúra, PDF s vloženým XML)
3. Vyťaženie bežného PDF alebo fotky cez AI (pozri modul AI). [všeobecné]
4. Druh výdavku po riadkoch, daňová uznateľnosť, nárok na odpočet, účtovná alokácia časti dokladu. [všeobecné]
5. Rekapitulácia DPH podľa dokladu (§ 73 ZDPH) a obdobie odpočtu. [CZ-špecifické] (SK: § 51 ZDPH)
6. Dodávateľ neplatiteľ = bez nároku na odpočet; snapshot statusu platiteľa dodávateľa. [všeobecné]
7. Reverse charge z EU: rozlíšenie nadobudnutia tovaru a služby, samovyrubenie. [všeobecné]
8. Faktúra v cudzej mene, kurz a jeho prenačítanie. [CZ-špecifické] (SK: kurz ECB/NBS)
9. Zaúčtovanie dobropisu. [všeobecné]
10. Párovanie zálohy s vyúčtovacou faktúrou, aby sa náklad nezapočítal dvakrát. [všeobecné]
11. Úhrada: QR kód pre mobilnú banku, označenie úhrady, hromadná platba, hotovosť z pokladnice priamo z editora. [všeobecné]
12. Prijaté doklady z klientskeho portálu (podateľňa) a fronta „Prichádzajúce doklady“. [všeobecné]
13. Scan inbox: automatický import z adresára (cron). [všeobecné]
14. Schvaľovanie prijatých dokladov (aj cez verejný odkaz pre schvaľovateľa, s pripomienkami) a dimenzie pri schvaľovaní. [všeobecné]
15. Oprava DPH zaradenia bez storna; vrátenie zo storna s rovnakým číslom. [všeobecné]
16. Prečíslovanie prijatých dokladov (renumber). [všeobecné]
17. Nákladové pravidlo a preúčtovanie z detailu. [všeobecné] [proprietárne]
18. Export prijatých: „naše PDF“ (rekonštrukcia), ISDOC, Pohoda XML, jednotlivo aj hromadne za obdobie. [CZ-špecifické] (SK: UBL, Pohoda SK XML, Money S3 SK)
19. Platobné príkazy: hromadná úhrada vybraných faktúr, overenie účtu príjemcu, QR a PDF náhľad, stav „Odovzdané na úhradu“, história príkazov. [všeobecné]
20. Formát ABO (KPC) platobného príkazu. [CZ-špecifické] (SK: SEPA pain.001; ABO/KPC sa v SK nepoužíva)
21. Formát SEPA pain.001. [všeobecné]
22. Priame odoslanie príkazu do banky cez API (ČS, KB Business BATCHDA…). [CZ-špecifické] (SK: PSD2 PIS – Tatra banka, SLSP, VÚB, ČSOB SK?)
23. Platby daní finančnému úradu z príkazov. [CZ-špecifické] (SK: platby na účty daňového subjektu v Štátnej pokladnici, VS podľa FS SR)
24. Štatistika nákupov (purchase stats). [všeobecné]
25. Ochrana proti duplicitným dokladom (rovnaký dodávateľ + číslo). [všeobecné]

---

## 4. AI funkcie

1. AI extrakcia prijatých faktúr z PDF a obrázkov (aj viacstranových): dodávateľ, sumy, sadzby, položky. [všeobecné]
2. AI import **vydaných** faktúr z PDF (migrácia histórie). [všeobecné]
3. Voľba AI poskytovateľa per firma: Anthropic Claude (predvolený), Azure OpenAI, OpenAI, Google Gemini, lokálne Ollama; automatický upgrade modelu, ladenie (poznámky, úroveň uvažovania). [všeobecné]
4. Bez potvrdenej spracovateľskej zmluvy (DPA) s poskytovateľom sa AI tvrdo zablokuje; súhlas sa dáva per poskytovateľ. EU rezidencia dát je vynútená. [všeobecné]
5. Sanity check: rozdiel súčtu položiek oproti základu nad 2 % označí doklad „Na kontrolu“; zachytí aj katastrofálny mismatch a dokáže dodatočne skontrolovať uložené faktúry. [všeobecné]
6. Kontrola identifikátorov a dátumov pri extrakcii, skupinová registrácia DPH (dve DIČ), doklad bez jednotkových cien (založenie zo súhrnnej rekapitulácie). [všeobecné]
7. Automatika pre reverse charge zo zahraničia. [všeobecné]
8. Kill-switch AI pre celú firmu so záznamom do logu; denné limity a metriky využitia per klient. [všeobecné]
9. AI návrhy účtovania (kontácie) pre prijaté doklady a bankové pohyby, s vysvetlením „Prečo“ a mierou istoty; hromadné schválenie nie je povolené. [všeobecné] [proprietárne]
10. AI rule miner: z opakovaných opráv navrhne nové pravidlo (cron). [všeobecné] [proprietárne]
11. AI asistent účtovníčky: konverzácie, otázky nad účtovníctvom, označenie aplikovanej odpovede. [všeobecné] [proprietárne]
12. Embeddingy dokumentov pre fulltextové a sémantické vyhľadávanie. [všeobecné]
13. Pripojenie skenov k existujúcim dokladom (dávka skenov → automatické párovanie → kontrola rozporov údajov sken vs. doklad). [všeobecné]
14. AI asistent nastavenia účtovania (setup assistant): analýza histórie, návrhy pravidiel, schválenie, rollback, preúčtovanie histórie. [všeobecné] [proprietárne]

---

## 5. Banka, pokladnica a platobné prostriedky

### 5a. Banka

1. Bankové účty firmy vo viacerých menách; analytika 221 a vlastný dokladový rad bankových zápisov. [všeobecné]
2. Import výpisov GPC/ABO, CSV a podporovaných PDF výpisov (KB, Fio, ČSOB, RB, ČS, mBank…). [CZ-špecifické] (SK: CAMT.053 XML, MT940, CSV/PDF výpisy SK bánk)
3. SHA-256 kontrola duplicitného výpisu a validácia čísla účtu v hlavičke. [všeobecné]
4. Inteligentné párovanie podľa VS (s normalizáciou), sumy, protistrany a histórie, s toleranciou ±0,01; návrhy párovania. [všeobecné]
5. Ručné párovanie, jedna platba na viac faktúr (zlúčená úhrada), čiastočné platby, ignorovanie, zrušenie párovania, audit párovania. [všeobecné]
6. Kurzový rozdiel pri cudzomenových spárovaných platbách. [všeobecné]
7. Vytvorenie prijatej faktúry priamo z bankového pohybu. [všeobecné]
8. Vlastné prevody medzi účtami cez 261 (peniaze na ceste). [všeobecné]
9. Rozpoznanie odvodov ČSSZ, zdravotným poisťovniam a FÚ. [CZ-špecifické] (SK: Sociálna poisťovňa, VšZP/Dôvera/Union, daňový úrad)
10. Pravidlá účtovania opakovaných platieb, šablóny bankových pravidiel, schvaľovanie návrhov, automatické zaúčtovanie spárovaných platieb. [všeobecné] [proprietárne]
11. Výpis s nespracovanými pohybmi, zoznam všetkých pohybov, export nespárovaných. [všeobecné]
12. Priame napojenie na banku cez API (ČSOB, Česká spořitelna, Raiffeisenbank, KB Business, Creditas, Moneta, Fio): sťahovanie pohybov a zostatkov, export GPC, cron synchronizácia, OAuth relácie, šifrovanie prístupov. [CZ-špecifické] (SK: PSD2 AIS – Tatra banka, SLSP, VÚB, ČSOB SK, 365.bank, Fio SK?)
13. E-mailové bankové avízá cez IMAP: parsery providerov (aj regex), overenie DKIM/DMARC, mapovanie na účty, rekonciliácia s neskorším výpisom, jedna schránka pre viac firiem. [všeobecné]
14. Načítanie PDF faktúr a PDF výpisov z príloh e-mailov; denné výpisy sa skladajú do mesačného. [všeobecné]
15. Cron: automatický scan adresára s výpismi. [všeobecné]
16. Stavy na účtoch (prehľad zostatkov). [všeobecné]

### 5b. Pokladnica

17. Číselník pokladníc vrátane valutových; PPD a VPD s vlastným číslovaním. [všeobecné]
18. Pokladničný doklad s DPH (DPH riadky), účel, prílohy (sken účtenky), náhľad zaúčtovania. [všeobecné]
19. Úhrada faktúry z pokladnice, doklad vzniknutý z faktúry pri hotovostnom vyrovnaní. [všeobecné]
20. Storno dokladu, tlač dokladu, pokladničná kniha. [všeobecné]

### 5c. Platobné a kreditné karty, platobné brány

21. Firemné platobné karty a držitelia, koncovka karty v pohyboch, párovanie platieb kartou s dokladmi, uzavretie platby bez dokladu, vozidlo podľa karty. [všeobecné]
22. Účtovanie kariet cez medzičlen (zúčtovací účet), analytika karty, predkontácie. [všeobecné] [proprietárne]
23. Kreditné karty ako úverový účet (231): import výpisu, mesačné odsúhlasenie, počiatočný dlh, nákupy bez dokladu, režim nákupov. [všeobecné]
24. GoPay: import vyúčtovania (XML) a výpisu obchodného účtu (XLS/XLSX), párovanie úhrad, poplatky, vratky, výplata na bankový účet, stavy vyúčtovania. [CZ-špecifické] (SK: GoPay funguje aj v SK; ďalej Besteron, TrustPay, ComGate, Stripe)

---

## 6. DPH

1. Priznanie k DPH (DPHDP3): zostavenie z riadkov dokladov, mapovanie na riadky, prepočet meny, samovyrubenie pri RC, nadmerný odpočet, typ podania (riadne, opravné, dodatočné), pole EPO a VetaP (FÚ, typ platiteľa, osoby, zástupca, zostavovateľ). [CZ-špecifické] (SK: Daňové priznanie k DPH, formulár FS SR)
2. Kontrolné hlásenie (DPHKH1): sekcie A/B, pravidlá zaradenia, typ podania (riadne, následné), odpoveď na výzvu, súpis dokladov oddielu KH pre FÚ. [CZ-špecifické] (SK: kontrolný výkaz DPH)
3. Súhrnné hlásenie (DPHSHV): riadne aj následné, zoskupenie, zaokrúhlenie. [CZ-špecifické] (SK: súhrnný výkaz DPH)
4. Kniha DPH (evidencia podľa § 100) s drill-down na doklad a riadok, obdobie odpočtu, pomerný a krátený odpočet. [CZ-špecifické] (SK: záznamová povinnosť § 70 ZDPH)
5. Krížová kontrola priznania s KH, SH a účtom 343; prevod DPH na zúčtovací účet (VAT clearing cron). [všeobecné]
6. Fronta „Doklady zmenené po podaní“ a zámok podaného obdobia DPH. [všeobecné]
7. Koeficient krátenia odpočtu (§ 76). [CZ-špecifické] (SK: koeficient § 50 ZDPH)
8. Oprava odpočtu pri neuhradených záväzkoch (§ 74b). [CZ-špecifické] [proprietárne] (SK: § 53b ZDPH – neuhradenie do 100 dní po splatnosti?)
9. Nedobytné pohľadávky (§ 46). [CZ-špecifické] [proprietárne] (SK: § 25a ZDPH)
10. Opravy DPH podľa § 43 a pri vzniku či zrušení registrácie (§ 79, § 79a). [CZ-špecifické] [proprietárne] (SK: § 25 oprava základu, § 54/§ 54a úprava odpočtu, § 81 zrušenie registrácie?)
11. Úprava odpočtu (vat_deduction_adjustments) za roky. [CZ-špecifické] (SK: § 54 ZDPH úprava odpočítanej dane pri investičnom majetku)
12. Pohyb odpočtu v čase (§ 73) s upozornením na doklad, ktorý rozdiel spôsobil. [CZ-špecifické] (SK: § 51)
13. Predikcia DPH z konceptov a plánovaných faktúr (oddelená od skutočnosti) a vývoj DPH za 12 mesiacov. [všeobecné]
14. Režim OSS: zapnutie, prah 10 000 EUR a jeho sledovanie, číselník sadzieb členských štátov, automatické odvodenie OSS riadku, ručné posúdenie sporných plnení, hromadná úprava, doložka na faktúre, účtovanie OSS dane, štvrťročné priznanie OSSEI1 s prepočtom meny a opravami minulých období, evidencia 110f. [CZ-špecifické] (SK: OSS je EU režim; formulár OSS na portáli FS SR, logika rovnaká) [proprietárne podľa manuálu]
15. Podpora daňového poradcu (zastupovanie, história zastúpenia). [všeobecné]
16. Platba DPH: platobný príkaz z podania. [CZ-špecifické] (SK: VS a účet daňového subjektu)
17. Mesačný hromadný export pre účtovníčku (ZIP: faktúry, výpisy, GoPay, kniha DPH). [všeobecné]
18. Audit kurzov ČNB (kurz na doklade vs. denný kurz). [CZ-špecifické] (SK: kurz ECB/NBS)

---

## 7. EPO podania, archív a datová schránka

1. Archív daňových výstupov: každé XML ako nemenný snapshot s hashom, výsledkom validácie a auditnou históriou; stavy výstupu, filtre a pravidlá mazania. [všeobecné] [proprietárne]
2. Asistované podanie: odovzdanie snapshotu do predvyplneného formulára EPO a spätné nahranie podaného XML a potvrdenia; dokumentová zložka sa založí automaticky. [CZ-špecifické] [proprietárne] (SK: eDane / portál FS SR – import XML do elektronickej podateľne?)
3. Priame podanie cez EPO API s kvalifikovaným certifikátom P12/PFX: testovací režim `test=1`, ostré podanie s novým potvrdením, archivácia podpísaného balíka, odpovede a doručenky, sledovanie stavu (cron). [CZ-špecifické] [proprietárne] (SK: podpis KEP/QES + eDane API / integračné rozhranie FS SR?)
4. Šifrované uloženie certifikátu, obmedzené na vlastníka a povolené firmy; vzájomná blokácia asistovaného a priameho toku. [všeobecné] [proprietárne]
5. Overovanie doručeniek (whitelist odtlačku podpisového certifikátu). [CZ-špecifické] [proprietárne] (SK: potvrdenie o podaní z eDane?)
6. Rekonciliácia: porovnanie podaného XML s dnešnými dátami. [všeobecné] [proprietárne]
7. Klientske podávanie pripravených výkazov (cez portál). [všeobecné]
8. Datová schránka (ISDS): odoslanie podania, načítanie doručeniek a prijatých správ, nejasný stav odoslania, zrušenie a mazanie, príjemcovia, výzvy na odstránenie vád, kategórie a pravidlá inboxu, purge. [CZ-špecifické] (SK: elektronická schránka slovensko.sk (ÚPVS) + schránka na portáli FS SR)
9. Globálna odosielacia brána ISDS (registrácia aplikácie, koncept správy potvrdzuje používateľ v ISDS). [CZ-špecifické] (SK: ÚPVS – integrácia cez eDesk/IAM?)
10. Elektronické podpisy: trezor certifikátov, podpisové profily (firemné, osobné, rolové), podpis PDF faktúr, výkazov práce, ISDOC a e-mailov S/MIME; časová pečiatka, dôvod podpisu, politika hesla, overenie podpísaného PDF, audit. [všeobecné]

---

## 8. Podvojné účtovníctvo – denník, hlavná kniha, výkazy

Celý modul [proprietárne] (čl. 1.4 a).

1. Účtový rozvrh per firma: automatické založenie osnovy, syntetické a analytické účty (bodkový zápis), deaktivácia, import a export, karta účtu; analytiky DPH 343.100/200/900, mesačné zúčtovanie DPH. [CZ-špecifické] (SK: rámcová účtová osnova pre podnikateľov podľa opatrenia MF SR 23054/2002-92; číslovanie účtov je takmer zhodné)
2. Účtovný denník: chronologický zoznam, drill-down na zdrojový doklad oboma smermi, filtre, export PDF/XLSX, koncept vs. zaúčtované. [všeobecné]
3. Ručný zápis: hlavička, riadky, kontrola vyrovnanosti, „uložiť a nový“, viacmenové riadky. [všeobecné]
4. Šablóny ručných zápisov a import zápisov z CSV. [všeobecné]
5. Prevod medzi účtami cez 261. [všeobecné]
6. Storno a oprava zaúčtovaného zápisu, automatické storno pri zmazaní alebo internom storne dokladu, zmazanie celej storno dvojice. [všeobecné]
7. Preúčtovanie z dokladu (sekcia Zaúčtovanie) a zobrazenie podkladu, podľa ktorého sa účtovalo. [všeobecné]
8. Idempotencia: doklad nejde zaúčtovať dvakrát. [všeobecné]
9. Prílohy zápisu (§ 33a ZoÚ), poznámky, inline editácia popisu, história zápisu, väzba doklad ↔ úhrada. [CZ-špecifické] (SK: účtovný záznam §§ 31–35 zákona 431/2002)
10. Zámok účtovania k dátumu. [všeobecné]
11. Nočná kontrola integrity denníka (MD = D) s nálezmi. [všeobecné]
12. Doúčtovanie nezaúčtovaných dokladov (backfill) a doplnenie popisov prevzatých zápisov. [všeobecné]
13. Ostatné pohľadávky a záväzky: položky mimo faktúr, účtovanie v PÚ aj DE, úhrady, storno, splátkový kalendár, opakovanie (cron), dimenzie, prílohy. [všeobecné]
14. Interné doklady: rýchle zadanie, číselný rad, zoznam, storno, prílohy. [všeobecné]
15. Automat účtovania: záložky Zaúčtované dnes / Na schválenie / Vyžaduje zásah / Pravidlá / Checklist / História / Odporúčania; tri úrovne (vypnuté, návrhy, plná automatika) per typ operácie; limit sumy na pravidlo, denný objemový strop, účtovanie len v otvorenom období a pri jednoznačnej kontácii; učenie z opráv; ranný e-mailový digest. [všeobecné]
16. Fronta „Na doúčtovanie“: bankový pohyb bez návrhu, nezaúčtovaný vydaný alebo prijatý doklad, otvorená žiadosť o dokument. [všeobecné]
17. Štyri vrstvy určujúce kontáciu: predkontácia, pravidlá nákladov, pravidlá banky, šablóny. [všeobecné]
18. Predkontácie a šablóny (zápisové, nákladové pravidlá, bankové pravidlá, šablóny bankových pravidiel), doplnenie predkontácií podľa osnovy. [všeobecné]
19. Hlavná kniha: PS, obraty, KS, mesačný rozpad, opis účtu, otvorené položky a ich párovanie, export. [všeobecné]
20. Obratová predvaha s kontrolami úplnosti a podvojnosti. [všeobecné]
21. Súvaha podľa vyhlášky 500 alebo 504: strom, výpočtové riadky, mapovanie účtov na riadky per firma, kategória účtovnej jednotky, verzie, návrh z podaného priznania, súvaha po účtoch. [CZ-špecifické] (SK: Súvaha Úč POD / Úč MÚJ podľa opatrenia MF SR 18008/2014 a 4455/2003)
22. Výsledovka druhová (500/504) vrátane odhadu do konca roka. [CZ-špecifické] (SK: Výkaz ziskov a strát Úč POD/MÚJ)
23. Výsledovka účelová (náklady podľa funkcie) s firemnou mapou. [všeobecné]
24. Prehľad o peňažných tokoch a zmenách vlastného imania (§ 18 ods. 2 ZoÚ). [CZ-špecifické] (SK: prehľad peňažných tokov a poznámky – veľké ÚJ / subjekty verejného záujmu)
25. Saldokonto: otvorené položky podľa partnera vs. zostatok účtu k dňu, zálohy 314/324, vyrovnanie zápisom, zápočet z výberu, návrh párovania, podklad pre banku (aging). [všeobecné]
26. Vzájomné zápočty: dohoda, výpočet otvorenej sumy, potvrdenie a zaúčtovanie, PDF dohody, zrušenie. [všeobecné]
27. Úplnosť dokladov: bankové pohyby bez dokladu, doklady po splatnosti, medzery v číselných radoch vydaných dokladov. [všeobecné]
28. Mesačná kontrola: rovnaká sada kontrol ako predbežný krok uzávierky, kedykoľvek počas roka. [všeobecné]
29. Mesačný prehľad (manažérsky report) pre klienta: náhľad, PDF, odoslanie e-mailom, história. [všeobecné]
30. Spojené osoby: súpis transakcií, merateľné cenové odchýlky, úpravy základu dane (§ 23 ods. 7 ZDP). [CZ-špecifické] (SK: závislé osoby § 2 písm. n) a transferové oceňovanie § 18 ZDP)
31. Dimenzie (strediská, projekty, vozidlá, lokality, vlastné typy): na dokladoch aj riadkoch denníka, rozpad na viac hodnôt, predvolené hodnoty klienta a zákazky, pravidlá podľa účtu, účtotvorná dimenzia, schvaľovanie, výkazy a cash-flow po dimenzii, súčty za skupinu firiem. [všeobecné]
32. Strediská (cost centers). [všeobecné]
33. Kurzový režim (denný/pevný kurz), kurzové prepočty a precenenie. [všeobecné]
34. Repo sadzba ČNB (pre úroky z omeškania). [CZ-špecifické] (SK: základná úroková sadzba ECB)
35. Zjednodušená mzdová rekapitulácia: z hrubej sumy štandardný rozpad, zaúčtovanie, mesačný podklad, ročný mzdový list, automatické mesačné zaúčtovanie (pre firmy s cudzou mzdovou agendou). [CZ-špecifické] (SK: odvody SP/ZP a preddavok dane podľa SK sadzieb)
36. Súbeh so starým systémom: mesačné porovnanie výstupov starého programu s MyÚčtom, klasifikácia rozdielov a uzavretie cyklu. [všeobecné]
37. Spoločné alokácie (rozpočítavacie kľúče nákladov medzi činnosti, verzie kľúčov). [všeobecné]
38. Inventarizácia súvahových účtov: protokol, odporúčané podklady, export. [CZ-špecifické] (SK: §§ 29–30 zákona 431/2002, rovnaké číslovanie)
39. Aktivácia podvojného účtovníctva (prechod z DE): dátum začatia, otváracia súvaha (predvyplnená z prechodového mostíka), kontrola nanečisto, ostré doúčtovanie, oprava zlyhaní. [všeobecné]
40. Výkaz o prechode (transition report) z DE na PÚ podľa príloh 2 a 3 ZDP. [CZ-špecifické] (SK: úprava základu dane pri prechode z JÚ na PÚ, § 17 ods. 9 ZDP?)

---

## 9. Účtovné obdobia a uzávierky

Celý modul [proprietárne] (čl. 1.4 b).

1. Účtovné obdobia: založenie, stavy, automaticky doplnené obdobia, hospodársky rok. [všeobecné]
2. Sprievodca ročnou uzávierkou v 10 krokoch v pevnom poradí: kontroly, odpisy, kurzové rozdiely, dohadné položky, časové rozlíšenie, opravné položky, daň z príjmov, zásoby, uzavretie kníh, otvorenie nového roka. Kroky sa dajú spustiť aj vrátiť. [všeobecné]
3. Predbežné kontroly so závažnosťou (chyba blokuje, varovanie prejde): technické účty 261/395/041/042/111/131/388/389/381–385, inventarizácia, spárované platby, ktoré nesedia, saldo 311/321, 343 vs. priznanie DPH, neobvyklá strana zostatku, kurz na doklade vs. kurz ČNB, majetok bez odpisov, drobný majetok vs. 501, rozdelenie VH 431. [všeobecné] (kurz: SK ECB/NBS)
4. Kontrolná mapa K1–K10/K11 a jej interpretácia. [všeobecné]
5. Návrhy pri uzávierke: dohadné položky, časové rozlíšenie (náklady, výnosy, drobný majetok), rezervy a zákonné opravné položky a rezervy, odložená daň (aj vlastnou metódou v rámci 504), kurzové rozdiely k súvahovému dňu. [CZ-špecifické] (SK: zákon 593/1992 o rezervách a OP sa líši; SK zákonné rezervy podľa § 20 ZDP?)
6. Prevzaté počiatočné stavy a otvorenie nového roka vrátane číselných radov. [všeobecné]
7. Schválenie závierky a znovuotvorenie obdobia (iba admin). [všeobecné]
8. Rozdelenie výsledku hospodárenia (431 → 428/421/364) s náhľadom a revertom. [všeobecné]
9. Uzávierkový (závierkový) balíček: export výkazov a podkladov. [všeobecné]
10. Poznámky k závierke (statement notes) s prenosom z minulého roka. [všeobecné]
11. Prechod medzi účtovnými rámcami 500 → 504 (riadená konverzia, bunky, rozhodnutia). [CZ-špecifické] (SK: prechod medzi podnikateľskou a neziskovou osnovou – opatrenie 24342/2007?)
12. Archív účtovníctva = kompletný export dát firmy. [všeobecné]
13. Retenčné lehoty a zadržanie skartácie (legal hold) pre účtovné záznamy. [CZ-špecifické] (SK: § 35 zákona 431/2002 – 10 rokov)

---

## 10. Daň z príjmov

1. DPFO (formulár DPFDP7) z účtovných dát alebo z DE: karty a stavy priznania, podklady, úpravy a odpočty, náhľad, export XML pre EPO. [CZ-špecifické] (SK: DPFO typ B)
2. DPPO (formulár DPPDP9): nedaňové náklady (§ 25), rozdiel účtovných a daňových odpisov, odpočty (§ 34), daňové straty, opravné položky a rezervy (tabuľka C), paušálny výdavok na dopravu, ručné položky s riadkom priznania. [CZ-špecifické] (SK: DPPO; § 21 ZDP nedaňové výdavky, § 30 odpočet straty, § 19 ods. 2 písm. l) paušál na auto)
3. Samostatný základ dane (§ 16a), sleva za zastavené exekúcie (§ 35 ods. 4). [CZ-špecifické] (SK: osobitný základ dane § 7/§ 8, zľavy?)
4. Riadne, opravné a dodatočné priznanie (§ 141 DŘ), hospodársky rok. [CZ-špecifické] (SK: § 16 daňového poriadku 563/2009)
5. Predfinalizačná kontrola, finalizácia, znovuotvorenie, rekonciliácia, import podaného DPPO do vstupov. [všeobecné]
6. Preddavky na daň podľa § 38a: automatické vygenerovanie rozpisu na ďalší rok z finalizovaného priznania, rozhodnutia FÚ, override, párovanie s platbami, potvrdenie. [CZ-špecifické] (SK: preddavky § 34 ZDP)
7. Prehľady OSVČ pre ČSSZ a zdravotné poisťovne (PDF a XML ČSSZ), poistné OSVČ. [CZ-špecifické] (SK: SP určuje poistné z DP automaticky; ZP ročné zúčtovanie – iba podklad?)
8. Oslobodený príjem a prefakturácia mimo základu dane. [všeobecné]
9. Oznámenie o príjmoch plynúcich do zahraničia a zabezpečenie dane. [CZ-špecifické] (SK: oznámenie o zrazenej dani / zabezpečení dane § 43–44 ZDP)
10. Priebežná projekcia dane z účtovných dát (s pôvodom každého riadku a drill-down do denníka), panel uzávierkových návrhov. [všeobecné]
11. Daňový profil (deti, manžel/manželka, činnosti) pre slevy a zvýhodnenia. [CZ-špecifické] (SK: NČZD, daňový bonus na dieťa)
12. Daňový optimalizátor OSVČ: porovnanie paušálnej dane a štandardného režimu, stráženie ročných limitov, zmena mesačnej zálohy paušálnej dane. [CZ-špecifické] (SK: porovnanie skutočných a paušálnych výdavkov 60 %, 15 % sadzba do limitu obratu, odvody?)
13. Daňové konštanty per rok (správca môže prepísať, pridať rok, vrátiť sa k predvoleným). [CZ-špecifické] (SK: vlastná sada konštánt – NČZD, sadzby 15/19/25 %, limity)
14. Právny profil organizácie v režime 504 a použitie daňovej úspory (neziskovky). [CZ-špecifické] (SK: neziskové ÚJ – § 12, § 13 ZDP)

---

## 11. Daňová evidencia (jednoduchá evidencia FO)

Podľa manuálu [proprietárne]; podľa LICENCE 1.12 je základná DE bezplatná.

1. Režim daňovej evidencie podľa § 7b ZDP per firma. [CZ-špecifické] (SK: jednoduché účtovníctvo / daňová evidencia podľa § 6 ods. 11 ZDP pre SZČO)
2. Peňažný denník: filtre, súhrn (PS, KS, sumy), tabuľka pohybov, daňová klasifikácia pohybov, nezaradené pohyby a varovania, poznámky a prílohy k pohybom, export. [všeobecné]
3. Kontrola voči priznanému príjmu. [všeobecné]
4. Pohľadávky a záväzky (filtre podľa meny, tabuľky). [všeobecné]
5. Ročná uzávierka DE (finalizácia, znovuotvorenie) a snapshot DPFO. [všeobecné]
6. Odhad dane a poistného počas roka. [CZ-špecifické] (SK: odvody SZČO a preddavky?)
7. Pravidlá nákladov a pravidlá bankových pohybov pre DE, aplikácia pravidiel. [všeobecné]
8. Dimenzie v DE, GoPay v DE (bez zápisov). [všeobecné]
9. Postupy: dobropis a zápočet, platobný kalendár energií, zálohy, bankové poplatky, prevody, vklady a výbery, hotovosť, majetok a odpisy. [všeobecné]
10. Prechodový mostík DE → PÚ (prílohy 2 a 3 ZDP) a sprievodca aktiváciou PÚ. [CZ-špecifické] (SK: prechod JÚ → PÚ?)

---

## 12. Majetok

Celý modul [proprietárne] (čl. 1.4 d).

1. Karty dlhodobého hmotného a nehmotného majetku: z prijatej faktúry, ručne, historický majetok prevzatý z iného programu. [všeobecné]
2. Zaradenie do užívania a zámky po zaradení a potvrdení. [všeobecné]
3. Plán odpisov: účtovný a daňový, hromadné zaúčtovanie ročných odpisov. [všeobecné]
4. Daňové odpisy podľa českých odpisových skupín, metód a § 26 ZDP. [CZ-špecifické] (SK: odpisové skupiny 0–6 a § 22–28 ZDP, rovnomerné a zrýchlené odpisovanie)
5. Prerušenie daňového odpisu (§ 26 ods. 8) a ručný prepis daňového odpisu roka. [CZ-špecifické] (SK: prerušenie podľa § 22 ods. 9?)
6. Technické zhodnotenie (§ 33). [CZ-špecifické] (SK: § 29 ZDP, limit 1 700 €)
7. Vyradenie, predaj (aj z vydanej faktúry), vrátenie vyradenia. [všeobecné]
8. Import a export kariet cez Excel, súhrnná karta z účtu. [všeobecné]
9. Majetok v daňovej evidencii. [všeobecné]
10. Drobný majetok: karty z prijatých faktúr (idempotentne, so zľavami a cudzou menou), vyradenie, predaj, vrátenie do užívania, inventúra, zostavy (inventár, pohyby, rozpad nákladov). [všeobecné]
11. Investičné zdroje a fondy (504): dokladovanie zdroja k majetku, rozpúšťanie 901/649 súčasne s odpismi. [CZ-špecifické] (SK: neziskové ÚJ – fondy, dotácie na obstaranie majetku 384/kapitálové fondy?)

---

## 13. Sklad a e-shop

Celý modul [proprietárne] (čl. 1.4 c).

### 13a. Sklad

1. Zapnutie skladu, viac skladov, lokácie v skladoch. [všeobecné]
2. Skladové karty (materiál, tovar, výrobky): hromadné založenie, hromadná úprava filtrovaných kariet, šablóny, duplikácia, životný cyklus, skladová kniha karty. [všeobecné]
3. Oceňovanie kĺzavým priemerom a ocenenie skladu k uzávierke. [všeobecné]
4. Príjemky, výdajky, prevodky so životným cyklom a číslovaním; editor dokladu; oprava stornom. [všeobecné]
5. Vedľajšie obstarávacie náklady (doprava, clo) rozpustené do ceny zásoby. [všeobecné]
6. Automatický výdaj pri vystavení faktúry a naskladnenie z prijatej faktúry. [všeobecné]
7. Počiatočný stav zásob (import) a prevzatie histórie skladu z iného programu. [všeobecné]
8. Inventúry s rozdielovými dokladmi a zaúčtovaním manka a prebytku; cyklické inventúry. [všeobecné]
9. Minimálne zásoby a návrh doobjednania (replenishment). [všeobecné]
10. Ponuky dodávateľov a import cenníka dodávateľa. [všeobecné]
11. Objednávky u dodávateľa: životný cyklus, PDF, príjem tovaru z objednávky, nedodaný zvyšok. [všeobecné]
12. Predajné objednávky s tvrdou rezerváciou; stavy skladom, rezervované, na ceste, u dodávateľa. [všeobecné]
13. Vychystanie, expedícia a vratky (fulfillment: úlohy, zásielky, skenovanie). [všeobecné]
14. Kompletácia výrobku (assemblies), virtuálne sety, balenie (packaging units). [všeobecné]
15. Šarže a sériové čísla (tracking units). [všeobecné]
16. Skladové zostavy a predaje skladových kariet. [všeobecné]
17. Intrastat export pre InstatEvo, údaje karty pre Intrastat. [CZ-špecifické] (SK: Intrastat-SK, ŠÚ SR)

### 13b. E-shop katalóg a cenotvorba

18. Číselníky: výrobcovia, kategórie v strome (presun, archivácia), atribúty a parametre (enum voľby), tagy, poplatky, meny, jazyky, baliace jednotky. [všeobecné]
19. Karta tovaru pre e-shop: viacjazyčné popisky, médiá, varianty (product masters a osi), vzťahy produktov. [všeobecné]
20. Import katalógu (XLSX/CSV) s predvoľbami ABRA Flexi a POHODA, profily importu. [všeobecné]
21. Cenotvorba: nákupná cenová báza, prirážka vs. marža, cenové profily, pravidlá, obchodné kurzy, cenová matica, zaokrúhlenie, cudzie meny, DPH a poplatky v cene, kontrola marže. [všeobecné]
22. Akciové ceny a výpredaje. [všeobecné]
23. Individuálne ceny zákazníkov a cenové hladiny s pravidlami; cenová hladina na doklade. [všeobecné]
24. Dodávatelia tovaru, preferovaný dodávateľ, dropshipping (tovar bez skladu). [všeobecné]
25. Integračné centrum: konektory, mapovanie číselníkov, vlastníctvo polí, prístupové údaje, webhooky, sťahovanie zmien katalógu, diagnostika a opakovanie, porovnanie úplnosti, outbox a inbox. [všeobecné]
26. Shoptet: import objednávok (súbor alebo automaticky), voľba, kto vystavuje doklady, objednávky na kontrolu DPH, vrátenie dávky, import dokladov, feed zásob a cien pre Shoptet. [CZ-špecifické] (SK: Shoptet.sk funguje aj v SK; ďalej WooCommerce, Shopify, Upgates?)
27. Asynchrónny export katalógu a dávkové čítanie cez API, efektívne ceny pre zákazníka. [všeobecné]
28. Prevzatie faktúr a dobropisov vystavených e-shopom (API). [všeobecné]

---

## 14. Mzdy a personalistika

V manuáli platený doplnok (mzdové pásma do 25, do 50 a neobmedzene zamestnancov); v čl. 1.4 nevymenované. Modul je v skúšobnej prevádzke a obsahuje 251 tabuliek (najväčší modul systému).

1. Zamestnanci: osobná karta, identita, výplatný účet, viac pracovných vzťahov a súbehov, druh vzťahu a predkontácia, životný cyklus vzťahu, checklist a časová os, personálny spis. [všeobecné]
2. Zákonná evidencia osoby: zdravotná poisťovňa, minimum ZP a výnimky, vymeriavací základ u iného zamestnávateľa, dôchodkový vek a dôchodok, povolenia cudzincov. [CZ-špecifické] (SK: ZP, SP, dôchodcovia, cudzinci – SK pravidlá)
3. Prehlásenie k dani a vyživované osoby s daňovým zvýhodnením na dieťa. [CZ-špecifické] (SK: vyhlásenie na uplatnenie NČZD a daňového bonusu)
4. Identifikátory ČSSZ (OIČ, ID PPV) a výzvy v dôchodkovom poistení. [CZ-špecifické] (SK: identifikátory SP – číslo zamestnávateľa, RČ?)
5. Skončenie vzťahu: spôsob a dôvod, nevyčerpaná a prečerpaná dovolenka, odstupné, úmrtie, zápočtový list, odložený príjem, hromadné ukončenie dohôd. [CZ-špecifické] (SK: Zákonník práce SR – odstupné, odchodné, potvrdenie o zamestnaní)
6. Absencie a dovolenka: nárok, čerpanie, náhrada, hromadný výpočet nároku, priemerný a pravdepodobný zárobok. [CZ-špecifické] (SK: priemerný zárobok podľa § 134 ZP SR)
7. Nemoc a karanténa (náhrada mzdy), prekážky v práci, materská, otcovská, ošetrovné. [CZ-špecifické] (SK: náhrada príjmu prvých 10 dní, ďalej SP; ePN)
8. Dochádzka a smeny: mesačná dochádzka, import z CSV/XLSX, nadčas a jeho limity, súhlas s nadčasom, náhradné voľno, zákonné príplatky (§ 114–118), pohotovosť (§ 140), pracovné kalendáre a sviatky. [CZ-špecifické] (SK: príplatky podľa §§ 121–123 ZP SR)
9. DPP a DPČ: rozsah a limity. [CZ-špecifické] (SK: dohody o prácach mimo PP – DoVP, DoPČ, DoBPŠ)
10. Cestovné náhrady: vyúčtovanie tuzemskej cesty, náhrady podľa vyhlášky, stravné a bezplatné jedlá, vyrovnanie zálohy, premietnutie do mzdy, zaúčtovanie. [CZ-špecifické] (SK: zákon 283/2002 Z. z., opatrenia MPSVR o stravnom)
11. Rýchly mesačný vstup: mzdy, nadčasy, odmeny a príplatky všetkým zamestnancom na jednej stránke, krátenie o absencie, prepis importovaných hodnôt. [všeobecné]
12. Mzdové zložky (katalóg) a vstupy: jednorazové, pravidelné predpisy, hromadné schválenie, import a export, klasifikácia zložky, zaradenie do JMHZ. [všeobecné] (JMHZ: CZ)
13. Povinné sporenie pri rizikovej práci. [CZ-špecifické] (SK: doplnkové dôchodkové sporenie pre rizikové práce – povinný príspevok?)
14. Mzdové behy: stavy, dátum výplaty, snímka a revízie, schvaľovanie vstupov, záporná čistá mzda, rozpad dane a čistej mzdy, obnova podkladov, oprava schváleného mesiaca, varovania a ich skrývanie. [všeobecné]
15. Legislatívne pravidlá miezd: verzované sady sadzieb, limitov a lehôt per rok (superadmin), kontrola pripravenosti sady na ďalší rok; bez sady sa ostrý beh odmietne. [CZ-špecifické] (SK: vlastná sada – odvody SP/ZP, minimálna mzda, NČZD, odvodová odpočítateľná položka…)
16. Zhoda účtovania miezd (mzdová revízia vs. denník vs. platobné záväzky). [všeobecné] [proprietárne]
17. Mzdové príkazy a úhrady: čisté mzdy, dane, poistné, zrážky; formát dávky, priame odovzdanie do banky, rozpoznanie zaplatených odvodov, párovanie, vratky. [CZ-špecifické] (SK: SEPA dávka; VS pre SP/ZP/FS)
18. Dokumenty a výstupy: výplatné pásky (bezpečné doručenie cez odkaz s kódom), mzdový list, potvrdenie o zdaniteľných príjmoch, mzdový výmer, dokumenty pri skončení, potvrdenie pre dôchodkové poistenie, šifrovaný archív miezd. [CZ-špecifické] (SK: výplatná páska, mzdový list, potvrdenie o zdaniteľnej mzde, potvrdenie o zamestnaní)
19. Ročné zúčtovanie preddavkov a daňového zvýhodnenia: jednotlivo aj hromadne, potvrdenie od iného platiteľa, preplatok a nedoplatok, vyúčtovanie dane FÚ. [CZ-špecifické] (SK: ročné zúčtovanie preddavkov + Hlásenie o vyúčtovaní dane)
20. Podania a hlásenia: JMHZ (jednotné mesačné hlásenie zamestnávateľa) so zmrazením, odoslaním, protokolom, opravou a stornom; prehľady pre ZP; nástup a odchod (REGZEC, PREZEC, A2 odhláška, A3 zmeny); NEMPRI, HZUPN, OZUSPOJ; ELDP; REGZEL; vyúčtovanie zálohovej a zrážkovej dane; žiadosť o daňový bonus; fronta „Na odoslanie“ a zákonné termíny. [CZ-špecifické] (SK: Sociálna poisťovňa – RLFO registrácia, Mesačný výkaz poistného, ELDP; ZP – mesačný výkaz preddavkov, oznámenie zmien; FS SR – Prehľad o zrazených preddavkoch, Hlásenie)
21. Kanály podania: ISDS a VREP (zmocnenie a registrácia certifikátu), ručné potvrdenie, „podané mimo aplikácie“, polling výsledkov (cron). [CZ-špecifické] (SK: e-Podateľňa SP, ePobočka, portály ZP, eDane)
22. Zrážky zo mzdy: dohody o zrážkach (právny titul, poradie, limit). [všeobecné]
23. Exekúcie a insolvencia: prípady, stavy, výpočet nezabaviteľnej sumy, poradie, depozitum, platobné inštrukcie, súčinnosť exekútorom (XMLZAM), skončenie PP povinného. [CZ-špecifické] (SK: Exekučný poriadok, nariadenie vlády o nepostihnuteľnej sume, osobný bankrot)
24. Koše benefitov: limity oslobodenia (zdravotné plnenie, rekreácia, sporenie na staroba, stravovanie, ubytovanie), sprepitné. [CZ-špecifické] (SK: oslobodenia § 5 ods. 7 ZDP, rekreačný príspevok, stravovanie)
25. Nastavenia miezd: zamestnávateľ, mzdové účtárne, VS ČSSZ, účty inštitúcií, zákonné poistenie zodpovednosti, automatické účtovanie, politiky a pripravenosť, dimenzie. [CZ-špecifické] (SK: Kooperatíva – zákonné poistenie? SK: úrazové poistenie je v SP)
26. Importy: zamestnanci z JMHZ a registrácií, export z ePortálu ČSSZ, dochádzka, OIČ z POHODY, prevzatie miezd v priebehu roka (ručne alebo súborom, počiatočné stavy, kontrola prevzatia). [CZ-špecifické] (SK: import z Olymp/Kros, Pohoda Mzdy SK?)
27. Retenčné lehoty mzdových dát, legal hold a výmaz alebo anonymizácia osobných údajov (GDPR). [všeobecné]
28. Ročná uzávierka miezd. [všeobecné]
29. Prevádzkový prehľad miezd a sprievodca prvým nastavením. [všeobecné]
30. Priradenie mzdových nákladov k dimenziám. [všeobecné]

---

## 15. Kniha jázd

1. Vozidlá: vodič, režim užívania, odpočet DPH. [všeobecné]
2. Jazdy: zápis, pravidelné cesty (šablóny), kategórie ciest, doplnenie km podľa tachometra, prepočet stavu tachometra, odhad a upozornenia na medzery. [všeobecné]
3. Import a export jázd z CSV/XLSX. [všeobecné]
4. Tankovanie a nabíjanie: ručne, z faktúr čerpacích staníc, z pokladničných dokladov, zo skenov účteniek, import zo súboru. [všeobecné]
5. Automatický stav tachometra z externého zdroja (napr. API výrobcu auta, cron). [všeobecné]
6. Ročné daňové súhrny (uznanie nákladov na PHM). [CZ-špecifické] (SK: kniha jázd ako podklad pre výdavky; paušálne výdavky na PHM 80 %?)

---

## 16. Dokumenty

1. Archív súborov so zložkami, tagmi a väzbami na faktúry, klientov a zákazky. [všeobecné]
2. Náhľady a fulltextové vyhľadávanie (aj vyťažený text a embeddingy). [všeobecné]
3. ZIP súbory a dátové správy ZFO. [CZ-špecifické] (SK: ZEP/ASiC kontajnery zo slovensko.sk)
4. Firemné vs. osobné dokumenty, oprávnenia, kôš a obnova. [všeobecné]
5. Požiadavky na chýbajúce podklady od klienta so stavmi a automatickými pripomienkami (cron). [všeobecné]
6. Prílohy účtovných zápisov v oddelenom úložisku. [všeobecné]
7. Zálohovanie dokumentov (cron). [všeobecné]
8. Dokumentové zložky k podaniam sa zakladajú automaticky. [všeobecné]

---

## 17. Neziskové organizácie, SVJ, spolky (rámec 504)

Mimoriadne rozsiahla časť (35 tabuliek nonprofit_*, 39 property_*, 33 unit_*). Pre SK by bola samostatným modulom.

1. Registr domov a jednotiek SVJ: domy, jednotky, podiely, vlastnícke účasti s viacerými osobami, služby na rozúčtovanie, časová účinnosť a história podkladov, potvrdzovanie zmien, import a export. [CZ-špecifické] (SK: spoločenstvo vlastníkov bytov a NP / správca podľa zákona 182/1993 Z. z.)
2. Predpisy záloh (zložky, verzie predpisov), vydanie a zaúčtovanie predpisov. [CZ-špecifické] (SK: predpis úhrad za plnenia a fond prevádzky, údržby a opráv)
3. Platby jednotiek: dlhy a príslušenstvo, rozpis platby, párovanie z banky, kredity, zápočet, rezervácie, vratky (ABO), opravy, storná. [CZ-špecifické] (SK: rovnaké potreby, SEPA namiesto ABO)
4. Saldo osôb, prechod dlhu a ručenie, nemenné potvrdenie o dlhu. [CZ-špecifické] (SK?)
5. Prevzatie počiatočných stavov SVJ. [všeobecné]
6. Vyúčtovanie služieb SVJ: dossier, príjemcovia, doručenie, lehoty a námietky, dohady a konečné náklady služieb, kontrola zhody salda, daňová predkontrola. [CZ-špecifické] (SK: vyúčtovanie zálohových platieb za plnenia)
7. Zálohy na správu a príjmy vlastníkov (čerpanie, dispozície). [CZ-špecifické] (SK: fond opráv)
8. Granty a účelové dary: zdroj, účel, rozpočet, skutočné použitie v daňovom posúdení, revízie. [CZ-špecifické] (SK: dotácie a dary v NO; 2 % podiel zaplatenej dane je SK špecifikum navyše)
9. Investičné zdroje a fondy (901/911). [CZ-špecifické] (SK: fondy NO podľa opatrenia 24342/2007?)
10. Členovia a členské príspevky spolku: revízie členstva, predpis príspevku, úhrady, kontrola zostatku. [CZ-špecifické] (SK: občianske združenie)
11. Daňové posúdenie neziskovky (DPPO 504): právny profil, druhy činností, daňové režimy, návrhy a potvrdenie, stránkované výpočty. [CZ-špecifické] (SK: § 12–13 ZDP pre neziskové ÚJ)
12. Kapitál neziskovky, cenné papiere a dlhy (504), spoločné alokácie nákladov medzi činnosti. [CZ-špecifické] (SK?)

---

## 18. EET 2.0 (evidencia tržieb)

1. Profil EET, evidenčné jednotky, pokladničné zariadenia, pokladničné certifikáty (vydanie, výmena, expirácia), diagnostika a aktivácia. [CZ-špecifické] (SK: eKasa – ORP/VRP, autentifikačné údaje a certifikát z eKasa portálu FS SR)
2. Prijatie platby pri faktúre (aj v cudzej mene), poukazy a kredit, QR platba na mieste, okamžité prijatie pri vystavení. [CZ-špecifické] (SK: eKasa)
3. Rýchla účtenka a účtenka pre zákazníka (verejný odkaz). [CZ-špecifické] (SK: doklad eKasa s UID/OKP)
4. Evidencia, fronta odoslania (cron), opakovanie, opravy a vratky (k dobropisu), export. [CZ-špecifické] (SK: eKasa)
5. Napojenie e-shopu, ktorý eviduje EET sám. [CZ-špecifické] (SK: eKasa sa na e-shop pri platbe kartou online nevzťahuje?)

---

## 19. Import / export formáty a migrácie z iných systémov

1. Export vydaných: PDF ZIP po mesiacoch (`<VS>-<typ>.pdf`), jedno zlúčené PDF. [všeobecné]
2. Export ISDOC 6.0.2 (validovaný proti XSD) vrátane meny a kurzu, aj ako príloha v PDF. [CZ-špecifické] (SK: UBL 2.1 / Peppol BIS 3.0 – od 2027 povinná e-faktúra cez IS EFA)
3. Export Pohoda XML (Stormware data package) s kódmi per firma a klasifikáciou DPH. [CZ-špecifické] (SK: Pohoda SK XML)
4. Export Stereo XML a Money S3 XML. [CZ-špecifické] (SK: Money S3 SK, Omega/Kros, Alfa?)
5. Export CSV a filtrovanie podľa obdobia, typu a stavu. [všeobecné]
6. Priamy prenos do Fakturoidu. [CZ-špecifické] (SK: SuperFaktúra API?)
7. Import vydaných a prijatých zo súboru: Pohoda XML, ISDOC, ISDOCX, PDF/A-3; pravidlo 30 dní pre stav, report importu. [CZ-špecifické] (SK: Pohoda SK XML, UBL)
8. API import z iDokladu a Fakturoidu (idempotentný, s platobným stavom a klasifikáciou DPH). [CZ-špecifické] (SK: SuperFaktúra, iKros, Kros Fakturácia API?)
9. Prevod celej agendy zo Stereo NX (zálohy), s mapovaním účtov a krajín partnerov. [CZ-špecifické] (SK?)
10. Prechod z Money S3: záloha agendy, skúška nanečisto, ostrý prevod, rekonciliácia, protokol, napojenie číselného radu, dávkový prevod viacerých firiem; majetok, mzdy. [CZ-špecifické] (SK: Money S3 SK)
11. Prechod z POHODY (XML, MDB cez lokálny prevodník, POHODA SQL). [CZ-špecifické] (SK: Pohoda SK)
12. Prechod z PAMICA (mzdy). [CZ-špecifické] (SK: Olymp, Kros mzdy, Pohoda Mzdy SK?)
13. Prechod z PREMIER. [CZ-špecifické] (SK: Omega, Alfa, Olymp – KROS?)
14. Prechod z ABRA Flexi cez API (faktúry, DPH, denník, úhrady, banka a pokladnica, produkty a sklad, OSS, viac období, inkrementálne načítanie). [všeobecné] (ABRA Flexi existuje aj v SK)
15. Prechod z MyInvoice (CLI, preflight bez straty dát). [všeobecné]
16. Kompletný export dát firmy a import firmy z tohto exportu (presun medzi inštaláciami). [všeobecné]
17. Hromadný export (ZIP) podkladov za mesiac alebo štvrťrok na pozadí. [všeobecné]
18. Číselníky, kurzové lístky ČNB (cron) a ECB. [CZ-špecifické] (SK: ECB kurzy)
19. Chunked upload veľkých súborov a importy na pozadí so zrušením. [všeobecné]

---

## 20. Integrácie a externé služby

1. ARES, CRP DPH (register plátců), VIES. [CZ-špecifické] (SK: RPO/ORSR/FinStat, zoznamy FS SR; VIES rovnaké)
2. Kurzy ČNB, repo sadzba ČNB, ECB kurzy. [CZ-špecifické] (SK: ECB)
3. Priame bankové API (7 CZ bánk), e-mailové avízá IMAP. [CZ-špecifické] (SK: PSD2 API SK bánk)
4. GoPay, Shoptet, iDoklad, Fakturoid, ABRA Flexi. [CZ-špecifické] (SK: GoPay/Besteron, Shoptet.sk, SuperFaktúra)
5. AI poskytovatelia (Anthropic, Azure OpenAI, OpenAI, Gemini, Ollama). [všeobecné]
6. EPO (Finančná správa), ISDS, VREP (ČSSZ), EET. [CZ-špecifické] (SK: eDane/FS SR, slovensko.sk, SP e-Podateľňa, eKasa)
7. SMTP s DKIM, odosielacie profily, schválené SMTP ciele inštancie, analýza SMTP logu, mail outbox s rate limitom. [všeobecné]
8. Webhooky (verejný endpoint integrácií), integračný outbox a inbox. [všeobecné]
9. Zdroje stavu tachometra (API automobiliek). [všeobecné]
10. Mobilné aplikácie a pokladnice cez API (rýchla účtenka, EET). [všeobecné]

---

## 21. Používatelia, práva, multi-firma

1. Multi-firma: prepínač firmy, založenie firmy (prevzatie AI kľúča a certifikátu z existujúcich firiem), základné údaje a režim účtovania per firma, číslovanie, branding, kópie e-mailov, Pohoda kódy, zmazanie firmy. [všeobecné]
2. Čo je izolované per firma a čo zdieľané (globálne číselníky). [všeobecné]
3. Prehľad firiem pre účtovnú kanceláriu: urgencia termínov, počty nezaúčtovaných dokladov a nespárovaných platieb, posledný import banky, pás akútnych kontrol. [všeobecné] [proprietárne podľa manuálu]
4. Role: Superadmin, Admin, Admin Plus, Správca inštancie (technický, bez prístupu k agendám), Iba na čítanie, vlastné roly typu staff a client. [všeobecné]
5. Matica oprávnení po moduloch a akciách s troma úrovňami: Neviditeľné / Čítanie / Zápis. Role sa dajú priradiť per firma. [všeobecné]
6. Oddelené oprávnenia na nastavenia firmy (odosielacie profily, e-mailové šablóny, branding, platobné QR, podpisové profily). [všeobecné]
7. Klientsky portál: KPI, mesačný graf, cash-flow, DPH a daňové termíny, pás „Účtovníčka čaká na doklady“, rýchle akcie, vystavenie faktúry klientom, podateľňa dokladov, odpovede na požiadavky, delegované nastavenie firmy, zámok zaúčtovaných dokladov, náhľad portálu pre účtovníčku. [všeobecné]
8. Vlastná doména klientskeho portálu (overenie domény, prihlásenie vrátane passkeys na vlastnej doméne). [všeobecné]
9. Autentifikácia: heslo (bcrypt + pepper, min. 12 znakov), passkeys (WebAuthn), TOTP, záložné kódy, e-mailový druhý faktor, vynútenie silného MFA, step-up pri citlivých zmenách. [všeobecné]
10. Zámok aplikácie (okamžite aj po nečinnosti), serverový zámok session, dôveryhodné zariadenia. [všeobecné]
11. Brute-force ochrana, CAPTCHA, CSRF + kontrola Origin, IP allowlist (IPv4/IPv6/CIDR, trusted proxies). [všeobecné]
12. Activity log (čo neukladá, ako zapisuje IP), log volaní API a MCP. [všeobecné]
13. Schvaľovania (admin/approvals). [všeobecné]
14. Užívateľské preferencie, obľúbené položky, uložené filtre, štýl menu, panely. [všeobecné]
15. Licencia a aktivácia: 60-dňová skúšobná doba, licenčný kľúč, tarify podľa počtu firiem, platba za aktívneho používateľa a mesiac, mzdové pásmo, kvóty, nákup a upgrade v aplikácii, prenos na inú inštaláciu, obnova (cron). [všeobecné] (pre vlastný produkt iba inšpirácia)

---

## 22. Nastavenia (firma a systém)

1. Údaje firmy a profil firmy (NACE kódy, kategória účtovnej jednotky, história DPH statusu a daňového zastúpenia). [CZ-špecifické] (SK: SK NACE, veľkostná kategória ÚJ podľa § 2 ZoÚ SR)
2. Číselníky: meny, krajiny, merné jednotky, sadzby DPH, klasifikácie DPH, štátne sviatky, kategórie nákladov a tržieb. [všeobecné] (sadzby a sviatky: SK)
3. E-mailové šablóny (systémové aj per firma) a odosielacie profily, diagnostika odoslaných e-mailov, analýza SMTP logu. [všeobecné]
4. Brandingové profily (PDF, e-maily). [všeobecné]
5. Nastavenie automatického účtovania (politiky auto-postingu). [všeobecné] [proprietárne]
6. AI nastavenia (poskytovateľ, kľúče, DPA, tuning). [všeobecné]
7. Bankové spojenia a e-mailové avízá. [všeobecné]
8. Podpisové certifikáty a profily, PDF podpisovanie. [všeobecné]
9. Datová schránka a ISDS brána. [CZ-špecifické] (SK: slovensko.sk)
10. Prepínače modulov: účtovníctvo, mzdy, sklad, OSS; náhľad zmeny režimu (DE ↔ PÚ); riadený prevod rámca 500 → 504. [všeobecné]
11. Systém: daňové konštanty, šablóny bankových pravidiel, cron joby (posledný beh, trvanie, chyby, dispatch), diagnostika (balíček, štruktúra DB, logy), aktualizácie (watcher, tlačidlo Aktualizovať), podpora, hosting. [všeobecné]
12. Ukážkové dáta (generovanie a odstránenie) a setup wizard (kontrola prostredia, administrátor, prvá firma s ARES lookupom). [všeobecné]
13. Anonymizovaná kópia databázy pre testovaciu inštanciu. [všeobecné]

---

## 23. Zálohy a prevádzka

1. Zálohy DB, PDF, dokumentov, mzdového a personálneho archívu (samostatné crony), voliteľne šifrované; stiahnutie záloh z aplikácie; upratovanie záloh a logov. [všeobecné]
2. Export celej inštancie (instance export) a obnova zo servera. [všeobecné]
3. Retenčný cron (výmaz po lehote). [všeobecné]
4. Kontrola invariantov a schémy DB (diagnostika podľa snapshotu štruktúry). [všeobecné]
5. Sledovanie využitia úložiska (storage usage). [všeobecné]
6. Kontrola novej verzie a samoaktualizácia (Docker watcher, natívne). [všeobecné]
7. Plánované úlohy (40 cron skriptov): ai-worker, ai-rule-miner, automation-digest, backup (5×), bank-connections, bank-email-notices, bank-scan, catalog-worker, cleanup, cnb-rates, dispatch, document-request-reminders, eet-dispatch, epo-status, generate-other-items, generate-recurring-invoices, jmhz-poll, jmhz-source-monitor, journal-integrity-check, license-renew, logbook-odometer-source, payment-thanks, payroll-document-worker, payroll-period-export-worker, payroll-post, payroll-registration-changes, purchase-approval-reminders, retention, scan-purchase-inbox, send-approval-reminders, send-reminders, shoptet-orders, storage-usage, vat-clearing, vat-status-apply, version-check. [všeobecné]

---

## 24. API a MCP

1. Verejné REST API v1 (OpenAPI 3.1, Scalar/Redoc dokumentácia v aplikácii), osobné tokeny so scope `read` / `read_write`, obmedzenie na firmu, platnosť, IP obmedzenie, rate limity, log volaní, výber firmy hlavičkou `X-Supplier-Id`. [všeobecné]
2. Účtovná a daňová vrstva cez API iba na čítanie (`403 token_write_forbidden`). Výnimky: koncepty ostatných pohľadávok a záväzkov a analytické účty. Mzdy cez API nie. [všeobecné]
3. Rozsah API: faktúry (aj export a číslovanie), prijaté faktúry, klienti, zákazky, opakované faktúry, banka, dokumenty, kniha jázd, e-shop a sklad (dávkové čítanie, efektívne ceny, asynchrónny export), dimenzie, účtovný kontext k dátumu, reporty, SVJ a neziskovky (čítanie), EET a rýchla účtenka. [všeobecné]
4. MCP server (asi 280 nástrojov): lokálny (Node `.mjs`) aj vzdialený s OAuth (dynamická registrácia klienta, `.well-known`), pre Claude, ChatGPT, Codex, Gemini a Copilot. [všeobecné]
5. Rozsah MCP: fakturácia (čítanie, vystavenie, odoslanie, úhrady, upomienky), výkazy práce, zákazky, dokumenty (fulltext), kniha jázd, pohľadávky a záväzky, dane (čítanie), účtovníctvo (čítanie), SVJ (čítanie), dimenzie, štatistiky, skupina firiem, e-shop a sklad (čítanie aj zápis, deštruktívne akcie s `confirm: true`), mzdy (obmedzene), globálne hľadanie, súbory (upload a download). [všeobecné]
6. Ochrana proti duplicitám a potvrdzovanie nevratných krokov v MCP. [všeobecné]

---

## 25. Reporty a dashboard (súhrn)

1. Dashboard „Prehľad“ s frontou „Akcie pre teba“ a daňovým kalendárom. [všeobecné]
2. Zisk, Tržby, Náklady, Dimenzie, Skupina firiem (portfolio). [všeobecné]
3. CRM analytika (DSO, DPO, churn, koncentrácia, cash-flow forecast). [všeobecné]
4. Účtovné výkazy (predvaha, hlavná kniha, súvaha, VZaS druhová a účelová, saldokonto, peňažné toky, vlastné imanie, inventarizácia, drobný majetok). [všeobecné] (formáty výkazov: SK) [proprietárne]
5. Daňové výkazy (DPH, KH, SH, OSS, kniha DPH, koeficient, opravy, DPFO/DPPO). [CZ-špecifické] (SK ekvivalenty pozri moduly 6 a 10)
6. Mesačný report pre klienta (PDF, e-mail, história). [všeobecné]
7. Kontrolné zostavy: úplnosť dokladov, úplnosť číselných radov, audit kurzov, mesačná kontrola, súbeh so starým systémom. [všeobecné]
8. Mzdové reporty a zostavy. [všeobecné]
9. Skladové zostavy a Intrastat. [všeobecné] / [CZ-špecifické]

---

## Dátový model (hlavné entity a tabuľky)

Spolu **803 tabuliek** (z `db/schema.snapshot.json`). Rozdelenie podľa prefixu: payroll 251, stock 42, property 39, nonprofit 35, unit 33, bank 27, accounting 26, tax 20, journal 14, submission 13, purchase 12, document 11, supplier 11, vat 10, product 9, ai 8, invoice 8, eet 7, fulfillment 7, sales 7 a ďalšie.

Jadro:
- `supplier` (+ `supplier_bank_accounts`, `supplier_accounting_modes`, `supplier_accounting_frameworks`, `supplier_vat_status_history`, `supplier_tax_representation_history`, `supplier_domains`, `supplier_email_templates`, `supplier_groups`): firma (účtovná jednotka) s režimom účtovania, rámcom, históriou DPH statusu a vlastnými doménami.
- `users`, `user_suppliers`, `roles`, `role_permissions`, `user_preferences`, `user_favorites`, `saved_filters`: používatelia, priradenie k firmám a RBAC.
- `sessions`, `login_attempts`, `login_otps`, `password_resets`, `webauthn_credentials`, `webauthn_ceremonies`, `mfa_recovery_codes`, `mfa_step_up_proofs`, `totp_used_steps`, `trusted_devices`: autentifikácia a MFA.
- `api_tokens`, `api_token_ips`, `api_request_log`, `mcp_oauth_*`, `oauth_sessions`: API tokeny, OAuth pre MCP, logy.
- `activity_log`, `activity_log_chain_head`: auditná stopa s hash reťazcom.
- `clients`, `client_bank_accounts`, `client_email_contacts`, `client_revenue_cache`: adresár odberateľov a dodávateľov.
- `projects`, `project_billing_emails`, `project_revenue_cache`: zákazky.

Predaj a nákup:
- `invoices`, `invoice_items`, `invoice_payments`, `invoice_pdfs`, `invoice_attachments`, `invoice_counters`, `invoice_settlements`, `invoice_advance_deductions`, `invoice_payment_schedule`: vydané doklady, položky, úhrady, nemenné PDF, číselné rady, čerpanie záloh.
- `credit_note_offsets`: zápočet dobropisov.
- `recurring_invoice_templates`, `recurring_invoice_template_items`: opakované fakturácie.
- `work_reports`, `work_report_items`, `work_report_materials`, `work_report_links*`: výkazy práce a materiálu a ich verejné odkazy.
- `price_list_items`, `price_list_item_prices`, `price_list_customer_overrides`: cenník služieb.
- `purchase_invoices`, `purchase_invoice_items`, `purchase_invoice_vat_allocations`, `purchase_invoice_counters`, `purchase_advance_deductions`, `purchase_invoice_approvals`, `purchase_invoice_submissions`, `purchase_invoice_inbox_dismissed`: prijaté doklady, schvaľovanie, podateľňa z portálu.
- `payment_orders`, `payment_order_items`, `bank_payment_order_submissions`: platobné príkazy.
- `revenue_categories`, `expense_categories`, `expense_classification_rules`, `expense_keyword_catalog`: kategórie a klasifikačné pravidlá.

Peniaze:
- `bank_statements`, `bank_transactions`, `bank_transaction_imports`, `bank_match_suggestions`, `bank_match_audit`, `payment_matches`, `payment_events`, `payment_event_allocations`, `bank_transfer_matches`: výpisy, pohyby, párovanie.
- `bank_connections`, `bank_oauth_*`, `bank_api_months`, `bank_connector_cooldowns`: priame napojenie bánk.
- `bank_email_*`, `remittance_map`, `bank_notice_ignore_transfers`: e-mailové avízá.
- `bank_posting_rules`, `bank_posting_suggestions`, `bank_rule_templates`, `bank_counterparty_map`, `bank_counterparty_observations`: pravidlá účtovania banky.
- `cash_registers`, `cash_documents`, `cash_document_vat_lines`: pokladnice.
- `payment_cards`, `card_clearing_settings`, `credit_card_accounts`, `credit_card_settings`: platobné a kreditné karty.
- `gopay_settings`, `gopay_clearings`, `gopay_movements`: GoPay.
- `exchange_rates`, `ecb_exchange_rates`, `ecb_exchange_rate_days`, `cnb_repo_rates`, `currencies`, `countries`: kurzy a číselníky.

Účtovníctvo:
- `chart_of_accounts`, `accounting_account_definition_history`: účtový rozvrh.
- `journal_entries`, `journal_entry_lines`, `journal_entry_line_dimensions`, `journal_entry_line_dimension_splits`, `journal_entry_attachments`, `journal_entry_notes`, `journal_entry_document_links`, `journal_document_postings`, `journal_integrity_findings`, `journal_line_pairings`, `journal_settlement_allocations`: účtovný denník a jeho väzby.
- `journal_entry_templates`, `journal_entry_template_lines`, `posting_rules`, `auto_posting_policy`, `automation_recommendation_*`: šablóny, predkontácie, automat.
- `accounting_periods`, `accounting_closing_steps`, `accounting_opening_balances`, `accounting_receivable_provisions`, `accounting_balance_inventory*`, `accounting_archives`, `accounting_corrections`, `accounting_document_series`, `accounting_fixed_exchange_rates`: obdobia, uzávierka, inventarizácia.
- `accounting_setup_runs`, `accounting_setup_proposals`, `accounting_setup_rule_bundles`, `accounting_backfill_jobs`, `accounting_reclassification_items`: asistent nastavenia a doúčtovanie.
- `accounting_framework_conversion*`: prevod rámca 500 → 504.
- `statement_rows`, `statement_versions`, `statement_account_map`, `statement_account_overrides`, `statement_function_map`, `statement_notes`, `entity_category_history`: definície výkazov a ich mapovanie.
- `other_items`, `other_item_*`: ostatné pohľadávky a záväzky (splátky, opakovanie).
- `internal_documents`, `internal_document_lines`: interné doklady.
- `offset_agreements`, `offset_agreement_items`: zápočty.
- `cost_centers`, `dimension_types`, `dimension_values`, `dimension_defaults`, `dimension_account_map`, `dimension_account_rules`, `document_dimensions`, `document_dimension_splits`: strediská a dimenzie.
- `joint_allocation_*`: spoločné alokácie nákladov.
- `migration_parallel_checks`, `migration_batch_items`: súbeh so starým systémom.

Dane:
- `vat_rates`, `vat_classifications`, `vat_coefficients`, `vat_clearing_runs`, `vat_deduction_adjustments`, `vat_deduction_adjustment_years`, `vat_registration_corrections`, `vat_s43_corrections`, `vat_s46_corrections`, `vat_s74b_corrections`: DPH a jej opravy.
- `oss_member_state_rates`, `oss_filing_evidence`: OSS.
- `vies_cache`, `ares_cache`, `crpdph_cache`: cache registrov.
- `tax_submissions`, `tax_submission_artifacts`, `tax_submission_attempts`, `tax_submission_status_events`, `tax_submission_settings`, `epo_signing_credentials*`: archív podaní a EPO.
- `submission_outbox*`, `submission_inbox_*`, `submission_recipients`, `submission_defect_notices`, `submission_channel_credentials`, `submission_isds_*`, `isds_gateway_*`: datová schránka.
- `income_tax_returns`, `income_tax_return_snapshots`, `income_tax_finalization_overrides`, `tax_advance_schedules`, `tax_advance_overrides`, `tax_losses`, `tax_loss_applications`, `tax_unpaid_liability_addbacks`, `tax_related_party_adjustments`: daň z príjmov.
- `tax_profiles`, `tax_profile_children`, `tax_profile_child_months`, `tax_profile_spouse_claims`, `tax_profile_activities`: daňový profil FO.
- `tax_constants`: daňové konštanty per rok.
- `tax_evidence_closings`, `tax_evidence_bank_rules`, `tax_evidence_non_cash_adjustments`, `de_movement_classification*`, `cash_journal_notes`, `supplier_osvc_month_statuses`: daňová evidencia.

Majetok:
- `assets`, `asset_improvements`, `depreciation_entries`, `small_assets`: dlhodobý a drobný majetok, odpisy, technické zhodnotenie.

Sklad a e-shop:
- `warehouses`, `warehouse_locations`, `stock_items`, `stock_levels`, `stock_documents`, `stock_document_lines`, `stock_landed_costs`, `stock_valuation_rows`, `stock_valuation_snapshots`, `stock_takes*`, `stock_cycle_count*`, `stock_tracking_units`, `stock_tracking_allocations`: sklad.
- `stock_categories`, `stock_attributes*`, `stock_tags`, `manufacturers`, `stock_fee_types`, `stock_locales`, `stock_media`, `stock_packaging_units`, `*_i18n`: katalóg.
- `stock_item_prices`, `stock_item_promo_prices`, `stock_item_customer_prices`, `stock_price_levels`, `stock_price_level_rules`, `stock_pricing_profiles`, `stock_pricing_rules`, `stock_pricing_exchange_rates`, `stock_item_vendors`: cenotvorba.
- `product_masters`, `product_variants`, `product_sets`, `product_assemblies`: varianty, sety, kompletácia.
- `purchase_orders`, `purchase_order_lines`, `sales_orders`, `sales_order_lines`, `sales_order_reservations`, `sales_order_returns`, `fulfillment_*`: objednávky, rezervácie, expedícia.
- `integration_connections`, `integration_outbox`, `integration_inbox`, `integration_change_log`, `external_entity_map`, `catalog_jobs*`, `catalog_import_*`, `shoptet_settings`, `shoptet_orders`, `shoptet_import_batches`: integrácie e-shopov.

Ostatné agendy:
- `cars`, `trips`, `trip_templates`, `trip_categories`, `fuelings`, `odometer_readings`, `odometer_source_*`, `logbook_fuel_scans`: kniha jázd.
- `documents`, `document_files`, `document_folders`, `document_tags`, `document_tag_map`, `document_links`, `document_requests`, `document_extractions`, `document_embeddings`, `document_dms_messages`: DMS.
- `scan_batch_items`, `scan_matches`, `attachment_checks`: pripojenie skenov.
- `ai_jobs`, `ai_suggestions`, `ai_metrics`, `ai_daily_usage`, `ai_embeddings`, `ai_source_mutes`, `ai_assistant_conversations`, `ai_assistant_messages`, `client_ai_extraction_usage`: AI.
- `eet_settings*`, `eet_units`, `eet_devices`, `eet_certificates`, `eet_records`, `eet_attempts`, `quick_receipts`, `receipt_links`: EET a účtenky.
- `signing_credentials`, `signing_profiles`, `signing_settings`, `signature_*`, `pdf_signature_output_settings`: elektronické podpisy.
- `email_templates`, `email_profiles`, `mail_outbox`, `mail_send_log`, `mail_rate_limit_events`, `instance_smtp_*`: e-maily.
- `branding_profiles`, `public_holidays`, `countries`, `currencies`: číselníky.
- `cron_runs`, `cron_settings`, `cron_heartbeat`, `cron_dispatch_claims`, `import_jobs`, `instance_exports`, `instance_storage_usage`, `backup_schedule_contract`, `license`, `app_meta`, `migrations`, `sample_data_entries`: prevádzka.
- `pohoda_imports`, `money_s3_imports`, `premier_imports`, `abra_flexi_*`, `stereo_nx_import_map` (+ `*_import_map`): migrácie z iných systémov.
- `crm_monthly_summary`, `crm_action_item_dismissals`: CRM cache.
- `monthly_report_sends`: odoslané mesačné reporty.
- `retention_holds`: legal hold.

Mzdy (251 tabuliek `payroll_*`): osoby a identity (`payroll_employees`, `payroll_person_*`), pracovné vzťahy (`payroll_employments`, `payroll_employment_*`), behy (`payroll_runs`, `payroll_run_*`, `payroll_net_results`, `payroll_monthly_records`), vstupy a zložky (`payroll_inputs`, `payroll_component_definitions`, `payroll_recurring_components`), dochádzka (`payroll_time_*`, `payroll_shifts`, `payroll_work_calendars`, `payroll_overtime_*`), absencie a PN (`payroll_absences`, `payroll_sickness_*`, `payroll_leave_*`), cestovné (`payroll_business_trips*`), zrážky a exekúcie (`payroll_deduction_*`, `payroll_enforcement_*`, `payroll_insolvency_*`), platby (`payroll_payment_*`, `payroll_obligations`), zaúčtovanie (`payroll_posting_*`), dokumenty (`payroll_document_*`, `payroll_generated_documents`, `payroll_annual_document_*`), ročné zúčtovanie (`payroll_annual_settlement_*`), podania (`payroll_submission_*`, `payroll_jmhz_*`, `payroll_registration_*`, `payroll_eldp_*`, `payroll_regzel_*`), legislatíva (`payroll_rulesets`, `payroll_ruleset_audit`), retencia a výmaz (`payroll_retention_policies`, `payroll_erasure_*`), prevzatie (`payroll_takeover_*`, `payroll_import_*`).

Neziskovky a SVJ: `nonprofit_*` (35; členovia, príspevky, kapitál, granty, daňové posúdenie), `property_*` (39; registre domov, revízie, vyúčtovanie služieb, počiatočné stavy), `unit_*` (33; predpisy, platby, dlhy, kredity), `accounting_securities_debt_*` (cenné papiere a dlhy 504).

---

## Rozsah (orientačné počty)

| Položka | Počet |
|---|---|
| Kapitoly manuálu | 121 (asi 4 MB Markdownu, asi 2 650 nadpisov) |
| Frontend: definície ciest v routeri | asi 312 (vrátane verejných stránok: schválenie, web faktúra, výkaz, výplatná páska, OAuth) |
| Frontend: stránkové komponenty (`pages` + `views`) | asi 360 (347 v `pages`, 14 v `views`); spolu 831 `.vue` súborov |
| Interné API routy (Slim) | asi 2 340 (GET 983, POST 914, PUT 207, DELETE 210, PATCH 21); najväčšie skupiny: accounting 470, payroll 236, settings 184, admin 158, eshop 150, stock 141, property 107, reports 88 |
| Verejné REST API v1 (OpenAPI) | asi 977 ciest (accounting 296, stock 107, eshop 80, reports 65, invoices 53…) |
| MCP nástroje | asi 280 |
| DB tabuľky | 803 (1 115 migrácií) |
| Cron skripty | 40 |
| Moduly v tomto inventári | 25 sekcií, spolu 419 funkcií (138 s označením CZ-špecifické) |

### Poznámky pre SK návrh

- Najväčšie CZ-viazané bloky, ktoré treba navrhnúť znova: DPH výkazy (priznanie, kontrolný výkaz, súhrnný výkaz), e-podania (eDane namiesto EPO, slovensko.sk namiesto ISDS), mzdy (SP/ZP/FS SR namiesto ČSSZ/ZP/JMHZ), eKasa namiesto EET, PAY by square namiesto SPAYD, UBL/Peppol (IS EFA od 2027) namiesto ISDOC, RPO/ORSR/FinStat namiesto ARES, PSD2 SK bánk a CAMT.053 namiesto GPC/ABO, kurzy ECB namiesto ČNB, SK výkazy (Úč POD/MÚJ, RÚZ) namiesto vyhlášok 500/504.
- SK špecifiká, ktoré MyÚčto nemá (príležitosť): Register účtovných závierok (RÚZ, uloženie závierky), tri identifikátory IČO/DIČ/IČ DPH, 2 % podiel dane pre NO, IS EFA e-fakturácia, paušálne výdavky SZČO, odvody SZČO podľa SP.
- Neziskový a SVJ modul (vyše 100 tabuliek) aj plné mzdy (251 tabuliek) sú samostatné veľké projekty; odporúčam ich odložiť za jadro (fakturácia, nákup, banka, DPH, PÚ a JÚ, uzávierka).
