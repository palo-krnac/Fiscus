// Prenos dát z iných programov: zdroje a zhrnutie náhľadu importu.
import { matches, searchTerms } from './search.js'

/** Programy, z ktorých sa dá preniesť (ready = import hotový). */
export const sources = [
  {
    id: 'pohoda',
    name: 'POHODA',
    ready: true,
    accept: '.xml,text/xml,application/xml',
    hint: 'V POHODE: Súbor → Dátová komunikácia → XML export. Vyberte adresár, zásoby, vydané a prijaté faktúry.',
  },
  {
    id: 'money',
    name: 'Money S3',
    ready: true,
    accept: '.xml,text/xml,application/xml',
    hint: 'V Money S3 cez XML prenosy (Money → Import/export) vyexportujte adresár, zásoby a faktúry vydané aj prijaté. Money v exporte neuvádza úhrady — nastavte nižšie, do akého dátumu sú faktúry uhradené.',
  },
  { id: 'kros', name: 'KROS Alfa / Omega', ready: false },
  { id: 'superfaktura', name: 'SuperFaktúra', ready: false },
  { id: 'idoklad', name: 'iDoklad', ready: false },
  { id: 'fakturoid', name: 'Fakturoid', ready: false },
  { id: 'csv', name: 'Excel / CSV', ready: false },
]

/** Najväčší súbor na import (zhodne so serverom, app.MaxPohodaBytes). */
export const maxImportBytes = 50 * 1024 * 1024

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
