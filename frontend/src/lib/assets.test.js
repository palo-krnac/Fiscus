import { describe, expect, it } from 'vitest'
import { assetStatus, canAccelerate, filterAssets, fixMethod, groupOptions, periodLabel, postingState } from './assets.js'

const items = [
  { id: 1, number: 'M001', name: 'Dodávka Ford Transit', kind: 'tangible', group: 1, period: 4, method: 'straight',
    acquired: '2024-04-12', cost: '28490.00', residual: '12000.00', thisYear: { claimed: '6410.25', disposal: '0' } },
  { id: 2, number: 'M002', name: 'Počítač', kind: 'tangible', group: 1, period: 4, acquired: '2023-11-20', cost: '1890.00',
    disposed: '2025-03-31', residual: '0', thisYear: { claimed: '0', disposal: '0' } },
  { id: 3, number: 'M003', name: 'Projekčný softvér', kind: 'intangible', years: 3, acquired: '2026-02-01', cost: '2990.00',
    residual: '1993.34', thisYear: { claimed: '996.66' }, note: 'licencia' },
]

describe('register majetku', () => {
  it.each([
    [{ kind: 'tangible', group: 2 }, true],
    [{ kind: 'tangible', group: 3 }, true],
    [{ kind: 'tangible', group: 1 }, false],
    [{ kind: 'intangible', group: 2 }, false],
  ])('zrýchlené odpisovanie %o → %s', (a, ok) => expect(canAccelerate(a)).toBe(ok))

  it('zmena skupiny vráti rovnomerné odpisovanie', () => {
    expect(fixMethod({ kind: 'tangible', group: 1, method: 'accelerated' }).method).toBe('straight')
    expect(fixMethod({ kind: 'tangible', group: 2, method: 'accelerated' }).method).toBe('accelerated')
  })

  it('skupiny so správnym tvarom slova', () => {
    expect(groupOptions[0].label).toBe('0. skupina – 2 roky')
    expect(groupOptions[6].label).toBe('6. skupina – 40 rokov')
  })

  it.each([
    ['', 2026, ['M001', 'M003']],
    ['softver', 2026, ['M003']],
    ['ford 28490', 2026, ['M001']],
    ['LICENCIA', 2026, ['M003']],
  ])('hľadanie „%s“ v roku %i', (q, year, want) => {
    expect(filterAssets(items, q, { year }).map((x) => x.number)).toEqual(want)
  })

  it('v roku vyradenia je položka v zozname (zostatková cena je výdavok)', () => {
    const y2025 = [{ ...items[1], thisYear: { claimed: '0', disposal: '1208.33' } }]
    expect(filterAssets(y2025, '', { year: 2025 })).toHaveLength(1)
  })

  it('všetky vrátane vyradených', () => {
    expect(filterAssets(items, '', { show: 'all', year: 2026 })).toHaveLength(3)
  })

  it.each([
    [items[0], 2026, 'v užívaní'],
    [items[1], 2026, 'vyradený'],
    [items[2], 2025, 'od 2026'],
    [{ ...items[0], residual: '0', thisYear: { claimed: '0' } }, 2030, 'odpísaný'],
  ])('stav %#', (it_, year, label) => expect(assetStatus(it_, year).label).toBe(label))

  it('doba v zozname', () => {
    expect(periodLabel(items[0])).toBe('sk. 1 – 4 r.')
    expect(periodLabel({ kind: 'tangible', group: 2, period: 6, method: 'accelerated' })).toBe('sk. 2 – 6 r. zrýchl.')
    expect(periodLabel(items[2])).toBe('nehm. 3 roky')
  })

  it.each([
    [{ posted: '0', totals: { expense: '0' } }, 'draft', ''],
    [{ posted: '0', totals: { expense: '7406.91' } }, 'overdue', 'Zaúčtovať do uzávierky'],
    [{ posted: '7000.00', totals: { expense: '7406.91' } }, 'overdue', 'Zaúčtovať znova'],
    [{ posted: '7406.91', totals: { expense: '7406.91' } }, 'paid', 'Zaúčtovať znova'],
  ])('zaúčtovanie %#', (reg, tone, action) => {
    const s = postingState(reg)
    expect(s.tone).toBe(tone)
    expect(s.action).toBe(action)
  })
})
