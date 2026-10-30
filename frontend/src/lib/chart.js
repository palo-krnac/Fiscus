// Účtový rozvrh: zobrazenie čísel účtov, triedy a filtre. Pravidlá overuje server.
import { matches, searchTerms } from './search.js'

/** Číslo účtu na zobrazenie: 221001 → „221.001“. */
export const accountDisplay = (n) => (n && n.length > 3 ? `${n.slice(0, 3)}.${n.slice(3)}` : n ?? '')

export const typeLabels = { A: 'aktívny', P: 'pasívny', X: 'aktívny / pasívny', N: 'nákladový', V: 'výnosový', Z: 'závierkový', S: 'podsúvahový' }
export const typeLabel = (t) => typeLabels[t] ?? t

/**
 * Účty po triedach na zobrazenie: [{ cls, name, accounts }]. Filtruje podľa
 * hľadania (číslo aj s bodkou, názov), triedy a skrytých účtov.
 */
export function groupAccounts(accounts, classes, { search = '', cls = null, hidden = false } = {}) {
  const terms = searchTerms(search)
  const out = []
  for (const a of accounts) {
    if (!hidden && !a.active) continue
    const c = Number(a.number[0])
    if (cls !== null && cls !== c) continue
    if (!matches(terms, [a.number, accountDisplay(a.number), a.name, typeLabel(a.type), a.note])) continue
    let g = out.find((x) => x.cls === c)
    if (!g) out.push((g = { cls: c, name: classes[c] ?? '', accounts: [] }))
    g.accounts.push(a)
  }
  return out.sort((x, y) => x.cls - y.cls)
}

/** Prázdny účet: analytický k zvolenému syntetickému. */
export const blankAccount = (synthetic = '') => ({ number: synthetic, name: '', type: 'A', nonTax: false, active: true, note: '' })

export const docLabels = { sale: 'Vydaná faktúra', purchase: 'Prijatá faktúra', bank: 'Banka', cash: 'Pokladňa', internal: 'Interný doklad' }
export const partLabels = { total: 'suma dokladu', base: 'základ', vat: 'DPH', vatNon: 'neodpočítaná DPH', self: 'samozdanenie DPH' }

/** Strany predkontácie v skratke: „MD 518, 343 / D 321“. */
export function templateSummary(t) {
  const side = (s) => [...new Set(t.lines.filter((l) => l.side === s).map((l) => accountDisplay(l.account)))].join(', ')
  return `MD ${side('MD')} / D ${side('D')}`
}

/** Predkontácie po hľadaní a druhu dokladu (doc = '' všetky). */
export function filterTemplates(list, { search = '', doc = '' } = {}) {
  const terms = searchTerms(search)
  return list.filter((t) => (!doc || t.doc === doc) && matches(terms, [t.code, t.name, docLabels[t.doc], templateSummary(t), t.note]))
}

/** Nová predkontácia: dva prázdne riadky. */
export const blankTemplate = (doc = 'purchase') => ({
  code: '', name: '', doc, active: true, note: '',
  lines: [{ side: 'MD', account: '', part: 'base' }, { side: 'D', account: '', part: 'total' }],
})
