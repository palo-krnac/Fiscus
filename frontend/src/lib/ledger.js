// Účtovný denník a hlavná kniha: zobrazenie, filtre a riadky na úpravu.
// Zápisy a obraty počíta server; tu sa len triedi a zobrazuje.
import { matches, searchTerms } from './search.js'
import { accountDisplay, docLabels } from './chart.js'

/** Filtre denníka. */
export const postingFilters = [
  { value: 'all', label: 'Všetky doklady' },
  { value: 'problem', label: 'Nezaúčtované' },
  { value: 'manual', label: 'Zaúčtované ručne' },
  ...Object.entries(docLabels).map(([value, label]) => ({ value, label })),
]

/** Zápisy denníka po hľadaní a filtri. */
export function filterPostings(list, { search = '', filter = 'all', month = 0 } = {}) {
  const terms = searchTerms(search)
  return list.filter((p) => {
    if (filter === 'problem' && !p.problem) return false
    if (filter === 'manual' && !p.manual) return false
    if (docLabels[filter] && p.doc !== filter) return false
    if (month && Number(p.date.slice(5, 7)) !== month) return false
    const accs = p.lines.flatMap((l) => [l.account, accountDisplay(l.account), l.name])
    return matches(terms, [p.number, p.partner, p.text, p.template, p.note, p.amount, p.center, p.job, docLabels[p.doc], ...accs])
  })
}

/** Riadky denníka na export: jeden riadok na zápis na účet. */
export function postingRows(list) {
  return list.flatMap((p) =>
    p.lines.length ? p.lines.map((l) => ({ ...p, line: l })) : [{ ...p, line: { account: '', name: '', debit: '', credit: '' } }],
  )
}

/** Stav so znamienkom → strana: { debit, credit } (kladný = MD). */
export function sideOf(v) {
  const n = Number(v)
  if (!n) return { debit: '', credit: '' }
  return n > 0 ? { debit: String(v), credit: '' } : { debit: '', credit: String(v).replace('-', '') }
}

/** Obraty účtu po hľadaní (číslo s bodkou aj bez, názov). */
export function filterTurnovers(list, search = '') {
  const terms = searchTerms(search)
  return list.filter((t) => matches(terms, [t.account, accountDisplay(t.account), t.name]))
}

/** Riadky zápisov na jednom účte (aj analytických, ak je účet syntetický). */
export function accountLines(postings, account) {
  const out = []
  for (const p of postings) {
    if (p.problem) continue
    for (const l of p.lines) {
      if (l.account === account || (account.length === 3 && l.account.startsWith(account))) out.push({ ...p, line: l })
    }
  }
  return out
}

/** Text sumy z formulára → reťazec pre API ("1 234,5" → "1234.5"). */
export const parseAmount = (s) => String(s ?? '').replace(/\s/g, '').replace(',', '.')

/** Riadky na úpravu z uložených riadkov (alebo dva prázdne). */
export function editLines(lines) {
  if (!lines?.length) return [{ account: '', debit: '', credit: '' }, { account: '', debit: '', credit: '' }]
  return lines.map((l) => ({ account: l.account, debit: Number(l.debit) ? String(l.debit) : '', credit: Number(l.credit) ? String(l.credit) : '' }))
}

/** Riadky z formulára pre API: prázdne riadky sa vynechajú. */
export function apiLines(lines) {
  return lines
    .filter((l) => l.account || l.debit || l.credit)
    .map((l) => ({ account: l.account, debit: parseAmount(l.debit) || '0', credit: parseAmount(l.credit) || '0' }))
}

/** Počiatočné stavy (účet → stav) na riadky formulára. */
export function openingLines(opening) {
  const keys = Object.keys(opening ?? {}).sort()
  if (!keys.length) return editLines([])
  return keys.map((k) => ({ account: k, ...sideOf(opening[k]) }))
}
