// Cudzie meny: číselník mien a opis kurzu. Kurz a sumy v eurách počíta server.
import { formatDate } from './format.js'
import { matches, searchTerms } from './search.js'

/** Je doklad v cudzej mene? */
export const isForeignCurrency = (currency) => !!currency && currency !== 'EUR'

/** Možnosti výberu meny: euro a meny s kurzom ECB. */
export function currencyOptions(currencies = []) {
  return [{ code: 'EUR', label: 'EUR – euro' }, ...currencies.map((c) => ({ code: c.code, label: `${c.code} – ${c.name}` }))]
}

/** Kurz v slovenskom zápise bez zbytočných núl: "25.1230" → "25,123". */
export function formatRate(rate) {
  const n = Number(rate)
  if (!rate || Number.isNaN(n) || n === 0) return ''
  return new Intl.NumberFormat('sk-SK', { maximumFractionDigits: 4, useGrouping: false }).format(n)
}

/** Opis kurzu faktúry: "1 EUR = 25,123 CZK (ECB 23. 10. 2026)". */
export function rateText(inv) {
  if (!isForeignCurrency(inv?.currency) || !Number(inv.fxRate)) return ''
  const src = inv.fxManual || !inv.fxDate ? 'dohodnutý' : `ECB ${formatDate(inv.fxDate)}`
  return `1 EUR = ${formatRate(inv.fxRate)} ${inv.currency} (${src})`
}

/** Nápoveda pod poľom kurzu v editore. */
export function rateHint(inv) {
  if (!isForeignCurrency(inv?.currency)) return ''
  if (inv.fxManual) return 'Zadaný ručne'
  if (inv.fxDate) return `Kurz ECB z ${formatDate(inv.fxDate)} (deň pred dodaním)`
  return 'Kurz ECB k dátumu dodania zatiaľ nie je'
}

/** Rekapitulácia v eurách (pri cudzej mene prepočet zo servera). */
export const eurSummary = (inv) => (isForeignCurrency(inv?.currency) && inv.local ? inv.local : inv?.summary)

export const sourceLabel = (s) => (s === 'manual' ? 'ručne' : 'ECB')

/** Hľadanie v kurzoch: mena, dátum (aj „23. 10. 2026“), kurz, zdroj. */
export function filterRates(rows, search) {
  const terms = searchTerms(search)
  return rows.filter((r) => matches(terms, [r.currency, r.date, formatDate(r.date), r.rate, formatRate(r.rate), sourceLabel(r.source)]))
}
