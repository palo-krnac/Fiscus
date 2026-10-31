// Prenos dát z iných programov: zdroje a zhrnutie náhľadu importu.
import { matches, searchTerms } from './search.js'

/** Programy, z ktorých sa dá preniesť (ready = import hotový). */
export const sources = [
  {
    id: 'pohoda',
    name: 'POHODA',
    ready: true,
    accept: '.xml,.zip,text/xml,application/xml,application/zip',
    hint: 'V POHODE: Súbor → Dátová komunikácia → XML export. Vyberte adresár, zásoby, vydané a prijaté faktúry. Viac súborov XML môžete nahrať naraz zabalených v ZIP.',
  },
  {
    id: 'money',
    name: 'Money S3',
    ready: true,
    accept: '.xml,text/xml,application/xml',
    hint: 'V Money S3 cez XML prenosy (Money → Import/export) vyexportujte adresár, zásoby a faktúry vydané aj prijaté. Money v exporte neuvádza úhrady — nastavte nižšie, do akého dátumu sú faktúry uhradené.',
  },
  {
    id: 'omega',
    name: 'KROS Omega',
    ready: true,
    accept: '.txt,text/plain',
    pick: 'Vybrať súbor TXT',
    hint: 'V Omege: Firma → Export → Export do textového súboru. Vyberte partnerov, skladové karty, fakturáciu alebo doklady EUD (odoslané a došlé faktúry aj s úhradami).',
  },
  {
    id: 'idoklad',
    name: 'iDoklad',
    ready: true,
    accept: '.zip,.xml,application/zip,text/xml',
    pick: 'Vybrať súbor ZIP',
    hint: 'V iDoklade: Nastavenia → Aplikácie → Export, program POHODA. Vyberte mesiac a vydané faktúry, dobropisy aj prijaté faktúry, potom stiahnite ZIP (pri viacerých mesiacoch nahrajte každý zvlášť). Kontakty sa založia z faktúr.',
  },
  {
    id: 'mrp',
    name: 'MRP-K/S',
    ready: true,
    accept: '.zip,.xml,application/zip,text/xml',
    pick: 'Vybrať súbor ZIP',
    hint: 'Z MRP-K/S exportujte vydané faktúry do XML — súbory FAKVY.XML (faktúry), FAKVYPOL.XML (položky) a FV_ADRES.XML (adresy) — a nahrajte ich spolu zabalené v ZIP. Odberateľ sa doplní z adresára podľa IČO.',
  },
  {
    id: 'superfaktura',
    name: 'SuperFaktúra',
    ready: true,
    accept: '.xml,.zip,.txt,text/xml,application/zip,text/plain',
    pick: 'Vybrať súbor',
    hint: 'V SuperFaktúre: Nástroje → Export, zvoľte export pre POHODU (faktúry, dobropisy aj náklady), prípadne Money, OMEGA alebo MRP, a obdobie. Nahrajte stiahnutý súbor (XML, TXT alebo ZIP) — formát sa rozpozná sám. Úhrady v exporte nie sú: nastavte nižšie, do kedy sú faktúry uhradené. Kontakty sa založia z faktúr.',
  },
  {
    id: 'fakturoid',
    name: 'Fakturoid',
    ready: true,
    accept: '.xml,.zip,text/xml,application/zip',
    pick: 'Vybrať súbor',
    hint: 'Vo Fakturoide: Přehled → Exporty → Export faktur, program Pohoda (náklady sa exportujú tiež pre Pohodu), prípadne Money S3. Nahrajte stiahnutý súbor — formát sa rozpozná sám. Kontakty sa založia z faktúr.',
  },
  {
    id: 'vcard',
    name: 'Kontakty (vCard)',
    ready: true,
    accept: '.vcf,text/vcard,text/x-vcard',
    pick: 'Vybrať súbor VCF',
    hint: 'Vizitky z telefónu, Google Kontaktov, Outlooku alebo Apple Kontaktov (export do .vcf). Názov kontaktu je firma, inak meno osoby; osoba pri firme sa zapíše do poznámky.',
  },
  {
    id: 'table',
    name: 'Excel / CSV',
    ready: true,
    kinds: true,
    pick: 'Vybrať tabuľku',
    accept: '.csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    hint: 'Tabuľka z ľubovoľného programu: v prvom riadku hlavička, potom jeden kontakt, položka alebo faktúra na riadok. Stĺpce sa rozpoznajú podľa názvu — stiahnite si vzor.',
  },
]

/** Prípony súborov s kontaktmi (Adresár → Import). */
export const contactAccept = '.vcf,.csv,.xlsx,text/vcard,text/x-vcard,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'

/** Text tlačidla prenosu: „Pridať 3 kontakty“. */
export function contactsLabel(n) {
  if (n === 1) return 'Pridať 1 kontakt'
  if (n >= 2 && n <= 4) return `Pridať ${n} kontakty`
  return `Pridať ${n} kontaktov`
}

/** Obsah tabuľky Excel/CSV. */
export const tableKinds = [
  { value: 'auto', label: 'Rozpoznať' },
  { value: 'contacts', label: 'Adresár' },
  { value: 'items', label: 'Cenník' },
  { value: 'invoices', label: 'Vydané faktúry' },
  { value: 'purchases', label: 'Prijaté faktúry' },
]

