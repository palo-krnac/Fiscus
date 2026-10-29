import { describe, expect, it } from 'vitest'
import { defaultPeriod, dueDate, periodLabel, periodOptions } from './vat.js'

const today = new Date(2026, 9, 29) // 29. 10. 2026

describe('obdobia DPH', () => {
  it('mesiace od aktuálneho po júl 2025', () => {
    const o = periodOptions('month', today)
    expect(o[0]).toEqual({ value: '2026-10', label: 'október 2026' })
    expect(o.at(-1).value).toBe('2025-07')
    expect(o).toHaveLength(16)
  })
  it('štvrťroky', () => {
    const o = periodOptions('quarter', today).map((x) => x.value)
    expect(o).toEqual(['2026-Q4', '2026-Q3', '2026-Q2', '2026-Q1', '2025-Q4', '2025-Q3'])
  })
  it('predvolené je predchádzajúce obdobie', () => {
    expect(defaultPeriod('month', today)).toBe('2026-09')
    expect(defaultPeriod('quarter', today)).toBe('2026-Q3')
    expect(defaultPeriod('month', new Date(2025, 6, 10))).toBe('2025-07')
  })
  it('termín podania 25. deň po období', () => {
    expect(dueDate('2026-10')).toBe('2026-11-25')
    expect(dueDate('2026-12')).toBe('2027-01-25')
    expect(dueDate('2026-Q3')).toBe('2026-10-25')
    expect(dueDate('2026-Q4')).toBe('2027-01-25')
    expect(periodLabel('2026-Q4')).toBe('4. štvrťrok 2026')
  })
})
