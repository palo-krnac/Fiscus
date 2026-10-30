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
