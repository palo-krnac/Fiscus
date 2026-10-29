// Register dlhodobého majetku: číselníky formulára, hľadanie a stavy.
// Odpisy a zostatkové ceny počíta backend (internal/asset), tu sa iba zobrazujú.
import { matches, searchTerms } from './search.js'

/** Doba odpisovania odpisových skupín 0–6 (príloha č. 1 zákona o dani z príjmov). */
export const groupYears = [2, 4, 6, 8, 12, 20, 40]

const roky = (n) => (n === 1 ? 'rok' : n < 5 ? 'roky' : 'rokov')

export const groupOptions = groupYears.map((y, g) => ({ value: g, label: `${g}. skupina – ${y} ${roky(y)}` }))

export const kindOptions = [
  { value: 'tangible', label: 'Hmotný' },
  { value: 'intangible', label: 'Nehmotný' },
]

export const methodOptions = [
  { value: 'straight', label: 'Rovnomerne' },
  { value: 'accelerated', label: 'Zrýchlene' },
]

export const disposalOptions = [
  { value: 'sale', label: 'Predaj' },
  { value: 'liquidation', label: 'Likvidácia' },
  { value: 'other', label: 'Iné (dar, škoda, vklad)' },
]

/** Zrýchlene sa smie odpisovať iba hmotný majetok v 2. a 3. skupine. */
export function canAccelerate(a) {
  return a.kind !== 'intangible' && (a.group === 2 || a.group === 3)
}

/** Opraví formulár po zmene druhu alebo skupiny, aby nezostal nepovolený spôsob. */
export function fixMethod(a) {
  if (!canAccelerate(a) && a.method === 'accelerated') a.method = 'straight'
  return a
}

export function blankAsset(today) {
  return { id: 0, number: '', name: '', kind: 'tangible', group: 1, years: null, method: 'straight', acquired: today,
    cost: null, share: 100, purchaseId: 0, disposed: '', disposal: '', note: '' }
}

/** Stav položky v roku: v užívaní, odpísaná, vyradená, zaradená neskôr. */
export function assetStatus(it, year) {
  if (it.disposed && Number(it.disposed.slice(0, 4)) <= year) return { label: 'vyradený', tone: 'cancelled' }
  if (Number(it.acquired?.slice(0, 4)) > year) return { label: `od ${it.acquired.slice(0, 4)}`, tone: 'draft' }
  if (Number(it.residual) === 0 && Number(it.thisYear?.claimed ?? 0) === 0) return { label: 'odpísaný', tone: 'paid' }
  return { label: 'v užívaní', tone: 'issued' }
}

/** Doba a skupina na zobrazenie v zozname. */
export function periodLabel(it) {
  if (it.kind === 'intangible') return `nehm. ${it.years} ${roky(it.years)}`
  return `sk. ${it.group} – ${it.period ?? groupYears[it.group]} r.${it.method === 'accelerated' ? ' zrýchl.' : ''}`
}

/**
 * Filter zoznamu: hľadanie (bez diakritiky, viac slov) a voľba
 * show = 'active' (v užívaní v roku) | 'all'.
 */
export function filterAssets(items, query, { show = 'active', year } = {}) {
  const terms = searchTerms(query)
  return items.filter((it) => {
    if (show === 'active') {
      const gone = it.disposed && Number(it.disposed.slice(0, 4)) < year
      const later = Number(it.acquired?.slice(0, 4)) > year
      const done = Number(it.residual) === 0 && Number(it.thisYear?.claimed ?? 0) === 0 && Number(it.thisYear?.disposal ?? 0) === 0
      if (gone || later || done) return false
    }
    return matches(terms, [it.number, it.name, it.note, it.purchaseNumber, it.cost])
  })
}

/**
 * Stav zaúčtovania odpisov do uzávierky roka.
 * posted = suma v uzávierke, expense = daňový výdavok z registra.
 */
export function postingState(reg) {
  if (!reg) return null
  const posted = Number(reg.posted ?? 0)
  const expense = Number(reg.totals?.expense ?? 0)
  if (posted === 0 && expense === 0) return { tone: 'draft', label: 'Žiadne odpisy', action: '' }
  if (posted === 0) return { tone: 'overdue', label: 'Nezaúčtované v uzávierke', action: 'Zaúčtovať do uzávierky' }
  if (posted !== expense) return { tone: 'overdue', label: 'Register sa zmenil po zaúčtovaní', action: 'Zaúčtovať znova' }
  return { tone: 'paid', label: 'Zaúčtované v uzávierke (stĺpec e6)', action: 'Zaúčtovať znova' }
}
