import { describe, expect, it } from 'vitest'
import { apiLines, docLabel, docRows, filterCards, filterDocs, needsCost } from './stock.js'

const cards = [
  { warehouse: 'HL', itemId: 1, code: 'R24', name: 'Relé 24 V', kind: 'goods', low: true },
  { warehouse: 'BB', itemId: 2, code: 'K3', name: 'Kábel CYKY', kind: 'material', low: false },
]
const docs = [
  { number: 'PR2026001', kind: 'receipt', warehouse: 'HL', partner: 'Veľkoobchod', lines: [{ name: 'Relé 24 V' }, { name: 'Kábel CYKY' }] },
  { number: 'VY2026001', kind: 'issue', warehouse: 'HL', linkNumber: 'FA20260001', lines: [{ name: 'Relé 24 V' }] },
  { number: 'PR2026002', kind: 'receipt', return: true, warehouse: 'BB', lines: [{ name: 'Kábel CYKY' }] },
]

describe('sklad', () => {
  it('karty', () => {
    expect(filterCards(cards, { search: 'rele' })).toHaveLength(1)
    expect(filterCards(cards, { warehouse: 'BB' })[0].code).toBe('K3')
    expect(filterCards(cards, { low: true })).toHaveLength(1)
    expect(filterCards(cards, { search: 'material' })).toHaveLength(1)
  })
  it('doklady', () => {
    expect(filterDocs(docs)[0].number).toBe('PR2026002')
    expect(filterDocs(docs, { search: 'fa20260001' })).toHaveLength(1)
    expect(filterDocs(docs, { kind: 'receipt' })).toHaveLength(2)
    expect(filterDocs(docs, { search: 'vratenie' })).toHaveLength(1)
    expect(docLabel(docs[2])).toBe('Príjemka – vrátenie')
    expect(docRows(docs)).toHaveLength(4)
  })
  it('formulár', () => {
    expect(apiLines([{ itemId: 1, quantity: '2,5', unitCost: '1 200,00' }, { itemId: null }])).toEqual([{ itemId: 1, quantity: '2.5', unitCost: '1200.00' }])
    expect(needsCost('receipt', '1')).toBe(true)
    expect(needsCost('receipt', '1', true)).toBe(false)
    expect(needsCost('count', '-1')).toBe(false)
    expect(needsCost('count', '2')).toBe(true)
    expect(needsCost('issue', '2')).toBe(false)
  })
})
