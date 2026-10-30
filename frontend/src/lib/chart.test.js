import { describe, expect, it } from 'vitest'
import { blankTemplate, filterTemplates, templateSummary, accountDisplay, groupAccounts, typeLabel } from './chart.js'

const classes = ['Dlhodobý majetok', 'Zásoby', 'Finančné účty', 'Zúčtovacie vzťahy', 'Kapitál', 'Náklady', 'Výnosy', 'Závierka']
const accounts = [
  { number: '013', name: 'Softvér', type: 'A', active: true },
  { number: '221', name: 'Bankové účty', type: 'A', active: true },
  { number: '221001', name: 'Tatra banka', type: 'A', active: true },
  { number: '232', name: 'Eskontné úvery', type: 'P', active: false },
  { number: '518', name: 'Ostatné služby', type: 'N', active: true },
]

describe('účtový rozvrh', () => {
  it('číslo a druh', () => {
    expect(accountDisplay('221001')).toBe('221.001')
    expect(accountDisplay('221')).toBe('221')
    expect(typeLabel('N')).toBe('nákladový')
  })
  it('triedy, hľadanie a skryté', () => {
    const g = groupAccounts(accounts, classes)
    expect(g.map((x) => x.cls)).toEqual([0, 2, 5])
    expect(g[1].accounts.map((a) => a.number)).toEqual(['221', '221001'])
    expect(groupAccounts(accounts, classes, { hidden: true })[1].accounts).toHaveLength(3)
    expect(groupAccounts(accounts, classes, { search: '221.001' })[0].accounts).toHaveLength(1)
    expect(groupAccounts(accounts, classes, { search: 'tatra' })[0].accounts[0].number).toBe('221001')
    expect(groupAccounts(accounts, classes, { cls: 5 })).toHaveLength(1)
  })
})

describe('predkontácie', () => {
  const list = [
    { code: 'FP-SLUZBY', name: 'Prijatá faktúra — služby', doc: 'purchase', lines: [
      { side: 'MD', account: '518', part: 'base' }, { side: 'MD', account: '343', part: 'vat' }, { side: 'D', account: '321', part: 'total' }] },
    { code: 'B-TATRA', name: 'Úhrada z Tatry', doc: 'bank', lines: [{ side: 'MD', account: '321', part: 'total' }, { side: 'D', account: '221001', part: 'total' }] },
  ]
  it('skratka strán', () => {
    expect(templateSummary(list[0])).toBe('MD 518, 343 / D 321')
    expect(templateSummary(list[1])).toBe('MD 321 / D 221.001')
  })
  it('hľadanie a druh dokladu', () => {
    expect(filterTemplates(list, { search: 'sluzby' })).toHaveLength(1)
    expect(filterTemplates(list, { search: '221.001' })[0].code).toBe('B-TATRA')
    expect(filterTemplates(list, { doc: 'bank' })).toHaveLength(1)
    expect(filterTemplates(list, { search: 'banka' })).toHaveLength(1)
  })
  it('nová predkontácia má dva riadky', () => {
    expect(blankTemplate('cash').lines).toHaveLength(2)
  })
})