/** Vzory tabuliek: hlavička a ukážkový riadok. */
export const tableTemplates = {
  contacts: [
    ['Názov', 'IČO', 'DIČ', 'IČ DPH', 'Ulica', 'Obec', 'PSČ', 'Štát', 'E-mail', 'Telefón', 'Poznámka'],
    ['Kováč a syn s.r.o.', '36123455', '2020123457', 'SK2020123457', 'Hlavná 12', 'Nitra', '94901', 'Slovensko', 'fakturacia@kovac.sk', '+421 905 123 456', ''],
  ],
  items: [
    ['Kód', 'Názov', 'MJ', 'Cena bez DPH', 'DPH (%)', 'Poznámka'],
    ['M1', 'Montáž', 'hod', '40,00', '23', ''],
  ],
  invoices: [
    ['Číslo', 'Odberateľ', 'IČO', 'IČ DPH', 'Obec', 'Štát', 'Vystavená', 'Dodanie', 'Splatnosť', 'Predmet', 'Základ 23 %', 'DPH 23 %', 'Základ 19 %', 'DPH 19 %', 'Základ 5 %', 'DPH 5 %', 'Základ 0 %', 'Spolu', 'Dátum úhrady', 'Pôvodná faktúra'],
    ['2026001', 'Kováč a syn s.r.o.', '36123455', 'SK2020123457', 'Nitra', 'Slovensko', '2.3.2026', '2.3.2026', '16.3.2026', 'Montáž rozvádzača', '100,00', '23,00', '', '', '', '', '', '123,00', '10.3.2026', ''],
  ],
  purchases: [
    ['Číslo dodávateľa', 'Dodávateľ', 'IČO', 'IČ DPH', 'Obec', 'Vystavená', 'Dodanie', 'Splatnosť', 'Predmet', 'Základ 23 %', 'DPH 23 %', 'Základ 19 %', 'DPH 19 %', 'Základ 0 %', 'Spolu', 'Dátum úhrady'],
    ['2026001234', 'Elektro Veľkoobchod a.s.', '35757442', 'SK2021757441', 'Zvolen', '5.3.2026', '5.3.2026', '19.3.2026', 'Materiál', '100,00', '23,00', '', '', '', '123,00', ''],
  ],
}

/** Stĺpce a riadky vzoru pre export (lib/export.js). */
export function templateExport(kind) {
  const [head, ...rows] = tableTemplates[kind]
  return { columns: head.map((h, i) => ({ header: h, value: (r) => r[i], width: 14 })), rows }
}

/** Najväčší súbor na import (zhodne so serverom, app.MaxPohodaBytes). */
export const maxImportBytes = 50 * 1024 * 1024

/** Názov agendy pre vzor a nadpis. */
export function kindLabel(kind) {
  return tableKinds.find((k) => k.value === kind)?.label ?? ''
}

const groups = [
  ['contacts', 'Kontakty'],
  ['items', 'Skladové karty do cenníka'],
  ['invoices', 'Vydané faktúry a dobropisy'],
  ['purchases', 'Prijaté faktúry'],
]

/** Riadky zhrnutia: čo pribudne, čo už je a čo sa preskočí. */
export function summaryRows(report) {
  if (!report) return []
  return groups.map(([key, label]) => {
    const c = report[key] ?? {}
    return { key, label, new: c.new ?? 0, existing: c.existing ?? 0, skipped: c.skipped ?? 0 }
  })
}

/** Počet nových záznamov spolu (či má zmysel spustiť prenos). */
export function newTotal(report) {
  return summaryRows(report).reduce((n, r) => n + r.new, 0)
}

const otherNames = {
  issuedProformaInvoice: 'zálohové faktúry',
  issuedAdvanceInvoice: 'zálohové faktúry',
  order: 'objednávky',
  offer: 'ponuky',
  enquiry: 'dopyty',
  bank: 'bankové doklady',
  voucher: 'pokladničné doklady',
  intDoc: 'interné doklady',
  // Money S3
  ObjPrij: 'prijaté objednávky',
  ObjVyd: 'vydané objednávky',
  NabVyd: 'ponuky',
  PoptPrij: 'dopyty',
  PokDokl: 'pokladničné doklady',
  BankDokl: 'bankové doklady',
  IntDokl: 'interné doklady',
  FaktVyd_DPP: 'daňové doklady k platbám',
  Prijemka: 'príjemky',
  Vydejka: 'výdajky',
  Zakazka: 'zákazky',
}

/** Agendy v súbore, ktoré sa neprenášajú, ako text „objednávky 2, …“. */
export function otherText(other) {
  return Object.entries(other ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, n]) => `${otherNames[k] ?? k} ${n}`)
    .join(', ')
}

/** Hľadanie v upozorneniach. */
export function filterWarnings(list, q) {
  const terms = searchTerms(q)
  return (list ?? []).map((text, i) => ({ n: i + 1, text })).filter((w) => matches(terms, [w.text]))
}
