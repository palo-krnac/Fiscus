import { describe, expect, it } from 'vitest'
import { blankConn, connErrors, formatWhen, splitError, orderStatus, stockSummary, toSave } from './eshops.js'

describe('napojenie e-shopov', () => {
  it('na uloženie idú iba zadávané polia', () => {
    const c = { ...blankConn('shopify'), id: 'a1', url: 'obchod', secret: '', hasSecret: true, lastSync: 'x', imported: 4 }
    expect(toSave([c])).toEqual([{ id: 'a1', platform: 'shopify', name: '', url: 'obchod', key: '', secret: '', onlyPaid: true, auto: false, since: '' }])
  })

  it.each([
    [{ 'conns[0].url': 'zlá', 'conns[1].key': 'chýba', 'conns[0].secret': 's' }, 0, { url: 'zlá', secret: 's' }],
    [{ 'conns[0].url': 'zlá' }, 1, {}],
    [undefined, 0, {}],
  ])('chyby obchodu %#', (map, i, want) => expect(connErrors(map, i)).toEqual(want))

  it.each([
    ['created', 'Koncept faktúry'],
    ['waiting', 'Čaká na úhradu'],
    ['error', 'Chyba'],
    ['iné', 'iné'],
  ])('stav %s', (s, label) => expect(orderStatus(s).label).toBe(label))

  it('čas načítania', () => {
    expect(formatWhen('')).toBe('')
    expect(formatWhen('nie')).toBe('')
    expect(formatWhen('2026-10-29T14:05:00')).toBe('29. 10. 2026 14:05')
  })

  it('zhrnutie zásob', () => {
    expect(stockSummary({ updated: 3, missing: [], errors: [] })).toBe('Aktualizované: 3')
    expect(stockSummary({ updated: 1, missing: ['A', 'B'], errors: ['x'] })).toBe('Aktualizované: 1 · v obchode chýba 2: A, B · chyby: 1')
  })

  it('chyba automatického načítania', () => {
    expect(splitError('')).toBe(null)
    expect(splitError('2026-10-29T14:05:00 obchod nedostupný')).toEqual({ when: '29. 10. 2026 14:05', text: 'obchod nedostupný' })
    expect(splitError('bez času')).toEqual({ when: '', text: 'bez času' })
  })
})
