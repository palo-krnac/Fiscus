// Sklad: zobrazenie kariet a dokladov, filtre. Ocenenie počíta server.
import { matches, searchTerms } from './search.js'

export const kindLabels = { receipt: 'Príjemka', issue: 'Výdajka', count: 'Inventúrny rozdiel', transfer: 'Presun' }
export const itemKindLabels = { goods: 'tovar', material: 'materiál', product: 'výrobky' }

/** Názov dokladu: „Príjemka“, pri vrátení „Príjemka – vrátenie“. */
export const docLabel = (d) => (d.return ? 'Príjemka – vrátenie' : kindLabels[d.kind] ?? d.kind)

/** Karty po hľadaní, sklade a filtri „pod minimom“. */
export function filterCards(cards, { search = '', warehouse = '', low = false } = {}) {
  const terms = searchTerms(search)
  return cards.filter(
    (c) => (!warehouse || c.warehouse === warehouse) && (!low || c.low) && matches(terms, [c.code, c.name, c.warehouse, itemKindLabels[c.kind]]),
  )
}

/** Doklady po hľadaní, druhu a sklade; najnovšie prvé. */
export function filterDocs(docs, { search = '', kind = '', warehouse = '' } = {}) {
  const terms = searchTerms(search)
  return docs
    .filter(
      (d) =>
        (!kind || d.kind === kind) &&
        (!warehouse || d.warehouse === warehouse) &&
        matches(terms, [d.number, d.partner, d.note, d.linkNumber, d.to, docLabel(d), ...d.lines.map((l) => l.name)]),
    )
    .slice()
    .reverse()
}

/** Riadky dokladov na export: jeden riadok na položku. */
export const docRows = (docs) => docs.flatMap((d) => d.lines.map((l) => ({ ...d, line: l })))

/** Prázdny riadok dokladu. */
export const blankLine = () => ({ itemId: null, quantity: '', unitCost: '' })

/** Riadky formulára pre API. */
export function apiLines(lines) {
  const num = (s) => String(s ?? '').replace(/\s/g, '').replace(',', '.') || '0'
  return lines.filter((l) => l.itemId).map((l) => ({ itemId: l.itemId, quantity: num(l.quantity), unitCost: num(l.unitCost) }))
}

/** Cena sa zadáva pri príjemke a pri prebytku (kladné množstvo inventúry). */
export const needsCost = (kind, quantity, ret = false) => (kind === 'receipt' && !ret) || (kind === 'count' && Number(String(quantity).replace(',', '.')) > 0)
