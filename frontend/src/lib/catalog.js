// Cenník: hľadanie položky a prevod na riadok faktúry (ceny počíta backend).
import { matches, searchTerms } from './search.js'

/** Aktívne položky zodpovedajúce hľadaniu (kód, názov, poznámka). */
export function filterItems(items, query, { archived = false } = {}) {
  const terms = searchTerms(query)
  return items.filter((it) => (archived || !it.archived) && matches(terms, [it.code, it.name, it.note, it.unit]))
}

/**
 * Riadok faktúry z položky cenníka. Cenu berie podľa režimu faktúry —
 * bez DPH alebo s DPH, oboje už prepočítal backend. Neplatiteľ DPH
 * fakturuje sumu bez DPH.
 */
export function lineFromItem(it, { priceMode = 'net', nonVat = false } = {}) {
  return {
    description: it.name,
    quantity: '1',
    unit: it.unit || 'ks',
    unitPrice: !nonVat && priceMode === 'gross' ? it.priceGross : it.priceNet,
    vatRate: it.vatRate,
    amount: '0',
  }
}

export const importActions = {
  create: { label: 'nová', tone: 'paid' },
  update: { label: 'zmena', tone: 'issued' },
  skip: { label: 'preskočí sa', tone: 'overdue' },
}

/** Text chýb riadku importu. */
export const importErrors = (row) => (row.errors ?? []).map((e) => e.message).join('; ')

/** Text tlačidla importu so správnym tvarom: 1 položku, 2–4 položky, 5 položiek. */
export function importLabel(n) {
  const word = n === 1 ? 'položku' : n >= 2 && n <= 4 ? 'položky' : 'položiek'
  return `Importovať ${n} ${word}`
}
